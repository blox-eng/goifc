package model

import (
	"testing"
)

func TestExtractFullWall(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/full_wall.ifc"))
	res, err := Extract(f)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Elements) != 2 {
		t.Fatalf("len(Elements) = %d want 2: %+v", len(res.Elements), res.Elements)
	}

	var wall, door *Element
	for i := range res.Elements {
		e := &res.Elements[i]
		switch e.IFCClass {
		case "IFCWALL":
			wall = e
		case "IFCDOOR":
			door = e
		}
	}
	if wall == nil {
		t.Fatalf("no IfcWall in %+v", res.Elements)
	}
	if door == nil {
		t.Fatalf("no IfcDoor in %+v", res.Elements)
	}

	if wall.GlobalID != "0GUIDwall0000000000020" {
		t.Errorf("wall.GlobalID = %q", wall.GlobalID)
	}
	if wall.ExpressID != 20 {
		t.Errorf("wall.ExpressID = %d want 20 (#20=IFCWALL(...) in fixture)", wall.ExpressID)
	}
	if inst, ok := f.ByID(wall.ExpressID); !ok || strVal(inst, attrGlobalID) != wall.GlobalID {
		t.Errorf("f.ByID(wall.ExpressID) round-trip failed: got %+v, ok=%v", inst, ok)
	}
	if wall.Category != "WALL" {
		t.Errorf("wall.Category = %q want WALL", wall.Category)
	}
	if wall.Storey != "Level 1" {
		t.Errorf("wall.Storey = %q want %q", wall.Storey, "Level 1")
	}
	if wall.QuantitySource != "qto" {
		t.Errorf("wall.QuantitySource = %q want qto", wall.QuantitySource)
	}
	if wall.Qto.Area == nil || *wall.Qto.Area < 12.49 || *wall.Qto.Area > 12.51 {
		t.Errorf("wall.Qto.Area = %v want ~12.5", wall.Qto.Area)
	}
	if !wall.Emit {
		t.Errorf("wall.Emit = false want true")
	}
	if wall.Material != "Concrete C25/30" {
		t.Errorf("wall.Material = %q", wall.Material)
	}
	if wall.IsExternal == nil || !*wall.IsExternal {
		t.Errorf("wall.IsExternal = %v want *true", wall.IsExternal)
	}
	// placement: base (10,0,0) then local translate (0,0,3) scaled by 1.0 (metre unit)
	x, y, z := wall.Placement.Translation()
	if x < 9.99 || x > 10.01 || y != 0 || z < 2.99 || z > 3.01 {
		t.Errorf("wall.Placement translation = (%v,%v,%v) want (10,0,3)", x, y, z)
	}

	if door.Emit {
		t.Errorf("door.Emit = true want false")
	}
	if door.QuantitySource != "none" {
		t.Errorf("door.QuantitySource = %q want none", door.QuantitySource)
	}
	if door.Qto != (Quantities{}) {
		t.Errorf("door.Qto = %+v want zero value (QuantitySource=none must pair with empty Qto)", door.Qto)
	}

	// container for both wall and door is #5 IfcBuildingStorey, which is NOT in
	// emitOrRenderClasses, so neither resolves to an in-result parent.
	if wall.ParentIndex != nil {
		t.Errorf("wall.ParentIndex = %v want nil (container is a storey, not an emitted element)", *wall.ParentIndex)
	}
	if door.ParentIndex != nil {
		t.Errorf("door.ParentIndex = %v want nil (container is a storey, not an emitted element)", *door.ParentIndex)
	}
	// No-Qto is signaled by QuantitySource="none" (asserted above), not a
	// per-element warning — the geometry tier back-fills these.
}

// TestExtractPartialQtoZeroedOnNone proves the `q = Quantities{}` guard in
// Extract: a Qto set with Volume+Length but NO Area yields hasQto=false
// (QtoQuantities' tier-1 gate requires Area), so QuantitySource="none" must
// still zero out the partial Volume/Length — never leak them. Without the
// guard this test fails (Volume/Length non-nil).
func TestExtractPartialQtoZeroedOnNone(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_qto_noarea.ifc"))
	res, err := Extract(f)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Elements) != 1 {
		t.Fatalf("len(Elements) = %d want 1: %+v", len(res.Elements), res.Elements)
	}
	beam := &res.Elements[0]

	if beam.QuantitySource != "none" {
		t.Errorf("beam.QuantitySource = %q want none", beam.QuantitySource)
	}
	if beam.Qto.Area != nil {
		t.Errorf("beam.Qto.Area = %v want nil", *beam.Qto.Area)
	}
	if beam.Qto.Volume != nil {
		t.Errorf("beam.Qto.Volume = %v want nil (partial Qto must be zeroed)", *beam.Qto.Volume)
	}
	if beam.Qto.Length != nil {
		t.Errorf("beam.Qto.Length = %v want nil (partial Qto must be zeroed)", *beam.Qto.Length)
	}
	if beam.Qto != (Quantities{}) {
		t.Errorf("beam.Qto = %+v want zero value", beam.Qto)
	}
	if beam.QuantitySource != QuantitySourceNone {
		t.Errorf("beam.QuantitySource = %q want none", beam.QuantitySource)
	}
}

