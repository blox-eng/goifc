package geometry

import (
	"math"
	"testing"
)

// A loop whose last ear is a sliver of nearly collinear points leaves no ear to
// clip; the cap must still use every point, or the mesh around it is open.
func TestEarClipKeepsCollinearSliverPoints(t *testing.T) {
	poly := [][2]float64{{0, 0}, {5, 4}, {2, 1.6}, {0.4, 0.32000000000000045}, {0, 8}}
	tris, ok := triangulatePolygon(poly, newBudget())
	if !ok {
		t.Fatal("triangulation declined")
	}
	used := map[uint32]bool{}
	var area float64
	for i := 0; i < len(tris); i += 3 {
		for _, v := range tris[i : i+3] {
			used[v] = true
		}
		area += cross2D(poly[tris[i]], poly[tris[i+1]], poly[tris[i+2]]) / 2
	}
	for i := range poly {
		if !used[uint32(i)] {
			t.Errorf("point %d %v is in no triangle", i, poly[i])
		}
	}
	var twice float64
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		twice += p[0]*q[1] - q[0]*p[1]
	}
	if want := twice / 2; math.Abs(area-want) > 1e-9 {
		t.Errorf("area %v, want %v", area, want)
	}
}
