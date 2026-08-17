package geometry

import (
	"math"
	"sort"
)

// paramEps collapses a split interval shorter than this fraction of an edge.
// Sub-edges below the stitcher's 1e-5 m weld quantum are welded away anyway;
// this only avoids emitting exactly-zero-length segments from coincident
// intersection parameters.
const paramEps = 1e-12

// subEdge is one directed segment of a projected triangle's CCW boundary, so
// the triangle that emitted it always lies to its LEFT. tri is the index of
// that triangle, carried through splitting so the coverage test can tell a
// triangle's own boundary from a triangle genuinely covering the segment.
type subEdge struct {
	a, b [2]float64
	tri  int
}

// silhouetteRings returns the projected-polygon UNION of the faces opposing
// p.N, in p's UV coordinates. Unlike the parity-boundary approach it replaces,
// overlapping patches at DIFFERENT depths along p.N merge into one filled
// outline rather than leaving internal boundary edges — the shape of a facade
// with recessed balconies, a set-back storey or a projecting bay.
//
// Exactness: the classification carries NO tolerance of its own. Every edge is
// split at every intersection first, so each resulting sub-edge has a uniformly
// covered region on either side, and coverage is then counted exactly (strict
// point-in-triangle plus directed-edge incidence). The only quantum in the path
// is the stitcher's existing 1e-5 m endpoint weld.
//
// For a CLOSED solid this outline is invariant under flipping p.N — each
// silhouette edge bounds the projection either way. An OPEN mesh has no such
// symmetry: a one-sided surface opposes exactly one of the two directions and
// yields nothing for the other, so callers holding non-closed geometry must
// choose p.N deliberately.
func silhouetteRings(w []v3, tris []uint32, p Plane) [][][2]float64 {
	facing := projectedFacing(w, tris, p)
	if len(facing) == 0 {
		return nil
	}
	// An outline the boundary walk could not close is not returned at all: a
	// partial ring set would render as a torn drawing and measure as nonsense.
	segs, ok := unionBoundary(facing)
	if !ok || len(segs) == 0 {
		return nil
	}
	// Each boundary sub-edge is emitted exactly once, so no parity cancellation:
	// keeping every welded edge IS the union outline.
	return stitchRings(segs, func(count int) bool { return count >= 1 })
}

// projectedFacing keeps the triangles whose normal opposes p.N, projects them
// into p's UV basis, and orients each CCW. Triangles that project to zero area
// (seen edge-on) are dropped: they cover nothing, and orienting them is
// meaningless.
func projectedFacing(w []v3, tris []uint32, p Plane) [][3][2]float64 {
	nv := uint32(len(w))
	var out [][3][2]float64
	for t := 0; t+2 < len(tris); t += 3 {
		i0, i1, i2 := tris[t], tris[t+1], tris[t+2]
		if i0 >= nv || i1 >= nv || i2 >= nv {
			continue
		}
		n := crossv(subv(w[i1], w[i0]), subv(w[i2], w[i0]))
		l := math.Sqrt(dotv(n, n))
		if l == 0 || dotv(n, p.N)/l >= -1e-6 { // keep only faces opposing p.N
			continue
		}
		out = appendProjected(out, projectUV(p, w[i0]), projectUV(p, w[i1]), projectUV(p, w[i2]))
	}
	return out
}

// appendProjected adds a projected triangle oriented CCW, skipping one that
// covers nothing: a triangle seen edge-on, or one too small for the weld
// quantum to represent.
//
// The weld test is not a size filter, it is a CONSISTENCY requirement. The
// boundary is stitched on vertices welded at 1e-5 m, so a triangle whose
// corners do not weld to three DISTINCT points is a segment there, not a
// triangle. Admitted anyway, its edges weld into phantom directed pairs that
// cancel the boundary edges of the real triangles beside it, and the outline
// opens — after which the area integral is unbounded garbage rather than a
// slightly-off number. Tessellators emit exactly this: a real 29 MB ArchiCAD
// IFC2X3 export carries slivers with corners 2e-7 m apart, a fiftieth of the
// quantum, alongside the genuine faces they belong to.
//
// The area given up is bounded by the quantum times the triangle's longest
// edge, which is below the resolution at which the boundary can be stated at
// all.
func appendProjected(dst [][3][2]float64, a, b, c [2]float64) [][3][2]float64 {
	ka, kb, kc := weldUV(a), weldUV(b), weldUV(c)
	if ka == kb || kb == kc || ka == kc {
		return dst
	}
	tri := [3][2]float64{a, b, c}
	switch s := signedArea2(tri); {
	case s == 0:
		return dst
	case s < 0:
		tri[1], tri[2] = tri[2], tri[1]
	}
	return append(dst, tri)
}

