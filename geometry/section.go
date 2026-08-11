package geometry

import (
	"math"
	"sort"
)

// Section-cut ring extraction: slice a triangle mesh at a horizontal plane
// z = cutZ and recover the closed world-XY poché rings. Manifold-hardened and
// deterministic — the same input always yields byte-identical output, so
// downstream source-model drift detection sees no false churn.

const (
	sectionWeldQuantum = 1e5  // 1e-5 m weld quantum (matches footprintPerimeter)
	sectionOnPlaneEps  = 1e-9 // |z-cutZ| below this counts a vertex as on-plane
	sectionAreaEps     = 1e-12
)

// weldXY quantizes a world-XY point to an integer key at 1e-5 m resolution.
// Go brep meshes emit unshared vertices per face, so section-segment endpoints
// must be welded by position or rings never close. Reused by later tasks.
func weldXY(p [2]float64) [2]int64 {
	return [2]int64{
		int64(math.Round(p[0] * sectionWeldQuantum)),
		int64(math.Round(p[1] * sectionWeldQuantum)),
	}
}

// planeSign classifies a signed z-distance against the on-plane epsilon.
func planeSign(d float64) int {
	if d > sectionOnPlaneEps {
		return 1
	}
	if d < -sectionOnPlaneEps {
		return -1
	}
	return 0
}

// triCrossing returns the crossing XY points of a triangle that genuinely spans
// the plane (it has at least one strictly-above and one strictly-below vertex).
// On-plane vertices count as crossing points. A spanning triangle yields exactly
// two distinct points; anything else is rejected by the caller.
func triCrossing(verts [3]v3, ds [3]float64, ss [3]int) [][2]float64 {
	var pts [][2]float64
	add := func(p [2]float64) {
		for _, q := range pts {
			if q == p {
				return
			}
		}
		pts = append(pts, p)
	}
	for i := 0; i < 3; i++ {
		j := (i + 1) % 3
		if ss[i] == 0 {
			add([2]float64{verts[i][0], verts[i][1]})
		}
		if ss[i]*ss[j] < 0 { // strict sign change across this edge
			tt := ds[i] / (ds[i] - ds[j])
			add([2]float64{
				verts[i][0] + tt*(verts[j][0]-verts[i][0]),
				verts[i][1] + tt*(verts[j][1]-verts[i][1]),
			})
		}
	}
	return pts
}

// sectionRings returns the closed world-XY rings where the triangle mesh crosses
// the horizontal plane z = cutZ. World XY, meters, Y-up (IFC-native — NEVER flip
// here). Empty when the plane misses the mesh. Each ring is wound CCW, canonical
// (rotated to start at its lexicographically-smallest vertex), and the returned
// slice is sorted deterministically — same input ALWAYS yields byte-identical output.
func sectionRings(w []v3, tris []uint32, cutZ float64) [][][2]float64 {
	nv := uint32(len(w))

	// Collect cut segments where triangles genuinely span the plane. Internal
	// shared walls (a face shared by two solids) emit their segment twice and
	// cancel by parity inside stitchParityRings.
	var segs [][2][2]float64
	for t := 0; t+2 < len(tris); t += 3 {
		i0, i1, i2 := tris[t], tris[t+1], tris[t+2]
		if i0 >= nv || i1 >= nv || i2 >= nv {
			continue
		}
		verts := [3]v3{w[i0], w[i1], w[i2]}
		ds := [3]float64{verts[0][2] - cutZ, verts[1][2] - cutZ, verts[2][2] - cutZ}
		ss := [3]int{planeSign(ds[0]), planeSign(ds[1]), planeSign(ds[2])}
		// Only genuine spans emit a segment; a face merely resting on / touching
		// the plane (coplanar, or one on-plane edge) does not span it.
		hasAbove := ss[0] > 0 || ss[1] > 0 || ss[2] > 0
		hasBelow := ss[0] < 0 || ss[1] < 0 || ss[2] < 0
		if !hasAbove || !hasBelow {
			continue
		}
		pts := triCrossing(verts, ds, ss)
		if len(pts) != 2 {
			continue
		}
		segs = append(segs, [2][2]float64{pts[0], pts[1]})
	}
	return stitchParityRings(segs)
}

