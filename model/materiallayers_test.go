package model

import (
	"os"
	"testing"

	"github.com/blox-eng/common/ifc/step"
)

func loadLayerAttrsFixture(t *testing.T) *step.File {
	t.Helper()
	b, err := os.ReadFile("testdata/synthetic/wall_layerset_attrs.ifc")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	f, err := step.ParseBytes(b)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return f
}

// The type carries the bare IfcMaterialLayerSet, so a type-level read yields the
// layers with no usage wrapper and therefore no direction.
func TestMaterialLayersFromType(t *testing.T) {
	f := loadLayerAttrsFixture(t)
	typ := f.ByType("IfcWallType")[0]

	got := MaterialLayers(f, typ, UnitScale(f))

	if len(got.Layers) != 3 {
		t.Fatalf("len(Layers) = %d, want 3", len(got.Layers))
	}
	// EXPRESS LIST order, not graph-DFS order.
	wantNames := []string{"Reinforced concrete", "Air", "EPS"}
	for i, want := range wantNames {
		if got.Layers[i].MaterialName != want {
			t.Errorf("Layers[%d].MaterialName = %q, want %q", i, got.Layers[i].MaterialName, want)
		}
	}
	if got.Direction != "" {
		t.Errorf("Direction = %q, want \"\" (a bare layer set carries no direction)", got.Direction)
	}
}

func TestMaterialLayersThicknessInMillimetres(t *testing.T) {
	f := loadLayerAttrsFixture(t)
	typ := f.ByType("IfcWallType")[0]

	got := MaterialLayers(f, typ, UnitScale(f))

	want := []float64{250, 40, 100}
	for i, w := range want {
		if got.Layers[i].ThicknessMm == nil {
			t.Fatalf("Layers[%d].ThicknessMm = nil, want %v", i, w)
		}
		if *got.Layers[i].ThicknessMm != w {
			t.Errorf("Layers[%d].ThicknessMm = %v, want %v", i, *got.Layers[i].ThicknessMm, w)
		}
	}
}

// TRUE, FALSE and UNKNOWN are three distinct outcomes. Collapsing .U. into false
// would make a cavity-without-exchange indistinguishable from a solid layer.
func TestMaterialLayersIsVentilatedIsThreeValued(t *testing.T) {
	f := loadLayerAttrsFixture(t)
	typ := f.ByType("IfcWallType")[0]

	got := MaterialLayers(f, typ, UnitScale(f))

	if got.Layers[0].IsVentilated == nil || *got.Layers[0].IsVentilated != false {
		t.Errorf("Layers[0].IsVentilated = %v, want pointer to false (.F.)", got.Layers[0].IsVentilated)
	}
	if got.Layers[1].IsVentilated != nil {
		t.Errorf("Layers[1].IsVentilated = %v, want nil (.U. is UNKNOWN, not false)", *got.Layers[1].IsVentilated)
	}
	if got.Layers[2].IsVentilated == nil || *got.Layers[2].IsVentilated != true {
		t.Errorf("Layers[2].IsVentilated = %v, want pointer to true (.T.)", got.Layers[2].IsVentilated)
	}
}

func TestMaterialLayersCategoryVerbatimAndAbsent(t *testing.T) {
	f := loadLayerAttrsFixture(t)
	typ := f.ByType("IfcWallType")[0]

	got := MaterialLayers(f, typ, UnitScale(f))

	if got.Layers[0].Category != "LoadBearing" {
		t.Errorf("Layers[0].Category = %q, want %q", got.Layers[0].Category, "LoadBearing")
	}
	// $ means absent. It must stay empty — never guessed from the material name.
	if got.Layers[1].Category != "" {
		t.Errorf("Layers[1].Category = %q, want \"\" (absent Category is never inferred)", got.Layers[1].Category)
	}
}

// The occurrence carries an IfcMaterialLayerSetUsage, which is where the axis and
// sense live. Reading through it must reach the same set, in the same order.
func TestMaterialLayersFromOccurrenceCarriesDirection(t *testing.T) {
	f := loadLayerAttrsFixture(t)
	wall := f.ByType("IfcWall")[0]

	got := MaterialLayers(f, wall, UnitScale(f))

	if len(got.Layers) != 3 {
		t.Fatalf("len(Layers) = %d, want 3", len(got.Layers))
	}
	if got.Layers[0].MaterialName != "Reinforced concrete" {
		t.Errorf("Layers[0].MaterialName = %q, want %q", got.Layers[0].MaterialName, "Reinforced concrete")
	}
	if got.Direction != "AXIS2" {
		t.Errorf("Direction = %q, want %q", got.Direction, "AXIS2")
	}
	if got.Sense != "POSITIVE" {
		t.Errorf("Sense = %q, want %q", got.Sense, "POSITIVE")
	}
}

// A millimetre file must produce the same millimetre thicknesses as a metre file.
func TestMaterialLayersScalesFromMillimetreFile(t *testing.T) {
	src := `ISO-10303-21;
HEADER;
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCSIUNIT(*,.LENGTHUNIT.,.MILLI.,.METRE.);
#2=IFCUNITASSIGNMENT((#1));
#3=IFCPROJECT('0GUIDproj0000000000011',$,'P',$,$,$,$,$,#2);
#10=IFCMATERIAL('Brick');
#20=IFCMATERIALLAYER(#10,250.,$,$,$,$,$);
#30=IFCMATERIALLAYERSET((#20),$,$);
#40=IFCWALLTYPE('0GUIDwtyp0000000000011',$,'W',$,$,$,$,$,$,.STANDARD.);
#41=IFCRELASSOCIATESMATERIAL('0GUIDrelm0000000000011',$,$,$,(#40),#30);
ENDSEC;
END-ISO-10303-21;
`
	f, err := step.ParseBytes([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := MaterialLayers(f, f.ByType("IfcWallType")[0], UnitScale(f))
	if len(got.Layers) != 1 {
		t.Fatalf("len(Layers) = %d, want 1", len(got.Layers))
	}
	if got.Layers[0].ThicknessMm == nil || *got.Layers[0].ThicknessMm != 250 {
		t.Errorf("ThicknessMm = %v, want 250", got.Layers[0].ThicknessMm)
	}
	// An absent LayerThickness/IsVentilated must not become a phantom zero/false.
	if got.Layers[0].Category != "" {
		t.Errorf("Category = %q, want \"\"", got.Layers[0].Category)
	}
}

// An instance with no material association at all yields an empty set, not a panic.
func TestMaterialLayersNoAssociation(t *testing.T) {
	f := loadLayerAttrsFixture(t)
	proj := f.ByType("IfcProject")[0]

	got := MaterialLayers(f, proj, UnitScale(f))

	if len(got.Layers) != 0 {
		t.Errorf("len(Layers) = %d, want 0", len(got.Layers))
	}
}

// The pre-existing flattening reader must be untouched by this task.
func TestMaterialLeavesRegression(t *testing.T) {
	f := loadLayerAttrsFixture(t)
	wall := f.ByType("IfcWall")[0]

	got := Materials(f, wall)

	if len(got) != 3 {
		t.Fatalf("len(Materials) = %d, want 3", len(got))
	}
	if strVal(got[0], attrMaterialName) != "Reinforced concrete" {
		t.Errorf("Materials[0] = %q, want %q", strVal(got[0], attrMaterialName), "Reinforced concrete")
	}
}
