package ifc_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/blox-eng/common/ifc"
	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

// boxIFC is a tiny self-contained IFC4 document with one IfcBuildingElementProxy
// carrying a real Brep body, so Assemble exercises the whole chain (extract ->
// build -> derive -> back-fill) without the gitignored corpus. The shell is
// open (top+bottom faces only) so it yields geometry-derived DIMENSIONS but no
// closed-mesh Volume — enough to prove the geometry tier flips on.
const boxIFC = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('box.ifc','',(''),(''),'','','');
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCPROJECT('0proj',$,'P',$,$,$,$,(#20),#10);
#10=IFCUNITASSIGNMENT((#11));
#11=IFCSIUNIT(*,.LENGTHUNIT.,$,.METRE.);
#20=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-05,#21,$);
#21=IFCAXIS2PLACEMENT3D(#22,$,$);
#22=IFCCARTESIANPOINT((0.,0.,0.));
#30=IFCCARTESIANPOINT((10.,20.,5.));
#31=IFCAXIS2PLACEMENT3D(#30,$,$);
#32=IFCLOCALPLACEMENT($,#31);
#40=IFCCARTESIANPOINT((0.,0.,0.));
#41=IFCCARTESIANPOINT((1.,0.,0.));
#42=IFCCARTESIANPOINT((1.,1.,0.));
#43=IFCCARTESIANPOINT((0.,1.,0.));
#44=IFCCARTESIANPOINT((0.,0.,1.));
#45=IFCCARTESIANPOINT((1.,0.,1.));
#46=IFCCARTESIANPOINT((1.,1.,1.));
#47=IFCCARTESIANPOINT((0.,1.,1.));
#50=IFCPOLYLOOP((#40,#41,#42,#43));
#51=IFCFACEOUTERBOUND(#50,.T.);
#52=IFCFACE((#51));
#53=IFCPOLYLOOP((#44,#45,#46,#47));
#54=IFCFACEOUTERBOUND(#53,.T.);
#55=IFCFACE((#54));
#60=IFCCLOSEDSHELL((#52,#55));
#61=IFCFACETEDBREP(#60);
#62=IFCSHAPEREPRESENTATION(#20,'Body','Brep',(#61));
#63=IFCPRODUCTDEFINITIONSHAPE($,$,(#62));
#70=IFCBUILDINGELEMENTPROXY('0box',$,'Box',$,$,#32,#63,$);
ENDSEC;
END-ISO-10303-21;`

// TestAssemble_SyntheticChainsAllStages is the CI-safe wiring guard: it proves
// Assemble runs all four stages and that a meshed element with no authored Qto
// is back-filled to quantity_source="geometry" (never a phantom 0.0), and that
// the returned Scene is GLB-writable — the two seams ⑤/⑦ depend on.
func TestAssemble_SyntheticChainsAllStages(t *testing.T) {
	f, err := step.ParseBytes([]byte(boxIFC))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	a, err := ifc.Assemble(f)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if a == nil || a.Result == nil || a.Scene == nil {
		t.Fatalf("assembled = %+v, want non-nil Result and Scene", a)
	}
	if len(a.Result.Elements) != 1 {
		t.Fatalf("Result.Elements = %d, want 1", len(a.Result.Elements))
	}
	// Scene and Result must stay index/GlobalID-aligned — the identity contract
	// doc.go promises to ⑤ (golden-diff) and ⑦ (flow cutover). If a future Build
	// change dropped or reordered elements, the two consumers would silently
	// desync; this is the one invariant this seam can guard that the geometry
	// package's hand-wired test cannot.
	if len(a.Scene.Elements) != len(a.Result.Elements) {
		t.Fatalf("Scene/Result element count mismatch: %d vs %d", len(a.Scene.Elements), len(a.Result.Elements))
	}
	if a.Scene.Elements[0].GlobalID != a.Result.Elements[0].GlobalID {
		t.Errorf("Scene/Result GlobalID desync: %q vs %q", a.Scene.Elements[0].GlobalID, a.Result.Elements[0].GlobalID)
	}
	e := a.Result.Elements[0]
	if e.QuantitySource != model.QuantitySourceGeometry {
		t.Errorf("quantity_source = %q, want %q", e.QuantitySource, model.QuantitySourceGeometry)
	}
	if e.Qto.IsEmpty() {
		t.Errorf("source=geometry but Qto is empty — phantom upgrade")
	}
	if e.Qto.Height == nil || *e.Qto.Height <= 0 {
		t.Errorf("Height = %v, want a positive geometry-derived extent", e.Qto.Height)
	}

	// The Scene is the WriteGLB seam the doc Example relies on.
	var buf bytes.Buffer
	if err := a.Scene.WriteGLB(&buf); err != nil {
		t.Fatalf("WriteGLB: %v", err)
	}
	if buf.Len() == 0 {
		t.Errorf("WriteGLB produced 0 bytes")
	}
}

// TestAssemble_NilFile guards the documented "single production entry point"
// against a caller programming error: a nil file returns an error, never panics
// deep inside Extract's unit lookup.
func TestAssemble_NilFile(t *testing.T) {
	if _, err := ifc.Assemble(nil); err == nil {
		t.Fatal("Assemble(nil) = nil error, want error")
	}
}

// TestAssemble_RealFile_kb645 is the end-to-end integration gate on the real
// corpus: Assemble must return a non-empty, correctly-tiered model with a
// populated geometry tier and zero fabricated 0.0. Skips gracefully when the
// gitignored corpus is absent so CI stays green without it.
func TestAssemble_RealFile_kb645(t *testing.T) {
	const path = "testdata/real/kb645.ifc"
	if _, err := os.Stat(path); err != nil {
		t.Skip("kb645.ifc absent")
	}
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	a, err := ifc.Assemble(f)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(a.Result.Elements) == 0 {
		t.Fatal("assembled output is empty")
	}
	// The bundle-parity invariant on a real scene — the unique value this seam
	// adds over geometry's tier test, which never sees the assembled bundle.
	if len(a.Scene.Elements) != len(a.Result.Elements) {
		t.Errorf("Scene/Result element count mismatch: %d vs %d", len(a.Scene.Elements), len(a.Result.Elements))
	}

	var qto, geom, none int
	for i := range a.Result.Elements {
		e := &a.Result.Elements[i]
		switch e.QuantitySource {
		case model.QuantitySourceQto:
			qto++
		case model.QuantitySourceGeometry:
			geom++
			if e.Qto.IsEmpty() {
				t.Errorf("%s source=geometry but Qto empty — phantom upgrade", e.GlobalID)
			}
		case model.QuantitySourceNone:
			none++
			if !e.Qto.IsEmpty() {
				t.Errorf("%s source=none but Qto not empty — no-phantom-0.0 violated", e.GlobalID)
			}
		default:
			t.Errorf("%s unknown quantity_source %q", e.GlobalID, e.QuantitySource)
		}
	}
	t.Logf("kb645 tiers: qto=%d geometry=%d none=%d (total=%d)", qto, geom, none, len(a.Result.Elements))
	if geom == 0 {
		t.Error("no elements got the geometry tier — the four stages are not chained")
	}
}
