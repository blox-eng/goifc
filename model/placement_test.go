package model

import (
	"fmt"
	"math"
	"strings"
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
	for _, tc := range []struct{ name, axis, refDir string }{
		{"ref parallel to axis", "(0.,0.,1.)", "(0.,0.,1.)"},
		{"ref antiparallel to axis", "(0.,0.,1.)", "(0.,0.,-1.)"},
		{"ref is the zero vector", "(0.,0.,1.)", "(0.,0.,0.)"},
		// A zero Axis reaches the same singularity through a different door:
		// normalize hands the zero vector straight back, and y = z X x
		// collapses along with it.
		{"axis is the zero vector", "(0.,0.,0.)", "(1.,0.,0.)"},
		{"both axis and ref are zero", "(0.,0.,0.)", "(0.,0.,0.)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
				"#1=IFCCARTESIANPOINT((1.,2.,3.));\n#2=IFCDIRECTION("+tc.axis+");\n"+
				"#3=IFCDIRECTION("+tc.refDir+");\n#4=IFCAXIS2PLACEMENT3D(#1,#2,#3);\n"+
				"#5=IFCLOCALPLACEMENT($,#4);\n#6=IFCWALL('g',$,'W',$,$,#5,$,$,$);\n"+
				"ENDSEC;\nEND-ISO-10303-21;\n")
			assertOrthonormal(t, LocalPlacement(f.ByType("IfcWall")[0]))
		})
	}
}

// IfcDirection carries no unit-length requirement, so a tiny-magnitude
// RefDirection is legal and its direction unambiguous. An ABSOLUTE degeneracy
// epsilon would mistake it for parallel-to-Axis and swap in the fallback basis,
// silently rotating the element's X axis by 90 degrees.
func TestAxis2Placement_TinyRefDirectionKeepsItsDirection(t *testing.T) {
	for _, refDir := range []string{"(1.,0.,0.)", "(1.E-11,0.,0.)", "(1.E-30,0.,0.)"} {
		t.Run(refDir, func(t *testing.T) {
			f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
				"#1=IFCCARTESIANPOINT((0.,0.,0.));\n#2=IFCDIRECTION((0.,0.,1.));\n"+
				"#3=IFCDIRECTION("+refDir+");\n#4=IFCAXIS2PLACEMENT3D(#1,#2,#3);\n"+
				"#5=IFCLOCALPLACEMENT($,#4);\n#6=IFCWALL('g',$,'W',$,$,#5,$,$,$);\n"+
				"ENDSEC;\nEND-ISO-10303-21;\n")
			m := LocalPlacement(f.ByType("IfcWall")[0])
			assertOrthonormal(t, m)
			if x := []float64{m[0], m[1], m[2]}; math.Abs(x[0]-1) > 1e-12 {
				t.Errorf("X axis = %v, want (1,0,0) — a legal small RefDirection was treated as degenerate", x)
			}
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

// A placement that refers to itself recursed forever, producing an
// unrecoverable `fatal error: stack overflow` — not a panic, so no consumer
// could defend against it. Reachable from goifc.Assemble on any untrusted
// file. https://github.com/blox-eng/goifc/issues/54
func TestLocalPlacement_SelfCycleTerminates(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
		"#1=IFCCARTESIANPOINT((1.,0.,0.));\n#2=IFCAXIS2PLACEMENT3D(#1,$,$);\n"+
		"#12=IFCLOCALPLACEMENT(#12,#2);\n"+
		"#20=IFCWALL('g',$,'W',$,$,#12,$,$,$);\n"+
		"ENDSEC;\nEND-ISO-10303-21;\n")
	m := LocalPlacement(f.ByType("IfcWall")[0])
	// The cyclic ANCESTOR contributes identity; the placement's own
	// RelativePlacement still applies, so the +1 translation lands once.
	x, y, z := m.Translation()
	if math.Abs(x-1) > 1e-9 || math.Abs(y) > 1e-9 || math.Abs(z) > 1e-9 {
		t.Fatalf("world origin = (%v,%v,%v), want (1,0,0)", x, y, z)
	}
}

// A cycle need not be at the root: a tail leading into a loop (A -> B -> C -> B)
// is what a malformed export actually produces. A parent-only check misses it.
func TestLocalPlacement_CycleBelowTheRootTerminates(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
		"#1=IFCCARTESIANPOINT((1.,0.,0.));\n#2=IFCAXIS2PLACEMENT3D(#1,$,$);\n"+
		"#10=IFCLOCALPLACEMENT(#11,#2);\n"+
		"#11=IFCLOCALPLACEMENT(#12,#2);\n"+
		"#12=IFCLOCALPLACEMENT(#11,#2);\n"+
		"#20=IFCWALL('g',$,'W',$,$,#10,$,$,$);\n"+
		"ENDSEC;\nEND-ISO-10303-21;\n")
	x, y, z := LocalPlacement(f.ByType("IfcWall")[0]).Translation()
	if math.IsNaN(x) || math.IsInf(x, 0) {
		t.Fatalf("world origin = (%v,%v,%v), want a finite transform", x, y, z)
	}
}

// PlacementRelTo pointing at something that is not a placement must yield
// identity, not a panic. The IsA guard already did this; pin it so the
// seen-set refactor cannot quietly drop it.
func TestLocalPlacement_NonPlacementParentIsIdentity(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n"+
		"#1=IFCCARTESIANPOINT((1.,0.,0.));\n#2=IFCAXIS2PLACEMENT3D(#1,$,$);\n"+
		"#3=IFCCARTESIANPOINT((7.,7.,7.));\n"+
		"#10=IFCLOCALPLACEMENT(#3,#2);\n"+
		"#20=IFCWALL('g',$,'W',$,$,#10,$,$,$);\n"+
		"ENDSEC;\nEND-ISO-10303-21;\n")
	x, y, z := LocalPlacement(f.ByType("IfcWall")[0]).Translation()
	if math.Abs(x-1) > 1e-9 || math.Abs(y) > 1e-9 || math.Abs(z) > 1e-9 {
		t.Fatalf("world origin = (%v,%v,%v), want (1,0,0) — the bogus parent must contribute identity", x, y, z)
	}
}