// belowRings is the top-down silhouette of a solid: the boundary of its
// downward-facing (−Z normal) faces, stitched into closed world-XY rings.
// World XY, Y-up (no flip). Reuses stitchParityRings.
//
// LIMITATION: true below-context is the projected-polygon UNION of all
// downward patches. This parity-boundary approach is a lighter approximation —
// for a non-convex/furniture solid whose downward faces sit at different
// heights, overlapping projected patches may leave internal boundary edges
// rather than one filled silhouette. Acceptable because "below" is drawn as
// light context only; a full 2D polygon-union library is a possible follow-up.
func belowRings(w []v3, tris []uint32) [][][2]float64 {
	nv := uint32(len(w))

	var segs [][2][2]float64
	for t := 0; t+2 < len(tris); t += 3 {
		i0, i1, i2 := tris[t], tris[t+1], tris[t+2]
		if i0 >= nv || i1 >= nv || i2 >= nv {
			continue
		}
		n := crossv(subv(w[i1], w[i0]), subv(w[i2], w[i0]))
		l := math.Sqrt(dotv(n, n))
		if l == 0 || n[2]/l >= -1e-6 { // keep only downward-facing faces
			continue
		}
		p0 := [2]float64{w[i0][0], w[i0][1]}
		p1 := [2]float64{w[i1][0], w[i1][1]}
		p2 := [2]float64{w[i2][0], w[i2][1]}
		segs = append(segs,
			[2][2]float64{p0, p1},
			[2][2]float64{p1, p2},
			[2][2]float64{p2, p0},
		)
	}
	return stitchParityRings(segs)
}

// LoopRole tags a footprint loop as section poché or light below-context.
type LoopRole string

const (
	LoopCut   LoopRole = "cut"   // section poché — the plane crosses the solid
	LoopBelow LoopRole = "below" // top-down silhouette, light context
)

// Loop is one closed ring of an element's plan footprint, world XY meters, Y-up
// (the FE applies the single Y-flip). Outer rings are wound CCW; HOLE rings
// (an inner boundary of a hollow/annular section) are wound CW and share the
// outer ring's Role — so an even-odd / nonzero Path2D fill on the FE renders
// them as cutouts with no extra field.
type Loop struct {
	Role   LoopRole
	Points [][2]float64
}

// Footprint is the plan geometry of ONE element at cut height cutZ (world
// meters): the section-cut rings (poché, hole-nested) if the plane crosses the
// solid, else the top-down silhouette drawn as "below" context, else the
// world-AABB rectangle as a last resort.
func Footprint(e Element, cutZ float64) []Loop {
	if len(e.Tris) < 3 || len(e.Verts) < 9 {
		return []Loop{{Role: LoopBelow, Points: aabbRing(e.BBoxMin, e.BBoxMax)}}
	}
	w := worldPoints(e.Verts, e.Placement)
	if cut := sectionRings(w, e.Tris, cutZ); len(cut) > 0 {
		return nestEvenOdd(cut, LoopCut)
	}
	if below := belowRings(w, e.Tris); len(below) > 0 {
		return nestEvenOdd(below, LoopBelow)
	}
	return []Loop{{Role: LoopBelow, Points: aabbRing(e.BBoxMin, e.BBoxMax)}}
}

// nestEvenOdd classifies each ring by containment depth and emits Loops: a ring
// contained by an odd number of others is a HOLE (wound CW, negative area); an
// even-depth ring stays an outer boundary (CCW). Rings arrive CCW from
// sectionRings/belowRings; a hollow cut returns the outer AND inner boundary as
// separate positive rings, so the inner one must be re-wound CW to render as a
// cutout. Iterates rings in the given (deterministic) order — never ranges a map.
//
// Containment tests each ring's representative interior point (see
// representativePoint) against every OTHER ring via an even-odd ray cast
// (pointInPolygon). A ring's centroid is deliberately NOT used: concentric
// rings (outer + inner of a hollow section) share a centroid that lands in the
// hole, which would misclassify the outer ring as contained.
func nestEvenOdd(rings [][][2]float64, role LoopRole) []Loop {
	out := make([]Loop, 0, len(rings))
	for i, r := range rings {
		c := representativePoint(r)
		depth := 0
		for j, other := range rings {
			if i == j {
				continue
			}
			if pointInPolygon(c, other) {
				depth++
			}
		}
		pts := r
		if depth%2 == 1 { // contained -> hole -> wind CW
			pts = reversePts(r)
		}
		out = append(out, Loop{Role: role, Points: pts})
	}
	return out
}

