package geometry

import (
	"math"
	"sort"
)

// unionRingClosure is how far the sum of a ring set's decomposed pieces may sit
// from the area its rings state, RELATIVE to that area, before the input is
// refused. The decomposition below is exact in real arithmetic, so the only
// gap a sound ring set can open is rounding in the sweep's interpolation —
// parts in 1e16, not 1e9. Anything larger means the rings do not describe the
// surface they claim: a hole outside its outer ring, two holes overlapping, a
// self-crossing boundary. See unionPolygonTriangles.
//
// This tolerance is RELATIVE while the rounding it bounds is ABSOLUTE: the
// rounding grows with the coordinates' distance from the frame origin, not
// with the area being measured. Left alone, that makes both the figure and the
// verdict a function of where the building stands — and a facade's (u, v) ARE
// world coordinates, because SilhouetteOn projects them and ElevationPlane's
// frame has no origin to subtract them against. On a projected national grid
// (eastings ~1e5–1e6 m, northings ~1e6–1e7 m, e.g. x≈4.6e5, y≈4.7e6) it
// failed in both directions: 1-5 m outlines were
// REFUSED (497 of 500 at 300 km), and a 50 m² outline whose raked edge crossed
// three slabs was ACCEPTED at 50.0001220703125.
//
// The union therefore recentres every ring on their shared bounding-box centre
// before measuring (see unionOffset). That removes the magnitude from the
// comparison, where loosening this number would only have moved the cliff
// further out.
const unionRingClosure = 1e-9

// Polygon2D is a hole-nested ring set in one plane's (u, v) frame, in metres:
// an outer ring and the voids inside it. A ring is implicitly closed — do not
// repeat its first point as its last.
//
// It is the shape [Element.SilhouetteOn] describes, one step further on. That
// method returns a FLAT []Loop, hole-nested by winding, and an outline can have
// SEVERAL outer loops: an element whose projection falls into disjoint
// patches, or an island standing inside an opening, comes back as one outer
// loop per patch. The conversion is therefore one Polygon2D per outer loop,
// each holding only the loops nested directly inside it — not one Polygon2D
// around the largest ring, which turns every other patch into a "hole" lying
// outside its outer and gets the whole outline refused.
// [PolygonsFromLoops] makes that conversion.
//
// Winding is not load-bearing here. The rings are filled even-odd, exactly as
// [Loop] says a renderer may treat them, so an outline that has been
// re-serialized by something with its own opinion about orientation still
// measures correctly.
type Polygon2D struct {
	Outer [][2]float64
	Holes [][][2]float64
}

// PolygonsFromLoops converts the flat, hole-nested rings of one outline —
// what [Element.SilhouetteOn], [Element.SectionOn] or [FootprintOn] returns
// for one element on one plane — into the Polygon2D values [UnionArea2D]
// takes: one per outer loop, in the order the outer loops appear, each
// holding the loops nested directly inside it.
//
// Nesting is decided by containment, not by winding, so loops that were
// stored and re-serialized by something with its own idea of orientation
// convert the same. A loop inside no other loop is an outer; a loop whose
// nearest enclosing loop is an outer is that outer's hole; a loop whose
// nearest enclosing loop is a hole is an outer again — an island standing in
// an opening.
//
// ok is false, and there are no polygons, when the loops cannot be nested
// honestly: a loop has fewer than three points, a non-finite coordinate or no
// area; two edges cross or run along each other, in one loop or across two,
// judged as [UnionArea2D] judges them; or a loop touches another at every
// point this could test it by, so which side of the other it lies on cannot
// be told, or it lies partly inside another, passing through it at a
// vertex. Loops may touch at points, as they may in [UnionArea2D].
//
// The loops of one outline never share an edge, so the overlap test refuses
// a loop set that repeats one: the same boundary listed twice describes no
// region, and measuring it would count a void as covered. Pass the loops of
// ONE element; the union of several elements is what [UnionArea2D] is for.
//
// Cost is O(e²) in the total edge count, for the same crossing check
// [UnionArea2D] makes, plus O(n·e) for nesting n loops.
func PolygonsFromLoops(loops []Loop) (polys []Polygon2D, ok bool) {
	rings := make([][][2]float64, len(loops))
	areas := make([]float64, len(loops))
	for i, l := range loops {
		for _, q := range l.Points {
			if math.IsNaN(q[0]) || math.IsNaN(q[1]) || math.IsInf(q[0], 0) || math.IsInf(q[1], 0) {
				return nil, false
			}
		}
		areas[i] = math.Abs(polygonArea2D(l.Points))
		if len(l.Points) < 3 || !(areas[i] > 0) {
			return nil, false
		}
		rings[i] = l.Points
	}
	if edgesCrossOrOverlap(rings) {
		return nil, false
	}

	// Largest first, so every loop's possible containers precede it. Two
	// loops of equal area cannot contain each other without overlapping.
	order := make([]int, len(rings))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return areas[order[a]] > areas[order[b]] })

	parent := make([]int, len(rings))
	hole := make([]bool, len(rings))
	for k, i := range order {
		parent[i] = -1
		// Walk the larger loops smallest first: the first that contains this
		// one is its nearest enclosing loop.
		for m := k - 1; m >= 0; m-- {
			j := order[m]
			inside, known := ringInsideRing(rings[i], rings[j])
			if !known {
				return nil, false
			}
			if inside {
				parent[i] = j
				hole[i] = !hole[j]
				break
			}
		}
	}

	index := make([]int, len(rings))
	for i, r := range rings {
		if !hole[i] {
			index[i] = len(polys)
			polys = append(polys, Polygon2D{Outer: r})
		}
	}
	for i, r := range rings {
		if hole[i] {
			p := &polys[index[parent[i]]]
			p.Holes = append(p.Holes, r)
		}
	}
	return polys, true
}

