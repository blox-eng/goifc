package model

import (
	"math"
	"testing"
)

func TestQtoQuantitiesTier1(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_qto.ifc"))
	w := f.ByType("IfcWall")[0]
	q, ok := QtoQuantities(f, w, 1.0)
	if !ok {
		t.Fatal("expected Qto present")
	}
	if q.Area == nil || math.Abs(*q.Area-12.5) > 1e-9 {
		t.Fatalf("area = %v want 12.5", q.Area)
	}
	if q.Volume == nil || math.Abs(*q.Volume-3.0) > 1e-9 {
		t.Fatalf("volume = %v want 3.0", q.Volume)
	}
	if q.Length == nil || math.Abs(*q.Length-5.0) > 1e-9 {
		t.Fatalf("length = %v want 5.0", q.Length)
	}
}

// TestQtoQuantitiesTier1_UnitScaling is an ABSOLUTE golden (child 5, #2211): it
// pins the length/area/volume unit exponents against hand-verified truth,
// independent of the Python oracle. wall_qto.ifc authors NetSideArea=12.5,
// NetVolume=3.0, Length=5.0 in file units; a millimeter file (scale=0.001) must
// scale length by scale, area by scale^2, volume by scale^3. A wrong exponent
// silently 1e4x's an area or 1e9x's a volume on the Python->Go cutover — this is
// the guard that fails loudly when it does. TestQtoQuantitiesTier1 only exercises
// scale=1.0, so the scaling path itself was previously unasserted.
func TestQtoQuantitiesTier1_UnitScaling(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_qto.ifc"))
	w := f.ByType("IfcWall")[0]
	q, ok := QtoQuantities(f, w, 0.001) // millimeter file: 1 mm = 0.001 m
	if !ok {
		t.Fatal("expected Qto present")
	}
	cases := []struct {
		name string
		got  *float64
		want float64
	}{
		{"Length (scale^1)", q.Length, 5.0e-3},
		{"Width (scale^1)", q.Width, 0.3e-3},
		{"Height (scale^1)", q.Height, 2.5e-3},
		{"Area (scale^2)", q.Area, 12.5e-6},
		{"Volume (scale^3)", q.Volume, 3.0e-9},
	}
	for _, c := range cases {
		if c.got == nil {
			t.Errorf("%s = nil, want %g", c.name, c.want)
			continue
		}
		if math.Abs(*c.got-c.want) > 1e-15 {
			t.Errorf("%s = %g, want %g (unit exponent wrong)", c.name, *c.got, c.want)
		}
	}
}

func TestQtoQuantitiesAbsent(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n#1=IFCWALL('g',$,'W',$,$,$,$,$,$);\nENDSEC;\nEND-ISO-10303-21;\n")
	if _, ok := QtoQuantities(f, f.ByType("IfcWall")[0], 1.0); ok {
		t.Fatal("expected no Qto")
	}
}
