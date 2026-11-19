package geometry

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// maxHoles caps the holes of one slab.
const maxHoles = 64

// HoleReason says why a slab's hole was refused.
type HoleReason string

const (
	// ReasonHoleOutside: a hole point lies outside the outline, or a hole edge
	// runs on or within 1 mm of it.
	ReasonHoleOutside HoleReason = "outside"
	// ReasonHolesOverlap: the hole overlaps or touches (within 1 mm) an earlier
	// hole, or lies inside or around it.
	ReasonHolesOverlap HoleReason = "overlaps"
	// ReasonHoleSelfCrossing: the hole's edges cross, overlap or pinch.
	ReasonHoleSelfCrossing HoleReason = "self_crossing"
)

// HoleError refuses one hole of a slab, by its index. A builder returns every
// refused hole at once, joined with errors.Join.
type HoleError struct {
	Hole   int
	Reason HoleReason
}

func (e *HoleError) Error() string { return fmt.Sprintf("hole %d: %s", e.Hole, e.Reason) }

// BuildSlab builds a slab from a plan outline (meters, either winding, no
// holes): its top face is at z and it extends down by thickness. Quantities
// come from the numbers; the mesh is a closed shell that agrees with them
// within float32 rounding.
// Invalid input returns an error and nothing else.
func BuildSlab(outline [][2]float64, z, thickness float64) (Slab, error) {
	return BuildSlabPolygon(Polygon2D{Outer: outline}, z, thickness)
}

// BuildSlabPolygon builds a slab as BuildSlab does, with holes: a stair or a
// shaft through the floor. Each hole is a ring of 3 or more points in either
// winding (a repeated closing point is allowed) that lies inside the outline
// and apart from the other holes, at least 1 mm from both. Holes that do not
// fit come back as *HoleError values, all at once. At most 64 holes and 2,048
// points in all.
// Invalid input returns an error and nothing else.
func BuildSlabPolygon(p Polygon2D, z, thickness float64) (Slab, error) {
	if err := checkOutline(p.Outer); err != nil {
		return Slab{}, err
	}
	if err := checkOrigin("z", z); err != nil {
		return Slab{}, err
	}
	if err := checkDim("thickness", thickness); err != nil {
		return Slab{}, err
	}
	rings, cap, err := slabCap(p)
	if err != nil {
		return Slab{}, err
	}
	return slabAt(p.Outer, rings, cap, z, thickness), nil
}

// slabCap checks the holes of a checked outline and triangulates the cap once,
// for every slab layer. rings holds the outline and then the holes, relative
// to the outline's first point; cap indexes their points in that order.
func slabCap(p Polygon2D) (rings [][][2]float64, cap []uint32, err error) {
	ring := localRing(p.Outer)
	holes, err := checkHoles(ring, p.Outer[0], p.Holes, newBudget())
	if err != nil {
		return nil, nil, err
	}
	rings = append([][][2]float64{ring}, holes...)
	if len(holes) > 0 {
		_, src, tris, err := triangulateWithHoles(ring, holes, newBudget())
		if err != nil {
			return nil, nil, err
		}
		cap = make([]uint32, len(tris))
		for i, t := range tris {
			cap[i] = src[t]
		}
		return rings, cap, nil
	}
	cap, ok := triangulatePolygon(ring, newBudget())
	if !ok || len(cap) == 0 {
		return nil, nil, errCapBudget
	}
	return rings, cap, nil
}

// checkHoles refuses too many holes or points, and a hole that is not a ring
// of 3 or more finite points with edges of 1 mm or more. ring is the checked
// outline relative to its first point, origin. It returns the holes relative
// to origin without a repeated closing point, and every hole that does not
// fit as a *HoleError. The checks are charged to b.
func checkHoles(ring [][2]float64, origin [2]float64, holes [][][2]float64, b *budget) ([][][2]float64, error) {
	if len(holes) > maxHoles {
		return nil, tooLarge("%d holes, more than %d", len(holes), maxHoles)
	}
	total := len(ring)
	for _, h := range holes {
		total += len(h)
	}
	if total > maxOutlinePoints {
		return nil, tooLarge("%d points in the outline and its holes, more than %d", total, maxOutlinePoints)
	}
	rings := make([][][2]float64, len(holes))
	for k, h := range holes {
		r, err := holeRing(k, h, origin)
		if err != nil {
			return nil, err
		}
		rings[k] = r
	}
	var errs []error
	var kept [][][2]float64
	for k, r := range rings {
		// Pairs compared: r's edges with each other and its points with its
		// edges; then each other ring's edges with r's, both ways.
		n := int64(len(r))
		work := 3 * n * n / 2
		work += 3 * int64(len(ring)) * n
		for _, o := range kept {
			work += 3 * int64(len(o)) * n
		}
		if !b.spend(work) {
			return nil, errors.New("holes could not be checked within the work budget")
		}
		var reason HoleReason
		switch {
		case ringPinches(r) || edgesCrossOrOverlap([][][2]float64{r}):
			reason = ReasonHoleSelfCrossing
		case !ringsApart(ring, r) || !pointInPolygon(r[0], ring):
			reason = ReasonHoleOutside
		default:
			for _, o := range kept {
				if !ringsApart(o, r) || pointInPolygon(r[0], o) || pointInPolygon(o[0], r) {
					reason = ReasonHolesOverlap
					break
				}
			}
		}
		if reason != "" {
			errs = append(errs, &HoleError{Hole: k, Reason: reason})
			continue
		}
		kept = append(kept, r)
	}
	return rings, errors.Join(errs...)
}