// ringInsideRing reports whether ring a lies inside ring b, for two rings whose
// edges neither cross nor overlap. It tests a's vertices and edge midpoints,
// skipping any within unionTouch of b's boundary: the midpoints are there
// because a ring may touch another at every vertex, as a diamond inscribed in
// a square does.
//
// Every tested point must agree. Two rings can still pass through each other
// AT a vertex — a diamond whose two corners sit on a square's edge, one half
// in and one half out — which the edge test calls touching; its points then
// disagree, and known is false. known is also false when no point clears b.
func ringInsideRing(a, b [][2]float64) (inside, known bool) {
	n := len(a)
	candidates := make([][2]float64, 0, 2*n)
	candidates = append(candidates, a...)
	for i := range a {
		p, q := a[i], a[(i+1)%n]
		candidates = append(candidates, [2]float64{p[0] + (q[0]-p[0])/2, p[1] + (q[1]-p[1])/2})
	}
	for _, c := range candidates {
		if distanceToRing(c, b) <= unionTouch {
			continue
		}
		in := pointInPolygon(c, b)
		if known && in != inside {
			return false, false
		}
		inside, known = in, true
	}
	return inside, known
}

// distanceToRing is the distance from p to the nearest point of r's boundary.
func distanceToRing(p [2]float64, r [][2]float64) float64 {
	best := math.Inf(1)
	for i := range r {
		a, b := r[i], r[(i+1)%len(r)]
		dx, dy := b[0]-a[0], b[1]-a[1]
		t := 0.0
		if l2 := dx*dx + dy*dy; l2 > 0 {
			t = math.Max(0, math.Min(1, ((p[0]-a[0])*dx+(p[1]-a[1])*dy)/l2))
		}
		best = math.Min(best, math.Hypot(p[0]-(a[0]+t*dx), p[1]-(a[1]+t*dy)))
	}
	return best
}

