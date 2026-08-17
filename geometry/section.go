package geometry

import (
	"math"
	"sort"
)

// Section-cut ring extraction: slice a triangle mesh at an arbitrary plane and
// recover the closed poché rings in that plane's UV coordinates. Manifold-hardened
// and deterministic — the same input always yields byte-identical output, so
// downstream source-model drift detection sees no false churn.

const (
	sectionWeldQuantum = 1e5  // 1e-5 m weld quantum (matches footprintPerimeter)
	sectionOnPlaneEps  = 1e-9 // signed distance from the plane below this counts a vertex as on-plane
	sectionAreaEps     = 1e-12
)

// weldUV quantizes a plane-UV point to an integer key at 1e-5 m resolution.
// Go brep meshes emit unshared vertices per face, so section-segment endpoints
// must be welded by position or rings never close.
func weldUV(q [2]float64) [2]int64 {
	return [2]int64{
		int64(math.Round(q[0] * sectionWeldQuantum)),
		int64(math.Round(q[1] * sectionWeldQuantum)),
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

// triCrossing returns the crossing UV points of a triangle that genuinely spans
// the plane (it has at least one strictly-above and one strictly-below vertex).
// On-plane vertices count as crossing points. A spanning triangle yields exactly
// two distinct points; anything else is rejected by the caller.
func triCrossing(p Plane, verts [3]v3, ds [3]float64, ss [3]int) [][2]float64 {
	var pts [][2]float64
	add := func(q [2]float64) {
		for _, r := range pts {
			if r == q {
				return
			}
		}
		pts = append(pts, q)
	}
	for i := 0; i < 3; i++ {
		j := (i + 1) % 3
		if ss[i] == 0 {
			add(projectUV(p, verts[i]))
		}
		if ss[i]*ss[j] < 0 { // strict sign change across this edge
			tt := ds[i] / (ds[i] - ds[j])
			// Interpolate in 3D, then project: projection is affine, so this
			// agrees with interpolating in-plane, and it stays correct for a
			// plane that is not axis-aligned.
			add(projectUV(p, v3{
				verts[i][0] + tt*(verts[j][0]-verts[i][0]),
				verts[i][1] + tt*(verts[j][1]-verts[i][1]),
				verts[i][2] + tt*(verts[j][2]-verts[i][2]),
			}))
		}
	}
	return pts
}

// sectionRings returns the closed rings where the triangle mesh crosses plane p,
// in p's UV coordinates, meters. Empty when the plane misses the mesh. Each ring
// is wound CCW, canonical (rotated to its lexicographically-smallest vertex),
// and the returned slice is sorted deterministically — same input ALWAYS yields
// byte-identical output.
func sectionRings(w []v3, tris []uint32, p Plane) [][][2]float64 {
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
		ds := [3]float64{signedDist(p, verts[0]), signedDist(p, verts[1]), signedDist(p, verts[2])}
		ss := [3]int{planeSign(ds[0]), planeSign(ds[1]), planeSign(ds[2])}
		// Only genuine spans emit a segment; a face merely resting on / touching
		// the plane (coplanar, or one on-plane edge) does not span it.
		hasAbove := ss[0] > 0 || ss[1] > 0 || ss[2] > 0
		hasBelow := ss[0] < 0 || ss[1] < 0 || ss[2] < 0
		if !hasAbove || !hasBelow {
			continue
		}
		pts := triCrossing(p, verts, ds, ss)
		if len(pts) != 2 {
			continue
		}
		segs = append(segs, [2][2]float64{pts[0], pts[1]})
	}
	return stitchParityRings(segs)
}

// LoopRole tags a footprint loop as section poché or light context.
//
// The string VALUES are a serialization contract: consumers persist them in
// drawing data and match them as literals in renderer code. LoopSilhouette's
// value stays "below" for that reason — it is the name that was wrong, not the
// value. Do not change the values without a coordinated consumer and data
// migration.
type LoopRole string

const (
	LoopCut LoopRole = "cut" // section poché — the plane crosses the solid

	// LoopSilhouette is the outline of faces opposing the plane normal, drawn
	// as light context. Named "below" historically, when the only supported
	// plane was horizontal and this was always the view from above.
	LoopSilhouette LoopRole = "below"
)

// Loop is one closed ring of an element's plan footprint, in the cutting
// plane's UV coordinates, meters (for HorizontalPlane these are world X and
// Y). Coordinates are emitted in the IFC-native orientation; a renderer whose
// Y axis points down applies its own flip. Outer rings are wound CCW; HOLE
// rings (an inner boundary of a hollow/annular section) are wound CW and
// share the outer ring's Role — so an even-odd or nonzero polygon fill
// renders them as cutouts with no extra field.
type Loop struct {
	Role   LoopRole
	Points [][2]float64
}

// SectionOn returns the closed CUT rings where e's mesh crosses p, in p's UV
// coordinates (meters), hole-nested and tagged LoopCut. Winding and determinism
// guarantees match the horizontal path exactly.
//
// Returns nil when the plane misses the mesh, the mesh is degenerate, p's
// basis is invalid, or the plane contains a solid's edges while bisecting it
// (a triangle only emits a crossing segment when it has a vertex strictly
// above AND a vertex strictly below the plane; a face that merely touches the
// plane along an edge contributes nothing, so the cut ring cannot close — a
// known limitation, not a genuine miss; see
// TestSectionOnPlaneContainingEdgesIsKnownGap). Unlike FootprintOn it never
// falls back to a silhouette or a bounding box: a caller building a section
// wants to know the plane missed rather than receive a fabricated outline.
func (e Element) SectionOn(p Plane) []Loop {
	if !p.Valid() || len(e.Tris) < 3 || len(e.Verts) < 9 {
		return nil
	}
	cut := sectionRings(worldPoints(e.Verts, e.Placement), e.Tris, p)
	if len(cut) == 0 {
		return nil
	}
	return nestEvenOdd(cut, LoopCut)
}

// FootprintOn is the plan geometry of ONE element on plane p (world meters, in
// p's UV frame): the section-cut rings (poché, hole-nested) if the plane crosses
// the solid, else the silhouette of faces opposing p.N drawn as context, else
// the element's world AABB projected into p's UV.
//
// Returns nil when p's basis is invalid (see Plane) — no rings rather than
// wrongly-wound ones — and likewise when the last-resort AABB fallback is
// reached with a non-finite bounding box, since projecting one yields NaN
// coordinates rather than a rectangle.
//
// The plane-contains-edges gap documented on SectionOn is WORSE here: instead
// of returning nil, FootprintOn falls through to the silhouette branch and
// returns one ring tagged LoopSilhouette — a genuine section rendered as light
// context rather than as cut poché, with no signal that a real cut was
// missed. A caller that needs to reliably distinguish a real cut from context
// must not rely on Role alone in this case.
//
// For a NON-CLOSED mesh the silhouette branch is direction-dependent: an open
// or one-sided surface opposes only one of the two normal directions, so the
// flipped direction yields no silhouette and falls through to the bounding-box
// fallback — a rectangle tagged LoopSilhouette in place of the real outline,
// with no signal. Closed solids are unaffected: their outline is invariant
// under flipping p.N (see silhouetteRings).
func FootprintOn(e Element, p Plane) []Loop {
	if !p.Valid() {
		return nil
	}
	if len(e.Tris) < 3 || len(e.Verts) < 9 {
		return aabbFallback(e, p)
	}
	w := worldPoints(e.Verts, e.Placement)
	if cut := sectionRings(w, e.Tris, p); len(cut) > 0 {
		return nestEvenOdd(cut, LoopCut)
	}
	if below := silhouetteRings(w, e.Tris, p); len(below) > 0 {
		return nestEvenOdd(below, LoopSilhouette)
	}
	return aabbFallback(e, p)
}

// aabbFallback is the last-resort projected-AABB loop, or nil when the box is
// not finite. An element with no mesh never had its bounds measured, so it can
// still carry worldAABB's empty sentinel (+Inf min, -Inf max) — and unlike the
// old world-XY rectangle, which passed those through, projecting them multiplies
// an infinity by a zero basis component and yields NaN. Emitting nil says "no
// footprint" honestly instead of shipping four NaN corners downstream.
func aabbFallback(e Element, p Plane) []Loop {
	if !finite3(e.BBoxMin) || !finite3(e.BBoxMax) {
		return nil
	}
	return []Loop{{Role: LoopSilhouette, Points: aabbRingOn(e.BBoxMin, e.BBoxMax, p)}}
}

// Footprint is FootprintOn at the horizontal plane z = cutZ, for callers that
// only ever wanted a floor plan. Behaviour is identical to the pre-Plane
// implementation for every finite cutZ.
//
// One deliberate difference: a non-finite cutZ now yields nil, where the old
// implementation returned a silhouette or AABB ring built around NaN. Callers
// that indexed the result unconditionally should check its length.
func Footprint(e Element, cutZ float64) []Loop {
	return FootprintOn(e, HorizontalPlane(cutZ))
}

// nestEvenOdd classifies each ring by containment depth and emits Loops: a ring
// contained by an odd number of others is a HOLE (wound CW, negative area); an
// even-depth ring stays an outer boundary (CCW). Rings arrive CCW from
// sectionRings/silhouetteRings; a hollow cut returns the outer AND inner boundary as
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

// aabbRingOn is the element's world AABB projected into p's UV frame: the 2D
// bounding rectangle of the box's eight corners, CCW. The last-resort footprint
// when a mesh yields no usable rings.
//
// This is a bounding rectangle, not a silhouette — the same approximation the
// horizontal path always made, now expressed in the caller's frame instead of
// world XY. For HorizontalPlane it reduces to exactly the old world-XY
// rectangle.
func aabbRingOn(min, max [3]float64, p Plane) [][2]float64 {
	uMin, vMin := math.Inf(1), math.Inf(1)
	uMax, vMax := math.Inf(-1), math.Inf(-1)
	for i := 0; i < 8; i++ {
		c := v3{min[0], min[1], min[2]}
		if i&1 != 0 {
			c[0] = max[0]
		}
		if i&2 != 0 {
			c[1] = max[1]
		}
		if i&4 != 0 {
			c[2] = max[2]
		}
		q := projectUV(p, c)
		uMin, uMax = math.Min(uMin, q[0]), math.Max(uMax, q[0])
		vMin, vMax = math.Min(vMin, q[1]), math.Max(vMax, q[1])
	}
	return [][2]float64{{uMin, vMin}, {uMax, vMin}, {uMax, vMax}, {uMin, vMax}}
}

// stitchParityRings welds the given plane-UV segment endpoints (quantum 1e-5 m),
// keeps odd-parity (unshared/boundary) edges, and walks them into closed rings —
// canonical (rotated to lex-smallest vertex), CCW, self-intersection-rejected,
// and deterministically sorted so identical input yields byte-identical output.
// Shared by sectionRings (cut crossings) and silhouetteRings (the union boundary
// of the faces opposing the plane normal).
func stitchParityRings(segs [][2][2]float64) [][][2]float64 {
	return stitchRings(segs, func(count int) bool { return count%2 == 1 })
}

// stitchRings is stitchParityRings with the edge-selection rule lifted out: it
// welds endpoints, counts coincident duplicates, keeps the edges keep() accepts,
// and walks them into canonical CCW rings. Parity is one such rule (the cut and
// legacy silhouette paths); the union silhouette keeps every welded edge,
// because it has already decided which edges bound the union and emits each
// exactly once.
func stitchRings(segs [][2][2]float64, keep func(count int) bool) [][][2]float64 {
	keyToID := map[[2]int64]int{}
	var pos [][2]float64
	idOf := func(p [2]float64) int {
		k := weldUV(p)
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

	// Build undirected adjacency from the edges keep() accepts. Under parity,
	// cancellation yields the union outline of multi-solid elements (a face
	// shared by two solids emits its cut segment twice and cancels), at the cost
	// of erasing a boundary segment a non-manifold mesh legitimately emits twice.
	adj := map[int][]int{}
	for e, cnt := range segCount {
		if !keep(cnt) {
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
