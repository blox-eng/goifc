package ifc

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/geometry"
	"github.com/blox-eng/goifc/step"
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
	check("Site A", "")           // root (its parent IfcProject is not emitted)
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
	if r, ok := role(level1, "IfcSlab"); !ok || r != geometry.LoopSilhouette {
		t.Fatalf("level1: slab should be below-context, got %q ok=%v", r, ok)
	}
}

func TestBuildImport_MultiStoreyPlansDeterministic(t *testing.T) {
	f := parseFixture(t, "testdata/two_storey_spanning.ifc")
	m1, _ := BuildImport(f)
	f2 := parseFixture(t, "testdata/two_storey_spanning.ifc")
	m2, _ := BuildImport(f2)
	if !reflect.DeepEqual(m1.StoreyPlans, m2.StoreyPlans) {
		t.Fatal("StoreyPlans not byte-identical across re-import (determinism drift protection)")
	}
}

// BuildImport must surface the type's build-up once per distinct IfcTypeObject,
// keyed by the type GlobalId ResolveObjectTypes already keys on. Per-occurrence
// duplication would multiply this against a 2 MB payload cap on real files.
func TestBuildImportTypeLayers(t *testing.T) {
	b, err := os.ReadFile("model/testdata/synthetic/wall_layerset_attrs.ifc")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	f, err := step.ParseBytes(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}

	if len(m.TypeLayers) != 1 {
		t.Fatalf("len(TypeLayers) = %d, want 1", len(m.TypeLayers))
	}
	set, ok := m.TypeLayers["0GUIDwtyp0000000000010"]
	if !ok {
		t.Fatalf("TypeLayers missing the wall type GlobalId; got keys %v", keysOf(m.TypeLayers))
	}
	if len(set.Layers) != 3 {
		t.Fatalf("len(Layers) = %d, want 3", len(set.Layers))
	}
	if set.Layers[0].MaterialName != "Reinforced concrete" {
		t.Errorf("Layers[0].MaterialName = %q, want %q", set.Layers[0].MaterialName, "Reinforced concrete")
	}
	if set.Layers[0].ThicknessMm == nil || *set.Layers[0].ThicknessMm != 250 {
		t.Errorf("Layers[0].ThicknessMm = %v, want 250", set.Layers[0].ThicknessMm)
	}
	// The occurrence's usage supplies the axis even though the type itself carries
	// only a bare layer set.
	if set.Direction != "AXIS2" {
		t.Errorf("Direction = %q, want %q", set.Direction, "AXIS2")
	}
}

func keysOf(m map[string]TypeLayerSet) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A model whose types carry no layer set must produce an empty (not nil-deref'd)
// map, and must not pay a repeated inverse walk per occurrence.
func TestBuildImportTypeLayersAbsent(t *testing.T) {
	b, err := os.ReadFile("model/testdata/synthetic/wall_layerset.ifc")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	f, err := step.ParseBytes(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}

	if len(m.TypeLayers) != 0 {
		t.Errorf("len(TypeLayers) = %d, want 0 (that fixture has no IfcTypeObject)", len(m.TypeLayers))
	}
}

// A type that resolves but carries no build-up must be PRESENT with zero layers.
// Absence means "no such type" and tells a re-import to leave that type's rows
// alone; an empty entry means "this type has no layers any more" and is the only
// way a build-up that shrank all the way to zero is visible downstream.
func TestBuildImportTypeLayersEmptyWhenTypeHasNone(t *testing.T) {
	b, err := os.ReadFile("model/testdata/synthetic/wall_type_no_layers.ifc")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	f, err := step.ParseBytes(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	m, err := BuildImport(f)
	if err != nil {
		t.Fatalf("BuildImport: %v", err)
	}

	set, ok := m.TypeLayers["0GUIDwtyp0000000000012"]
	if !ok {
		t.Fatalf("TypeLayers omitted a resolved type that has no layers; got keys %v", keysOf(m.TypeLayers))
	}
	if len(set.Layers) != 0 {
		t.Errorf("len(Layers) = %d, want 0", len(set.Layers))
	}
	// Two occurrences of the same type still produce exactly one entry.
	if len(m.TypeLayers) != 1 {
		t.Errorf("len(TypeLayers) = %d, want 1; got keys %v", len(m.TypeLayers), keysOf(m.TypeLayers))
	}
}
