package geometry

import (
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

func TestBrep_FacetedFaces(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/known_box.ifc")
	if err != nil {
		t.Fatal(err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Elements[0]
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	// Two quad faces → 2 triangles each → 12 indices.
	if len(e.Tris) != 12 {
		t.Errorf("brep tris = %d indices, want 12", len(e.Tris))
	}
	// World AABB must still be the known box (brep path must not move it).
	if !closeVec(e.BBoxMin, [3]float64{10, 20, 5}, 1e-6) {
		t.Errorf("brep world min = %v, want {10 20 5}", e.BBoxMin)
	}
}