// silhouetteAreaAxis returns the area of the projected-polygon UNION of the
// faces pointing toward +axis, measured on the plane perpendicular to axis
// (0 = YZ, 1 = XZ, 2 = XY) — the same face family and the same plane as
// sideArea, counted once per covered square metre instead of once per face.
//
// The two agree on any solid whose faces do not hide one another along the
// axis, which is every prismatic wall. They diverge exactly where a Σ is wrong:
// a pilaster, a projecting bay or a brise-soleil puts one outward face in front
// of another, and only the union reports the area you can actually draw.
//
// The plane's in-plane basis is left to PlaneFromNormal: an area is invariant
// under rotation within the plane, so the choice cannot reach the result.
func silhouetteAreaAxis(w []v3, tris []uint32, axis int) (float64, bool) {
	// projectedFacing keeps the faces OPPOSING the normal, so aim it inward to
	// select the same outward family sideArea sums.
	var inward [3]float64
	inward[axis] = -1
	p, ok := PlaneFromNormal([3]float64{}, inward)
	if !ok {
		return 0, false
	}
	return unionArea2D(projectedFacing(w, tris, p))
}

// maxSilhouetteAxis is maxSideAreaAxis measured as a union: the largest
// projected silhouette over the three world axes, and the DROPPED coordinate
// index of that winning projection.
//
// The axis is voted on by union area too, not just measured that way. A Σ vote
// is swayed by depth — a stack of fins reads as five faces along the axis they
// hide one another on — and would hand back a plane that shows less than
// another one does.
//
// Ties break by lowest axis index, deterministically, so a host and its
// openings measured separately always agree on the winning plane.
// ok is false when an axis could not be measured AND could still have been the
// winner. A refused axis is only ignorable when something already bounds it
// below the best axis that did measure — sideArea is exactly that bound, since
// a union can never exceed the Σ of the same faces. Without that, refusing is
// the honest answer: reporting the best of the axes that happened to work would
// silently under-report a host whose true winner is the one that failed.
func maxSilhouetteAxis(w []v3, tris []uint32) (area float64, axis int, ok bool) {
	var refused [3]bool
	have := false
	for i := 0; i < 3; i++ {
		a, measured := silhouetteAreaAxis(w, tris, i)
		if !measured {
			refused[i] = true
			continue
		}
		if !have || a > area { // ties keep the lowest axis index
			area, axis, have = a, i, true
		}
	}
	if !have {
		return 0, 0, false
	}
	for i := 0; i < 3; i++ {
		if refused[i] && sideArea(w, tris, i) > area {
			return 0, 0, false
		}
	}
	return area, axis, true
}

// unionArea2D returns the EXACT area covered by the union of the given
// triangles, counting each covered point once however many triangles contain
// it. It integrates the union boundary directly (Green's theorem) rather than
// assembling rings: unionBoundary directs every edge with the covered side on
// its LEFT, so an enclosed void's boundary runs the other way and subtracts
// itself without needing to be identified as a hole.
// ok is false when the boundary did not close, in which case there is NO area
// to report — see unionBoundary. A caller must not substitute zero: zero is a
// measurement, and this is the absence of one.
func unionArea2D(tris [][3][2]float64) (area float64, ok bool) {
	segs, ok := unionBoundary(tris)
	if !ok {
		return 0, false
	}
	var twice float64
	for _, s := range segs {
		twice += s[0][0]*s[1][1] - s[1][0]*s[0][1]
	}
	return math.Abs(twice) / 2, true
}

// boundaryCloses reports whether every vertex of the directed boundary has as
// many segments leaving it as arriving, which is what lets the segments
// decompose into closed rings.
//
// This check is load-bearing rather than defensive. The area integral below is
// taken about the WORLD ORIGIN, so on an unclosed boundary it does not return a
// slightly wrong number — it returns the residual multiplied by the model's
// distance from the origin. A 0.58 m² facade panel 47 m out reports 30.69 m²,
// and every consumer downstream believes it. Refusing is the only honest answer
// available until the boundary walk is exact on real coordinates.
func boundaryCloses(segs [][2][2]float64) bool {
	degree := make(map[[2]int64]int, 2*len(segs))
	for _, s := range segs {
		degree[weldUV(s[0])]++
		degree[weldUV(s[1])]--
	}
	for _, d := range degree {
		if d != 0 {
			return false
		}
	}
	return true
}

