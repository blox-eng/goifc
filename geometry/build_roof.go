package geometry

import (
	"cmp"
	"errors"
	"math"
	"slices"
)

// RoofQuantities are one roof layer's exact quantities. SlopeArea is its top
// surface, PlanArea that surface's projection: the outline offset by the
// overhang. The line lengths are in 3D on the top and the same on every
// layer.
type RoofQuantities struct {
	SlopeArea, PlanArea, Volume float64
	// Eaves: the sloped edges of the plumb line. Ridge: level lines between
	// two sloped planes. Hip and Valley: sloping lines where the roof folds
	// outward and inward. Verge and Top: the top's sloping and level edges
	// along gable ends.
	Eaves, Ridge, Hip, Valley, Verge, Top float64
}

// Roof is one layer of what BuildRoofLayers returns.
type Roof struct {
	Element    Element
	Quantities RoofQuantities
	Planes     []Face // one per sloped plane of the layer's top, by its first outline edge
}

// BuildRoofLayers builds a pitched roof over a plan outline (meters, either
// winding) in layers, top first. Every edge slopes up and inward at
// spec.Pitch, except the gable ends spec.Gables names. Each layer ends in
// vertical sides on the plumb line, the outline offset outward by
// spec.Overhang; a layer's thickness is measured normal to the slope, and the
// lowest layer's underside meets the outline line at spec.Eaves. Each layer is
// a Roof with its own volume; all share the plan area and the line lengths.
// Quantities come from the numbers; each mesh is a closed shell that agrees
// with them within float32 rounding. A roof the skeleton cannot build is a
// *RoofError naming the outline's edges.
// Invalid input returns an error and nothing else.
func BuildRoofLayers(outline [][2]float64, spec RoofSpec, layers []float64) ([]Roof, error) {
	gable, total, err := checkRoof(outline, spec, layers)
	if err != nil {
		return nil, err
	}
	ring, edgeOf, err := plumbLine(outline, spec.Overhang)
	if err != nil {
		return nil, err
	}
	speed := make([]float64, len(ring))
	for m := range ring {
		if !gable[edgeOf[m]] {
			speed[m] = 1
		}
	}
	planArea := polygonArea2D(ring)
	ring, edgeOf, speed = joinCollinear(ring, edgeOf, speed, skelPointTol*math.Sqrt(planArea))
	pitch := spec.Pitch * math.Pi / 180
	slope, c := math.Tan(pitch), math.Cos(pitch)
	b := newBudget()
	faces, arcs, err := weightedSkeleton(ring, speed, slope, b)
	if err != nil {
		return nil, callerEdges(err, edgeOf)
	}
	var rise float64
	for _, f := range faces {
		for _, p := range f.Poly {
			rise = max(rise, p[2])
		}
	}
	if h := rise + total/c; h > maxLocalExtent {
		return nil, tooLarge("the roof's height %.6g m is over the %g m maximum", h, maxLocalExtent)
	}
	slices.SortFunc(faces, func(x, y skelFace) int { return cmp.Compare(edgeOf[x.Edge], edgeOf[y.Edge]) })
	sh, err := roofShellOf(faces, b)
	if err != nil {
		return nil, err
	}
	lines := roofLines(ring, speed, arcs, slope)
	lines.PlanArea = planArea
	lines.SlopeArea = lines.PlanArea / c

	origin := [3]float64{outline[0][0], outline[0][1], spec.Eaves}
	// The plumb line sits Overhang beyond the outline, so that much lower.
	drop := spec.Overhang * slope
	roofs := make([]Roof, len(layers))
	var above float64
	for k, t := range layers {
		top := (total-above)/c - drop
		above += t
		q := lines
		q.Volume = q.PlanArea * t / c
		roofs[k] = Roof{
			Element:    newElement(sh.verts(top, (total-above)/c-drop), sh.tris(), origin),
			Quantities: q,
			Planes:     roofPlanes(ring, faces, origin, top, pitch),
		}
	}
	return roofs, nil
}

// joinCollinear drops each ring point between two sloped edges on one line,
// as a T-join leaves, so that one plane is one face with no line across it.
// The joined edge is the lower-numbered of the caller's two. A point stays
// when it is more than tol metres off the line its neighbours span, or not
// between them along it, whatever the length of the edges it splits.
func joinCollinear(ring [][2]float64, edgeOf []int, speed []float64, tol float64) ([][2]float64, []int, []float64) {
	for k := 0; k < len(ring); {
		n := len(ring)
		p := (k + n - 1) % n
		a, b, c := ring[p], ring[k], ring[(k+1)%n]
		if n > 3 && speed[p] != 0 && speed[k] != 0 && (b[0]-a[0])*(c[0]-b[0])+(b[1]-a[1])*(c[1]-b[1]) > 0 &&
			math.Abs(cross2D(a, b, c)) <= tol*dist2(a, c) {
			edgeOf[p] = min(edgeOf[p], edgeOf[k])
			ring, edgeOf, speed = slices.Delete(ring, k, k+1), slices.Delete(edgeOf, k, k+1), slices.Delete(speed, k, k+1)
			continue
		}
		k++
	}
	return ring, edgeOf, speed
}

