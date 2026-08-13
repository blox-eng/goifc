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

// A RefDirection parallel to Axis leaves nothing for Gram-Schmidt to project.
// Without a fallback the first two basis columns collapse to zero and the
// placement is singular — rotating any direction through it returns a
// zero-length vector, which NaNs the moment a caller normalizes it.
func TestAxis2Placement_DegenerateRefDirectionStaysOrthonormal(t *testing.T) {
	for _, tc := range []struct{ name, refDir string }{
		{"parallel to axis", "(0.,0.,1.)"},
		{"antiparallel to axis", "(0.,0.,-1.)"},
		{"zero vector", "(0.,0.,0.)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
				"#1=IFCCARTESIANPOINT((1.,2.,3.));\n#2=IFCDIRECTION((0.,0.,1.));\n"+
				"#3=IFCDIRECTION("+tc.refDir+");\n#4=IFCAXIS2PLACEMENT3D(#1,#2,#3);\n"+
				"#5=IFCLOCALPLACEMENT($,#4);\n#6=IFCWALL('g',$,'W',$,$,#5,$,$,$);\n"+
				"ENDSEC;\nEND-ISO-10303-21;\n")
			assertOrthonormal(t, LocalPlacement(f.ByType("IfcWall")[0]))
		})
	}
}

// assertOrthonormal checks m's 3x3 basis is unit-length, mutually
// perpendicular and right-handed (determinant +1) — the property that makes a
// rotation-only transform the correct one for a direction, so no consumer
// needs the inverse-transpose.
func assertOrthonormal(t *testing.T, m Mat4) {
	t.Helper()
	cols := [3][]float64{{m[0], m[1], m[2]}, {m[4], m[5], m[6]}, {m[8], m[9], m[10]}}
	for i, c := range cols {
		if n := math.Sqrt(dot(c, c)); math.Abs(n-1) > 1e-12 {
			t.Errorf("column %d length = %v, want 1 (basis %v)", i, n, cols)
		}
		for j := i + 1; j < 3; j++ {
			if d := dot(c, cols[j]); math.Abs(d) > 1e-12 {
				t.Errorf("columns %d,%d dot = %v, want 0 (basis %v)", i, j, d, cols)
			}
		}
	}
	if det := dot(cross(cols[0], cols[1]), cols[2]); math.Abs(det-1) > 1e-12 {
		t.Errorf("determinant = %v, want +1 (mirrored or scaled basis)", det)
	}
}

func TestLocalPlacementMissingIsIdentity(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n#1=IFCWALL('g',$,'W',$,$,$,$,$,$);\nENDSEC;\nEND-ISO-10303-21;\n")
	m := LocalPlacement(f.ByType("IfcWall")[0])
	if m != Identity() {
		t.Fatalf("no placement should yield identity, got %v", m)
	}
}
