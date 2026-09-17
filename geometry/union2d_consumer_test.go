package geometry_test

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/geometry"
	"github.com/blox-eng/goifc/model"
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

// TestPolygonsFromLoopsThroughTheExportedDoor measures an element whose
// silhouette is two disjoint patches, converted the way the guide shows. A
// conversion that builds one polygon around the largest ring makes the other
// patch a "hole" outside its outer, and the union is refused.
func TestPolygonsFromLoopsThroughTheExportedDoor(t *testing.T) {
	// Two 1 x 1 x 1 m boxes, 2 m apart along Y.
	e := geometry.Element{
		GlobalID:  "pair",
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   [3]float64{0, 0, 0},
		BBoxMax:   [3]float64{1, 4, 1},
		Source:    geometry.SourceBrep,
	}
	for _, y0 := range []float32{0, 3} {
		base := uint32(len(e.Verts) / 3)
		for _, c := range [][3]float32{
			{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0},
			{0, 0, 1}, {1, 0, 1}, {1, 1, 1}, {0, 1, 1},
		} {
			e.Verts = append(e.Verts, c[0], c[1]+y0, c[2])
		}
		for _, i := range []uint32{
			0, 2, 1, 0, 3, 2, 4, 5, 6, 4, 6, 7, // bottom, top
			0, 1, 5, 0, 5, 4, 1, 2, 6, 1, 6, 5, // sides
			2, 3, 7, 2, 7, 6, 3, 0, 4, 3, 4, 7,
		} {
			e.Tris = append(e.Tris, base+i)
		}
	}
	p, ok := geometry.ElevationPlane([3]float64{-1, 0, 0})
	if !ok {
		t.Fatal("geometry.ElevationPlane(ok) = false")
	}
	loops := e.SilhouetteOn(p)
	if len(loops) != 2 {
		t.Fatalf("SilhouetteOn returned %d loops, want 2 — the fixture no longer poses two patches", len(loops))
	}
	polys, ok := geometry.PolygonsFromLoops(loops)
	if !ok || len(polys) != 2 {
		t.Fatalf("geometry.PolygonsFromLoops = %d polygons, %v; want 2, true", len(polys), ok)
	}
	area, per, ok := geometry.UnionMeasure2D(polys)
	if !ok || math.Abs(area-2) > 1e-9 || math.Abs(per-8) > 1e-9 {
		t.Errorf("geometry.UnionMeasure2D = %v, %v, %v; want 2, 8, true", area, per, ok)
	}
}
