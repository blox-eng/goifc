package ifc

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/step"
)

// nodeByGID returns the built node with the given GlobalID.
func nodeByGID(t *testing.T, m *ImportModel, gid string) ImportNode {
	t.Helper()
	for _, n := range m.Nodes {
		if n.GlobalID == gid {
			return n
		}
	}
	t.Fatalf("no node with global_id %q", gid)
	return ImportNode{}
}

func buildImport(t *testing.T, path string) *ImportModel {
	t.Helper()
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport %s: %v", path, err)
	}
	return m
}

// TestBuildImport_MaterialAndIsExternal: the wall in full_wall.ifc carries
// Pset_WallCommon.IsExternal=.T. and an associated IfcMaterial. BuildImport must
// surface both onto the node (they were extracted onto model.Element, then
// dropped before this PR).
func TestBuildImport_MaterialAndIsExternal(t *testing.T) {
	m := buildImport(t, "model/testdata/synthetic/full_wall.ifc")
	n := nodeByGID(t, m, "0GUIDwall0000000000020")
	if n.Material != "Concrete C25/30" {
		t.Errorf("Material = %q, want %q", n.Material, "Concrete C25/30")
	}
	if n.IsExternal == nil || *n.IsExternal != true {
		t.Errorf("IsExternal = %v, want *true", n.IsExternal)
	}
}

// TestBuildImport_NetAreaTrusted: a wall with one rectangular window
// (gross 12, opening 1) yields a trusted Net of 11 on its host node. Every
// non-voided node keeps NetArea nil (engine returns hosts-with-voids only).
func TestBuildImport_NetAreaTrusted(t *testing.T) {
	m := buildImport(t, "geometry/testdata/synthetic/netarea_rect_window.ifc")
	var withNet int
	for _, n := range m.Nodes {
		if n.NetArea == nil {
			continue
		}
		withNet++
		if math.Abs(*n.NetArea-11.0) > 1e-6 {
			t.Errorf("node %s NetArea = %v, want 11.0", n.GlobalID, *n.NetArea)
		}
	}
	if withNet != 1 {
		t.Fatalf("nodes with NetArea = %d, want exactly 1", withNet)
	}
}

// TestBuildImport_NetAreaNilWithoutVoids: a wall with no openings has no net
// reconciliation — NetArea stays nil on every node.
func TestBuildImport_NetAreaNilWithoutVoids(t *testing.T) {
	m := buildImport(t, "model/testdata/synthetic/wall_no_openings.ifc")
	for _, n := range m.Nodes {
		if n.NetArea != nil {
			t.Errorf("node %s NetArea = %v, want nil (no voids)", n.GlobalID, *n.NetArea)
		}
	}
}