// representativePoint returns a point strictly inside the (CCW) ring, close to
// its boundary so it stays within the ring's own area rather than a nested
// child's. It nudges the first edge's midpoint inward along the edge's left
// normal (interior side for CCW winding) by a small fraction of the edge
// length. Robust for the axis-aligned box footprints this v1 targets.
func representativePoint(r [][2]float64) [2]float64 {
	a, b := r[0], r[1]
	mid := [2]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}
	dx, dy := b[0]-a[0], b[1]-a[1]
	if dx == 0 && dy == 0 {
		return mid
	}
	// Unit left normal (-dy, dx)/l points into a CCW ring's interior; nudge by
	// a small fraction (1e-3) of the edge length l, i.e. normal*(l*1e-3).
	const frac = 1e-3
	return [2]float64{mid[0] - dy*frac, mid[1] + dx*frac}
}

// reversePts returns a reversed copy of the point slice (flips winding).
func reversePts(r [][2]float64) [][2]float64 {
	out := make([][2]float64, len(r))
	for i, p := range r {
		out[len(r)-1-i] = p
	}
	return out
}

// pointInPolygon reports whether p is inside poly by even-odd ray casting.
func pointInPolygon(p [2]float64, poly [][2]float64) bool {
	inside := false
	n := len(poly)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		yi, yj := poly[i][1], poly[j][1]
		if (yi > p[1]) != (yj > p[1]) {
			xAt := poly[i][0] + (p[1]-yi)/(yj-yi)*(poly[j][0]-poly[i][0])
			if p[0] < xAt {
				inside = !inside
			}
		}
	}
	return inside
}

// aabbRing is the world-XY rectangle of an element's AABB — the last-resort
// footprint when a mesh yields no usable rings. CCW, 4 points.
func aabbRing(min, max [3]float64) [][2]float64 {
	return [][2]float64{
		{min[0], min[1]}, {max[0], min[1]}, {max[0], max[1]}, {min[0], max[1]},
	}
}

// stitchParityRings welds the given XY segment endpoints (quantum 1e-5 m), keeps
// odd-parity (unshared/boundary) edges, and walks them into closed rings —
// canonical (rotated to lex-smallest vertex), CCW, self-intersection-rejected,
// and deterministically sorted so identical input yields byte-identical output.
// Shared by sectionRings (cut crossings) and belowRings (downward-face edges).
func stitchParityRings(segs [][2][2]float64) [][][2]float64 {
	keyToID := map[[2]int64]int{}
	var pos [][2]float64
	idOf := func(p [2]float64) int {
		k := weldXY(p)
		if id, ok := keyToID[k]; ok {
			return id
		}
		id := len(pos)
		keyToID[k] = id
		pos = append(pos, p)
		return id
	}

	// Weld endpoints to ids and count coincident duplicates so shared edges
	// cancel by parity.
	type edge [2]int
	segCount := map[edge]int{}
	for _, s := range segs {
		a, b := idOf(s[0]), idOf(s[1])
		if a == b { // zero-length welded segment
			continue
		}
		if a > b {
			a, b = b, a
		}
		segCount[edge{a, b}]++
	}

	// Build undirected adjacency from odd-parity edges only. Parity cancellation
	// yields the union outline of multi-solid elements (a face shared by two
	// solids emits its cut segment twice and cancels), at the cost of erasing a
	// boundary segment a non-manifold mesh legitimately emits twice.
	adj := map[int][]int{}
	for e, cnt := range segCount {
		if cnt%2 == 0 {
			continue
		}
		adj[e[0]] = append(adj[e[0]], e[1])
		adj[e[1]] = append(adj[e[1]], e[0])
	}
	if len(adj) == 0 {
		return nil
	}

	// Deterministic start order over directed half-edges (by endpoint position).
	var starts [][2]int
	for u, ns := range adj {
		for _, v := range ns {
			starts = append(starts, [2]int{u, v})
		}
	}
	sort.Slice(starts, func(i, j int) bool {
		a, b := starts[i], starts[j]
		if !ptEqual(pos[a[0]], pos[b[0]]) {
			return ptLess(pos[a[0]], pos[b[0]])
		}
		return ptLess(pos[a[1]], pos[b[1]])
	})

	used := map[[2]int]bool{}
	var rings [][][2]float64
	for _, st := range starts {
		if used[st] {
			continue
		}
		loop := walkFace(st, adj, pos, used)
		if len(loop) < 3 {
			continue
		}
		poly := make([][2]float64, len(loop))
		for i, id := range loop {
			poly[i] = pos[id]
		}
		// Keep interior faces (walked CCW, positive area); the outer boundary of
		// each component is walked CW (negative) and dropped as a duplicate.
		if polygonArea2D(poly) <= sectionAreaEps {
			continue
		}
		poly = ensureCCW(poly)
		if ringSelfIntersects(poly) { // reject bowties
			continue
		}
		rings = append(rings, canonicalRing(poly))
	}

	sort.Slice(rings, func(i, j int) bool { return ringLess(rings[i], rings[j]) })
	return rings
}