// unionBoundary returns the sub-edges lying on the boundary of the union of the
// given CCW triangles, each directed so the covered side is on its LEFT.
//
// A sub-edge is on the boundary exactly when one of its two sides is covered by
// at least one triangle and the other by none. Because every edge has already
// been split at every intersection, no triangle boundary crosses a sub-edge's
// interior, so each side is uniformly covered and one sample settles it.
//
// That argument is exact in real arithmetic, and only there. The inputs are
// float32-derived world coordinates carrying tessellation noise, and the
// endpoints are welded at 1e-5 m, so on real geometry the classification can
// disagree with itself and leave the boundary open. ok reports whether it
// closed; when it did not, no segments are returned, because a partial boundary
// is worse than none — see boundaryCloses.
func unionBoundary(tris [][3][2]float64) ([][2][2]float64, bool) {
	edges := make([]subEdge, 0, 3*len(tris))
	for i, t := range tris {
		edges = append(edges,
			subEdge{t[0], t[1], i}, subEdge{t[1], t[2], i}, subEdge{t[2], t[0], i})
	}
	split := splitAtIntersections(edges)

	// Group coincident sub-edges, counting how many run each way. A triangle
	// covers the side its CCW edge faces, so direction IS the coverage record.
	type tally struct {
		a, b           [2]float64 // canonical (lexicographically ordered)
		along, against int
		owners         map[int]bool // triangles with an edge ALONG this sub-edge
	}
	groups := map[[2][2]int64]*tally{}
	for _, e := range split {
		a, b := e.a, e.b
		along := true
		if ptLess(b, a) {
			a, b, along = b, a, false
		}
		k := [2][2]int64{weldUV(a), weldUV(b)}
		g := groups[k]
		if g == nil {
			g = &tally{a: a, b: b, owners: map[int]bool{}}
			groups[k] = g
		}
		g.owners[e.tri] = true
		if along {
			g.along++
		} else {
			g.against++
		}
	}

	// Deterministic iteration: map order must never reach the output.
	keys := make([][2][2]int64, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		gi, gj := groups[keys[i]], groups[keys[j]]
		if !ptEqual(gi.a, gj.a) {
			return ptLess(gi.a, gj.a)
		}
		return ptLess(gi.b, gj.b)
	})

	// Index the triangles so the midpoint coverage test visits only the ones
	// whose bounding box can contain it, instead of all of them.
	triGrid := newGrid(len(tris), func(i int) (x0, y0, x1, y1 float64) {
		return triBounds(tris[i])
	})

	var out [][2][2]float64
	for _, k := range keys {
		g := groups[k]
		mid := [2]float64{(g.a[0] + g.b[0]) / 2, (g.a[1] + g.b[1]) / 2}
		// Triangles strictly containing the midpoint cover BOTH sides.
		//
		// A triangle whose own edge runs ALONG this sub-edge is excluded, and the
		// exclusion is what makes the test robust rather than an optimization. Such
		// a triangle has the midpoint ON its boundary, so strictlyInsideCCW must
		// return false — but that answer rests on an orientation cross-product
		// evaluating to an exact zero, which only happens when the midpoint is
		// exactly representable on its own edge's line. At real world coordinates
		// it is not: the sign becomes rounding noise, and on a sliver triangle all
		// three signs can agree, so the triangle swallows its own boundary edge and
		// the edge is dropped as interior. The union then leaks through the gap.
		//
		// Skipping them costs nothing, because along/against ALREADY records
		// exactly which side each of them covers. And no OTHER triangle's boundary
		// can pass through the midpoint — splitAtIntersections guarantees nothing
		// crosses a sub-edge's interior — so for every triangle still tested the
		// midpoint sits at a real distance from its boundary and the predicate is
		// well conditioned.
		var both int
		triGrid.query(mid[0], mid[1], mid[0], mid[1], func(i int) {
			if !g.owners[i] && strictlyInsideCCW(mid, tris[i]) {
				both++
			}
		})
		left, right := both+g.along, both+g.against
		if (left == 0) == (right == 0) {
			continue // both sides covered (interior) or neither (degenerate)
		}
		if left > 0 {
			out = append(out, [2][2]float64{g.a, g.b})
		} else {
			out = append(out, [2][2]float64{g.b, g.a})
		}
	}
	if !boundaryCloses(out) {
		return nil, false
	}
	return out, true
}

