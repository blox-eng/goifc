package geometry

import (
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

func TestMappedItem_LandsAtOffset(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/mapped_box.ifc")
	if err != nil {
		t.Fatal(err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Elements[0]
	if len(e.Tris) == 0 {
		t.Fatal("mapped item produced no geometry (stacked/empty)")
	}
	// MappingOrigin lifts +10 Z, MappingTarget shifts +100 X → world min ≈ (100,0,10).
	// Two assertions so BOTH halves of target·origin are exercised (identity origin
	// would leave the origin path silently untested).
	if e.BBoxMin[0] < 99.9 {
		t.Errorf("mapped box min X = %f, want ~100 (MappingTarget not composed)", e.BBoxMin[0])
	}
	if e.BBoxMin[2] < 9.9 {
		t.Errorf("mapped box min Z = %f, want ~10 (MappingOrigin not composed)", e.BBoxMin[2])
	}
}

// TestMappedItem_NestedTwoLevels proves the recursion depth guard (maxMapDepth)
// coexists with legitimate nesting: a mapped item whose mapped representation
// is itself another mapped item pointing at the real brep (depth 2) must still
// resolve, since real IFC mapped items nest 0-2 levels deep in practice.
func TestMappedItem_NestedTwoLevels(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/mapped_nested.ifc")
	if err != nil {
		t.Fatal(err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Elements[0]
	if len(e.Tris) == 0 {
		t.Fatal("2-level nested mapped item produced no geometry — depth guard is blocking legitimate nesting")
	}
}

// TestMappedItem_CyclicGuard proves the depth guard bounds a self-referential
// IfcMappedItem chain: a malformed/adversarial IFC upload with a cyclic mapped
// structure must return gracefully (empty geometry) instead of recursing
// unbounded and stack-overflowing the import. Without the guard this test
// overflows the stack; testing's normal flow fails it rather than crashing
// silently.
func TestMappedItem_CyclicGuard(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/mapped_cycle.ifc")
	if err != nil {
		t.Fatal(err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Elements[0]
	if len(e.Tris) != 0 {
		t.Fatalf("cyclic mapped item should resolve to no geometry, got %d tris", len(e.Tris))
	}
	if len(s.Warnings) == 0 {
		t.Error("expected a warning for the empty cyclic-mapped element")
	}
}
