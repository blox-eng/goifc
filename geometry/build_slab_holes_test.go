package geometry

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"
)

var (
	holeOuter = [][2]float64{{0, 0}, {10, 0}, {10, 8}, {0, 8}}
	holeH     = [][2]float64{{2, 2}, {4.5, 2}, {4.5, 3.2}, {2, 3.2}}
)

func shifted(ring [][2]float64, dx, dy float64) [][2]float64 {
	out := make([][2]float64, len(ring))
	for i, p := range ring {
		out[i] = [2]float64{p[0] + dx, p[1] + dy}
	}
	return out
}

func holeSquare(x, y, side float64) [][2]float64 {
	return [][2]float64{{x, y}, {x + side, y}, {x + side, y + side}, {x, y + side}}
}

// checkSlabQuantities checks a slab's quantities against the numbers and its
// mesh against them: a closed shell whose volume agrees within float32 rounding.
func checkSlabQuantities(t *testing.T, name string, s Slab, area, holePerim, perim, volume float64) {
	t.Helper()
	q := s.Quantities
	if !near(q.Area, area, 1e-9) || !near(q.HolePerimeter, holePerim, 1e-9) ||
		!near(q.Perimeter, perim, 1e-9) || !near(q.Volume, volume, 1e-9) {
		t.Errorf("%s: got %+v, want area %v hole perimeter %v perimeter %v volume %v",
			name, q, area, holePerim, perim, volume)
	}
	if !isClosedManifold(s.Element.Verts, s.Element.Tris) {
		t.Errorf("%s: mesh is not a closed shell", name)
	}
	v, ok := meshVolume(s.Element.Verts, s.Element.Tris)
	if !ok || !nearRel(v, volume, 1e-4) {
		t.Errorf("%s: mesh volume %v (ok %v), want %v", name, v, ok, volume)
	}
}

func TestSlabHoleQuantities(t *testing.T) {
	s, err := BuildSlabPolygon(Polygon2D{Outer: holeOuter, Holes: [][][2]float64{holeH}}, 3, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	// area 10·8 − 2.5·1.2; hole perimeter 2·(2.5+1.2); perimeter 2·(10+8); volume 77·0.2
	checkSlabQuantities(t, "10×8 with one hole", s, 77, 7.4, 36, 15.4)
	if !reflect.DeepEqual(s.Top.Holes, [][][2]float64{holeH}) {
		t.Errorf("Top.Holes = %v, want %v", s.Top.Holes, [][][2]float64{holeH})
	}
	if !reflect.DeepEqual(s.Top.Outline, holeOuter) {
		t.Errorf("Top.Outline = %v, want the outer ring %v", s.Top.Outline, holeOuter)
	}
}

func TestSlabHoleNoHolesIsBuildSlab(t *testing.T) {
	for _, o := range [][][2]float64{holeOuter, rect54, lOutline} {
		want, err := BuildSlab(o, 3, 0.2)
		if err != nil {
			t.Fatal(err)
		}
		got, err := BuildSlabPolygon(Polygon2D{Outer: o}, 3, 0.2)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("BuildSlabPolygon without holes differs from BuildSlab:\n got %+v\nwant %+v", got, want)
		}
		layers := []float64{0.05, 0.1, 0.05}
		wantL, err := BuildSlabLayers(o, 3, layers)
		if err != nil {
			t.Fatal(err)
		}
		gotL, err := BuildSlabPolygonLayers(Polygon2D{Outer: o}, 3, layers)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotL, wantL) {
			t.Errorf("BuildSlabPolygonLayers without holes differs from BuildSlabLayers")
		}
	}
}