// splitAtIntersections cuts every edge at every point where another edge meets
// it — proper crossings, T-junctions, and collinear overlaps alike — so that no
// resulting sub-edge has another edge crossing its interior.
func splitAtIntersections(edges []subEdge) []subEdge {
	cuts := make([][]float64, len(edges))
	// Only edges whose bounding boxes overlap can meet, so a uniform grid turns
	// the pairwise scan into a local one. A pair sharing several cells is tested
	// more than once, which is harmless: the extra parameters are identical and
	// collapse when the split points are sorted.
	grid := newGrid(len(edges), func(i int) (x0, y0, x1, y1 float64) {
		return edgeBounds(edges[i])
	})
	for i := range edges {
		x0, y0, x1, y1 := edgeBounds(edges[i])
		grid.query(x0, y0, x1, y1, func(j int) {
			if j <= i {
				return
			}
			ti, tj := crossParams(edges[i], edges[j])
			cuts[i] = append(cuts[i], ti...)
			cuts[j] = append(cuts[j], tj...)
		})
	}
	var out []subEdge
	for i, e := range edges {
		ps := append([]float64{0, 1}, cuts[i]...)
		sort.Float64s(ps)
		for k := 0; k+1 < len(ps); k++ {
			if ps[k+1]-ps[k] <= paramEps {
				continue
			}
			out = append(out, subEdge{lerp2(e, ps[k]), lerp2(e, ps[k+1]), e.tri})
		}
	}
	return out
}

// gridMaxAxis caps the cells per axis so a sliver-shaped input cannot allocate
// an enormous grid. Exceeding it only costs locality, never correctness.
const gridMaxAxis = 512

// grid is a uniform bucketing of axis-aligned boxes over the input's own
// extent. It is a pure ACCELERATOR: it narrows which candidates get an exact
// test and never decides an outcome, so no tolerance of its own enters the
// result. Everything it returns is still checked exactly by the caller.
type grid struct {
	minX, minY, cell float64
	nx, ny           int
	cells            [][]int
}

