package geometry

import (
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

// full_wall.ifc: a rectangle-profile wall extruded to a known height.
func TestExtrude_RectangleWall(t *testing.T) {
	f, err := step.ParseFile("../model/testdata/synthetic/full_wall.ifc")
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
	var wall *Element
	for i := range s.Elements {
		if s.Elements[i].Source == SourceExtrude {
			wall = &s.Elements[i]
			break
		}
	}
	if wall == nil {
		t.Fatal("no extrude-sourced element; dispatch not wired")
	}
	// A rectangle prism = 8 unique corners, 12 triangles (36 indices).
	if len(wall.Tris) != 36 {
		t.Errorf("rectangle prism tris = %d indices, want 36", len(wall.Tris))
	}
	dz := wall.BBoxMax[2] - wall.BBoxMin[2]
	if dz <= 0 {
		t.Errorf("prism has no height: dz=%f", dz)
	}
}