func TestSlabHoleLayers(t *testing.T) {
	slabs, err := BuildSlabPolygonLayers(Polygon2D{Outer: holeOuter, Holes: [][][2]float64{holeH}}, 3, []float64{0.05, 0.1, 0.05})
	if err != nil {
		t.Fatal(err)
	}
	if len(slabs) != 3 {
		t.Fatalf("got %d layers, want 3", len(slabs))
	}
	var vol float64
	for k, s := range slabs {
		if !near(s.Quantities.Area, 77, 1e-9) || !near(s.Quantities.HolePerimeter, 7.4, 1e-9) {
			t.Errorf("layer %d: got %+v, want area 77 hole perimeter 7.4", k, s.Quantities)
		}
		if !reflect.DeepEqual(s.Top.Holes, [][][2]float64{holeH}) {
			t.Errorf("layer %d: Top.Holes = %v, want %v", k, s.Top.Holes, [][][2]float64{holeH})
		}
		if !isClosedManifold(s.Element.Verts, s.Element.Tris) {
			t.Errorf("layer %d: mesh is not a closed shell", k)
		}
		vol += s.Quantities.Volume
	}
	if !near(vol, 15.4, 1e-9) {
		t.Errorf("layer volumes sum to %v, want 15.4", vol)
	}
}

func TestSlabHoleWinding(t *testing.T) {
	reversed := slices.Clone(holeH)
	slices.Reverse(reversed)
	cwOuter := slices.Clone(holeOuter)
	slices.Reverse(cwOuter)
	// H closed by repeating its first point, with an extra point on its bottom edge.
	closed := [][2]float64{{2, 2}, {3, 2}, {4.5, 2}, {4.5, 3.2}, {2, 3.2}, {2, 2}}
	for _, tc := range []struct {
		name  string
		outer [][2]float64
		hole  [][2]float64
	}{
		{"hole wound against the outer ring", holeOuter, holeH},
		{"hole wound with the outer ring", holeOuter, reversed},
		{"clockwise outer ring", cwOuter, holeH},
		{"repeated closing point and a collinear point", holeOuter, closed},
	} {
		s, err := BuildSlabPolygon(Polygon2D{Outer: tc.outer, Holes: [][][2]float64{tc.hole}}, 3, 0.2)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		checkSlabQuantities(t, tc.name, s, 77, 7.4, 36, 15.4)
	}
}

