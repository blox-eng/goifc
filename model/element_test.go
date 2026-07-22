package model

import (
	"testing"

	"github.com/blox-eng/common/ifc/step"
)

func TestPsetsReadsCommon(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_pset.ifc"))
	w := f.ByType("IfcWall")[0]
	ps := Psets(f, w, false)
	common, ok := ps["Pset_WallCommon"]
	if !ok {
		t.Fatalf("no Pset_WallCommon in %v", ps)
	}
	if common["IsExternal"] != true {
		t.Fatalf("IsExternal = %v want true", common["IsExternal"])
	}
	if common["LoadBearing"] != false {
		t.Fatalf("LoadBearing = %v want false", common["LoadBearing"])
	}
}

func TestStoreyContainment(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/storey_tree.ifc"))
	if s := Storey(f, f.ByType("IfcWall")[0]); s != "Level 1" {
		t.Fatalf("storey = %q want Level 1", s)
	}
}

// TestStoreyViaFilledVoid ports ifcopenshell get_container's recursive
// get_parent chain: a door is not directly contained in any spatial
// structure — it resolves storey via door→(filled_void)→opening→
// (voided_element)→wall→(contained)→storey. Matches decompose._storey (which
// calls get_container). The wall itself stays directly resolvable.
func TestStoreyViaFilledVoid(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/door_filled_void.ifc"))
	if s := Storey(f, f.ByType("IfcWall")[0]); s != "Level 1" {
		t.Fatalf("wall storey = %q want Level 1", s)
	}
	if s := Storey(f, f.ByType("IfcDoor")[0]); s != "Level 1" {
		t.Fatalf("door storey = %q want Level 1 (via filled-void/voided-element chain)", s)
	}
}

func TestMaterialName(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_material.ifc"))
	mats := Materials(f, f.ByType("IfcWall")[0])
	if len(mats) != 1 || strVal(mats[0], 0) != "Concrete C25/30" {
		t.Fatalf("materials = %v", mats)
	}
}

func TestMaterialLayerSetOrder(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_layerset.ifc"))
	mats := Materials(f, f.ByType("IfcWall")[0])
	if len(mats) != 2 {
		t.Fatalf("materials = %v, want 2 leaves", mats)
	}
	if got := strVal(mats[0], 0); got != "EPS-Insulation" {
		t.Fatalf("first material = %q, want %q (first-declared layer, not graph-traversal order)", got, "EPS-Insulation")
	}
	if got := strVal(mats[1], 0); got != "Brick" {
		t.Fatalf("second material = %q, want %q", got, "Brick")
	}
}

func TestMaterialProfileSet(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/beam_profileset.ifc"))
	mats := Materials(f, f.ByType("IfcBeam")[0])
	if len(mats) != 1 || strVal(mats[0], 0) != "Steel S355" {
		t.Fatalf("materials = %v", mats)
	}
}

func TestMaterialConstituentSet(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/window_constituentset.ifc"))
	mats := Materials(f, f.ByType("IfcWindow")[0])
	if len(mats) != 2 {
		t.Fatalf("materials = %v, want 2 leaves", mats)
	}
	if got := strVal(mats[0], 0); got != "Aluminium" {
		t.Fatalf("first material = %q, want %q (first-declared constituent order)", got, "Aluminium")
	}
	if got := strVal(mats[1], 0); got != "Glass" {
		t.Fatalf("second material = %q, want %q", got, "Glass")
	}
}

func TestIsExternalTriState(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_pset.ifc"))
	ext := IsExternal(f, f.ByType("IfcWall")[0])
	if ext == nil || *ext != true {
		t.Fatalf("IsExternal = %v want *true", ext)
	}
}

// TestPartialOccurrenceOverrideKeepsTypeKeys ports ifcopenshell get_psets'
// psets.setdefault(name, {}).update(props) semantics: an occurrence
// Pset_WallCommon that only sets LoadBearing must MERGE into the
// type-seeded Pset_WallCommon per-property, not replace it wholesale — so
// the type-only IsExternal key survives alongside the occurrence-overridden
// LoadBearing.
func TestPartialOccurrenceOverrideKeepsTypeKeys(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/type_pset_partial_override.ifc"))
	w := f.ByType("IfcWall")[0]

	ext := IsExternal(f, w)
	if ext == nil || *ext != true {
		t.Fatalf("IsExternal = %v want *true (type-only key must survive a partial occurrence override)", ext)
	}

	common := Psets(f, w, false)["Pset_WallCommon"]
	if common["LoadBearing"] != true {
		t.Fatalf("LoadBearing = %v want true (occurrence wins per-key)", common["LoadBearing"])
	}
	if common["IsExternal"] != true {
		t.Fatalf("IsExternal = %v want true (type-only key survives)", common["IsExternal"])
	}
}

func bareWall(t *testing.T, f *step.File) *step.Instance {
	t.Helper()
	for _, w := range f.ByType("IfcWall") {
		if strVal(w, attrName) == "W-BARE" {
			return w
		}
	}
	t.Fatalf("no W-BARE wall in fixture")
	return nil
}

func overrideWall(t *testing.T, f *step.File) *step.Instance {
	t.Helper()
	for _, w := range f.ByType("IfcWall") {
		if strVal(w, attrName) == "W-OVERRIDE" {
			return w
		}
	}
	t.Fatalf("no W-OVERRIDE wall in fixture")
	return nil
}

func TestTypeInheritedIsExternal(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/type_inheritance.ifc"))
	w := bareWall(t, f)
	ext := IsExternal(f, w)
	if ext == nil || *ext != true {
		t.Fatalf("IsExternal = %v want *true (inherited from IfcWallType's Pset_WallCommon)", ext)
	}
}

func TestTypeInheritedQto(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/type_inheritance.ifc"))
	w := bareWall(t, f)
	q, ok := QtoQuantities(f, w, 1.0)
	if !ok {
		t.Fatalf("QtoQuantities ok = false, want true (inherited NetSideArea from IfcWallType)")
	}
	if q.Area == nil || *q.Area < 7.49 || *q.Area > 7.51 {
		t.Fatalf("Area = %v want ~7.5", q.Area)
	}
}

func TestTypeInheritedMaterial(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/type_inheritance.ifc"))
	w := bareWall(t, f)
	mats := Materials(f, w)
	if len(mats) == 0 || strVal(mats[0], 0) != "Concrete" {
		t.Fatalf("materials = %v, want first leaf 'Concrete' (inherited from IfcWallType)", mats)
	}
}

func TestOccurrenceOverridesType(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/type_inheritance.ifc"))
	w := overrideWall(t, f)
	ext := IsExternal(f, w)
	if ext == nil || *ext != false {
		t.Fatalf("IsExternal = %v want *false (occurrence Pset_WallCommon overrides type's .T.)", ext)
	}
}
