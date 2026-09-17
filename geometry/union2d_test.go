package geometry

import (
	"math"
	"testing"
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
		"a hole outside its outer": {{
			Outer: [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
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

// TestUnionArea2DIsInvariantFarFromTheOrigin: the union area integral is taken
// about the world origin, which is why boundaryCloses is load-bearing. A
// facade 47 m out must measure the same as one at the origin.
func TestUnionArea2DIsInvariantFarFromTheOrigin(t *testing.T) {
	const d = 47
	near := Polygon2D{Outer: [][2]float64{{0, 0}, {2, 0}, {2, 3}, {0, 3}}}
	far := Polygon2D{Outer: [][2]float64{{d, d}, {d + 2, d}, {d + 2, d + 3}, {d, d + 3}}}
	a, aok := UnionArea2D([]Polygon2D{near})
	b, bok := UnionArea2D([]Polygon2D{far})
	if !aok || !bok || math.Abs(a-6) > 1e-9 || math.Abs(b-6) > 1e-6 {
		t.Errorf("UnionArea2D = %v (%v) at the origin and %v (%v) %d m out; want 6 both", a, aok, b, bok, d)
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
func TestUnionArea2DMeasuresASilhouette(t *testing.T) {
	e := boxElement("box", v3{0, 0, 0}, v3{3, 5, 2}) // seen along -X: 5 x 2 m
	p, ok := ElevationPlane([3]float64{-1, 0, 0})
	if !ok {
		t.Fatal("ElevationPlane(ok) = false")
	}
	loops := e.SilhouetteOn(p)
	if len(loops) == 0 {
		t.Fatal("SilhouetteOn returned no loops")
	}
	best, bestArea := -1, 0.0
	for i, l := range loops {
		if a := math.Abs(polygonArea2D(l.Points)); a > bestArea {
			best, bestArea = i, a
		}
	}
	poly := Polygon2D{Outer: loops[best].Points}
	for i, l := range loops {
		if i != best {
			poly.Holes = append(poly.Holes, l.Points)
		}
	}
	area, ok := UnionArea2D([]Polygon2D{poly})
	if !ok || math.Abs(area-10) > 1e-9 {
		t.Errorf("UnionArea2D of the silhouette = %v, %v; want 10, true", area, ok)
	}
}