// newGrid buckets n items, reading each one's bounds from bounds(i).
func newGrid(n int, bounds func(i int) (x0, y0, x1, y1 float64)) *grid {
	g := &grid{nx: 1, ny: 1}
	if n == 0 {
		g.cells = make([][]int, 1)
		return g
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for i := 0; i < n; i++ {
		x0, y0, x1, y1 := bounds(i)
		minX, minY = math.Min(minX, x0), math.Min(minY, y0)
		maxX, maxY = math.Max(maxX, x1), math.Max(maxY, y1)
	}
	w, h := maxX-minX, maxY-minY
	// Aim for about one item per cell, so roughly sqrt(n) cells per axis.
	axis := int(math.Sqrt(float64(n))) + 1
	if axis > gridMaxAxis {
		axis = gridMaxAxis
	}
	span := math.Max(w, h)
	if span <= 0 || math.IsInf(span, 0) || math.IsNaN(span) {
		g.minX, g.minY, g.cell = minX, minY, 1
		g.cells = make([][]int, 1)
		for i := 0; i < n; i++ {
			g.cells[0] = append(g.cells[0], i)
		}
		return g
	}
	g.minX, g.minY = minX, minY
	g.cell = span / float64(axis)
	g.nx = int(w/g.cell) + 1
	g.ny = int(h/g.cell) + 1
	g.cells = make([][]int, g.nx*g.ny)
	for i := 0; i < n; i++ {
		x0, y0, x1, y1 := bounds(i)
		ix0, iy0, ix1, iy1 := g.span(x0, y0, x1, y1)
		for iy := iy0; iy <= iy1; iy++ {
			for ix := ix0; ix <= ix1; ix++ {
				g.cells[iy*g.nx+ix] = append(g.cells[iy*g.nx+ix], i)
			}
		}
	}
	return g
}

// span clamps a box to the grid's cell index range.
func (g *grid) span(x0, y0, x1, y1 float64) (ix0, iy0, ix1, iy1 int) {
	clamp := func(v float64, n int) int {
		i := int(v)
		if i < 0 {
			return 0
		}
		if i >= n {
			return n - 1
		}
		return i
	}
	return clamp((x0-g.minX)/g.cell, g.nx), clamp((y0-g.minY)/g.cell, g.ny),
		clamp((x1-g.minX)/g.cell, g.nx), clamp((y1-g.minY)/g.cell, g.ny)
}

// query calls fn for every item whose cell the box touches. An item spanning
// several of the queried cells is reported once per shared cell, so fn must
// tolerate repeats.
func (g *grid) query(x0, y0, x1, y1 float64, fn func(i int)) {
	if len(g.cells) == 1 {
		for _, i := range g.cells[0] {
			fn(i)
		}
		return
	}
	ix0, iy0, ix1, iy1 := g.span(x0, y0, x1, y1)
	for iy := iy0; iy <= iy1; iy++ {
		for ix := ix0; ix <= ix1; ix++ {
			for _, i := range g.cells[iy*g.nx+ix] {
				fn(i)
			}
		}
	}
}

func edgeBounds(e subEdge) (x0, y0, x1, y1 float64) {
	return math.Min(e.a[0], e.b[0]), math.Min(e.a[1], e.b[1]),
		math.Max(e.a[0], e.b[0]), math.Max(e.a[1], e.b[1])
}

func triBounds(t [3][2]float64) (x0, y0, x1, y1 float64) {
	return math.Min(t[0][0], math.Min(t[1][0], t[2][0])),
		math.Min(t[0][1], math.Min(t[1][1], t[2][1])),
		math.Max(t[0][0], math.Max(t[1][0], t[2][0])),
		math.Max(t[0][1], math.Max(t[1][1], t[2][1]))
}

// crossParams returns the parameters in [0,1] at which a and b meet, reported
// separately along each. Collinear overlaps yield each segment's endpoints
// expressed in the other's parameter.
func crossParams(a, b subEdge) (onA, onB []float64) {
	d1 := [2]float64{a.b[0] - a.a[0], a.b[1] - a.a[1]}
	d2 := [2]float64{b.b[0] - b.a[0], b.b[1] - b.a[1]}
	den := d1[0]*d2[1] - d1[1]*d2[0]
	off := [2]float64{b.a[0] - a.a[0], b.a[1] - a.a[1]}
	if den != 0 {
		t := (off[0]*d2[1] - off[1]*d2[0]) / den
		u := (off[0]*d1[1] - off[1]*d1[0]) / den
		if t < 0 || t > 1 || u < 0 || u > 1 {
			return nil, nil
		}
		return []float64{t}, []float64{u}
	}
	// Parallel. Only a COLLINEAR pair can overlap; a mere offset cannot.
	if off[0]*d1[1]-off[1]*d1[0] != 0 {
		return nil, nil
	}
	return projectOnto(a, b.a, b.b), projectOnto(b, a.a, a.b)
}

// projectOnto expresses pts as parameters along e, keeping those strictly
// inside it (the endpoints are already split points).
func projectOnto(e subEdge, pts ...[2]float64) []float64 {
	d := [2]float64{e.b[0] - e.a[0], e.b[1] - e.a[1]}
	den := d[0]*d[0] + d[1]*d[1]
	if den == 0 {
		return nil
	}
	var out []float64
	for _, p := range pts {
		t := ((p[0]-e.a[0])*d[0] + (p[1]-e.a[1])*d[1]) / den
		if t > paramEps && t < 1-paramEps {
			out = append(out, t)
		}
	}
	return out
}

func lerp2(e subEdge, t float64) [2]float64 {
	return [2]float64{
		e.a[0] + t*(e.b[0]-e.a[0]),
		e.a[1] + t*(e.b[1]-e.a[1]),
	}
}

// signedArea2 is twice the signed area of a projected triangle: positive when
// its winding is CCW.
func signedArea2(t [3][2]float64) float64 {
	return (t[1][0]-t[0][0])*(t[2][1]-t[0][1]) - (t[2][0]-t[0][0])*(t[1][1]-t[0][1])
}

// strictlyInsideCCW reports whether p lies strictly inside the CCW triangle t —
// on an edge does NOT count, which is what lets directed-edge incidence carry
// the boundary cases exactly instead of an epsilon deciding them.
func strictlyInsideCCW(p [2]float64, t [3][2]float64) bool {
	for i := 0; i < 3; i++ {
		a, b := t[i], t[(i+1)%3]
		if (b[0]-a[0])*(p[1]-a[1])-(b[1]-a[1])*(p[0]-a[0]) <= 0 {
			return false
		}
	}
	return true
}