// callerEdges maps a skeleton's *RoofError from ring edges to the caller's.
func callerEdges(err error, edgeOf []int) error {
	var re *RoofError
	if !errors.As(err, &re) {
		return err
	}
	at := func(m int) int {
		if m < 0 {
			return -1
		}
		return edgeOf[m]
	}
	return &RoofError{Reason: re.Reason, Edge: at(re.Edge), Other: at(re.Other)}
}

// roofShell is a roof layer's mesh before its heights: the top's points with
// their skeleton heights, its triangles, and the top's boundary edges, where
// the vertical sides stand. The bottom repeats the top's points.
type roofShell struct {
	pts [][3]float64
	cap []uint32 // counter-clockwise from above
	rim [][2]uint32
}

func roofShellOf(faces []skelFace, b *budget) (roofShell, error) {
	var sh roofShell
	// Points closer than the mesh grid snap are one point, and so is a chain
	// of them: a skeleton line can end a hair from a corner, too close for
	// float32 to keep apart, and a partial weld would pinch a face.
	var raw [][3]float64
	for _, f := range faces {
		raw = append(raw, f.Poly...)
	}
	root := make([]int, len(raw))
	for i := range root {
		root[i] = i
	}
	find := func(i int) int {
		for root[i] != i {
			root[i] = root[root[i]]
			i = root[i]
		}
		return i
	}
	cells := map[[2]int64][]int{}
	for i, p := range raw {
		c := [2]int64{int64(math.Floor(p[0] / gridSnap)), int64(math.Floor(p[1] / gridSnap))}
		for dx := int64(-1); dx <= 1; dx++ {
			for dy := int64(-1); dy <= 1; dy++ {
				near := cells[[2]int64{c[0] + dx, c[1] + dy}]
				if !b.spend(int64(len(near))) {
					return roofShell{}, errSkelTooComplex()
				}
				for _, j := range near {
					if q := raw[j]; math.Hypot(q[0]-p[0], q[1]-p[1]) < gridSnap {
						ri, rj := find(i), find(j)
						root[max(ri, rj)] = min(ri, rj)
					}
				}
			}
		}
		cells[c] = append(cells[c], i)
	}
	index := map[int]uint32{} // a cluster's first point to its mesh point
	next := 0
	for _, f := range faces {
		var plan [][2]float64
		var ids []uint32
		for range f.Poly {
			r := find(next)
			next++
			id, ok := index[r]
			if !ok {
				id = uint32(len(sh.pts))
				index[r] = id
				sh.pts = append(sh.pts, raw[r])
			}
			if len(ids) > 0 && id == ids[len(ids)-1] {
				continue
			}
			ids = append(ids, id)
			plan = append(plan, [2]float64{raw[r][0], raw[r][1]})
		}
		for len(ids) > 1 && ids[len(ids)-1] == ids[0] {
			ids, plan = ids[:len(ids)-1], plan[:len(plan)-1]
		}
		if len(ids) < 3 {
			continue // a sliver the weld closed
		}
		tris, ok := triangulatePolygon(plan, b)
		if !ok || len(tris) == 0 {
			return roofShell{}, errSkelTooComplex()
		}
		// One sign orients the face's triangles: the largest one's (see slabAt).
		var big float64
		for i := 0; i+2 < len(tris); i += 3 {
			if c := cross2D(plan[tris[i]], plan[tris[i+1]], plan[tris[i+2]]); math.Abs(c) > math.Abs(big) {
				big = c
			}
		}
		for i := 0; i+2 < len(tris); i += 3 {
			p, q, r := ids[tris[i]], ids[tris[i+1]], ids[tris[i+2]]
			if big < 0 {
				q, r = r, q
			}
			sh.cap = append(sh.cap, p, q, r)
		}
	}
	// The sides stand on the top's boundary: every cap edge no other
	// triangle shares, run as its triangle runs, the solid on its left. Faces
	// that overlap, however slightly, use an edge twice, and a pinch puts two
	// boundary edges through one point: either way the shell would not close.
	half := make(map[[2]uint32]bool, len(sh.cap))
	for i := 0; i+2 < len(sh.cap); i += 3 {
		t := sh.cap[i : i+3]
		if t[0] == t[1] || t[1] == t[2] || t[0] == t[2] {
			return roofShell{}, errSkelTooComplex()
		}
		for k := range 3 {
			e := [2]uint32{t[k], t[(k+1)%3]}
			if half[e] {
				return roofShell{}, errSkelTooComplex()
			}
			half[e] = true
		}
	}
	from, to := map[uint32]bool{}, map[uint32]bool{}
	for i := 0; i+2 < len(sh.cap); i += 3 {
		for k := range 3 {
			a, e := sh.cap[i+k], sh.cap[i+(k+1)%3]
			if half[[2]uint32{e, a}] {
				continue
			}
			if from[a] || to[e] {
				return roofShell{}, errSkelTooComplex()
			}
			from[a], to[e] = true, true
			sh.rim = append(sh.rim, [2]uint32{a, e})
		}
	}
	return sh, nil
}

