package ifc

import (
	"testing"
)

// TestBuildImport_OpeningPerimeterAccompaniesNetArea pins the reachability of
// the perimeter through the import contract. NetArea.OpeningPerimeter existing
// on the geometry type is not enough: consumers read ImportNode, so a field the
// import layer never copies is a measurement nobody can obtain.
//
// It also pins the pairing. Both fields are published from one trusted
// reconciliation, so a node must never carry a confident perimeter beside an
// absent net — a consumer seeing a perimeter is entitled to assume the net it
// belongs to is there too.
func TestBuildImport_OpeningPerimeterAccompaniesNetArea(t *testing.T) {
	f := parseFixture(t, "geometry/testdata/synthetic/netarea_rect_window.ifc")
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}

	var withNet int
	for _, n := range m.Nodes {
		if (n.NetArea == nil) != (n.OpeningPerimeter == nil) {
			t.Errorf("node %q: NetArea and OpeningPerimeter disagree about presence "+
				"(net=%v perimeter=%v)", n.Name, n.NetArea, n.OpeningPerimeter)
		}
		if n.NetArea == nil {
			continue
		}
		withNet++
		// The fixture's void is a 1x1 window in a 4x3 wall: 4 m of outline.
		// A Σ over the void's own edges would agree here (one void, no seam),
		// so this pins transport and units, not the union semantics — those are
		// pinned in geometry's TestUnionPerimeter.
		if got := *n.OpeningPerimeter; got < 3.999 || got > 4.001 {
			t.Errorf("node %q: OpeningPerimeter = %v, want 4 (1x1 void)", n.Name, got)
		}
	}
	if withNet == 0 {
		t.Fatal("fixture produced no node with a trusted NetArea; the assertion above never ran")
	}
}

// TestBuildImport_UntrustedHostPublishesNeither confirms the untrusted path
// stays silent in both fields rather than leaking a perimeter for a host whose
// deduction was refused. The fixture's openings claim >=95% of gross, which is
// the over-subtraction gate.
func TestBuildImport_UntrustedHostPublishesNeither(t *testing.T) {
	f := parseFixture(t, "geometry/testdata/synthetic/netarea_mostly_glass.ifc")
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}
	for _, n := range m.Nodes {
		if n.NetArea != nil || n.OpeningPerimeter != nil {
			t.Errorf("node %q: untrusted host published net=%v perimeter=%v, want both absent",
				n.Name, n.NetArea, n.OpeningPerimeter)
		}
	}
}
