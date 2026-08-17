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
// the triangle that emitted it always lies to its LEFT.
type subEdge struct{ a, b [2]float64 }

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
	segs := unionBoundary(facing)
	if len(segs) == 0 {
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
// projects to zero area (seen edge-on): it covers nothing, and orienting it is
// meaningless.
func appendProjected(dst [][3][2]float64, a, b, c [2]float64) [][3][2]float64 {
	tri := [3][2]float64{a, b, c}
	switch s := signedArea2(tri); {
	case s == 0:
		return dst
	case s < 0:
		tri[1], tri[2] = tri[2], tri[1]
	}
	return append(dst, tri)
}

// unionArea2D returns the EXACT area covered by the union of the given
// triangles, counting each covered point once however many triangles contain
// it. It integrates the union boundary directly (Green's theorem) rather than
// assembling rings: unionBoundary directs every edge with the covered side on
// its LEFT, so an enclosed void's boundary runs the other way and subtracts
// itself without needing to be identified as a hole.
func unionArea2D(tris [][3][2]float64) float64 {
	var twice float64
	for _, s := range unionBoundary(tris) {
		twice += s[0][0]*s[1][1] - s[1][0]*s[0][1]
	}
	return math.Abs(twice) / 2
}

// unionBoundary returns the sub-edges lying on the boundary of the union of the
// given CCW triangles, each directed so the covered side is on its LEFT.
//
// A sub-edge is on the boundary exactly when one of its two sides is covered by
// at least one triangle and the other by none. Because every edge has already
// been split at every intersection, no triangle boundary crosses a sub-edge's
// interior, so each side is uniformly covered and one sample settles it.
func unionBoundary(tris [][3][2]float64) [][2][2]float64 {
	edges := make([]subEdge, 0, 3*len(tris))
	for _, t := range tris {
		edges = append(edges,
			subEdge{t[0], t[1]}, subEdge{t[1], t[2]}, subEdge{t[2], t[0]})
	}
	split := splitAtIntersections(edges)

	// Group coincident sub-edges, counting how many run each way. A triangle
	// covers the side its CCW edge faces, so direction IS the coverage record.
	type tally struct {
		a, b             [2]float64 // canonical (lexicographically ordered)
		along, against   int
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
			g = &tally{a: a, b: b}
			groups[k] = g
		}
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
		var both int
		triGrid.query(mid[0], mid[1], mid[0], mid[1], func(i int) {
			if strictlyInsideCCW(mid, tris[i]) {
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
	return out
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
			out = append(out, subEdge{lerp2(e, ps[k]), lerp2(e, ps[k+1])})
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