// UnionArea2D returns the area of the union of polys: the surface they cover
// together, with anything two of them share counted ONCE, and holes subtracted
// unless another polygon fills them.
//
// It answers the question a facade asks. One element's own area is
// [Scene.NetAreas] or [Facing.FaceArea]; this is what several
// elements measure to when they clad one plane and overlap. A cladding band in
// front of the wall behind it is one surface, and adding the two areas
// overstates it by the shared strip.
//
// ok is false when the input cannot be measured honestly, and there is then NO
// figure to report — a caller must not substitute zero, because zero is a
// measurement and this is the absence of one. That happens when:
//
//   - there are no polygons at all;
//   - a ring has fewer than three points, or encloses no area;
//   - any coordinate is NaN or infinite;
//   - two edges of one polygon cross each other, or run along each other for
//     any length — a bow-tie, a ring that doubles back over its own edge, two
//     same-sided squares joined by a bridge walked there and back, a hole
//     that cuts through its outer ring. Such rings are not an outline, and a
//     crossing inside a slab is exactly what the decomposition cannot pair,
//     so some of them used to measure to a confident wrong figure;
//   - the rings do not describe the surface they claim — a hole that is not
//     inside its outer ring, two holes that overlap each other — so that
//     "outer minus holes" and the region the rings actually enclose are two
//     different numbers;
//   - the union boundary did not close, the same refusal [Element.SilhouetteOn]
//     already makes.
//
// Rings may TOUCH, at a point: a ring may pass through one of its own
// vertices twice, a vertex may sit on another edge, and a hole may meet its
// outer ring at a corner. That is deliberate. Such a pinch has one honest area
// and one honest boundary, and [Element.SilhouetteOn] emits it — an outline
// whose opening reaches the corner of a notch comes back as one ring passing
// twice through that corner — so refusing it would refuse the library's own
// output. Touching is judged at the quantum below: an endpoint that close to
// an edge is on it.
//
// Two points closer than 1e-5 m are ONE point here. That quantum is not this
// function's: it is the weld the boundary walk has always used, and a piece
// whose corners do not weld to three distinct points is dropped as a segment
// rather than admitted as a triangle. The area given up is bounded by the
// quantum times the piece's longest edge — below the resolution at which a
// boundary can be stated at all — but note that the gate described above
// cannot see it: that comparison is made BEFORE the weld, so area the weld
// removes is not a mismatch it is able to report. An outline whose detail is
// at that scale is not an outline this measures; simplify it first.
//
// The polygons must already be expressed in ONE plane's frame. Projecting them
// there is the caller's business: [Element.SilhouetteOn] does it for an
// element, and [PlaneFromNormal] or [ElevationPlane] builds the frame. Two
// outlines taken on different frames will union into a figure that means
// nothing, and nothing here can detect it.
//
// Cost is superlinear and SHAPE-dependent, not one exponent. The sweep's bound
// is O(v²) in a single polygon's vertex count — every vertex opens a slab, and
// every edge may cross every slab — with the boundary walk over the resulting
// pieces on top of that. The crossing check before the sweep is O(v²) always:
// every edge of a polygon is compared with every other, with no spatial index. Measured between 32 and 512 vertices, growth ran from
// roughly linear on an outline whose slabs each hold two crossings to well
// above quadratic on one whose slabs hold many, so treat O(v²) as the bound
// and not as a prediction. A facade outline is tens of vertices, and nothing
// here refuses a polygon for being large.
func UnionArea2D(polys []Polygon2D) (area float64, ok bool) {
	area, _, ok = UnionMeasure2D(polys)
	return area, ok
}

// UnionMeasure2D returns the area of the union of polys and the length of its
// boundary. See [UnionArea2D] for what the union means and when ok is false.
//
// perimeter is the whole boundary of the union, not only its outer silhouette:
// a void the union still has is edge, and a facade's returns are measured
// around its openings as well as around its edge. What it excludes is a SEAM —
// where two polygons merge, the line between them is interior and carries no
// boundary, exactly as the shared area is counted once.
//
// Both numbers come from one walk of one boundary, deliberately. Taking them
// from two walks risks an area and a perimeter that describe different shapes.
func UnionMeasure2D(polys []Polygon2D) (area, perimeter float64, ok bool) {
	if len(polys) == 0 {
		return 0, 0, false
	}
	// ONE offset for every polygon, so they stay in a common frame and still
	// union. Area and perimeter are invariant under it.
	ox, oy := unionOffset(polys)
	var tris [][3][2]float64
	for _, p := range polys {
		pieces, ok := unionPolygonTriangles(p, ox, oy)
		if !ok {
			return 0, 0, false
		}
		for _, t := range pieces {
			tris = appendProjected(tris, t[0], t[1], t[2])
		}
	}
	if len(tris) == 0 {
		return 0, 0, false
	}
	return unionMeasure2D(tris)
}

