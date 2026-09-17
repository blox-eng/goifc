package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
)

// TestUnionArea2DCountsOverlapOnce: two 2 x 2 squares sharing a 1 x 2 strip
// cover 6 m², not the 8 m² their own areas sum to. This is the whole reason
// the function exists — a facade whose cladding band sits in front of its wall
// is one surface, and adding the two is an overstatement.
func TestUnionArea2DCountsOverlapOnce(t *testing.T) {
	a := Polygon2D{Outer: [][2]float64{{0, 0}, {2, 0}, {2, 2}, {0, 2}}}
	b := Polygon2D{Outer: [][2]float64{{1, 0}, {3, 0}, {3, 2}, {1, 2}}}
	area, ok := UnionArea2D([]Polygon2D{a, b})
	if !ok {
		t.Fatal("UnionArea2D(ok) = false, want true")
	}
	if math.Abs(area-6) > 1e-9 {
		t.Errorf("UnionArea2D = %v, want 6 (8 gross, 2 shared)", area)
	}
}

// TestUnionArea2DSubtractsAHole: a hole is a void, not a ring to add.
func TestUnionArea2DSubtractsAHole(t *testing.T) {
	p := Polygon2D{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
	}
	area, ok := UnionArea2D([]Polygon2D{p})
	if !ok || math.Abs(area-15) > 1e-9 {
		t.Errorf("UnionArea2D = %v, %v; want 15, true", area, ok)
	}
}

// TestUnionArea2DCountsAFilledHoleOnce: the case a facade actually hits. A
// cladding band sits exactly where the wall behind it has its opening. The
// surface is continuous, and 16 is the honest answer — not 15 + 1 counted
// twice, and not 15 with the patch swallowed.
func TestUnionArea2DCountsAFilledHoleOnce(t *testing.T) {
	holed := Polygon2D{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
	}
	patch := Polygon2D{Outer: [][2]float64{{1, 1}, {2, 1}, {2, 2}, {1, 2}}}
	area, ok := UnionArea2D([]Polygon2D{holed, patch})
	if !ok || math.Abs(area-16) > 1e-9 {
		t.Errorf("UnionArea2D = %v, %v; want 16, true", area, ok)
	}
}