// A legal chain just under the cap must compose in full: the cap exists to
// stop an attack, and a cap that truncates real work is a wrong transform
// reported as a right one. Generated rather than committed as a fixture — a
// thousand-entity file would be unreadable and the depth is the only thing
// under test.
func TestLocalPlacement_DeepLegalChainComposesInFull(t *testing.T) {
	const n = maxPlacementDepth - 1
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	b.WriteString("#1=IFCCARTESIANPOINT((1.,0.,0.));\n#2=IFCAXIS2PLACEMENT3D(#1,$,$);\n")
	b.WriteString("#10=IFCLOCALPLACEMENT($,#2);\n")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, "#%d=IFCLOCALPLACEMENT(#%d,#2);\n", 10+i, 9+i)
	}
	fmt.Fprintf(&b, "#900000=IFCWALL('g',$,'W',$,$,#%d,$,$,$);\n", 10+n-1)
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")

	x, _, _ := LocalPlacement(parseString(t, b.String()).ByType("IfcWall")[0]).Translation()
	if math.Abs(x-float64(n)) > 1e-6 {
		t.Fatalf("x = %v, want %v — each of the %d placements translates +1, so the cap must not truncate", x, float64(n), n)
	}
}

// Past the cap the walk stops instead of running until the stack dies. The
// exact surviving translation is an implementation detail; that it terminates
// and stays bounded is the contract.
func TestLocalPlacement_OverDeepChainIsBounded(t *testing.T) {
	const n = maxPlacementDepth + 50
	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n")
	b.WriteString("#1=IFCCARTESIANPOINT((1.,0.,0.));\n#2=IFCAXIS2PLACEMENT3D(#1,$,$);\n")
	b.WriteString("#10=IFCLOCALPLACEMENT($,#2);\n")
	for i := 1; i < n; i++ {
		fmt.Fprintf(&b, "#%d=IFCLOCALPLACEMENT(#%d,#2);\n", 10+i, 9+i)
	}
	fmt.Fprintf(&b, "#900000=IFCWALL('g',$,'W',$,$,#%d,$,$,$);\n", 10+n-1)
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")

	x, _, _ := LocalPlacement(parseString(t, b.String()).ByType("IfcWall")[0]).Translation()
	if x > float64(maxPlacementDepth) {
		t.Fatalf("x = %v, want no more than %d — the cap did not bound the walk", x, maxPlacementDepth)
	}
}
