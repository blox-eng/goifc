package geometry

import (
	"errors"
	"slices"
	"testing"
)

// The #91 room: 2.40 × 1.80, counter-clockwise, measured on the interior.
var room2418 = [][2]float64{{0, 0}, {2.40, 0}, {2.40, 1.80}, {0, 1.80}}

func roomSums(ws []Wall) (measured, other, vol, netVol float64) {
	for _, w := range ws {
		measured += w.Quantities.MeasuredGross
		other += w.Quantities.OtherGross
		vol += w.Quantities.GrossVolume
		netVol += w.Quantities.NetVolume
	}
	return
}

func checkRoomMeshes(t *testing.T, name string, ws []Wall) {
	t.Helper()
	for i, w := range ws {
		checkWallMesh(t, name+" wall "+string(rune('0'+i)), w)
	}
}

func TestBuildRoomWallsOracle(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		outline                 [][2]float64
		height                  float64
		measured, other, volume float64
	}{
		// measured 2·(2.40+1.80)·2.60; other 2·(2.60+2.00)·2.60; volume (2.60·2.00 − 2.40·1.80)·2.60
		{"rect", room2418, 2.60, 21.84, 23.92, 2.288},
		// measured 16·2.5; other 16.8·2.5 (outer ring 4.2+2.2+2+2+2.2+4.2); volume (4.2² − 2² − 12)·2.5
		{"L", lOutline, 2.50, 40.0, 42.0, 4.1},
	} {
		ws, err := BuildRoomWalls(tc.outline, 0, tc.height, 0.10, SideLeft, nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(ws) != len(tc.outline) {
			t.Fatalf("%s: %d walls, want %d", tc.name, len(ws), len(tc.outline))
		}
		m, o, v, _ := roomSums(ws)
		if !near(m, tc.measured, 1e-9) || !near(o, tc.other, 1e-9) || !near(v, tc.volume, 1e-9) {
			t.Errorf("%s: measured %v other %v volume %v, want %v %v %v", tc.name, m, o, v, tc.measured, tc.other, tc.volume)
		}
		checkRoomMeshes(t, tc.name, ws)
	}
}

func TestBuildRoomWallsWithOpenings(t *testing.T) {
	ops := make([][]Opening, 4)
	ops[0] = []Opening{door}
	ops[1] = []Opening{{Offset: 0.40, Sill: 0.90, Width: 1.00, Height: 1.20}}
	ws, err := BuildRoomWalls(room2418, 2.0, 2.60, 0.10, SideLeft, ops)
	if err != nil {
		t.Fatal(err)
	}
	m, _, _, net := roomSums(ws)
	// net volume 2.288 − (0.80·2.00 + 1.00·1.20)·0.10
	if !near(m, 21.84, 1e-9) || !near(net, 2.008, 1e-9) {
		t.Errorf("measured %v net volume %v, want 21.84 and 2.008", m, net)
	}
	if ws[1].Measured.Origin != [3]float64{2.40, 0, 2.0} {
		t.Errorf("wall 1 origin %v", ws[1].Measured.Origin)
	}
	checkRoomMeshes(t, "openings", ws)
}

func TestBuildRoomWallsInCorner(t *testing.T) {
	// Wall 2 runs (4,2)→(2,2). The room's concave corner is at (2,2): the
	// opposite face stops at u = 1.90, short of the measured face's 2.00.
	ops := make([][]Opening, len(lOutline))
	ops[2] = []Opening{{Offset: 1.80, Sill: 0.5, Width: 0.15, Height: 1.0}}
	_, err := BuildRoomWalls(lOutline, 0, 2.5, 0.10, SideLeft, ops)
	wantRefusals(t, err, PlacementError{Wall: 2, Opening: 0, Reason: ReasonInCorner})

	ops[2] = []Opening{{Offset: 1.70, Sill: 0.5, Width: 0.15, Height: 1.0}}
	ws, err := BuildRoomWalls(lOutline, 0, 2.5, 0.10, SideLeft, ops)
	if err != nil {
		t.Fatal(err)
	}
	checkRoomMeshes(t, "clear of the corner", ws)
}