// verts places the top at the skeleton heights raised by top and the bottom
// raised by bottom, top points first.
func (sh roofShell) verts(top, bottom float64) []float32 {
	v := make([]float32, 0, 6*len(sh.pts))
	for _, h := range [2]float64{top, bottom} {
		for _, p := range sh.pts {
			v = append(v, float32(p[0]), float32(p[1]), float32(p[2]+h))
		}
	}
	return v
}

func (sh roofShell) tris() []uint32 {
	n := uint32(len(sh.pts))
	t := make([]uint32, 0, 2*len(sh.cap)+6*len(sh.rim))
	for i := 0; i+2 < len(sh.cap); i += 3 {
		a, b, c := sh.cap[i], sh.cap[i+1], sh.cap[i+2]
		t = append(t, a, b, c, a+n, c+n, b+n)
	}
	for _, r := range sh.rim {
		a, b := r[0], r[1]
		t = append(t, a+n, b+n, b, a+n, b, a)
	}
	return t
}

// roofLevel is the rise, per metre of plan and over the slope, under which
// a line between planes is level: a ridge between eaves edges about 1° off
// parallel, or a top along a gable about ½° off its eaves, rises that much.
// Edges any closer are one roof drawn a little out of square.
const roofLevel = 1e-2

// roofFold is the sine of the turn between two sloped edges under which no
// line runs between their planes: they fold by under 1 mm per metre, finer
// than a survey or a drawing fixes a wall line, so the turn is noise and
// not a hip or a valley to build or price.
const roofFold = 1e-3

// roofLines measures the plumb line's sloped edges and the skeleton's arcs.
func roofLines(ring [][2]float64, speed []float64, arcs []skelArc, slope float64) RoofQuantities {
	var q RoofQuantities
	n := len(ring)
	inward := make([][2]float64, n)
	for m, a := range ring {
		e := ring[(m+1)%n]
		l := math.Hypot(e[0]-a[0], e[1]-a[1])
		inward[m] = [2]float64{-(e[1] - a[1]) / l, (e[0] - a[0]) / l}
		if speed[m] != 0 {
			q.Eaves += l
		}
	}
	for _, a := range arcs {
		d := [3]float64{a.B[0] - a.A[0], a.B[1] - a.A[1], a.B[2] - a.A[2]}
		l := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
		flat := math.Abs(d[2]) <= roofLevel*slope*math.Hypot(d[0], d[1])
		nl, nr := inward[a.Left], inward[a.Right]
		switch sl, sr := speed[a.Left] != 0, speed[a.Right] != 0; {
		case sl && sr:
			if nl[0]*nr[0]+nl[1]*nr[1] > 0 && math.Abs(nl[0]*nr[1]-nl[1]*nr[0]) <= roofFold {
				continue // no line: the edges are on one line, or near enough
			}
			// Just left of the arc the roof is Left's plane; Right's plane
			// rising above it there means the roof folds outward.
			outward := (nr[0]-nl[0])*-d[1]+(nr[1]-nl[1])*d[0] > 0
			switch {
			case flat:
				q.Ridge += l
			case outward:
				q.Hip += l
			default:
				q.Valley += l
			}
		case sl || sr:
			if flat {
				q.Top += l
			} else {
				q.Verge += l
			}
		}
	}
	return q
}

// roofPlanes frames each sloped face of a layer whose top is raised by top:
// U along its eaves edge, V up the slope, Normal = U × V.
func roofPlanes(ring [][2]float64, faces []skelFace, origin [3]float64, top, pitch float64) []Face {
	n := len(ring)
	sin, cos := math.Sin(pitch), math.Cos(pitch)
	planes := make([]Face, len(faces))
	for k, f := range faces {
		a, e := ring[f.Edge], ring[(f.Edge+1)%n]
		l := math.Hypot(e[0]-a[0], e[1]-a[1])
		d := [2]float64{(e[0] - a[0]) / l, (e[1] - a[1]) / l}
		in := [2]float64{-d[1], d[0]}
		out := make([][2]float64, len(f.Poly))
		for i, p := range f.Poly {
			r := [2]float64{p[0] - a[0], p[1] - a[1]}
			out[i] = [2]float64{r[0]*d[0] + r[1]*d[1], (r[0]*in[0] + r[1]*in[1]) / cos}
		}
		planes[k] = Face{
			Origin:  [3]float64{origin[0] + a[0], origin[1] + a[1], origin[2] + top},
			U:       [3]float64{d[0], d[1], 0},
			V:       [3]float64{in[0] * cos, in[1] * cos, sin},
			Normal:  [3]float64{-in[0] * sin, -in[1] * sin, cos},
			Outline: out,
		}
	}
	return planes
}