// holeRing returns hole k relative to origin, without a repeated closing
// point, or the input error that refuses it.
func holeRing(k int, h [][2]float64, origin [2]float64) ([][2]float64, error) {
	if len(h) < 3 {
		return nil, fmt.Errorf("hole %d needs at least 3 points", k)
	}
	r := make([][2]float64, len(h))
	for i, p := range h {
		if !finite(p[0]) || !finite(p[1]) {
			return nil, fmt.Errorf("hole %d: point %d is not finite", k, i)
		}
		r[i] = [2]float64{p[0] - origin[0], p[1] - origin[1]}
		if math.Hypot(r[i][0], r[i][1]) > maxLocalExtent {
			return nil, tooLarge("hole %d: point %d is more than %g m from the outline's first point", k, i, maxLocalExtent)
		}
	}
	if r[0] == r[len(r)-1] {
		r = r[:len(r)-1]
	}
	if len(r) < 3 {
		return nil, fmt.Errorf("hole %d needs at least 3 points", k)
	}
	for i := range r {
		a, b := r[i], r[(i+1)%len(r)]
		if math.Hypot(b[0]-a[0], b[1]-a[1]) < minDim {
			return nil, fmt.Errorf("hole %d: edge %d is under 1 mm: repeated point?", k, i)
		}
	}
	return r, nil
}

// ringPinches reports whether a point of r lies within 1 mm of an edge it is
// not on. A ring that neither pinches nor crosses itself is at least 1 mm
// across, so it needs no area check.
func ringPinches(r [][2]float64) bool {
	n := len(r)
	for i, p := range r {
		for j := range r {
			if j != i && (j+1)%n != i && segmentDistance(p, r[j], r[(j+1)%n]) < minDim {
				return true
			}
		}
	}
	return false
}

// ringsApart reports whether two simple rings neither cross nor come within
// 1 mm of each other. Segments that do not cross are closest at an end of one
// of them. Each ring is already checked against itself.
func ringsApart(a, b [][2]float64) bool {
	if ringsCrossOrOverlap(a, b) {
		return false
	}
	near := func(pts, ring [][2]float64) bool {
		for _, p := range pts {
			for j := range ring {
				if segmentDistance(p, ring[j], ring[(j+1)%len(ring)]) < minDim {
					return true
				}
			}
		}
		return false
	}
	return !near(a, b) && !near(b, a)
}

// slabAt builds the slab whose top face is at z, from a triangulated cap:
// rings[0] is the outline and rings[1:] the holes, relative to outline[0].
func slabAt(outline [][2]float64, rings [][][2]float64, cap []uint32, z, thickness float64) Slab {
	var pts [][2]float64
	for _, r := range rings {
		pts = append(pts, r...)
	}
	n := uint32(len(pts))
	verts := make([]float32, 0, 6*n)
	for _, h := range []float64{0, -thickness} {
		for _, p := range pts {
			verts = append(verts, float32(p[0]), float32(p[1]), float32(h))
		}
	}
	// Ear clipping emits every triangle in one winding, so one sign orients
	// the whole cap: the largest triangle's. A small triangle's own sign
	// cannot: a straight-through point clips to a zero-area ear whose sign is
	// rounding noise.
	var big float64
	for i := 0; i+2 < len(cap); i += 3 {
		if c := cross2D(pts[cap[i]], pts[cap[i+1]], pts[cap[i+2]]); math.Abs(c) > math.Abs(big) {
			big = c
		}
	}
	tris := make([]uint32, 0, 2*len(cap)+6*int(n))
	for i := 0; i+2 < len(cap); i += 3 {
		a, b, c := cap[i], cap[i+1], cap[i+2]
		if big < 0 {
			b, c = c, b
		}
		tris = append(tris, a, b, c)       // top, facing +Z
		tris = append(tris, a+n, c+n, b+n) // bottom, facing -Z
	}
	// Sides face out of the solid: each ring walked with the solid on its
	// left, the outline counter-clockwise and a hole clockwise.
	var base uint32
	for k, r := range rings {
		m := uint32(len(r))
		solidLeft := (polygonArea2D(r) > 0) == (k == 0)
		for i := uint32(0); i < m; i++ {
			a, b := base+i, base+(i+1)%m
			if !solidLeft {
				a, b = b, a
			}
			tris = append(tris, a+n, b+n, b, a+n, b, a)
		}
		base += m
	}

	ring := rings[0]
	area := math.Abs(polygonArea2D(ring))
	var holes [][][2]float64
	var holePerim float64
	for _, h := range rings[1:] {
		area -= math.Abs(polygonArea2D(h))
		holePerim += ringPerimeter(h)
		holes = append(holes, slices.Clone(h))
	}
	origin := [3]float64{outline[0][0], outline[0][1], z}
	return Slab{
		Element: newElement(verts, tris, origin),
		Quantities: SlabQuantities{
			Area:          area,
			Perimeter:     ringPerimeter(ring),
			Volume:        area * thickness,
			HolePerimeter: holePerim,
		},
		Top: Face{
			Origin:  origin,
			U:       [3]float64{1, 0, 0},
			V:       [3]float64{0, 1, 0},
			Normal:  [3]float64{0, 0, 1},
			Outline: slices.Clone(ring),
			Holes:   holes,
		},
	}
}

func ringPerimeter(ring [][2]float64) float64 {
	var perim float64
	for i := range ring {
		a, b := ring[i], ring[(i+1)%len(ring)]
		perim += math.Hypot(b[0]-a[0], b[1]-a[1])
	}
	return perim
}
