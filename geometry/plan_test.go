package geometry

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBuildPlanMatchesBuilders(t *testing.T) {
	edge := [2][2]float64{{0, 0}, {4, 0}}
	layers := []float64{0.10, 0.05, 0.10}
	rlayers := []float64{0.10, 0.05}
	ops := []Opening{door}
	got, err := BuildPlan(Plan{
		Walls: []PlanWall{
			{ID: "w1", Edge: edge, Base: 0, Height: 2.6, Thickness: 0.2, Side: SideLeft, Openings: ops},
			{ID: "w2", Edge: edge, Base: 0, Height: 2.6, Layers: layers, Side: SideRight},
		},
		Rooms: []PlanRoom{
			{ID: "r1", Outline: room2418, Height: 2.6, Thickness: 0.1, Side: SideLeft},
			{ID: "r2", Outline: room2418, Height: 2.6, Layers: rlayers, Side: SideLeft},
		},
		Slabs: []PlanSlab{
			{ID: "s1", Outline: rect54, Z: 0, Thickness: 0.2},
			{ID: "s2", Outline: rect54, Z: 0, Layers: rlayers},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var wantWalls []BuiltWall
	add := func(id string, w Wall) {
		w.Element.GlobalID = id
		wantWalls = append(wantWalls, BuiltWall{ID: id, Wall: w})
	}
	w1, _ := BuildWall(edge, 0, 2.6, 0.2, SideLeft, ops)
	add("w1", w1)
	w2, _ := BuildWallLayers(edge, 0, 2.6, layers, SideRight, nil)
	for j, w := range w2 {
		add("w2/"+string(rune('0'+j)), w)
	}
	r1, _ := BuildRoomWalls(room2418, 0, 2.6, 0.1, SideLeft, nil)
	for i, w := range r1 {
		add("r1/"+string(rune('0'+i)), w)
	}
	r2, _ := BuildRoomWallLayers(room2418, 0, 2.6, rlayers, SideLeft, nil)
	for i, ls := range r2 {
		for j, w := range ls {
			add("r2/"+string(rune('0'+i))+"/"+string(rune('0'+j)), w)
		}
	}
	var wantSlabs []BuiltSlab
	s1, _ := BuildSlab(rect54, 0, 0.2)
	s1.Element.GlobalID = "s1"
	wantSlabs = append(wantSlabs, BuiltSlab{ID: "s1", Slab: s1})
	s2, _ := BuildSlabLayers(rect54, 0, rlayers)
	for j, s := range s2 {
		id := "s2/" + string(rune('0'+j))
		s.Element.GlobalID = id
		wantSlabs = append(wantSlabs, BuiltSlab{ID: id, Slab: s})
	}

	var ids []string
	for _, w := range got.Walls {
		ids = append(ids, w.ID)
	}
	wantIDs := []string{"w1", "w2/0", "w2/1", "w2/2", "r1/0", "r1/1", "r1/2", "r1/3",
		"r2/0/0", "r2/0/1", "r2/1/0", "r2/1/1", "r2/2/0", "r2/2/1", "r2/3/0", "r2/3/1"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("wall IDs %v, want %v", ids, wantIDs)
	}
	if !reflect.DeepEqual(got.Walls, wantWalls) {
		t.Error("walls differ from the builders")
	}
	if !reflect.DeepEqual(got.Slabs, wantSlabs) {
		t.Error("slabs differ from the builders")
	}
}

func TestBuildPlanRefusesIDs(t *testing.T) {
	wall := func(id string) PlanWall {
		return PlanWall{ID: id, Edge: [2][2]float64{{0, 0}, {4, 0}}, Height: 2.6, Thickness: 0.2, Side: SideLeft}
	}
	slab := func(id string) PlanSlab { return PlanSlab{ID: id, Outline: rect54, Thickness: 0.2} }
	for name, c := range map[string]struct {
		p    Plan
		item string
	}{
		"empty":     {Plan{Walls: []PlanWall{wall("")}}, ""},
		"too long":  {Plan{Walls: []PlanWall{wall(strings.Repeat("a", 65))}}, strings.Repeat("a", 65)},
		"space":     {Plan{Walls: []PlanWall{wall("a b")}}, "a b"},
		"duplicate": {Plan{Walls: []PlanWall{wall("x")}, Slabs: []PlanSlab{slab("x")}}, "x"},
	} {
		_, err := BuildPlan(c.p)
		if err == nil {
			t.Errorf("%s: no error", name)
		}
		for _, e := range unjoin(err) {
			if ie, ok := e.(*ItemError); !ok || ie.Item != c.item {
				t.Errorf("%s: refusal %#v, want an *ItemError of %q", name, e, c.item)
			}
		}
		if name == "duplicate" && (err == nil || !strings.Contains(err.Error(), `"x"`)) {
			t.Errorf("duplicate: %v does not name the ID", err)
		}
	}
	if _, err := BuildPlan(Plan{Walls: []PlanWall{wall(strings.Repeat("a", 64))}}); err != nil {
		t.Errorf("64 characters: %v", err)
	}
}

func TestBuildPlanJoinsEveryRefusal(t *testing.T) {
	_, err := BuildPlan(Plan{
		Walls: []PlanWall{{
			ID: "w", Edge: [2][2]float64{{0, 0}, {4, 0}}, Height: 2.6, Thickness: 0.2, Side: SideLeft,
			Openings: []Opening{door, door},
		}},
		Rooms: []PlanRoom{{ID: "r", Outline: [][2]float64{{0, 0}, {1, 0}}, Height: 2.6, Thickness: 0.1, Side: SideLeft}},
	})
	if err == nil {
		t.Fatal("no error")
	}
	items := map[string]*ItemError{}
	for _, e := range err.(interface{ Unwrap() []error }).Unwrap() {
		var ie *ItemError
		if errors.As(e, &ie) {
			items[ie.Item] = ie
		}
	}
	if items["w"] == nil || items["r"] == nil {
		t.Fatalf("items %v, want w and r in %v", items, err)
	}
	var pe *PlacementError
	if !errors.As(items["w"], &pe) || pe.Reason != ReasonOverlaps {
		t.Errorf("w: %v, want a PlacementError overlaps", items["w"])
	}
}

func TestBuildPlanThicknessOrLayers(t *testing.T) {
	edge := [2][2]float64{{0, 0}, {4, 0}}
	for name, w := range map[string]PlanWall{
		"both":    {ID: "a", Edge: edge, Height: 2.6, Thickness: 0.2, Layers: []float64{0.1}},
		"neither": {ID: "b", Edge: edge, Height: 2.6},
	} {
		_, err := BuildPlan(Plan{Walls: []PlanWall{w}})
		if !errors.Is(err, errThicknessOrLayers) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
