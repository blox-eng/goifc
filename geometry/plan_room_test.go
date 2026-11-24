package geometry

import (
	"errors"
	"slices"
	"testing"
)

func builtRoom(t *testing.T, b Built, id string) RoomQuantities {
	t.Helper()
	for _, r := range b.Rooms {
		if r.ID == id {
			return r.Quantities
		}
	}
	t.Fatalf("no room %q in %+v", id, b.Rooms)
	return RoomQuantities{}
}

func checkRoomQuantities(t *testing.T, name string, it PlanRoom, want RoomQuantities) {
	t.Helper()
	checkPlanRoom(t, name, Plan{Rooms: []PlanRoom{it}}, it.ID, want)
}

func checkPlanRoom(t *testing.T, name string, p Plan, id string, want RoomQuantities) {
	t.Helper()
	b, err := BuildPlan(p)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	got := builtRoom(t, b, id)
	if !near(got.Area, want.Area, 1e-9) || !near(got.Perimeter, want.Perimeter, 1e-9) ||
		!near(got.NetPerimeter, want.NetPerimeter, 1e-9) || !near(got.Volume, want.Volume, 1e-9) {
		t.Errorf("%s: got %+v, want %+v", name, got, want)
	}
}

var (
	roomOut = PlanRoom{ID: "r", Outline: [][2]float64{{0, 0}, {4, 0}, {4, 5}, {0, 5}}, Height: 2.5, Thickness: 0.2, Side: SideLeft}
	roomIn  = PlanRoom{ID: "r", Outline: [][2]float64{{0, 0}, {4.4, 0}, {4.4, 5.4}, {0, 5.4}}, Height: 2.5, Thickness: 0.2, Side: SideRight}
	room4x5 = RoomQuantities{Area: 20, Perimeter: 18, NetPerimeter: 18, Volume: 50}
)

func TestRoomQuantitiesGrowOut(t *testing.T) {
	checkRoomQuantities(t, "out", roomOut, room4x5)
}

func TestRoomQuantitiesGrowIn(t *testing.T) {
	checkRoomQuantities(t, "in, counter-clockwise", roomIn, room4x5)
	cw := roomIn
	cw.Outline = slices.Clone(roomIn.Outline)
	slices.Reverse(cw.Outline)
	cw.Side = SideLeft
	checkRoomQuantities(t, "in, clockwise", cw, room4x5)
}

func TestRoomQuantitiesLayeredGrowIn(t *testing.T) {
	it := roomIn
	it.Thickness, it.Layers = 0, []float64{0.05, 0.15}
	checkRoomQuantities(t, "layered in", it, room4x5)
}

func TestRoomQuantitiesL(t *testing.T) {
	it := roomOut
	it.Outline = [][2]float64{{0, 0}, {6, 0}, {6, 3}, {3, 3}, {3, 5}, {0, 5}}
	checkRoomQuantities(t, "L", it, RoomQuantities{Area: 24, Perimeter: 22, NetPerimeter: 22, Volume: 60})
}

func TestRoomQuantitiesDoors(t *testing.T) {
	door := Opening{Offset: 1, Width: 0.9, Height: 2.1}
	window := Opening{Offset: 1, Sill: 0.9, Width: 1.2, Height: 1.2}
	want := RoomQuantities{Area: 20, Perimeter: 18, NetPerimeter: 17.1, Volume: 50}

	out := roomOut
	out.Openings = [][]Opening{{door}, {window}, nil, nil}
	checkRoomQuantities(t, "out", out, want)

	in := roomIn
	in.Openings = [][]Opening{{door}, nil, nil, nil}
	checkRoomQuantities(t, "in", in, want)
}

// A partition stands in the room: its feet come off the net perimeter, and
// nothing comes off the area.
func TestRoomQuantitiesPartition(t *testing.T) {
	p := PlanWall{ID: "P", Edge: [2][2]float64{{2, 3}, {2, 0}}, Height: 2.5, Thickness: 0.1, Side: SideLeft,
		Joins: []Join{{End: EndA, Wall: "R/2"}, {End: EndB, Wall: "R/0"}}}
	checkPlanRoom(t, "partition", Plan{Walls: []PlanWall{p}, Rooms: []PlanRoom{joinR}}, "R",
		RoomQuantities{Area: 12, Perimeter: 14, NetPerimeter: 13.8, Volume: 30})
}

func TestRoomQuantitiesRefusal(t *testing.T) {
	it := roomIn
	it.Outline = [][2]float64{{0, 0}, {4, 0}, {4, 0.1}, {0, 0.1}}
	_, want := BuildRoomWalls(it.Outline, it.Base, it.Height, it.Thickness, it.Side, nil)
	if want == nil {
		t.Fatal("BuildRoomWalls accepted a room narrower than two walls")
	}
	b, err := BuildPlan(Plan{Rooms: []PlanRoom{it}})
	var ie *ItemError
	if !errors.As(err, &ie) || ie.Err.Error() != want.Error() {
		t.Errorf("BuildPlan: %v, want %v", err, want)
	}
	if len(b.Rooms) != 0 {
		t.Errorf("Rooms %+v, want none", b.Rooms)
	}
}

func TestRoomQuantitiesOrder(t *testing.T) {
	a, c := roomOut, roomIn
	a.ID, c.ID = "A", "B"
	w := PlanWall{ID: "w", Edge: [2][2]float64{{20, 0}, {24, 0}}, Height: 2.5, Thickness: 0.2, Side: SideLeft}
	b, err := BuildPlan(Plan{Walls: []PlanWall{w}, Rooms: []PlanRoom{a, c}})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Rooms) != 2 || b.Rooms[0].ID != "A" || b.Rooms[1].ID != "B" {
		t.Errorf("Rooms %+v, want A then B", b.Rooms)
	}
}
