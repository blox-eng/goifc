package model

import (
	"math"
	"testing"
)

func TestLocalPlacementChain(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/placement_chain.ifc"))
	walls := f.ByType("IfcWall")
	if len(walls) != 1 {
		t.Fatalf("want 1 wall, got %d", len(walls))
	}
	m := LocalPlacement(walls[0])
	x, y, z := m.Translation()
	if math.Abs(x-10) > 1e-9 || math.Abs(y-0) > 1e-9 || math.Abs(z-3) > 1e-9 {
		t.Fatalf("world origin = (%v,%v,%v), want (10,0,3)", x, y, z)
	}
}

func TestLocalPlacementMissingIsIdentity(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n#1=IFCWALL('g',$,'W',$,$,$,$,$,$);\nENDSEC;\nEND-ISO-10303-21;\n")
	m := LocalPlacement(f.ByType("IfcWall")[0])
	if m != Identity() {
		t.Fatalf("no placement should yield identity, got %v", m)
	}
}
