package ifc

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/blox-eng/common/ifc/geometry"
	"github.com/blox-eng/common/ifc/step"
)

func parseFixture(t *testing.T, path string) *step.File {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	f, err := step.ParseBytes(src)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return f
}

func nodeByName(m *ImportModel, name string) (ImportNode, int, bool) {
	for i, n := range m.Nodes {
		if n.Name == name {
			return n, i, true
		}
	}
	return ImportNode{}, -1, false
}

// parentName returns the Name of node i's parent, or "" if root.
func parentName(m *ImportModel, i int) string {
	if m.Nodes[i].ParentIndex == nil {
		return ""
	}
	return m.Nodes[*m.Nodes[i].ParentIndex].Name
}

// TestBuildImport_Tree exercises the spatial+physical import contract:
// forward parent-map (C1: no storey self-parent), topo/parents-first order
// (C2), spatial Qto (C3), and the fills-element door → null parent semantics
// of import_emit._parent_map.
func TestBuildImport_Tree(t *testing.T) {
	f := parseFixture(t, "model/testdata/synthetic/spatial_full.ifc")
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}

	// Expected node set: site, building, storey, space, wall, door, beam, assembly.
	// (IfcProject is NOT emitted.)
	if len(m.Nodes) != 8 {
		names := make([]string, len(m.Nodes))
		for i, n := range m.Nodes {
			names[i] = n.Name
		}
		t.Fatalf("want 8 nodes, got %d: %v", len(m.Nodes), names)
	}

	// C2: parents-first — every non-root parent index strictly precedes the child.
	// C1: implies no self-parent.
	for i, n := range m.Nodes {
		if n.ParentIndex != nil && *n.ParentIndex >= i {
			t.Errorf("node %d (%s) parent_index %d not before it (topo/self-parent violation)", i, n.Name, *n.ParentIndex)
		}
	}

	// Spatial tree by parent Name (checks parent_index resolves to the right node).
	check := func(child, wantParent string) {
		_, i, ok := nodeByName(m, child)
		if !ok {
			t.Fatalf("node %q missing", child)
		}
		if got := parentName(m, i); got != wantParent {
			t.Errorf("%s parent = %q want %q", child, got, wantParent)
		}
	}
	check("Site A", "")          // root (its parent IfcProject is not emitted)
	check("Building A", "Site A") // aggregate chain
	check("Level 1", "Building A")
	check("Room 101", "Level 1") // space aggregated into storey
	check("W-1", "Level 1")      // wall contained in storey
	check("D-1", "")             // door: fills-element only → parent_index null
	check("B-1", "ASM-1")        // beam aggregated into assembly (physical-under-physical)
	check("ASM-1", "Level 1")    // assembly contained in storey

	// C3: spatial node carries authored Qto (IfcSpace GrossFloorArea = 25.0).
	space, _, _ := nodeByName(m, "Room 101")
	if space.Qto.Area == nil || *space.Qto.Area != 25.0 {
		t.Errorf("Room 101 Qto.Area = %v want 25.0 (C3 spatial Qto)", space.Qto.Area)
	}

	if m.Scene == nil {
		t.Error("Scene is nil (needed for GLB)")
	}
}

// TestBuildImport_KB645_Invariants validates C1/C2 on the real corpus: every
// node is parents-first (no forward parent_index, no self-parent) and the
// spatial containers are present. Self-skips when the gitignored kb645 is absent.
func TestBuildImport_KB645_Invariants(t *testing.T) {
	const path = "step/testdata/real/kb645.ifc"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no kb645.ifc corpus; skipping")
	}
	f := parseFixture(t, path)
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}

	spatial := 0
	for i, n := range m.Nodes {
		if n.ParentIndex != nil {
			if *n.ParentIndex == i {
				t.Fatalf("node %d (%s %s) is its own parent (C1 self-parent)", i, n.IFCClass, n.GlobalID)
			}
			if *n.ParentIndex > i {
				t.Fatalf("node %d (%s) parent_index %d is forward — not parents-first (C2)", i, n.Name, *n.ParentIndex)
			}
		}
		switch n.IFCClass {
		case "IFCSITE", "IFCBUILDING", "IFCBUILDINGSTOREY", "IFCSPACE":
			spatial++
		}
	}
	if spatial == 0 {
		t.Error("no spatial container nodes emitted for kb645 (expected site/building/storeys)")
	}
	// physical set unchanged from the parity baseline (1922) + at least one spatial node.
	if len(m.Nodes) <= 1922 {
		t.Errorf("kb645 import nodes = %d, want > 1922 (1922 physical + spatial)", len(m.Nodes))
	}
	t.Logf("kb645 import nodes=%d spatial=%d", len(m.Nodes), spatial)
}

// TestBuildImport_MultiStoreyPlans is the capstone: a REAL parsed multi-storey
// IFC file (STEP parse -> BuildImport -> StoreyPlans), not synthetic hand-set
// nodes. It exercises the full producer, including the case-insensitive storey
// detection that a real parse (uppercased IFCBUILDINGSTOREY) requires.
func TestBuildImport_MultiStoreyPlans(t *testing.T) {
	f := parseFixture(t, "testdata/two_storey_spanning.ifc")
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}
	if len(m.StoreyPlans) != 2 {
		t.Fatalf("want 2 storey plans from a 2-storey building, got %d", len(m.StoreyPlans))
	}
	// Ordered by floorZ: Ground (0) then Level 1 (2.8).
	ground, level1 := m.StoreyPlans[0], m.StoreyPlans[1]

	role := func(sp StoreyPlan, gidSubstr string) (geometry.LoopRole, bool) {
		for _, e := range sp.Entities {
			// match by IFC class since GlobalIDs are opaque; one element per class here
			if strings.EqualFold(e.IFCClass, gidSubstr) && len(e.Loops) > 0 {
				return e.Loops[0].Role, true
			}
		}
		return "", false
	}

	// Ground floor: column CUT, wall CUT, no slab.
	if r, ok := role(ground, "IfcColumn"); !ok || r != geometry.LoopCut {
		t.Fatalf("ground: column should be cut, got %q ok=%v", r, ok)
	}
	if r, ok := role(ground, "IfcWall"); !ok || r != geometry.LoopCut {
		t.Fatalf("ground: wall should be cut, got %q ok=%v", r, ok)
	}
	if _, ok := role(ground, "IfcSlab"); ok {
		t.Fatalf("ground plan must not include the Level-1 slab")
	}

	// Level 1: the SPANNING COLUMN appears (CUT); wall absent; slab BELOW.
	if r, ok := role(level1, "IfcColumn"); !ok || r != geometry.LoopCut {
		t.Fatalf("level1: spanning column should be cut on the upper floor, got %q ok=%v", r, ok)
	}
	if _, ok := role(level1, "IfcWall"); ok {
		t.Fatalf("level1 plan must not include the ground-floor wall")
	}
	if r, ok := role(level1, "IfcSlab"); !ok || r != geometry.LoopBelow {
		t.Fatalf("level1: slab should be below-context, got %q ok=%v", r, ok)
	}
}

func TestBuildImport_MultiStoreyPlansDeterministic(t *testing.T) {
	f := parseFixture(t, "testdata/two_storey_spanning.ifc")
	m1, _ := BuildImport(f)
	f2 := parseFixture(t, "testdata/two_storey_spanning.ifc")
	m2, _ := BuildImport(f2)
	if !reflect.DeepEqual(m1.StoreyPlans, m2.StoreyPlans) {
		t.Fatal("StoreyPlans not byte-identical across re-import (determinism / #1344 drift protection)")
	}
}