// unionPolygonTriangles decomposes one hole-nested ring set into triangles that
// cover exactly the filled region, so the existing triangle-soup union can
// measure several of them together.
//
// The decomposition is a vertical sweep, not ear clipping. Every vertex x is a
// slab boundary, so within a slab no edge starts, ends or changes neighbour:
// the edges crossing it are a fixed set, their vertical order is fixed, and
// pairing them even-odd from the bottom gives the filled intervals directly.
// Each interval is a trapezoid whose corners are the two edges evaluated at the
// slab's own boundaries — exact, because an edge spanning the slab is linear
// across it. Two triangles per trapezoid.
//
// Ear clipping would have been fewer triangles, and would have needed the holes
// bridged into the outer ring first. A bridge is a zero-width slit: it doubles
// two vertices, and the ear test then has to distinguish a vertex lying on a
// candidate ear from its own twin. That is the part of earcut that has the
// fallback paths, and it fails by emitting a plausible wrong triangulation
// rather than by refusing. The sweep has no such case to get wrong.
//
// ok is false when a ring encloses no area, when two edges cross or overlap
// (see edgesCrossOrOverlap), and when the pieces do not sum to the area the
// rings state. The second is the honesty gate for the whole
// function: a sound ring set decomposes exactly, so a mismatch is not a
// rounding story, it is the rings describing something other than an outer
// with voids inside it.
func unionPolygonTriangles(p Polygon2D, ox, oy float64) ([][3][2]float64, bool) {
	rings := make([][][2]float64, 0, 1+len(p.Holes))
	rings = append(rings, translateRing(p.Outer, ox, oy))
	for _, h := range p.Holes {
		rings = append(rings, translateRing(h, ox, oy))
	}

	want := 0.0
	for i, r := range rings {
		// A non-finite coordinate is refused here, up front, although the
		// area test below would also catch it — a NaN or an infinity poisons
		// the shoelace, and that test is negated so NaN takes the refusing
		// branch. Up front because the alternative is relying on how NaN
		// compares, in two places, forever: the sweep sorts on these values,
		// and a NaN in an ordering has no defined position at all.
		for _, q := range r {
			if math.IsNaN(q[0]) || math.IsNaN(q[1]) || math.IsInf(q[0], 0) || math.IsInf(q[1], 0) {
				return nil, false
			}
		}
		// A ring of fewer than three points, or of collinear ones, has a
		// shoelace area of exactly zero: it is a line, not a boundary. This
		// has to be caught per RING rather than left to the gate below, which
		// compares totals — a void enclosing nothing subtracts nothing, so the
		// totals agree and the ring set measures as though the caller had not
		// mentioned it. A ring that describes no void is not a void the
		// library may quietly discard.
		a := math.Abs(polygonArea2D(r))
		if !(a > 0) {
			return nil, false
		}
		if i == 0 {
			want = a
		} else {
			want -= a
		}
	}

	// Before the sweep, not after it: a crossing inside a slab is the one thing
	// the sweep's pairing assumes away, and the area gate below does not
	// reliably see it. A bow-tie whose lobes wind in opposite senses is refused
	// there, but a ring that crosses itself can also decompose into pieces
	// whose absolute areas sum to what the shoelace says while the union of
	// those pieces is a different number.
	if edgesCrossOrOverlap(rings) {
		return nil, false
	}
	tris := sweepRings(rings)
	var got float64
	for _, t := range tris {
		got += math.Abs(signedArea2(t)) / 2 // signedArea2 is TWICE the area
	}
	// The gate: the rings must state a real positive area, and the pieces must
	// add up to it. An empty decomposition lands here too, as got == 0, which
	// is why sweepRings has no failure result of its own. One condition rather than two, because a separate
	// want > 0 arm turned out to catch nothing this one does not — when want
	// is zero or negative the tolerance is too, so the comparison refuses.
	if !(want > 0) || math.Abs(got-want) > unionRingClosure*want {
		return nil, false
	}
	return tris, true
}

