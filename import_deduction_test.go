package ifc

import (
	"math"
	"testing"
)

// TestBuildImport_DeductionAndGrossReachNetArea pins the reachability of the
// two halves NetArea is the difference of. NetArea alone cannot be aggregated:
// a host with no IfcRelVoidsElement is absent from the reconciliation, so
// summing nets over a facade silently drops every solid wall. Netting a total
// means subtracting the DEDUCTION from the gross being totalled, which is only
// possible if both cross the import contract.
//
// The identity is the assertion that matters. ProjectedGross - OpeningDeduction
// must equal NetArea exactly, because all three are read from one
// reconciliation on one plane; if they ever disagree, the pair cannot be used
// to net anything and a consumer would be quietly inventing area.
func TestBuildImport_DeductionAndGrossReachNetArea(t *testing.T) {
	f := parseFixture(t, "geometry/testdata/synthetic/netarea_rect_window.ifc")
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}

	var withNet int
	for _, n := range m.Nodes {
		if (n.NetArea == nil) != (n.OpeningDeduction == nil) {
			t.Errorf("node %q: NetArea and OpeningDeduction disagree about presence "+
				"(net=%v deduction=%v)", n.Name, n.NetArea, n.OpeningDeduction)
		}
		if (n.NetArea == nil) != (n.ProjectedGross == nil) {
			t.Errorf("node %q: NetArea and ProjectedGross disagree about presence "+
				"(net=%v gross=%v)", n.Name, n.NetArea, n.ProjectedGross)
		}
		if n.NetArea == nil {
			continue
		}
		// Report the missing field and move on rather than dereferencing it: a
		// test that panics on the very regression it exists to catch reports a
		// stack trace instead of a diagnosis, and takes the remaining nodes'
		// assertions down with it.
		if n.OpeningDeduction == nil || n.ProjectedGross == nil {
			continue
		}
		withNet++

		if got := math.Abs(*n.ProjectedGross - *n.OpeningDeduction - *n.NetArea); got > 1e-9 {
			t.Errorf("node %q: gross(%v) - deduction(%v) != net(%v), off by %v",
				n.Name, *n.ProjectedGross, *n.OpeningDeduction, *n.NetArea, got)
		}
		// The fixture is a 4x3 wall with a 1x1 void: 12 gross, 1 deducted.
		// Pins units and transport; the union semantics live in geometry's
		// own tests.
		if got := *n.ProjectedGross; got < 11.999 || got > 12.001 {
			t.Errorf("node %q: ProjectedGross = %v, want 12 (4x3 wall)", n.Name, got)
		}
		if got := *n.OpeningDeduction; got < 0.999 || got > 1.001 {
			t.Errorf("node %q: OpeningDeduction = %v, want 1 (1x1 void)", n.Name, got)
		}
	}
	if withNet == 0 {
		t.Fatal("fixture produced no node with a trusted NetArea; the assertions above never ran")
	}
}

// TestBuildImport_UntrustedHostPublishesNoDeduction confirms the untrusted path
// stays silent in the new fields too. NetArea.OpeningDeduction is documented as
// 0 when untrusted, and a published 0 is the failure mode worth guarding: it
// satisfies a presence check and reads as "this wall has no openings", turning
// a refused measurement into a confident claim that the whole gross is net.
func TestBuildImport_UntrustedHostPublishesNoDeduction(t *testing.T) {
	f := parseFixture(t, "geometry/testdata/synthetic/netarea_mostly_glass.ifc")
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}
	var sawUntrusted int
	for _, n := range m.Nodes {
		if n.OpeningDeduction != nil || n.ProjectedGross != nil {
			t.Errorf("node %q: untrusted host published deduction=%v gross=%v, want both absent",
				n.Name, n.OpeningDeduction, n.ProjectedGross)
		}
		if n.HasOpenings {
			sawUntrusted++
			// The discriminator that makes the absence above readable. Without
			// it a consumer sees the same nils a solid wall produces and nets
			// this fully-glazed host at its whole gross.
			if n.NetArea != nil {
				t.Errorf("node %q: HasOpenings with a trusted net; fixture is meant to be refused", n.Name)
			}
		}
	}
	if sawUntrusted == 0 {
		t.Fatal("fixture produced no host with openings; the untrusted path was never exercised")
	}
}

// TestBuildImport_UnvoidedHostIsDistinguishableFromUntrusted is the whole point
// of HasOpenings. Both a solid wall and a refused reconciliation publish nil in
// all three area fields, and they are OPPOSITE answers: the solid wall's net
// equals its gross, the refused one's net is unknown. A consumer totalling a
// facade has to branch on something, and this is the only thing to branch on.
func TestBuildImport_UnvoidedHostIsDistinguishableFromUntrusted(t *testing.T) {
	// A wall WITH a trusted window: HasOpenings, and every area field present.
	var voided int
	for _, n := range buildImport(t, "geometry/testdata/synthetic/netarea_rect_window.ifc").Nodes {
		if !n.HasOpenings {
			continue
		}
		voided++
		if n.NetArea == nil {
			t.Errorf("node %q: HasOpenings but no net; this fixture's window is trusted", n.Name)
		}
	}
	if voided == 0 {
		t.Error("rect_window produced no host with openings; the true branch went untested")
	}

	// A wall with NO openings: not in the reconciliation at all, so every area
	// field is nil — the same nils the untrusted fixture produces above. Only
	// HasOpenings separates "its net IS its gross" from "its net is unknown".
	var unvoided int
	for _, n := range buildImport(t, "model/testdata/synthetic/wall_no_openings.ifc").Nodes {
		if n.HasOpenings {
			t.Errorf("node %q: HasOpenings on a fixture with no voids", n.Name)
			continue
		}
		unvoided++
		if n.NetArea != nil || n.OpeningDeduction != nil || n.ProjectedGross != nil {
			t.Errorf("node %q: !HasOpenings yet published net=%v deduction=%v gross=%v",
				n.Name, n.NetArea, n.OpeningDeduction, n.ProjectedGross)
		}
	}
	if unvoided == 0 {
		t.Error("wall_no_openings produced no nodes; the false branch went untested")
	}
}
