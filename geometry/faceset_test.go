package geometry

import (
	"strings"
	"testing"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// faceSetIFC wraps representation item #61 (plus whatever it references) in a
// minimal metre-unit project: one proxy placed at (10, 20, 5).
func faceSetIFC(data string) string {
	return "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n" +
		"#1=IFCPROJECT('0proj',$,'P',$,$,$,$,(#20),#10);\n" +
		"#10=IFCUNITASSIGNMENT((#11));\n" +
		"#11=IFCSIUNIT(*,.LENGTHUNIT.,$,.METRE.);\n" +
		"#20=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-05,#21,$);\n" +
		"#21=IFCAXIS2PLACEMENT3D(#22,$,$);\n" +
		"#22=IFCCARTESIANPOINT((0.,0.,0.));\n" +
		"#30=IFCCARTESIANPOINT((10.,20.,5.));\n" +
		"#31=IFCAXIS2PLACEMENT3D(#30,$,$);\n" +
		"#32=IFCLOCALPLACEMENT($,#31);\n" +
		data +
		"#62=IFCSHAPEREPRESENTATION(#20,'Body','Tessellation',(#61));\n" +
		"#63=IFCPRODUCTDEFINITIONSHAPE($,$,(#62));\n" +
		"#70=IFCBUILDINGELEMENTPROXY('0fset',$,'FaceSet',$,$,#32,#63,$);\n" +
		"ENDSEC;\nEND-ISO-10303-21;\n"
}

func buildFaceSet(t *testing.T, data string) Element {
	t.Helper()
	f, err := step.Parse(strings.NewReader(faceSetIFC(data)))
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
	if len(s.Elements) != 1 {
		t.Fatalf("want 1 element, got %d", len(s.Elements))
	}
	return s.Elements[0]
}

func faceSetItem(t *testing.T, data string) *step.Instance {
	t.Helper()
	f, err := step.Parse(strings.NewReader(faceSetIFC(data)))
	if err != nil {
		t.Fatal(err)
	}
	item, ok := f.ByID(61)
	if !ok {
		t.Fatal("fixture instance #61 missing")
	}
	return item
}

// unitCubePoints is the corner list both cube fixtures index into.
const unitCubePoints = "#60=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(1.,0.,0.),(1.,1.,0.),(0.,1.,0.),(0.,0.,1.),(1.,0.,1.),(1.,1.,1.),(0.,1.,1.)));\n"

const unitCubeTriangles = "((1,3,2),(1,4,3),(5,6,7),(5,7,8),(1,2,6),(1,6,5),(2,3,7),(2,7,6),(3,4,8),(3,8,7),(4,1,5),(4,5,8))"

func wantBox(t *testing.T, e Element, min, max [3]float64) {
	t.Helper()
	if e.BBoxMin != min || e.BBoxMax != max {
		t.Errorf("world AABB = %v..%v, want %v..%v", e.BBoxMin, e.BBoxMax, min, max)
	}
}

// IfcTriangulatedFaceSet is IFC4's native tessellated body — what SketchUp,
// BlenderBIM and Revit's IFC4 Reference View export. With no dispatch case
// every element built from one produced no triangles and no box at all.
func TestTriangulatedFaceSet_Tessellates(t *testing.T) {
	e := buildFaceSet(t, unitCubePoints+
		"#61=IFCTRIANGULATEDFACESET(#60,$,.T.,"+unitCubeTriangles+",$);\n")
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	if len(e.Tris) != 36 {
		t.Errorf("got %d triangle indices, want 36 (12 triangles)", len(e.Tris))
	}
	wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{11, 21, 6})
}

