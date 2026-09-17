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
const unionRingClosure = 1e-9

// Polygon2D is a hole-nested ring set in one plane's (u, v) frame, in metres:
// an outer ring and the voids inside it. A ring is implicitly closed — do not
// repeat its first point as its last.
//
// It is the shape [Element.SilhouetteOn] describes, one step further on. That
// method returns a FLAT []Loop, hole-nested by winding (outer counter-
// clockwise, holes clockwise); splitting those rings into an outer and its
// holes is the consumer's one conversion, and the largest ring by absolute
// area is the outer one.
//
// Winding is not load-bearing here. The rings are filled even-odd, exactly as
// [Loop] says a renderer may treat them, so an outline that has been
// re-serialized by something with its own opinion about orientation still
// measures correctly.
type Polygon2D struct {
	Outer [][2]float64
	Holes [][][2]float64
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
//   - the rings do not describe the surface they claim — a hole that is not
//     inside its outer ring, two holes that overlap each other, a boundary
//     that crosses itself — so that "outer minus holes" and the region the
//     rings actually enclose are two different numbers;
//   - the union boundary did not close, the same refusal [Element.SilhouetteOn]
//     already makes.
//
// The polygons must already be expressed in ONE plane's frame. Projecting them
// there is the caller's business: [Element.SilhouetteOn] does it for an
// element, and [PlaneFromNormal] or [ElevationPlane] builds the frame. Two
// outlines taken on different frames will union into a figure that means
// nothing, and nothing here can detect it.
//
// Cost is O(v²) in the vertex count of the largest polygon, plus the cost of
// the boundary walk over the pieces. A facade outline is tens of vertices.
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
	var tris [][3][2]float64
	for _, p := range polys {
		pieces, ok := unionPolygonTriangles(p)
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
// ok is false when a ring encloses no area, and when the pieces do not sum to
// the area the rings state. The second is the honesty gate for the whole
// function: a sound ring set decomposes exactly, so a mismatch is not a
// rounding story, it is the rings describing something other than an outer
// with voids inside it.
func unionPolygonTriangles(p Polygon2D) ([][3][2]float64, bool) {
	rings := make([][][2]float64, 0, 1+len(p.Holes))
	rings = append(rings, p.Outer)
	rings = append(rings, p.Holes...)

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

	tris, ok := sweepRings(rings)
	if !ok {
		return nil, false
	}
	var got float64
	for _, t := range tris {
		got += math.Abs(signedArea2(t)) / 2 // signedArea2 is TWICE the area
	}
	// The gate: the rings must state a real positive area, and the pieces must
	// add up to it. One condition rather than two, because a separate
	// want > 0 arm turned out to catch nothing this one does not — when want
	// is zero or negative the tolerance is too, so the comparison refuses.
	if !(want > 0) || math.Abs(got-want) > unionRingClosure*want {
		return nil, false
	}
	return tris, true
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
// is a sweep and what ok means.
func sweepRings(rings [][][2]float64) ([][3][2]float64, bool) {
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
	if len(xs) < 2 || len(edges) == 0 {
		return nil, false
	}
	sort.Float64s(xs)

	// One crossing of a slab: the edge's y at the slab's two boundaries, keyed
	// for ordering by its y at the middle, where no two distinct edges of a
	// sound ring set can meet.
	type crossing struct{ mid, lo, hi float64 }
	var cs []crossing
	var out [][3][2]float64
	for s := 0; s+1 < len(xs); s++ {
		x0, x1 := xs[s], xs[s+1]
		xm := x0 + (x1-x0)/2
		if !(xm > x0 && xm < x1) {
			continue // the slab is thinner than the gap between two float64s
		}
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
		sort.Slice(cs, func(i, j int) bool { return cs[i].mid < cs[j].mid })
		for i := 0; i+1 < len(cs); i += 2 {
			lo, hi := cs[i], cs[i+1]
			a := [2]float64{x0, lo.lo}
			b := [2]float64{x1, lo.hi}
			c := [2]float64{x1, hi.hi}
			d := [2]float64{x0, hi.lo}
			out = append(out, [3][2]float64{a, b, c}, [3][2]float64{a, c, d})
		}
	}
	return out, len(out) > 0
}