// TestUnionArea2DRefusesWhatItCannotMeasure pins the refusal contract. Each
// input below names a surface nothing can measure honestly, and a figure
// nobody can trust is worse than no figure — the same stance unionBoundary
// takes on an outline that did not close.
func TestUnionArea2DRefusesWhatItCannotMeasure(t *testing.T) {
	for name, polys := range map[string][]Polygon2D{
		"no polygons at all":           {},
		"a ring of two points":         {{Outer: [][2]float64{{0, 0}, {1, 1}}}},
		"a zero-area ring":             {{Outer: [][2]float64{{0, 0}, {1, 1}, {2, 2}}}},
		"a ring of one repeated point": {{Outer: [][2]float64{{1, 1}, {1, 1}, {1, 1}}}},
		"a hole enclosing no area": {{
			Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
			Holes: [][][2]float64{{{1, 1}, {2, 2}, {3, 3}}},
		}},
		// UNEQUAL areas on purpose. With a 1x1 hole outside a 1x1 outer the
		// two cancel to want == 0 and the gate's positive-area arm refuses it,
		// so the fixture would pass while the closure term — the part that
		// actually notices a hole is not where it claims to be — was gone. At
		// 4x4 and 1x1 the closure term is the only thing standing between this
		// input and a confident 17 m².
		"a hole outside its outer": {{
			Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
			Holes: [][][2]float64{{{5, 5}, {5, 6}, {6, 6}, {6, 5}}},
		}},
		"a hole larger than its outer": {{
			Outer: [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
			Holes: [][][2]float64{{{-1, -1}, {-1, 2}, {2, 2}, {2, -1}}},
		}},
		"two holes overlapping each other": {{
			Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
			Holes: [][][2]float64{
				{{1, 1}, {1, 3}, {3, 3}, {3, 1}},
				{{2, 2}, {2, 4}, {4, 4}, {4, 2}},
			},
		}},
		"a non-finite coordinate": {{
			Outer: [][2]float64{{0, 0}, {math.NaN(), 0}, {1, 1}, {0, 1}},
		}},
		"an infinite coordinate": {{
			Outer: [][2]float64{{0, 0}, {math.Inf(1), 0}, {1, 1}, {0, 1}},
		}},
		"one good polygon and one that is not": {
			{Outer: [][2]float64{{0, 0}, {2, 0}, {2, 2}, {0, 2}}},
			{Outer: [][2]float64{{0, 0}, {1, 1}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if area, ok := UnionArea2D(polys); ok {
				t.Errorf("UnionArea2D = %v, true; want ok = false — a figure nobody can trust is worse than none", area)
			}
		})
	}
}

// TestUnionArea2DRefusesAHoleThatPokesOutOfItsOuter is the fixture that pins
// how TIGHT unionRingClosure has to be. Nothing else in the package does: the
// constant could be loosened by six orders of magnitude and every other test
// would still pass.
//
// A 1 x 1 hole overhangs its outer ring by a hair. That is a drafting slip,
// not an adversarial input, and it is what the gate exists to catch: "outer
// minus holes" says 15 m² while the rings actually enclose slightly more,
// because even-odd fills the sliver that escaped. At a 1 mm overhang the
// discrepancy is 1.3e-4 relative, and a constant of 1e-3 hands back
// 15.002 m² of facade with ok = true.
//
// The three overhangs descend to 1e-5 m, which is the weld quantum and the
// floor on how tight this can honestly be pinned — below it the library does
// not claim to resolve geometry at all. Together they refuse any loosening to
// 1e-5 or beyond. (The gate still SEES a sub-quantum sliver, because it
// compares the decomposition against the rings before the weld; that is the
// same pre-weld ordering documented on UnionArea2D, from the other side.)
func TestUnionArea2DRefusesAHoleThatPokesOutOfItsOuter(t *testing.T) {
	for _, overhang := range []float64{1e-3, 1e-4, 1e-5} {
		p := Polygon2D{
			Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
			Holes: [][][2]float64{{
				{3 + overhang, 1}, {4 + overhang, 1}, {4 + overhang, 2}, {3 + overhang, 2},
			}},
		}
		if area, ok := UnionArea2D([]Polygon2D{p}); ok {
			t.Errorf("overhang %g m: UnionArea2D = %v, true; want ok = false — the rings enclose %v m², not the 15 they claim",
				overhang, area, 15+2*overhang)
		}
	}
}

// TestUnionMeasure2DReturnsTheUnionBoundary: the two 2 x 2 squares merge into
// one 3 x 2 rectangle, so the seam between them carries no boundary — 10 m of
// perimeter, not the 16 m the two squares have on their own.
func TestUnionMeasure2DReturnsTheUnionBoundary(t *testing.T) {
	a := Polygon2D{Outer: [][2]float64{{0, 0}, {2, 0}, {2, 2}, {0, 2}}}
	b := Polygon2D{Outer: [][2]float64{{1, 0}, {3, 0}, {3, 2}, {1, 2}}}
	area, per, ok := UnionMeasure2D([]Polygon2D{a, b})
	if !ok || math.Abs(area-6) > 1e-9 || math.Abs(per-10) > 1e-9 {
		t.Errorf("UnionMeasure2D = %v, %v, %v; want 6, 10, true", area, per, ok)
	}
}

// TestUnionMeasure2DCountsAHoleBoundary: a void's edge is boundary too. A
// caller computing a facade's returns from this number needs it to include
// the reveal around an opening, not just the outer silhouette.
func TestUnionMeasure2DCountsAHoleBoundary(t *testing.T) {
	p := Polygon2D{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
	}
	area, per, ok := UnionMeasure2D([]Polygon2D{p})
	if !ok || math.Abs(area-15) > 1e-9 || math.Abs(per-20) > 1e-9 {
		t.Errorf("UnionMeasure2D = %v, %v, %v; want 15, 20, true", area, per, ok)
	}
}

// TestUnionMeasure2DOrdersCrossingsWithinTheSlab: a 6 x 6 face with a
// triangular opening whose APEX lands on a slab boundary. Two of that hole's
// edges start at the same height and separate inside the slab, so the only key
// that orders them correctly is their height at the slab's MIDDLE — at either
// boundary they are equal, and the pairing that follows is then a coin toss.
//
// All four listings of the same hole are asserted because the bug is
// winding-sensitive: the wrong key still happens to sort correctly for three of
// them, so a single fixture would have passed while the code was broken.
func TestUnionMeasure2DOrdersCrossingsWithinTheSlab(t *testing.T) {
	wantPer := 24 + 4 + 4*math.Sqrt2 // the face, plus the opening's reveal
	for name, hole := range map[string][][2]float64{
		"apex last":   {{1, 2}, {5, 2}, {3, 4}},
		"apex first":  {{3, 4}, {5, 2}, {1, 2}},
		"apex middle": {{1, 2}, {3, 4}, {5, 2}},
		"reversed":    {{5, 2}, {1, 2}, {3, 4}},
	} {
		t.Run(name, func(t *testing.T) {
			p := Polygon2D{
				Outer: [][2]float64{{0, 0}, {6, 0}, {6, 6}, {0, 6}},
				Holes: [][][2]float64{hole},
			}
			area, per, ok := UnionMeasure2D([]Polygon2D{p})
			if !ok || math.Abs(area-32) > 1e-9 || math.Abs(per-wantPer) > 1e-9 {
				t.Errorf("UnionMeasure2D = %v, %v, %v; want 32, %v, true", area, per, ok, wantPer)
			}
		})
	}
}

// TestUnionMeasure2DIsIndependentOfRingOrder: the same ring, listed forwards
// and backwards, must reach the same verdict.
//
// The fixture is the one FuzzUnionArea2D found: a ring that returns to a vertex
// it has already used and runs a zero-width whisker out and back. That presents
// the sweep with two crossings at exactly the same height, and sort.Slice is
// not stable — with the tie unbroken, the answer was decided by the order the
// edges happened to be collected in, so the ring measured 0.03125 one way and
// was refused the other. Both refuse now, which is what the contract promises
// for a boundary that meets itself.
//
// The crashing input is committed under testdata/fuzz, so `go test` replays it
// whether or not anyone is fuzzing.
func TestUnionMeasure2DIsIndependentOfRingOrder(t *testing.T) {
	ring := [][2]float64{{12, 12.25}, {12.25, 12}, {12, 12}, {24.75, 24.75}, {12, 12}}
	reversed := make([][2]float64, len(ring))
	for i, q := range ring {
		reversed[len(ring)-1-i] = q
	}
	fa, fp, fok := UnionMeasure2D([]Polygon2D{{Outer: ring}})
	ra, rp, rok := UnionMeasure2D([]Polygon2D{{Outer: reversed}})
	if fok != rok || fa != ra || fp != rp {
		t.Errorf("forwards = %v, %v, %v; backwards = %v, %v, %v — the same ring", fa, fp, fok, ra, rp, rok)
	}
	if fok {
		t.Errorf("UnionMeasure2D(ok) = true on a ring that meets itself; want false")
	}
}

// TestUnionArea2DIgnoresRingWinding: the outline a consumer stored may have
// been re-serialized by anything. Winding decides nothing here — the ring set
// is filled even-odd, exactly as Loop's own doc says a renderer may treat it.
func TestUnionArea2DIgnoresRingWinding(t *testing.T) {
	ccw := Polygon2D{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
	}
	cw := Polygon2D{
		Outer: [][2]float64{{0, 4}, {4, 4}, {4, 0}, {0, 0}},
		Holes: [][][2]float64{{{1, 1}, {2, 1}, {2, 2}, {1, 2}}},
	}
	a, aok := UnionArea2D([]Polygon2D{ccw})
	b, bok := UnionArea2D([]Polygon2D{cw})
	if !aok || !bok || math.Abs(a-b) > 1e-9 {
		t.Errorf("UnionArea2D = %v (%v) wound one way and %v (%v) the other; want the same figure", a, aok, b, bok)
	}
}

// TestUnionArea2DMeasuresAConcaveRing: an L is the shape a set-back storey
// leaves, and a fan triangulation would have measured the missing corner too.
func TestUnionArea2DMeasuresAConcaveRing(t *testing.T) {
	l := Polygon2D{Outer: [][2]float64{{0, 0}, {4, 0}, {4, 1}, {1, 1}, {1, 4}, {0, 4}}}
	area, ok := UnionArea2D([]Polygon2D{l})
	if !ok || math.Abs(area-7) > 1e-9 {
		t.Errorf("UnionArea2D = %v, %v; want 7, true", area, ok)
	}
}

// TestUnionMeasure2DMeasuresARakedEdge: a gable. Every other fixture here is
// axis-aligned, and on those a sloping edge and a horizontal one at the same
// average height enclose the same area — so an area assertion alone cannot
// tell a trapezoid from the rectangle of its mean height. The PERIMETER can,
// and a facade with a raked top or a canted return has exactly this shape.
func TestUnionMeasure2DMeasuresARakedEdge(t *testing.T) {
	gable := Polygon2D{Outer: [][2]float64{{0, 0}, {4, 0}, {4, 2}, {2, 4}, {0, 2}}}
	wantPer := 8 + 4*math.Sqrt2
	area, per, ok := UnionMeasure2D([]Polygon2D{gable})
	if !ok || math.Abs(area-12) > 1e-9 || math.Abs(per-wantPer) > 1e-9 {
		t.Errorf("UnionMeasure2D = %v, %v, %v; want 12, %v, true", area, per, ok, wantPer)
	}
}

// TestUnionMeasure2DIsInvariantFarFromTheOrigin: the same outline measures the
// same wherever the building stands.
//
// This is not a formality. SilhouetteOn projects WORLD coordinates and
// ElevationPlane's frame has no origin to subtract them against, so a model
// on a projected national grid (eastings ~1e5–1e6 m, northings ~1e6–1e7 m)
// arrives here at x≈4.6e5, y≈4.7e6. The sweep interpolates ABSOLUTELY at
// that magnitude while the closure gate is RELATIVE to the area, and the two
// meet in two different ways: a 1-5 m outline is REFUSED (a soak through
// this door went 0/500 at 47 m, 6/500 at 10 km, 497/500 at 300 km), and the
// shapes below are worse — they are ACCEPTED, at a wrong figure.
//
// TWO shapes, because they catch it at COMPLEMENTARY origins and neither alone
// is enough: the rake bites on the exactly-representable grid and the gable
// off it. Both are raked on purpose — an outline whose every edge is axis
// aligned interpolates nothing, survives the bug untouched, and reads as
// reassurance. The rake's edge additionally spans THREE slabs, because
// interpolation only happens at a slab boundary an edge crosses.
//
// The near origins are controls rather than coverage: they bite at no
// magnitude, and exist so that a regression breaking measurement everywhere is
// told apart from one that only breaks it far out.
func TestUnionMeasure2DIsInvariantFarFromTheOrigin(t *testing.T) {
	const t3 = 1.0 / 3.0
	for _, shape := range []struct {
		name     string
		at       func(ox, oy float64) Polygon2D
		wantArea float64
		wantPer  float64
	}{{
		// A 10 x 10 right triangle, its base split so the hypotenuse crosses
		// two interior slab boundaries at a non-dyadic x.
		name: "a rake over three slabs",
		at: func(ox, oy float64) Polygon2D {
			return Polygon2D{Outer: [][2]float64{
				{ox, oy}, {ox + 3 + t3, oy}, {ox + 7 + t3, oy}, {ox + 10, oy}, {ox, oy + 10},
			}}
		},
		wantArea: 50, wantPer: 20 + 10*math.Sqrt2,
	}, {
		name: "a gable over two",
		at: func(ox, oy float64) Polygon2D {
			return Polygon2D{Outer: [][2]float64{
				{ox, oy}, {ox + 4, oy}, {ox + 4, oy + 2}, {ox + 2, oy + 4}, {ox, oy + 2},
			}}
		},
		wantArea: 12, wantPer: 8 + 4*math.Sqrt2,
	}} {
		for _, origin := range [][2]float64{
			{0, 0},
			{47, 47},
			{10000, 10000},
			{460000, 4700000},     // a projected national grid (eastings ~1e5-1e6 m, northings ~1e6-1e7 m)
			{460000.1, 4700000.7}, // and the same, off the representable grid
			{-460000, -4700000},
		} {
			area, per, ok := UnionMeasure2D([]Polygon2D{shape.at(origin[0], origin[1])})
			if !ok || math.Abs(area-shape.wantArea) > 1e-9 || math.Abs(per-shape.wantPer) > 1e-9 {
				t.Errorf("%s at %v: UnionMeasure2D = %v, %v, %v; want %v, %v, true",
					shape.name, origin, area, per, ok, shape.wantArea, shape.wantPer)
			}
		}
	}
}

// TestUnionArea2DDoesNotMutateItsInput: the caller keeps the outline it stored.
func TestUnionArea2DDoesNotMutateItsInput(t *testing.T) {
	p := Polygon2D{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
	}
	want := Polygon2D{
		Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
		Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
	}
	if _, ok := UnionArea2D([]Polygon2D{p}); !ok {
		t.Fatal("UnionArea2D(ok) = false, want true")
	}
	for i := range p.Outer {
		if p.Outer[i] != want.Outer[i] {
			t.Fatalf("Outer[%d] = %v after the call, was %v", i, p.Outer[i], want.Outer[i])
		}
	}
	for h := range p.Holes {
		for i := range p.Holes[h] {
			if p.Holes[h][i] != want.Holes[h][i] {
				t.Fatalf("Holes[%d][%d] = %v after the call, was %v", h, i, p.Holes[h][i], want.Holes[h][i])
			}
		}
	}
}

// TestUnionArea2DMeasuresASilhouette walks the seam a consumer actually
// crosses: an outline from [Element.SilhouetteOn], whose rings are a FLAT list
// hole-nested by winding, split into Outer and Holes and handed back for
// measurement. That split is the one conversion between the two APIs and it is
// the consumer's to make, so it is exercised here rather than assumed.
//
// BOTH shapes are measured, and the frame is the point. A solid box yields a
// single loop, so the largest-|area| selection runs over one candidate and the
// hole branch appends nothing — the half of the conversion the guide's
// headline example exists for, a wall with an opening, would never run. Each
// case therefore asserts the loop count it depends on, so a fixture that stops
// producing a hole fails here instead of quietly measuring the other branch.
func TestUnionArea2DMeasuresASilhouette(t *testing.T) {
	p, ok := ElevationPlane([3]float64{-1, 0, 0}) // looking along -X
	if !ok {
		t.Fatal("ElevationPlane(ok) = false")
	}
	for _, tc := range []struct {
		name      string
		elem      Element
		wantLoops int
		wantHoles int
		wantArea  float64
	}{
		// A 3 x 5 x 2 m box seen along -X is a solid 5 x 2 m rectangle.
		{"a solid box", boxElement("box", v3{0, 0, 0}, v3{3, 5, 2}), 1, 0, 10},
		// A 6 x 6 m frame with a 2 x 2 m opening: 36 gross, 32 net.
		{"a frame with an opening", frameElement(), 2, 1, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loops := elem(t, tc.elem, p)
			if len(loops) != tc.wantLoops {
				t.Fatalf("SilhouetteOn returned %d loops, want %d — the fixture no longer poses the case this test is for", len(loops), tc.wantLoops)
			}
			poly := polygonFromLoops(loops)
			if len(poly.Holes) != tc.wantHoles {
				t.Fatalf("the conversion produced %d holes, want %d", len(poly.Holes), tc.wantHoles)
			}
			area, ok := UnionArea2D([]Polygon2D{poly})
			if !ok || math.Abs(area-tc.wantArea) > 1e-9 {
				t.Errorf("UnionArea2D = %v, %v; want %v, true", area, ok, tc.wantArea)
			}
		})
	}
}

// elem returns an element's silhouette, failing rather than returning nothing.
func elem(t *testing.T, e Element, p Plane) []Loop {
	t.Helper()
	loops := e.SilhouetteOn(p)
	if len(loops) == 0 {
		t.Fatal("SilhouetteOn returned no loops")
	}
	return loops
}

// polygonFromLoops is the conversion the guide documents: the ring with the
// largest absolute area is the outer one, every other ring a hole.
func polygonFromLoops(loops []Loop) Polygon2D {
	outer := 0
	for i := range loops {
		if math.Abs(polygonArea2D(loops[i].Points)) > math.Abs(polygonArea2D(loops[outer].Points)) {
			outer = i
		}
	}
	poly := Polygon2D{Outer: loops[outer].Points}
	for i := range loops {
		if i != outer {
			poly.Holes = append(poly.Holes, loops[i].Points)
		}
	}
	return poly
}

// frameElement is a 6 x 6 m wall with a 2 x 2 m opening, built as four boxes so
// the mesh genuinely has a hole in it rather than a ring drawn to look like
// one. Seen along -X its silhouette is two loops: the outer wound CCW and the
// opening wound CW, which is the hole-nesting convention Loop documents.
func frameElement() Element {
	boxes := [][2]v3{
		{{0, 0, 0}, {1, 6, 2}}, // below the opening
		{{0, 0, 4}, {1, 6, 6}}, // above it
		{{0, 0, 2}, {1, 2, 4}}, // left of it
		{{0, 4, 2}, {1, 6, 4}}, // right of it
	}
	var verts []float32
	var tris []uint32
	for _, b := range boxes {
		w, bt := boxMeshWorld(b[0], b[1])
		base := uint32(len(verts) / 3)
		for _, q := range w {
			verts = append(verts, float32(q[0]), float32(q[1]), float32(q[2]))
		}
		for _, i := range bt {
			tris = append(tris, base+i)
		}
	}
	return Element{
		GlobalID: "frame",
		Verts:    verts,
		Tris:     tris,
		Placement: model.Mat4{
			1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
		BBoxMin: [3]float64{0, 0, 0},
		BBoxMax: [3]float64{1, 6, 6},
		Source:  SourceBrep,
	}
}
