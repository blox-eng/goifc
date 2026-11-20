package geometry

import (
	"fmt"
	"math"
)

const (
	// maxRoofPoints caps a roof outline: the skeleton is O(n²) or worse.
	maxRoofPoints = 256
	maxPitch      = 75.0 // degrees
	maxOverhang   = 3.0  // metres
	// roofCollinear is the sine of the angle under which two edges count as
	// one line.
	roofCollinear = 1e-9
)

// RoofSpec describes a pitched roof over an outline. Edge i runs outline[i]
// to outline[i+1], wrapping at the end.
type RoofSpec struct {
	Pitch    float64 // degrees from horizontal, every sloped edge; (0, 75]
	Gables   []int   // outline edges that are vertical gable ends
	Overhang float64 // metres, horizontal, beyond every edge (gable ends too); [0, 3]
	Eaves    float64 // z of the underside of the lowest layer, above the outline line
}

// RoofReason says why a roof was refused.
type RoofReason string

const (
	ReasonPitchOutOfRange       RoofReason = "pitch_out_of_range"
	ReasonGableOutOfRange       RoofReason = "gable_out_of_range"
	ReasonGableRepeated         RoofReason = "gable_repeated"
	ReasonNoSlopedEdge          RoofReason = "no_sloped_edge"
	ReasonRoofStep              RoofReason = "roof_step"
	ReasonGableCollinear        RoofReason = "gable_collinear"
	ReasonOverhangOutOfRange    RoofReason = "overhang_out_of_range"
	ReasonOverhangClosesOutline RoofReason = "overhang_closes_outline"
	ReasonRoofTooComplex        RoofReason = "roof_too_complex"
)

// RoofError refuses a roof. Edge is the outline edge at fault (a gable, or one
// of two edges), Other a second edge, and either is -1 when none applies.
type RoofError struct {
	Reason RoofReason
	Edge   int
	Other  int
}

func (e *RoofError) Error() string {
	switch {
	case e.Edge >= 0 && e.Other >= 0:
		return fmt.Sprintf("roof: %s: edges %d and %d", e.Reason, e.Edge, e.Other)
	case e.Edge >= 0:
		return fmt.Sprintf("roof: %s: edge %d", e.Reason, e.Edge)
	}
	return fmt.Sprintf("roof: %s", e.Reason)
}

// checkRoof refuses a roof's inputs, except a step between planes and a
// skeleton over budget, which the skeleton finds. It returns which edges are
// gables and the layers' total thickness.
func checkRoof(outline [][2]float64, spec RoofSpec, layers []float64) (gable []bool, total float64, err error) {
	if len(outline) > maxRoofPoints {
		return nil, 0, tooLarge("%d outline points, more than %d for a roof", len(outline), maxRoofPoints)
	}
	if err := checkOutline(outline); err != nil {
		return nil, 0, err
	}
	if p := spec.Pitch; !(p > 0 && p <= maxPitch) {
		return nil, 0, &RoofError{Reason: ReasonPitchOutOfRange, Edge: -1, Other: -1}
	}
	if o := spec.Overhang; !(o >= 0 && o <= maxOverhang) {
		return nil, 0, &RoofError{Reason: ReasonOverhangOutOfRange, Edge: -1, Other: -1}
	}
	n := len(outline)
	gable = make([]bool, n)
	for _, g := range spec.Gables {
		switch {
		case g < 0 || g >= n:
			return nil, 0, &RoofError{Reason: ReasonGableOutOfRange, Edge: g, Other: -1}
		case gable[g]:
			return nil, 0, &RoofError{Reason: ReasonGableRepeated, Edge: g, Other: -1}
		}
		gable[g] = true
	}
	if len(spec.Gables) == n {
		return nil, 0, &RoofError{Reason: ReasonNoSlopedEdge, Edge: -1, Other: -1}
	}
	if err := checkOrigin("eaves", spec.Eaves); err != nil {
		return nil, 0, err
	}
	if total, err = checkLayers(layers); err != nil {
		return nil, 0, err
	}
	if drop := total / math.Cos(spec.Pitch*math.Pi/180); drop > maxLocalExtent {
		return nil, 0, tooLarge("the roof's drop %.6g m is over the %g m maximum", drop, maxLocalExtent)
	}
	area := math.Abs(polygonArea2D(localRing(outline)))
	for g := range gable {
		p, q := (g+n-1)%n, (g+1)%n
		switch {
		case !gable[g]:
		case !gable[p] && onLine(outline[p], outline[g], outline[q], area):
			return nil, 0, &RoofError{Reason: ReasonGableCollinear, Edge: g, Other: p}
		case !gable[q] && onLine(outline[g], outline[q], outline[(q+1)%n], area):
			return nil, 0, &RoofError{Reason: ReasonGableCollinear, Edge: g, Other: q}
		}
	}
	return gable, total, nil
}