// IfcTriangulatedIrregularNetwork is an IFC4X3 subtype. IsA matches the exact
// keyword only, so the dispatch has to name it.
func TestTriangulatedIrregularNetwork_Tessellates(t *testing.T) {
	e := buildFaceSet(t, unitCubePoints+
		"#61=IFCTRIANGULATEDIRREGULARNETWORK(#60,$,.T.,"+unitCubeTriangles+",$,(1,1,1,1,1,1,1,1,1,1,1,1));\n")
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	if len(e.Tris) != 36 {
		t.Errorf("got %d triangle indices, want 36 (12 triangles)", len(e.Tris))
	}
	wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{11, 21, 6})
}

// The original IFC4 release (2013) had NormalIndex, a list of index lists, in
// the slot ADD2 later gave to PnIndex. Normals do not affect the mesh, so that
// shape is ignored rather than read as a malformed PnIndex that would box every
// face set an older exporter wrote.
func TestTriangulatedFaceSet_LegacyNormalIndexIgnored(t *testing.T) {
	e := buildFaceSet(t, unitCubePoints+
		"#61=IFCTRIANGULATEDFACESET(#60,$,.T.,"+unitCubeTriangles+","+unitCubeTriangles+");\n")
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{11, 21, 6})
}

// A malformed point that no face references cannot change the mesh, so it does
// not decline the set.
func TestTriangulatedFaceSet_UnreferencedBadPointIgnored(t *testing.T) {
	e := buildFaceSet(t,
		"#60=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(1.,0.,0.),(0.,1.,0.),(0.,0.,1.),(9.,9.)));\n"+
			"#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,3,2),(1,2,4),(1,4,3),(2,3,4)),$);\n")
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{11, 21, 6})
}

// With PnIndex, CoordIndex values index into PnIndex, whose values index into
// the point list. The first point is an outlier nothing references: it must not
// reach the mesh, or it would inflate the element's bounds.
func TestTriangulatedFaceSet_PnIndexRemaps(t *testing.T) {
	e := buildFaceSet(t,
		"#60=IFCCARTESIANPOINTLIST3D(((100.,100.,100.),(0.,0.,0.),(1.,0.,0.),(0.,1.,0.),(0.,0.,1.)));\n"+
			"#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,3,2),(1,2,4),(1,4,3),(2,3,4)),(2,3,4,5));\n")
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	if len(e.Tris) != 12 {
		t.Errorf("got %d triangle indices, want 12", len(e.Tris))
	}
	wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{11, 21, 6})
}

func TestPolygonalFaceSet_Tessellates(t *testing.T) {
	e := buildFaceSet(t, unitCubePoints+
		"#50=IFCINDEXEDPOLYGONALFACE((1,4,3,2));\n"+
		"#51=IFCINDEXEDPOLYGONALFACE((5,6,7,8));\n"+
		"#52=IFCINDEXEDPOLYGONALFACE((1,2,6,5));\n"+
		"#53=IFCINDEXEDPOLYGONALFACE((2,3,7,6));\n"+
		"#54=IFCINDEXEDPOLYGONALFACE((3,4,8,7));\n"+
		"#55=IFCINDEXEDPOLYGONALFACE((4,1,5,8));\n"+
		"#61=IFCPOLYGONALFACESET(#60,.T.,(#50,#51,#52,#53,#54,#55),$);\n")
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	if len(e.Tris) != 36 {
		t.Errorf("got %d triangle indices, want 36 (two per quad)", len(e.Tris))
	}
	wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{11, 21, 6})
}

// A face with a void is tessellated from its outer loop only, so the hole is
// filled. That over-reports area, which is the conservative side — the same
// choice brepMesh makes for inner face bounds.
func TestPolygonalFaceSet_VoidIsFilled(t *testing.T) {
	e := buildFaceSet(t,
		"#60=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(4.,0.,0.),(4.,4.,0.),(0.,4.,0.),(1.,1.,0.),(3.,1.,0.),(3.,3.,0.),(1.,3.,0.)));\n"+
			"#50=IFCINDEXEDPOLYGONALFACEWITHVOIDS((1,2,3,4),((5,8,7,6)));\n"+
			"#61=IFCPOLYGONALFACESET(#60,.F.,(#50),$);\n")
	if e.Source != SourceBrep {
		t.Fatalf("source = %q, want brep", e.Source)
	}
	if len(e.Tris) != 6 {
		t.Errorf("got %d triangle indices, want 6 (the outer square, hole filled)", len(e.Tris))
	}
	wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{14, 24, 5})
}