func TestBuildRoomWallsClockwise(t *testing.T) {
	cw := slices.Clone(room2418)
	slices.Reverse(cw)
	ws, err := BuildRoomWalls(cw, 0, 2.60, 0.10, SideRight, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, o, v, _ := roomSums(ws)
	if !near(m, 21.84, 1e-9) || !near(o, 23.92, 1e-9) || !near(v, 2.288, 1e-9) {
		t.Errorf("clockwise SideRight: %v %v %v, want 21.84 23.92 2.288", m, o, v)
	}
	checkRoomMeshes(t, "clockwise", ws)
}

func TestBuildRoomWallsStraightThrough(t *testing.T) {
	split := [][2]float64{{0, 0}, {1.20, 0}, {2.40, 0}, {2.40, 1.80}, {0, 1.80}}
	ops := make([][]Opening, len(split))
	ops[1] = []Opening{{Offset: 0.30, Sill: 0, Width: 0.80, Height: 2.00}}
	ws, err := BuildRoomWalls(split, 0, 2.60, 0.10, SideLeft, ops)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 5 {
		t.Fatalf("%d walls, want 5", len(ws))
	}
	m, o, v, _ := roomSums(ws)
	if !near(m, 21.84, 1e-9) || !near(o, 23.92, 1e-9) || !near(v, 2.288, 1e-9) {
		t.Errorf("split room: %v %v %v, want 21.84 23.92 2.288", m, o, v)
	}
	if s := ws[0].Quantities; !near(s.OtherGross, (1.20+0.10)*2.60, 1e-9) {
		t.Errorf("wall 0 opposite face %v, want a square cut at 1.20", s.OtherGross)
	}
	checkRoomMeshes(t, "split", ws)
}

func TestBuildRoomWallsRefuses(t *testing.T) {
	spike := [][2]float64{{0, 0}, {5, 0}, {0, 0.7}} // an 8° corner at (5, 0)
	notch := [][2]float64{{0, 0}, {3, 0}, {3, 2}, {1.575, 2}, {1.575, 0.5}, {1.425, 0.5}, {1.425, 2}, {0, 2}}
	for _, tc := range []struct {
		name    string
		outline [][2]float64
		ops     [][]Opening
	}{
		{"8° spike", spike, nil},
		{"notch narrower than two walls", notch, nil},
		// A 0.15 slot into a wide cavity: the slot walls are 1 m long, so only
		// the overlap of their opposite faces can refuse it.
		{"keyhole slot", [][2]float64{{0, 0}, {4, 0}, {4, 3}, {2.075, 3}, {2.075, 2}, {3, 2}, {3, 1}, {1, 1}, {1, 2}, {1.925, 2}, {1.925, 3}, {0, 3}}, nil},
		{"openings per edge mismatch", room2418, make([][]Opening, 3)},
		{"bowtie", [][2]float64{{0, 0}, {1, 1}, {1, 0}, {0, 1}}, nil},
		{"two points", [][2]float64{{0, 0}, {1, 0}}, nil},
	} {
		if _, err := BuildRoomWalls(tc.outline, 0, 2.5, 0.10, SideLeft, tc.ops); err == nil {
			t.Errorf("%s: want an error", tc.name)
		}
	}
	if _, err := BuildRoomWalls(room2418, 0, 2.5, 0.10, SideLeft, nil); err != nil {
		t.Errorf("the plain room must build: %v", err)
	}
}

func TestBuildRoomWallsRefusesTooMuchMesh(t *testing.T) {
	square := [][2]float64{{0, 0}, {30, 0}, {30, 30}, {0, 30}}
	ops := [][]Opening{staircase(), staircase(), staircase(), staircase()}
	_, err := BuildRoomWalls(square, 0, 3, 0.2, SideLeft, ops)
	var pe *PlacementError
	if err == nil || errors.As(err, &pe) {
		t.Fatalf("four staircase walls: want a plain too-complex error, got %v", err)
	}
}

func TestBuildRoomWallsJoinsRefusalsFlat(t *testing.T) {
	ops := make([][]Opening, 4)
	ops[0] = []Opening{{2.00, 0, 0.50, 2.0, 0}}
	ops[2] = []Opening{{0.50, 0.9, 0.80, 1.70, 0}}
	_, err := BuildRoomWalls(room2418, 0, 2.60, 0.10, SideLeft, ops)
	j, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("want a joined error, got %T", err)
	}
	for i, e := range j.Unwrap() {
		if _, ok := e.(*PlacementError); !ok {
			t.Errorf("element %d is %T, want *PlacementError", i, e)
		}
	}
	wantRefusals(t, err,
		PlacementError{Wall: 0, Opening: 0, Reason: ReasonTooWide},
		PlacementError{Wall: 2, Opening: 0, Reason: ReasonAboveWall})
}
