package geometry

import (
	"errors"
	"math"
	"testing"
)

var roofR = [][2]float64{{0, 0}, {10, 0}, {10, 8}, {0, 8}}

func TestCheckRoofRefusals(t *testing.T) {
	collinear := [][2]float64{{0, 0}, {5, 0}, {10, 0}, {10, 8}, {0, 8}}
	many := regularPolygon(257, 10)
	tests := []struct {
		name    string
		outline [][2]float64
		spec    RoofSpec
		want    RoofReason
		edge    int
		other   int
	}{
		{"pitch 0", roofR, RoofSpec{Pitch: 0}, ReasonPitchOutOfRange, -1, -1},
		{"pitch negative", roofR, RoofSpec{Pitch: -1}, ReasonPitchOutOfRange, -1, -1},
		{"pitch over 75", roofR, RoofSpec{Pitch: 75.0001}, ReasonPitchOutOfRange, -1, -1},
		{"pitch NaN", roofR, RoofSpec{Pitch: math.NaN()}, ReasonPitchOutOfRange, -1, -1},
		{"pitch 75", roofR, RoofSpec{Pitch: 75}, "", 0, 0},
		{"gable out of range", roofR, RoofSpec{Pitch: 30, Gables: []int{4}}, ReasonGableOutOfRange, 4, -1},
		{"gable negative", roofR, RoofSpec{Pitch: 30, Gables: []int{-1}}, ReasonGableOutOfRange, -1, -1},
		{"gable repeated", roofR, RoofSpec{Pitch: 30, Gables: []int{1, 1}}, ReasonGableRepeated, 1, -1},
		{"every edge a gable", roofR, RoofSpec{Pitch: 30, Gables: []int{0, 1, 2, 3}}, ReasonNoSlopedEdge, -1, -1},
		{"overhang negative", roofR, RoofSpec{Pitch: 30, Overhang: -0.1}, ReasonOverhangOutOfRange, -1, -1},
		{"overhang over 3", roofR, RoofSpec{Pitch: 30, Overhang: 3.0001}, ReasonOverhangOutOfRange, -1, -1},
		{"overhang NaN", roofR, RoofSpec{Pitch: 30, Overhang: math.NaN()}, ReasonOverhangOutOfRange, -1, -1},
		{"overhang 3", roofR, RoofSpec{Pitch: 30, Overhang: 3}, "", 0, 0},
		{"gable collinear with sloped edge", collinear, RoofSpec{Pitch: 30, Gables: []int{0}}, ReasonGableCollinear, 0, 1},
		{"gable collinear from the other side", collinear, RoofSpec{Pitch: 30, Gables: []int{1}}, ReasonGableCollinear, 1, 0},
		{"collinear point, no gables", collinear, RoofSpec{Pitch: 30}, "", 0, 0},
		{"both collinear edges gables", collinear, RoofSpec{Pitch: 30, Gables: []int{0, 1}}, "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gable, total, err := checkRoof(tt.outline, tt.spec, []float64{0.2})
			if tt.want == "" {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				if len(gable) != len(tt.outline) || math.Abs(total-0.2) > 1e-12 {
					t.Errorf("gable %v total %v", gable, total)
				}
				return
			}
			var re *RoofError
			if !errors.As(err, &re) {
				t.Fatalf("err = %v, want a *RoofError", err)
			}
			if re.Reason != tt.want || re.Edge != tt.edge || re.Other != tt.other {
				t.Errorf("got %+v, want %s edge %d other %d", *re, tt.want, tt.edge, tt.other)
			}
		})
	}

	if _, _, err := checkRoof(many, RoofSpec{Pitch: 30}, []float64{0.2}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("257 points: err = %v, want ErrTooLarge", err)
	}
	if _, _, err := checkRoof(roofR, RoofSpec{Pitch: 75}, []float64{300}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a 1,159 m drop: err = %v, want ErrTooLarge", err)
	}
	if _, _, err := checkRoof(roofR, RoofSpec{Pitch: 30, Eaves: math.Inf(1)}, []float64{0.2}); err == nil {
		t.Error("infinite eaves accepted")
	}
	if _, _, err := checkRoof(roofR, RoofSpec{Pitch: 30}, nil); err == nil {
		t.Error("no layers accepted")
	}
	if _, _, err := checkRoof(roofR[:2], RoofSpec{Pitch: 30}, []float64{0.2}); err == nil {
		t.Error("two points accepted")
	}
}