// unionOffset is the centre of the bounding box of every ring of every polygon
// — the one translation that puts the whole input as close to the origin as a
// single shift can.
//
// It is a bounding-box centre rather than, say, the first vertex, because what
// has to shrink is the LARGEST coordinate magnitude the sweep interpolates on,
// and the box centre minimises exactly that.
//
// A non-finite coordinate is ignored here rather than refused: the ring
// carrying it is refused a moment later, by name, and poisoning the offset
// would take every other polygon down with it. If nothing finite is found the
// offset is zero, which changes nothing about the refusal that follows.
func unionOffset(polys []Polygon2D) (ox, oy float64) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range polys {
		for _, r := range append([][][2]float64{p.Outer}, p.Holes...) {
			for _, q := range r {
				if math.IsNaN(q[0]) || math.IsNaN(q[1]) || math.IsInf(q[0], 0) || math.IsInf(q[1], 0) {
					continue
				}
				minX, maxX = math.Min(minX, q[0]), math.Max(maxX, q[0])
				minY, maxY = math.Min(minY, q[1]), math.Max(maxY, q[1])
			}
		}
	}
	if math.IsInf(minX, 0) || math.IsInf(minY, 0) {
		return 0, 0
	}
	return minX + (maxX-minX)/2, minY + (maxY-minY)/2
}

// translateRing copies a ring shifted by (-ox, -oy). A copy, not a shift in
// place: the outline belongs to the caller, who stored it and will read it
// again.
func translateRing(r [][2]float64, ox, oy float64) [][2]float64 {
	out := make([][2]float64, len(r))
	for i, q := range r {
		out[i] = [2]float64{q[0] - ox, q[1] - oy}
	}
	return out
}

// sweepEdge is one non-vertical ring edge, oriented left to right. A vertical
// edge is dropped: it lies on a slab boundary and crosses no slab's interior,
// so it bounds nothing the sweep has to pair.
type sweepEdge struct {
	x0, y0, x1, y1 float64
}

// yAt evaluates the edge at x. It is exact at x0 and x1, which is what makes a
// trapezoid's corners land exactly on the neighbouring slab's corners and the
// seam between two slabs cancel instead of leaving a hairline boundary.
func (e sweepEdge) yAt(x float64) float64 {
	return e.y0 + (e.y1-e.y0)*(x-e.x0)/(e.x1-e.x0)
}

// sweepRings is the decomposition itself. See unionPolygonTriangles for why it
// is a sweep. It returns no verdict: rings it cannot decompose yield pieces
// that do not sum to the area they state, and the caller's gate refuses that.
func sweepRings(rings [][][2]float64) [][3][2]float64 {
	var edges []sweepEdge
	seen := make(map[float64]struct{})
	var xs []float64
	for _, r := range rings {
		for i := range r {
			a, b := r[i], r[(i+1)%len(r)]
			if _, dup := seen[a[0]]; !dup {
				seen[a[0]] = struct{}{}
				xs = append(xs, a[0])
			}
			if a[0] == b[0] {
				continue
			}
			if a[0] > b[0] {
				a, b = b, a
			}
			edges = append(edges, sweepEdge{a[0], a[1], b[0], b[1]})
		}
	}
	sort.Float64s(xs)

	// One crossing of a slab: the edge's y at the slab's two boundaries, keyed
	// for ordering by its y at the middle, where no two distinct edges of a
	// sound ring set can meet. A slab one float64 wide has no middle, and xm
	// then rounds onto one of its boundaries, where two edges sharing a vertex
	// do meet. That needs no special case: the tie-break below orders such a
	// pair by where the two edges are at the other boundary, which is their
	// true order across the slab.
	type crossing struct{ mid, lo, hi float64 }
	var cs []crossing
	var out [][3][2]float64
	for s := 0; s+1 < len(xs); s++ {
		x0, x1 := xs[s], xs[s+1]
		xm := x0 + (x1-x0)/2
		cs = cs[:0]
		for _, e := range edges {
			if e.x0 <= x0 && e.x1 >= x1 {
				cs = append(cs, crossing{e.yAt(xm), e.yAt(x0), e.yAt(x1)})
			}
		}
		if len(cs) == 0 {
			continue
		}
		// No check for an odd count: a CLOSED ring crosses a vertical line an
		// even number of times, always, and every ring here is closed by
		// construction. Were one somehow not, the unpaired crossing would drop
		// out of the loop below and the pieces would no longer sum to the area
		// the rings state, which unionPolygonTriangles' gate refuses.
		// A TOTAL order, not just a comparison on mid. Two edges can cross a
		// slab at the same mid height — a ring that doubled back along its own
		// edge did, and sort.Slice is not stable, so a tie left unbroken was
		// resolved by whatever order the edges happened to be collected in.
		// That made the SAME ring measure 0.03125 listed one way and refuse
		// listed the other, which the fuzz target's winding invariant caught.
		// edgesCrossOrOverlap now refuses that ring before it gets here, but it
		// judges at the weld quantum, so two edges closer than that still
		// arrive and can still tie. Ranking ties by the slab's two boundaries
		// keeps the order a function of the geometry alone.
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].mid != cs[j].mid {
				return cs[i].mid < cs[j].mid
			}
			if cs[i].lo != cs[j].lo {
				return cs[i].lo < cs[j].lo
			}
			return cs[i].hi < cs[j].hi
		})
		for i := 0; i+1 < len(cs); i += 2 {
			lo, hi := cs[i], cs[i+1]
			a := [2]float64{x0, lo.lo}
			b := [2]float64{x1, lo.hi}
			c := [2]float64{x1, hi.hi}
			d := [2]float64{x0, hi.lo}
			out = append(out, [3][2]float64{a, b, c}, [3][2]float64{a, c, d})
		}
	}
	return out
}