func TestExtractDeterministicOrder(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/full_wall.ifc"))
	res1, err := Extract(f)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	res2, err := Extract(f)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for i := range res1.Elements {
		if res1.Elements[i].GlobalID != res2.Elements[i].GlobalID {
			t.Fatalf("non-deterministic order: %v vs %v", res1.Elements, res2.Elements)
		}
	}

	// full_wall.ifc declares #20=IFCWALL before #60=IFCDOOR, so ascending step-id
	// order must place the wall first regardless of ByType map iteration order.
	want := []string{"0GUIDwall0000000000020", "0GUIDdoor0000000000060"}
	got := make([]string, len(res1.Elements))
	for i, e := range res1.Elements {
		got[i] = e.GlobalID
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v want %v (ascending step #id: wall #20 before door #60)", got, want)
		}
	}
}

// TestExtractScalesPlacementToMeters proves the m[12,13,14]*=scale step in
// Extract actually runs: a MILLI METRE file places a wall at (5000,0,3000) mm,
// which must come out as (5.0,0.0,3.0) m. Deleting the scaling line would leave
// this at (5000,0,3000).
func TestExtractScalesPlacementToMeters(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/wall_mm_placed.ifc"))
	res, err := Extract(f)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Elements) != 1 {
		t.Fatalf("len(Elements) = %d want 1: %+v", len(res.Elements), res.Elements)
	}
	if res.UnitScale != 0.001 {
		t.Fatalf("UnitScale = %v want 0.001", res.UnitScale)
	}
	wall := res.Elements[0]
	x, y, z := wall.Placement.Translation()
	const tol = 1e-9
	if abs(x-5.0) > tol || abs(y-0.0) > tol || abs(z-3.0) > tol {
		t.Errorf("wall.Placement translation = (%v,%v,%v) want (5.0,0.0,3.0) m", x, y, z)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// TestExtractParentIndexAggregation exercises Container()'s IfcRelAggregates
// branch (attrRel4=RelatingObject) via a plate aggregated into a curtain wall —
// both classes are in emitOrRenderClasses, so the plate must resolve a positive
// ParentIndex pointing at the curtain wall's Element.
func TestExtractParentIndexAggregation(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/curtainwall_aggregate.ifc"))
	res, err := Extract(f)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Elements) != 2 {
		t.Fatalf("len(Elements) = %d want 2: %+v", len(res.Elements), res.Elements)
	}

	var curtainwall, plate *Element
	for i := range res.Elements {
		e := &res.Elements[i]
		switch e.IFCClass {
		case "IFCCURTAINWALL":
			curtainwall = e
		case "IFCPLATE":
			plate = e
		}
	}
	if curtainwall == nil {
		t.Fatalf("no IfcCurtainWall in %+v", res.Elements)
	}
	if plate == nil {
		t.Fatalf("no IfcPlate in %+v", res.Elements)
	}

	if plate.ParentIndex == nil {
		t.Fatalf("plate.ParentIndex = nil want non-nil (aggregated into curtain wall)")
	}
	parent := res.Elements[*plate.ParentIndex]
	if parent.GlobalID != curtainwall.GlobalID {
		t.Errorf("plate's parent GlobalID = %q want %q", parent.GlobalID, curtainwall.GlobalID)
	}

	// The curtain wall is the aggregation ROOT — it is RelatingObject in #30, not a
	// member of RelatedObjects, so it must NOT resolve a parent (and must certainly
	// not resolve itself as its own parent — the self-cycle this test guards against).
	if curtainwall.ParentIndex != nil {
		t.Errorf("curtainwall.ParentIndex = %v want nil (aggregation root; f.Inverse(#10) also returns rel #30, but #10 is RelatingObject not a RelatedObjects member)", *curtainwall.ParentIndex)
	}
}
