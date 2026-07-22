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

func TestQtoQuantitiesAbsent(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n#1=IFCWALL('g',$,'W',$,$,$,$,$,$);\nENDSEC;\nEND-ISO-10303-21;\n")
	if _, ok := QtoQuantities(f, f.ByType("IfcWall")[0], 1.0); ok {
		t.Fatal("expected no Qto")
	}
}
