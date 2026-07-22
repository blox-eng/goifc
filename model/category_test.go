package model

import (
	"testing"

	"github.com/blox-eng/common/ifc/step"
)

// firstInstance returns the single instance of a one-instance fixture file.
func firstInstance(f *step.File) *step.Instance {
	for inst := range f.All() {
		return inst
	}
	return nil
}

func TestCategory(t *testing.T) {
	cases := map[string]string{
		"IFCWALLSTANDARDCASE": "WALL",
		"IFCWALL":             "WALL",
		"IFCCOLUMN":           "COLUMN",
		"IFCBEAM":             "BEAM",
		"IFCROOF":             "ROOF",
		"IFCSLAB":             "FLOOR",
		"IFCSLABSTANDARDCASE": "FLOOR",
	}
	for typ, want := range cases {
		f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n#1="+typ+"('g',$,'n',$,$,$,$,$,$);\nENDSEC;\nEND-ISO-10303-21;\n")
		if got := Category(f, firstInstance(f)); got != want {
			t.Errorf("%s → %s want %s", typ, got, want)
		}
	}
}

func TestCategorySlabRoof(t *testing.T) {
	data := mustRead(t, "testdata/synthetic/slab_roof.ifc")
	file := parseString(t, data)
	inst := firstInstance(file)
	if got := Category(file, inst); got != "ROOF" {
		t.Fatalf("IfcSlab PredefinedType .ROOF. → %s want ROOF", got)
	}
}

func TestPredefinedTypeDoorIFC4(t *testing.T) {
	data := mustRead(t, "testdata/synthetic/door_predefined.ifc")
	f := parseString(t, data)
	door := firstInstance(f)
	if got := predefinedType(f, door); got != "DOOR" {
		t.Fatalf("IfcDoor PredefinedType (idx10) → %q want DOOR", got)
	}
}

func TestIsStructural(t *testing.T) {
	if !isStructural("IFCWALL") || !isStructural("IFCSLAB") {
		t.Fatal("walls/slabs should be structural")
	}
	if !isStructural("IfcSlabStandardCase") {
		t.Fatal("IfcSlabStandardCase should be structural (StandardCase leaf of IfcSlab)")
	}
	if isStructural("IFCDOOR") || isStructural("IFCWINDOW") {
		t.Fatal("doors/windows should not be structural")
	}
}