func TestPlumbLineOffsetsEveryEdge(t *testing.T) {
	cw := [][2]float64{roofR[3], roofR[2], roofR[1], roofR[0]}
	for name, outline := range map[string][][2]float64{"ccw": roofR, "cw": cw} {
		t.Run(name, func(t *testing.T) {
			ring, edgeOf, err := plumbLine(outline, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(ring) != 4 || polygonArea2D(ring) <= 0 {
				t.Fatalf("ring %v, area %v: want 4 points, counter-clockwise", ring, polygonArea2D(ring))
			}
			// Every ring edge lies 1 m outside the caller's edge edgeOf[m].
			for m := range ring {
				e := edgeOf[m]
				a, b := outline[e], outline[(e+1)%4]
				mid := [2]float64{(ring[m][0] + ring[(m+1)%4][0]) / 2, (ring[m][1] + ring[(m+1)%4][1]) / 2}
				mid[0] += outline[0][0]
				mid[1] += outline[0][1]
				if d := segmentDistance(mid, a, b); math.Abs(d-1) > 1e-9 {
					t.Errorf("ring edge %d (caller's %d): midpoint %.3g m from it, want 1", m, e, d)
				}
			}
			if a := polygonArea2D(ring); math.Abs(a-12*10) > 1e-9 {
				t.Errorf("area %v, want 120", a)
			}
		})
	}
}

func TestPlumbLineZeroOverhangKeepsTheOutline(t *testing.T) {
	ring, edgeOf, err := plumbLine(roofR, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range localRing(roofR) {
		if ring[i] != p || edgeOf[i] != i {
			t.Errorf("point %d: %v edge %d", i, ring[i], edgeOf[i])
		}
	}
}

func TestPlumbLineMitresAConcaveCorner(t *testing.T) {
	// An L: the reflex corner at (6,4) moves into the notch, to (7,5).
	l := [][2]float64{{0, 0}, {10, 0}, {10, 4}, {6, 4}, {6, 8}, {0, 8}}
	ring, _, err := plumbLine(l, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := [2]float64{7, 5}; ring[3] != want {
		t.Errorf("reflex corner at %v, want %v", ring[3], want)
	}
	if want := [2]float64{-1, -1}; ring[0] != want {
		t.Errorf("first corner at %v, want %v", ring[0], want)
	}
}

func TestPlumbLineRefusesWhatClosesTheOutline(t *testing.T) {
	// A slot 1 m wide: 1 m of overhang on each wall closes it.
	u := [][2]float64{{0, 0}, {10, 0}, {10, 8}, {6, 8}, {6, 3}, {5, 3}, {5, 8}, {0, 8}}
	cw := make([][2]float64, len(u))
	for i, p := range u {
		cw[len(u)-1-i] = p
	}
	for name, outline := range map[string][][2]float64{"ccw": u, "cw": cw} {
		t.Run(name, func(t *testing.T) {
			_, _, err := plumbLine(outline, 1)
			var re *RoofError
			if !errors.As(err, &re) || re.Reason != ReasonOverhangClosesOutline {
				t.Fatalf("err = %v, want overhang_closes_outline", err)
			}
			if re.Edge == re.Other || re.Edge < 0 || re.Other < 0 || re.Edge >= len(u) || re.Other >= len(u) {
				t.Errorf("edges %d and %d", re.Edge, re.Other)
			}
			// Both named edges are slot edges of the caller's outline; the
			// slot is bounded by caller edges 3, 4 and 5 in ccw order.
			slot := map[int]bool{3: true, 4: true, 5: true}
			if name == "cw" {
				slot = map[int]bool{1: true, 2: true, 3: true} // reversed outline: edge k is u's edge 6-k
			}
			if !slot[re.Edge] || !slot[re.Other] {
				t.Errorf("edges %d and %d are not slot edges %v", re.Edge, re.Other, slot)
			}
		})
	}
	if _, _, err := plumbLine(u, 0.4); err != nil {
		t.Errorf("0.4 m leaves the slot open: %v", err)
	}
}

func TestPlumbLineRefusesACrossing(t *testing.T) {
	// A chamber behind a 1 m neck: 1 m of overhang pushes the neck walls past
	// each other while every edge keeps its direction, so only the crossing
	// test sees it.
	chamber := [][2]float64{{0, 0}, {10, 0}, {10, 10}, {6, 10}, {6, 7}, {9, 7}, {9, 3}, {2, 3}, {2, 7}, {5, 7}, {5, 10}, {0, 10}}
	_, _, err := plumbLine(chamber, 1)
	var re *RoofError
	if !errors.As(err, &re) || re.Reason != ReasonOverhangClosesOutline {
		t.Fatalf("err = %v, want overhang_closes_outline", err)
	}
	if _, _, err := plumbLine(chamber, 0.4); err != nil {
		t.Errorf("0.4 m leaves the neck open: %v", err)
	}
}
