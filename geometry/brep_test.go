package geometry

import (
	"strings"
	"testing"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
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

// IfcFaceBasedSurfaceModel had no dispatch case at all, so every element built
// from one degraded to a bounding box. It is the top entry of the measured gap
// list (65 occurrences), and in duplex_a it hides 235 IfcConnectedFaceSets no
// other path can reach.
func TestFaceBasedSurfaceModel_Tessellates(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/face_based_surface_model.ifc")
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
		t.Fatalf("source = %q, want brep — the element fell back to a box", e.Source)
	}
	// Two quad faces -> 2 triangles each -> 12 indices, same as known_box.ifc.
	if len(e.Tris) != 12 {
		t.Errorf("tris = %d indices, want 12", len(e.Tris))
	}
	if !closeVec(e.BBoxMin, [3]float64{10, 20, 5}, 1e-6) {
		t.Errorf("world min = %v, want {10 20 5}", e.BBoxMin)
	}
}

// An empty or unusable FbsmFaces set must decline so the element falls back to
// its OBB box. Returning an empty mesh that still claims SourceBrep would
// produce a zero-volume element — the "Collapsed" case the coverage page
// exists to surface, reported as a success.
func TestFaceBasedSurfaceModel_EmptyDeclines(t *testing.T) {
	for _, tc := range []struct{ name, faces string }{
		{"empty set", "()"},
		{"non-face-set member", "(#22)"}, // an IfcCartesianPoint
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := step.Parse(strings.NewReader(
				"ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n" +
					"#22=IFCCARTESIANPOINT((0.,0.,0.));\n" +
					"#61=IFCFACEBASEDSURFACEMODEL(" + tc.faces + ");\n" +
					"ENDSEC;\nEND-ISO-10303-21;\n"))
			if err != nil {
				t.Fatal(err)
			}
			item, ok := f.ByID(61)
			if !ok {
				t.Fatal("fixture instance #61 missing")
			}
			if _, _, gotOK := surfaceModelMesh(item, attrFbsmFaces); gotOK {
				t.Error("surfaceModelMesh accepted an unusable FbsmFaces set; want ok=false so the caller boxes the element")
			}
		})
	}
}
