package geometry

import (
	"math"
	"testing"
)

func TestTriangulateWithHoles(t *testing.T) {
	l := [][2]float64{{0, 0}, {18, 0}, {18, 6}, {12, 6}, {12, 10}, {0, 10}}
	for _, tc := range []struct {
		name  string
		outer [][2]float64
		holes [][][2]float64
	}{
		{"one hole", holeOuter, [][][2]float64{holeH}},
		{"holes side by side", holeOuter, [][][2]float64{holeSquare(1, 1, 2), holeSquare(4, 1, 2), holeSquare(7, 1, 2)}},
		// Each hole's ray hits the hole to its right, already bridged: the bridge must pick
		// the copy of the vertex that faces it.
		{"holes in a row", holeOuter, [][][2]float64{holeSquare(1, 3, 1), holeSquare(3, 3, 1), holeSquare(5, 3, 1), holeSquare(7, 3, 1)}},
		{"holes one above the other", holeOuter, [][][2]float64{holeSquare(4, 1, 1), holeSquare(4, 3, 1), holeSquare(4, 5, 1)}},
		{"L with a hole in each arm", l, [][][2]float64{{{14, 2}, {16, 2}, {16, 3.5}, {14, 3.5}}, {{4, 6}, {7, 6}, {7, 8}, {4, 8}}}},
		{"a hole in the L's notch shadow", l, [][][2]float64{holeSquare(2, 7, 1)}},
		{"a round hole", holeOuter, [][][2]float64{shifted(regularPolygon(40, 1.5), 5, 4)}},
	} {
		ring, tris, ok := triangulateWithHoles(tc.outer, tc.holes, newBudget())
		if !ok {
			t.Errorf("%s: not triangulated", tc.name)
			continue
		}
		want := math.Abs(polygonArea2D(tc.outer))
		for _, h := range tc.holes {
			want -= math.Abs(polygonArea2D(h))
		}
		var got float64
		for i := 0; i+2 < len(tris); i += 3 {
			a, b, c := ring[tris[i]], ring[tris[i+1]], ring[tris[i+2]]
			got += math.Abs(cross2D(a, b, c)) / 2
			centroid := [2]float64{(a[0] + b[0] + c[0]) / 3, (a[1] + b[1] + c[1]) / 3}
			for k, h := range tc.holes {
				if pointInPolygon(centroid, h) {
					t.Errorf("%s: triangle %v has its centroid in hole %d", tc.name, [3][2]float64{a, b, c}, k)
				}
			}
		}
		if !nearRel(got, want, 1e-9) {
			t.Errorf("%s: triangles cover %v, want %v", tc.name, got, want)
		}
	}
}

func TestTriangulateWithHolesBudget(t *testing.T) {
	if _, _, ok := triangulateWithHoles(holeOuter, [][][2]float64{holeH}, sizedBudget(10)); ok {
		t.Error("a budget of 10 triangulated a holed cap")
	}
}
