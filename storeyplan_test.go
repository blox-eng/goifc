package ifc

import (
	"reflect"
	"testing"

	"github.com/blox-eng/common/ifc/geometry"
	"github.com/blox-eng/common/ifc/model"
)

func boxElem(gid string, min, max [3]float64) geometry.Element {
	c := [8][3]float64{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]}, {max[0], max[1], min[2]}, {min[0], max[1], min[2]},
		{min[0], min[1], max[2]}, {max[0], min[1], max[2]}, {max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	var verts []float32
	for _, p := range c {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	tris := []uint32{0, 2, 1, 0, 3, 2, 4, 5, 6, 4, 6, 7, 0, 1, 5, 0, 5, 4, 1, 2, 6, 1, 6, 5, 2, 3, 7, 2, 7, 6, 3, 0, 4, 3, 4, 7}
	return geometry.Element{GlobalID: gid, Verts: verts, Tris: tris,
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   min, BBoxMax: max}
}

func iptr(i int) *int { return &i }

func TestStoreyPlansSpanningColumn(t *testing.T) {
	// nodes: [0]=storey A, [1]=storey B, [2]=column(parent A), [3]=wall(parent A), [4]=slab(parent B)
	nodes := []ImportNode{
		{GlobalID: "A", IFCClass: "IfcBuildingStorey"},
		{GlobalID: "B", IFCClass: "IfcBuildingStorey"},
		{GlobalID: "COL", IFCClass: "IfcColumn", ParentIndex: iptr(0)},
		{GlobalID: "W1", IFCClass: "IfcWall", ParentIndex: iptr(0)},
		{GlobalID: "SLAB2", IFCClass: "IfcSlab", ParentIndex: iptr(1)},
	}
	mesh := map[string]geometry.Element{
		"COL":   boxElem("COL", [3]float64{0, 0, 0}, [3]float64{0.4, 0.4, 6}),
		"W1":    boxElem("W1", [3]float64{1, 0, 0}, [3]float64{1.2, 3, 2.5}),
		"SLAB2": boxElem("SLAB2", [3]float64{0, 0, 2.8}, [3]float64{4, 4, 3.0}),
	}
	elev := map[string]float64{"A": 0, "B": 2.8}

	plans := buildStoreyPlans(nodes, mesh, elev)
	if len(plans) != 2 {
		t.Fatalf("want 2 storey plans, got %d", len(plans))
	}
	// Ordered by floorZ: A (0) then B (2.8).
	if plans[0].StoreyGlobalID != "A" || plans[1].StoreyGlobalID != "B" {
		t.Fatalf("plans not ordered by floor: %s, %s", plans[0].StoreyGlobalID, plans[1].StoreyGlobalID)
	}
	gids := func(p StoreyPlan) map[string]geometry.LoopRole {
		m := map[string]geometry.LoopRole{}
		for _, e := range p.Entities {
			role := geometry.LoopRole("")
			if len(e.Loops) > 0 {
				role = e.Loops[0].Role
			}
			m[e.GlobalID] = role
		}
		return m
	}
	a, b := gids(plans[0]), gids(plans[1])

	// Plan A: column CUT + wall CUT; slab NOT present.
	if a["COL"] != geometry.LoopCut {
		t.Fatalf("plan A: column should be cut, got %q (present=%v)", a["COL"], a)
	}
	if a["W1"] != geometry.LoopCut {
		t.Fatalf("plan A: wall should be cut, got %q", a["W1"])
	}
	if _, ok := a["SLAB2"]; ok {
		t.Fatalf("plan A should not include the floor-2 slab, got %v", a)
	}
	// Plan B: the SPANNING COLUMN appears (CUT) — the R2 headline. Wall absent.
	if b["COL"] != geometry.LoopCut {
		t.Fatalf("plan B: spanning column should be cut on the upper floor, got %q (present=%v)", b["COL"], b)
	}
	if _, ok := b["W1"]; ok {
		t.Fatalf("plan B should not include the floor-1 wall, got %v", b)
	}
	if b["SLAB2"] != geometry.LoopBelow {
		t.Fatalf("plan B: slab should be below-context, got %q", b["SLAB2"])
	}
}

func TestStoreyPlansDeterministic(t *testing.T) {
	nodes := []ImportNode{
		{GlobalID: "A", IFCClass: "IfcBuildingStorey"},
		{GlobalID: "COL", IFCClass: "IfcColumn", ParentIndex: iptr(0)},
	}
	mesh := map[string]geometry.Element{"COL": boxElem("COL", [3]float64{0, 0, 0}, [3]float64{0.4, 0.4, 3})}
	elev := map[string]float64{"A": 0}
	if !reflect.DeepEqual(buildStoreyPlans(nodes, mesh, elev), buildStoreyPlans(nodes, mesh, elev)) {
		t.Fatal("buildStoreyPlans not deterministic")
	}
}
