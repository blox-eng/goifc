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

// The same sliver, reached through a valid outline: the cap must use every
// point, or the slab's shell is open.
func TestBuildSlabSliverOutlineIsClosed(t *testing.T) {
	outline := [][2]float64{{4, 0}, {2, 7}, {-4, 3}, {-5.5, -1}, {-7, -5},
		{-3.666666666666667, -6}, {-0.3333333333333339, -7}, {3, -8}}
	s, err := BuildSlab(outline, 0, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	meshAgrees(t, s.Element, s.Quantities.Volume, 2*s.Quantities.Area+s.Quantities.Perimeter*0.2, 0,
		s.Quantities.Area, s.Quantities.Volume)
}
