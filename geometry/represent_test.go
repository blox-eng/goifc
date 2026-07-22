package geometry

import (
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

func TestRepresentationItems_Extrusion(t *testing.T) {
	// full_wall.ifc has one IfcWall with an extruded-area-solid body.
	f, err := step.ParseFile("../model/testdata/synthetic/full_wall.ifc")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	walls := f.ByType("IfcWall")
	if len(walls) == 0 {
		t.Fatal("no wall")
	}
	items := representationItems(f, walls[0].ID())
	if len(items) == 0 {
		t.Fatal("no representation items found")
	}
	foundSolid := false
	for _, it := range items {
		if it.IsA("IfcExtrudedAreaSolid") {
			foundSolid = true
		}
	}
	if !foundSolid {
		t.Errorf("expected an IfcExtrudedAreaSolid among items, got %d items", len(items))
	}
}

func TestApplyMat_Translation(t *testing.T) {
	m := model.Identity()
	m[12], m[13], m[14] = 1, 2, 3
	got := applyMat(m, v3{10, 0, 0})
	want := v3{11, 2, 3}
	if got != want {
		t.Errorf("applyMat = %v, want %v", got, want)
	}
}
