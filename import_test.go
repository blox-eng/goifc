package ifc

import (
	"os"
	"testing"

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
