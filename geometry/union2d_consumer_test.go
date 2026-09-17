package geometry_test

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/geometry"
)

// TestUnionThroughTheExportedDoor enters exactly as a consumer outside the
// module does — qualified names, and no access to anything unexported. Every
// other test of this code is internal to the package, so without this one a
// symbol could be tested thoroughly and still not be reachable from outside.
func TestUnionThroughTheExportedDoor(t *testing.T) {
	polys := []geometry.Polygon2D{
		{
			Outer: [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}},
			Holes: [][][2]float64{{{1, 1}, {1, 2}, {2, 2}, {2, 1}}},
		},
		{Outer: [][2]float64{{1, 1}, {2, 1}, {2, 2}, {1, 2}}},
	}
	area, ok := geometry.UnionArea2D(polys)
	if !ok || math.Abs(area-16) > 1e-9 {
		t.Errorf("geometry.UnionArea2D = %v, %v; want 16, true", area, ok)
	}
	area, per, ok := geometry.UnionMeasure2D(polys)
	if !ok || math.Abs(area-16) > 1e-9 || math.Abs(per-16) > 1e-9 {
		t.Errorf("geometry.UnionMeasure2D = %v, %v, %v; want 16, 16, true", area, per, ok)
	}
	if _, ok := geometry.UnionArea2D(nil); ok {
		t.Error("geometry.UnionArea2D(nil) = ok; want false")
	}
}