func TestSlabHoleTranslated(t *testing.T) {
	const dx, dy = 500000, 4600000
	p := Polygon2D{Outer: shifted(holeOuter, dx, dy), Holes: [][][2]float64{shifted(holeH, dx, dy)}}
	s, err := BuildSlabPolygon(p, 3, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	checkSlabQuantities(t, "national grid", s, 77, 7.4, 36, 15.4)
	if s.Top.Origin != [3]float64{dx, dy, 3} {
		t.Errorf("Top.Origin = %v, want the outer ring's first point at z", s.Top.Origin)
	}
	for k, q := range s.Top.Holes[0] {
		if !near(q[0], holeH[k][0], 1e-9) || !near(q[1], holeH[k][1], 1e-9) {
			t.Errorf("Top.Holes[0][%d] = %v, want %v", k, q, holeH[k])
		}
	}
}

func TestSlabHoleConcave(t *testing.T) {
	l := [][2]float64{{0, 0}, {18, 0}, {18, 6}, {12, 6}, {12, 10}, {0, 10}}
	// One hole in each arm of the L: 2×1.5 in the long arm, 3×2 in the tall one.
	a := [][2]float64{{14, 2}, {16, 2}, {16, 3.5}, {14, 3.5}}
	b := [][2]float64{{4, 6}, {7, 6}, {7, 8}, {4, 8}}
	s, err := BuildSlabPolygon(Polygon2D{Outer: l, Holes: [][][2]float64{a, b}}, 0, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	// area 18·6 + 12·4 − 3 − 6; hole perimeter 7 + 10; perimeter 2·(18+10)
	checkSlabQuantities(t, "L with two holes", s, 156-9, 17, 56, 147*0.25)
}

func TestSlabHoleMany(t *testing.T) {
	// 64 holes, 0.5 m squares on a 1.2 m grid, in a 10 × 10 slab: the hole cap,
	// a closed shell, within the work budget.
	var holes [][][2]float64
	for i := range 8 {
		for j := range 8 {
			holes = append(holes, holeSquare(0.5+1.2*float64(i), 0.5+1.2*float64(j), 0.5))
		}
	}
	s, err := BuildSlabPolygon(Polygon2D{Outer: holeSquare(0, 0, 10), Holes: holes}, 0, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	// area 100 − 64·0.25; hole perimeter 64·2
	checkSlabQuantities(t, "64 holes", s, 84, 128, 40, 84*0.2)
}

func TestSlabHoleRefusals(t *testing.T) {
	crossing := [][2]float64{{9, 2}, {11, 2}, {11, 3}, {9, 3}}
	nearEdge := [][2]float64{{8, 2}, {9.9995, 2}, {9.9995, 3}, {8, 3}}
	touching := [][2]float64{{4.5, 2.5}, {6, 2.5}, {6, 4}, {4.5, 4}}
	bowtie := [][2]float64{{6, 5}, {7, 6}, {7, 5}, {6, 6}}
	l := [][2]float64{{0, 0}, {18, 0}, {18, 6}, {12, 6}, {12, 10}, {0, 10}}
	for _, tc := range []struct {
		name  string
		outer [][2]float64 // holeOuter when nil
		holes [][][2]float64
		want  []HoleError
	}{
		{"a hole across the outer edge", nil, [][][2]float64{crossing}, []HoleError{{0, ReasonHoleOutside}}},
		{"a hole 0.5 mm inside the outer edge", nil, [][][2]float64{nearEdge}, []HoleError{{0, ReasonHoleOutside}}},
		{"a hole wholly outside", nil, [][][2]float64{holeSquare(20, 20, 1)}, []HoleError{{0, ReasonHoleOutside}}},
		{"a hole around the whole slab", nil, [][][2]float64{holeSquare(-1, -1, 12)}, []HoleError{{0, ReasonHoleOutside}}},
		{"a hole touching H's edge", nil, [][][2]float64{holeH, touching}, []HoleError{{1, ReasonHolesOverlap}}},
		{"a hole inside H", nil, [][][2]float64{holeH, holeSquare(2.5, 2.3, 0.5)}, []HoleError{{1, ReasonHolesOverlap}}},
		{"a hole around H", nil, [][][2]float64{holeH, holeSquare(1, 1, 3)}, []HoleError{{1, ReasonHolesOverlap}}},
		{"a bowtie hole", nil, [][][2]float64{bowtie}, []HoleError{{0, ReasonHoleSelfCrossing}}},
		{"a collinear hole", nil, [][][2]float64{{{2, 2}, {3, 2}, {4, 2}}}, []HoleError{{0, ReasonHoleSelfCrossing}}},
		{"a hole pinched 0.5 mm", nil, [][][2]float64{{{2, 2}, {4, 2}, {3, 2.0005}, {3, 3}}}, []HoleError{{0, ReasonHoleSelfCrossing}}},
		// Every point inside the L, but an edge cuts across the notch.
		{"a hole across the L's notch", l, [][][2]float64{{{11, 5}, {14, 5}, {14, 5.5}, {11, 8}}}, []HoleError{{0, ReasonHoleOutside}}},
		{"a hole in the L's notch", l, [][][2]float64{holeSquare(14, 7, 2)}, []HoleError{{0, ReasonHoleOutside}}},
		{"two bad holes", nil, [][][2]float64{crossing, holeH, bowtie}, []HoleError{{0, ReasonHoleOutside}, {2, ReasonHoleSelfCrossing}}},
	} {
		if tc.outer == nil {
			tc.outer = holeOuter
		}
		_, err := BuildSlabPolygon(Polygon2D{Outer: tc.outer, Holes: tc.holes}, 3, 0.2)
		var he *HoleError
		if !errors.As(err, &he) {
			t.Errorf("%s: got %v, want a *HoleError", tc.name, err)
			continue
		}
		if *he != tc.want[0] {
			t.Errorf("%s: got %v, want %v", tc.name, *he, tc.want[0])
		}
		var got []HoleError
		for _, e := range joined(err) {
			if errors.As(e, &he) {
				got = append(got, *he)
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
		if _, err := BuildSlabPolygonLayers(Polygon2D{Outer: tc.outer, Holes: tc.holes}, 3, []float64{0.1, 0.1}); !errors.As(err, &he) {
			t.Errorf("%s: layers got %v, want a *HoleError", tc.name, err)
		}
	}
	if got := (&HoleError{Hole: 2, Reason: ReasonHolesOverlap}).Error(); got != "hole 2: overlaps" {
		t.Errorf("Error() = %q", got)
	}
}

func joined(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		return j.Unwrap()
	}
	return []error{err}
}

func TestSlabHoleInputErrors(t *testing.T) {
	var many [][][2]float64
	for i := range 65 {
		many = append(many, holeSquare(0.2+0.15*float64(i), 4, 0.1))
	}
	for _, tc := range []struct {
		name     string
		holes    [][][2]float64
		tooLarge bool
	}{
		{"a hole of two points", [][][2]float64{{{2, 2}, {3, 2}}}, false},
		{"a hole of two points closed", [][][2]float64{{{2, 2}, {3, 2}, {2, 2}}}, false},
		{"a NaN hole point", [][][2]float64{{{2, 2}, {math.NaN(), 2}, {3, 3}}}, false},
		{"an Inf hole point", [][][2]float64{{{2, 2}, {3, math.Inf(-1)}, {3, 3}}}, false},
		{"a hole edge under 1 mm", [][][2]float64{{{2, 2}, {3, 2}, {3.0005, 2}, {3, 3}}}, false},
		{"a hole point 1,001 m away", [][][2]float64{{{2, 2}, {1001, 2}, {3, 3}}}, true},
		{"65 holes", many, true},
		{"2,049 points in all", [][][2]float64{shifted(regularPolygon(maxOutlinePoints+1-len(holeOuter), 1), 5, 4)}, true},
	} {
		_, err := BuildSlabPolygon(Polygon2D{Outer: holeOuter, Holes: tc.holes}, 3, 0.2)
		var he *HoleError
		switch {
		case err == nil:
			t.Errorf("%s: want an error", tc.name)
		case errors.As(err, &he):
			t.Errorf("%s: got hole refusal %v, want an input error", tc.name, err)
		case errors.Is(err, ErrTooLarge) != tc.tooLarge:
			t.Errorf("%s: got %v, want too large %v", tc.name, err, tc.tooLarge)
		}
	}
	// The cap counts every point: 2,048 in all builds.
	if _, err := BuildSlabPolygon(Polygon2D{Outer: holeOuter, Holes: [][][2]float64{shifted(regularPolygon(maxOutlinePoints-len(holeOuter), 1), 5, 4)}}, 3, 0.2); err != nil {
		t.Errorf("2,048 points in all: %v", err)
	}
}

func TestSlabHoleCheckWork(t *testing.T) {
	// A 1,856-point outline with 64 triangular holes: 2,048 points in all.
	// Each hole is compared with the outline's edges, not the outline with
	// itself again: 3·1,856·3 units per hole, about 1.1 million in all, where
	// re-checking the outline per hole cost over 200 million.
	outer := shifted(regularPolygon(1856, 20), 20, 20)
	var holes [][][2]float64
	for i := range 8 {
		for j := range 8 {
			x, y := 10+2.2*float64(i), 10+2.2*float64(j)
			holes = append(holes, [][2]float64{{x, y}, {x + 1, y}, {x, y + 1}})
		}
	}
	b := newBudget()
	if _, err := checkHoles(localRing(outer), outer[0], holes, b); err != nil {
		t.Fatal(err)
	}
	if spent := b.size - b.left; spent > 2_000_000 {
		t.Errorf("checking 64 holes in a 1,856-point outline cost %d work units, want at most 2,000,000", spent)
	}
	if _, err := BuildSlabPolygon(Polygon2D{Outer: outer, Holes: holes}, 0, 0.2); err != nil {
		t.Error(err)
	}
}
