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
//
// Note what the second case does and does not pin. surfaceModelMesh and
// brepMesh do not type-check set members: brepMesh reads attribute 0 of
// whatever it is handed. An IfcCartesianPoint's attribute 0 is Coordinates,
// which IS a list, so it gets that far — and declines only because the list
// holds reals rather than entity references. So this pins "attribute 0 is not
// a list of face refs", not "the member is rejected for having the wrong
// type". A wrong-typed member whose attribute 0 happened to be a list of
// references would still be walked as a shell.
func TestFaceBasedSurfaceModel_EmptyDeclines(t *testing.T) {
	for _, tc := range []struct{ name, faces string }{
		{"empty set", "()"},
		{"member whose attribute 0 is not a list of face refs", "(#22)"}, // an IfcCartesianPoint
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

// edgeLoopFaceAt5 is a triangular face whose only bound is an IfcEdgeLoop — a
// loop type brepMesh cannot read — lifted to z=5, well above anything else in
// the shell. Its points are the element's true top.
const edgeLoopFaceAt5 = "#80=IFCCARTESIANPOINT((0.,0.,5.));\n" +
	"#81=IFCCARTESIANPOINT((1.,0.,5.));\n" +
	"#82=IFCCARTESIANPOINT((1.,1.,5.));\n" +
	"#83=IFCVERTEXPOINT(#80);\n#84=IFCVERTEXPOINT(#81);\n#85=IFCVERTEXPOINT(#82);\n" +
	"#86=IFCEDGE(#83,#84);\n#87=IFCEDGE(#84,#85);\n#88=IFCEDGE(#85,#83);\n" +
	"#89=IFCORIENTEDEDGE(*,*,#86,.T.);\n#90=IFCORIENTEDEDGE(*,*,#87,.T.);\n#91=IFCORIENTEDEDGE(*,*,#88,.T.);\n" +
	"#92=IFCEDGELOOP((#89,#90,#91));\n" +
	"#93=IFCFACEOUTERBOUND(#92,.T.);\n" +
	"#94=IFCFACE((#93));\n"

// floorQuad is a readable unit square at z=0.
const floorQuad = "#40=IFCCARTESIANPOINT((0.,0.,0.));\n#41=IFCCARTESIANPOINT((1.,0.,0.));\n" +
	"#42=IFCCARTESIANPOINT((1.,1.,0.));\n#43=IFCCARTESIANPOINT((0.,1.,0.));\n" +
	"#50=IFCPOLYLOOP((#40,#41,#42,#43));\n#51=IFCFACEOUTERBOUND(#50,.T.);\n#52=IFCFACE((#51));\n"

// A shell with one unreadable face used to tessellate the faces it could read
// and report success, so the element shipped the partial mesh as "brep" and its
// AABB stopped short of the face it skipped. Any unreadable face must decline
// the whole shell, so the element falls back to a box over every point it has.
func TestBrep_PartiallyReadableShellDeclines(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"closed shell with an IfcEdgeLoop face", floorQuad + edgeLoopFaceAt5 +
			"#60=IFCCLOSEDSHELL((#52,#94));\n#61=IFCFACETEDBREP(#60);\n"},
		{"face-based surface model with one unreadable face set", floorQuad + edgeLoopFaceAt5 +
			"#58=IFCCONNECTEDFACESET((#52));\n#59=IFCCONNECTEDFACESET((#94));\n" +
			"#61=IFCFACEBASEDSURFACEMODEL((#58,#59));\n"},
		{"shell-based surface model with one unreadable shell", floorQuad + edgeLoopFaceAt5 +
			"#58=IFCOPENSHELL((#52));\n#59=IFCOPENSHELL((#94));\n" +
			"#61=IFCSHELLBASEDSURFACEMODEL((#58,#59));\n"},
		// The outer bound is unreadable, so the inner bound (a hole) used to be
		// promoted to the face's outline: a 1x1 hole standing in for a 5x5 face.
		{"face whose outer bound is unreadable", "#40=IFCCARTESIANPOINT((2.,2.,0.));\n#41=IFCCARTESIANPOINT((3.,2.,0.));\n" +
			"#42=IFCCARTESIANPOINT((3.,3.,0.));\n#43=IFCCARTESIANPOINT((2.,3.,0.));\n" +
			"#50=IFCPOLYLOOP((#40,#41,#42,#43));\n#51=IFCFACEBOUND(#50,.T.);\n" +
			"#80=IFCCARTESIANPOINT((0.,0.,0.));\n#81=IFCCARTESIANPOINT((5.,0.,0.));\n#82=IFCCARTESIANPOINT((5.,5.,5.));\n" +
			"#83=IFCVERTEXPOINT(#80);\n#84=IFCVERTEXPOINT(#81);\n#85=IFCVERTEXPOINT(#82);\n" +
			"#86=IFCEDGE(#83,#84);\n#87=IFCEDGE(#84,#85);\n#88=IFCEDGE(#85,#83);\n" +
			"#89=IFCORIENTEDEDGE(*,*,#86,.T.);\n#90=IFCORIENTEDEDGE(*,*,#87,.T.);\n#91=IFCORIENTEDEDGE(*,*,#88,.T.);\n" +
			"#92=IFCEDGELOOP((#89,#90,#91));\n#93=IFCFACEOUTERBOUND(#92,.T.);\n" +
			"#94=IFCFACE((#93,#51));\n#60=IFCCLOSEDSHELL((#94));\n#61=IFCFACETEDBREP(#60);\n"},
		// A two-coordinate point is not a 3D loop vertex; dropping it left a
		// triangle of the three that parsed.
		{"loop with an unreadable point", "#40=IFCCARTESIANPOINT((0.,0.,0.));\n#41=IFCCARTESIANPOINT((1.,0.,0.));\n" +
			"#42=IFCCARTESIANPOINT((1.,1.,0.));\n#43=IFCCARTESIANPOINT((0.,5.));\n#44=IFCCARTESIANPOINT((0.,0.,5.));\n" +
			"#50=IFCPOLYLOOP((#40,#41,#42,#43));\n#51=IFCFACEOUTERBOUND(#50,.T.);\n#52=IFCFACE((#51));\n" +
			"#53=IFCPOLYLOOP((#40,#41,#44));\n#54=IFCFACEOUTERBOUND(#53,.T.);\n#55=IFCFACE((#54));\n" +
			"#60=IFCCLOSEDSHELL((#52,#55));\n#61=IFCFACETEDBREP(#60);\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := buildFaceSet(t, tc.data)
			if e.Source != SourceOBB {
				t.Errorf("source = %q, want obb: a partially read shell must not count as a mesh", e.Source)
			}
			// The element sits at (10, 20, 5); every fixture's true top is z=5.
			if e.BBoxMax[2] < 10 {
				t.Errorf("world max z = %v, want >= 10: the bound stops short of the skipped face", e.BBoxMax[2])
			}
		})
	}
}