// unionTouch is how close two things must be to count as touching, in metres:
// the boundary walk's weld quantum, so that this check and the weld agree on
// what one point is.
const unionTouch = 1 / sectionWeldQuantum

// edgesCrossOrOverlap reports whether any two edges of rings, in the same ring
// or in different ones, cross each other or run along each other for more
// than a point. Meeting at a point is allowed: see the pinch paragraph on
// [UnionArea2D].
//
// Every pair is compared, so this is O(e²) in the polygon's edge count. An
// edge no longer than unionTouch is skipped: the weld makes it a point, and a
// point can only touch.
//
// The comparison uses a tolerance rather than exact signs, which is why
// section.go's ringSelfIntersects is not reused. The rings arrive recentred,
// so a vertex lying exactly on another edge in the caller's coordinates can
// sit a rounding error to either side of it here. Exact signs would call half
// of those a crossing, and whether an outline is refused would then depend on
// where the model stands.
func edgesCrossOrOverlap(rings [][][2]float64) bool {
	type edge struct {
		a, b [2]float64
		len  float64
	}
	var edges []edge
	for _, r := range rings {
		for i := range r {
			a, b := r[i], r[(i+1)%len(r)]
			if l := math.Hypot(b[0]-a[0], b[1]-a[1]); l > unionTouch {
				edges = append(edges, edge{a, b, l})
			}
		}
	}
	for i, s := range edges {
		for _, t := range edges[i+1:] {
			sa, sb := touchSide(s.a, s.b, s.len, t.a), touchSide(s.a, s.b, s.len, t.b)
			ta, tb := touchSide(t.a, t.b, t.len, s.a), touchSide(t.a, t.b, t.len, s.b)
			switch {
			case sa*sb < 0 && ta*tb < 0:
				return true // each edge has the other's ends on opposite sides
			case sa == 0 && sb == 0:
				if collinearOverlap(s.a, s.b, s.len, t.a, t.b) {
					return true
				}
			case ta == 0 && tb == 0:
				// From t's side as well: a short edge lying along a long one
				// can have the long one's far ends well off its own line.
				if collinearOverlap(t.a, t.b, t.len, s.a, s.b) {
					return true
				}
			}
		}
	}
	return false
}

// touchSide is the side of the line through a and b (l apart) that p lies on:
// +1 left, -1 right, 0 when p is within unionTouch of the line.
func touchSide(a, b [2]float64, l float64, p [2]float64) int {
	d := ((b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0])) / l
	switch {
	case d > unionTouch:
		return 1
	case d < -unionTouch:
		return -1
	}
	return 0
}

// collinearOverlap reports whether c–d, already known to lie along a–b (l
// apart), shares more than unionTouch of its length with it.
func collinearOverlap(a, b [2]float64, l float64, c, d [2]float64) bool {
	ux, uy := (b[0]-a[0])/l, (b[1]-a[1])/l
	pc := (c[0]-a[0])*ux + (c[1]-a[1])*uy
	pd := (d[0]-a[0])*ux + (d[1]-a[1])*uy
	lo, hi := math.Max(0, math.Min(pc, pd)), math.Min(l, math.Max(pc, pd))
	return hi-lo > unionTouch
}