// walkFace traces one closed face starting along directed edge start, always
// taking the sharpest left turn at each vertex so degree>2 junctions resolve to
// the correct planar-subdivision walk instead of a tangled loop. Returns the
// ordered vertex ids (implicitly closed).
func walkFace(start [2]int, adj map[int][]int, pos [][2]float64, used map[[2]int]bool) []int {
	prev, cur := start[0], start[1]
	used[[2]int{prev, cur}] = true
	loop := []int{prev}
	for cur != start[0] {
		loop = append(loop, cur)
		next, ok := nextEdge(prev, cur, adj, pos, used)
		if !ok {
			return nil // dangling — malformed input
		}
		used[[2]int{cur, next}] = true
		prev, cur = cur, next
		if len(loop) > len(pos)+1 { // safety against pathological cycles
			return nil
		}
	}
	return loop
}

// nextEdge picks the unused neighbor of cur making the sharpest left turn
// relative to the incoming direction cur-prev. It avoids reversing back to prev
// unless that is the only remaining option. Ties break by smallest vertex id, so
// the choice never depends on map iteration order.
func nextEdge(prev, cur int, adj map[int][]int, pos [][2]float64, used map[[2]int]bool) (int, bool) {
	inx := pos[cur][0] - pos[prev][0]
	iny := pos[cur][1] - pos[prev][1]
	best, reverse := -1, -1
	bestTurn := math.Inf(-1)
	for _, n := range adj[cur] {
		if used[[2]int{cur, n}] {
			continue
		}
		if n == prev {
			reverse = n
			continue
		}
		ox := pos[n][0] - pos[cur][0]
		oy := pos[n][1] - pos[cur][1]
		turn := math.Atan2(inx*oy-iny*ox, inx*ox+iny*oy)
		if best == -1 || turn > bestTurn || (turn == bestTurn && n < best) {
			bestTurn = turn
			best = n
		}
	}
	if best != -1 {
		return best, true
	}
	if reverse != -1 {
		return reverse, true
	}
	return 0, false
}

// canonicalRing rotates a ring to begin at its lexicographically-smallest vertex
// (winding preserved) so equal rings compare byte-identical.
func canonicalRing(r [][2]float64) [][2]float64 {
	min := 0
	for i := range r {
		if ptLess(r[i], r[min]) {
			min = i
		}
	}
	out := make([][2]float64, len(r))
	for i := range r {
		out[i] = r[(min+i)%len(r)]
	}
	return out
}

// ringSelfIntersects reports whether any non-adjacent edge pair of the closed
// ring properly crosses (a bowtie). O(n²); rings are small.
func ringSelfIntersects(r [][2]float64) bool {
	n := len(r)
	for i := 0; i < n; i++ {
		a1, a2 := r[i], r[(i+1)%n]
		for j := i + 1; j < n; j++ {
			// skip edges sharing an endpoint
			if j == i || (i+1)%n == j || (j+1)%n == i {
				continue
			}
			if segsProperlyCross(a1, a2, r[j], r[(j+1)%n]) {
				return true
			}
		}
	}
	return false
}

func segsProperlyCross(p1, p2, p3, p4 [2]float64) bool {
	d1 := cross2D(p3, p4, p1)
	d2 := cross2D(p3, p4, p2)
	d3 := cross2D(p1, p2, p3)
	d4 := cross2D(p1, p2, p4)
	return ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0))
}

func ptEqual(a, b [2]float64) bool { return a[0] == b[0] && a[1] == b[1] }

func ptLess(a, b [2]float64) bool {
	if a[0] != b[0] {
		return a[0] < b[0]
	}
	return a[1] < b[1]
}

func ringLess(a, b [][2]float64) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if !ptEqual(a[i], b[i]) {
			return ptLess(a[i], b[i])
		}
	}
	return len(a) < len(b)
}