// onLine reports whether b lies between a and c and on their line, within
// a nanometre per metre of a plan of the given area: edges a-b and b-c are
// one line however short either is.
func onLine(a, b, c [2]float64, area float64) bool {
	return (b[0]-a[0])*(c[0]-b[0])+(b[1]-a[1])*(c[1]-b[1]) > 0 &&
		math.Abs(cross2D(a, b, c)) <= skelPointTol*math.Sqrt(area)*dist2(a, c)
}

// plumbLine returns the outline as a counter-clockwise ring relative to
// outline[0], each edge moved outward by overhang along its own normal and
// the corners mitred. Ring edge m is the caller's edge edgeOf[m]: the two
// differ only when the outline is clockwise, which reverses the ring.
// An overhang that closes a notch, or a corner that runs into another edge,
// is a *RoofError naming the caller's edges.
func plumbLine(outline [][2]float64, overhang float64) (ring [][2]float64, edgeOf []int, err error) {
	n := len(outline)
	ring = localRing(outline)
	edgeOf = make([]int, n)
	for m := range edgeOf {
		edgeOf[m] = m
	}
	if polygonArea2D(ring) < 0 {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			ring[i], ring[j] = ring[j], ring[i]
		}
		for m := range edgeOf {
			edgeOf[m] = (n - 2 - m + n) % n // ring edge m is the caller's edge n-2-m, run backwards
		}
	}
	refuse := func(i, j int) error {
		return &RoofError{Reason: ReasonOverhangClosesOutline, Edge: edgeOf[i], Other: edgeOf[j]}
	}
	normal := make([][2]float64, n)
	for i := range ring {
		a, b := ring[i], ring[(i+1)%n]
		l := math.Hypot(b[0]-a[0], b[1]-a[1])
		normal[i] = [2]float64{(b[1] - a[1]) / l, -(b[0] - a[0]) / l}
	}
	out := make([][2]float64, n)
	for k := range ring {
		p, q := (k+n-1)%n, k
		d := 1 + normal[p][0]*normal[q][0] + normal[p][1]*normal[q][1]
		if d < 1e-9 {
			return nil, nil, refuse(p, q)
		}
		f := overhang / d
		out[k] = [2]float64{ring[k][0] + f*(normal[p][0]+normal[q][0]), ring[k][1] + f*(normal[p][1]+normal[q][1])}
		if math.Hypot(out[k][0], out[k][1]) > maxLocalExtent {
			return nil, nil, tooLarge("a mitred corner is more than %g m from the first point", maxLocalExtent)
		}
	}
	for i := range out {
		j := (i + 1) % n
		dx, dy := out[j][0]-out[i][0], out[j][1]-out[i][1]
		if math.Hypot(dx, dy) < minDim || dx*(ring[j][0]-ring[i][0])+dy*(ring[j][1]-ring[i][1]) <= 0 {
			return nil, nil, refuse(i, j) // the edge shrank to nothing or turned round
		}
	}
	if i, j, ok := plumbCrossing(out); ok {
		return nil, nil, refuse(i, j)
	}
	return out, edgeOf, nil
}

// plumbCrossing finds two edges of ring that cross or overlap, or a point
// within 1 mm of an edge it is not on (as checkOutline refuses them), and
// returns the edges.
func plumbCrossing(ring [][2]float64) (i, j int, found bool) {
	n := len(ring)
	for a := 0; a < n; a++ {
		sa := touchEdge{ring[a], ring[(a+1)%n], 0}
		sa.len = math.Hypot(sa.b[0]-sa.a[0], sa.b[1]-sa.a[1])
		for b := a + 1; b < n; b++ {
			sb := touchEdge{ring[b], ring[(b+1)%n], 0}
			sb.len = math.Hypot(sb.b[0]-sb.a[0], sb.b[1]-sb.a[1])
			if sa.len > unionTouch && sb.len > unionTouch && sa.crossesOrOverlaps(sb) {
				return a, b, true
			}
		}
	}
	for k, p := range ring {
		for e := range ring {
			if e == k || (e+1)%n == k {
				continue
			}
			if segmentDistance(p, ring[e], ring[(e+1)%n]) < minDim {
				return (k + n - 1) % n, e, true
			}
		}
	}
	return 0, 0, false
}