// A face set that is malformed anywhere is declined whole, never meshed in
// part: a partial mesh is smaller than the element and would under-report its
// bounds without falling back to the box.
func TestFaceSet_MalformedDeclines(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"index zero", unitCubePoints + "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((0,2,3),(1,2,3)),$);\n"},
		{"index past the point list", unitCubePoints + "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,2,3),(1,2,9)),$);\n"},
		{"not a triple", unitCubePoints + "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,2,3),(1,2)),$);\n"},
		{"no triangles", unitCubePoints + "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,(),$);\n"},
		{"PnIndex past the point list", unitCubePoints + "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,2,3)),(1,2,9));\n"},
		{"corner past the end of PnIndex", unitCubePoints + "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,2,5)),(1,2,3,4));\n"},
		{"PnIndex of reals", unitCubePoints + "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,2,3)),(1.,2.,3.));\n"},
		{"referenced point with two coordinates", "#60=IFCCARTESIANPOINTLIST3D(((0.,0.,0.),(1.,0.),(0.,1.,0.)));\n#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,2,3)),$);\n"},
		{"polygon face index past the point list", unitCubePoints + "#50=IFCINDEXEDPOLYGONALFACE((1,2,3));\n#51=IFCINDEXEDPOLYGONALFACE((1,2,9));\n#61=IFCPOLYGONALFACESET(#60,.F.,(#50,#51),$);\n"},
		// Beside a valid face, so the set would otherwise mesh in part.
		{"polygon face with two indices", unitCubePoints + "#50=IFCINDEXEDPOLYGONALFACE((1,2,3));\n#51=IFCINDEXEDPOLYGONALFACE((1,2));\n#61=IFCPOLYGONALFACESET(#60,.F.,(#50,#51),$);\n"},
		// The parser is schema-less, so a member of another type, given an index
		// list in attribute 0, gets as far as the type check and no further.
		{"polygon face member of the wrong type", unitCubePoints + "#50=IFCINDEXEDPOLYGONALFACE((1,2,3));\n#51=IFCINDEXEDTRIANGLETEXTUREMAP((1,2,3));\n#61=IFCPOLYGONALFACESET(#60,.F.,(#50,#51),$);\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := faceSetMesh(faceSetItem(t, tc.data)); ok {
				t.Error("faceSetMesh accepted a malformed face set; want ok=false so the caller boxes the element")
			}
		})
	}
}

// When a face set is declined, the element must still get its bounding box.
// collectPoints only read IfcCartesianPoint, and a face set keeps its points in
// an IfcCartesianPointList3D, so the fallback found nothing and the element
// vanished: no triangles and a zero AABB. An element that vanishes is a total
// under-report; a box is only an over-report.
func TestFaceSet_DeclinedStillBoxes(t *testing.T) {
	for _, tc := range []struct{ name, item string }{
		{"triangulated", "#61=IFCTRIANGULATEDFACESET(#60,$,.T.,((1,2,3),(1,2,9)),$);\n"},
		{"polygonal", "#50=IFCINDEXEDPOLYGONALFACE((1,2,3,9));\n#61=IFCPOLYGONALFACESET(#60,.T.,(#50),$);\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := buildFaceSet(t, unitCubePoints+tc.item)
			if e.Source != SourceOBB {
				t.Fatalf("source = %q, want obb", e.Source)
			}
			if len(e.Tris) == 0 {
				t.Fatal("declined face set produced no box: the element vanished")
			}
			wantBox(t, e, [3]float64{10, 20, 5}, [3]float64{11, 21, 6})
		})
	}
}
