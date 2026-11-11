package geometry

import (
	"errors"
	"math"
	"testing"
)

// niche is the addendum's oracle niche: 0.40 × 0.60, 0.08 deep, in the #91
// wall (2.40 × 2.60 × 0.10).
var niche = Opening{Offset: 1.00, Sill: 1.20, Width: 0.40, Height: 0.60, Depth: 0.08}

func TestNicheOracle(t *testing.T) {
	w, err := BuildWall(doorEdge, 0, 2.60, 0.10, SideLeft, []Opening{niche})
	if err != nil {
		t.Fatal(err)
	}
	q := w.Quantities
	for _, c := range []struct {
		what      string
		got, want float64
	}{
		{"MeasuredNet", q.MeasuredNet, 6.00},      // 6.24 − 0.40·0.60
		{"OtherNet", q.OtherNet, 6.24},            // a niche leaves the other face whole
		{"NicheBack", q.NicheBack, 0.24},          // 0.40·0.60
		{"NetVolume", q.NetVolume, 0.6048},        // 0.624 − 0.40·0.60·0.08
		{"RevealPerimeter", q.RevealPerimeter, 2}, // 2·0.40 + 2·0.60
	} {
		if !near(c.got, c.want, 1e-9) {
			t.Errorf("%s = %v, want %v", c.what, c.got, c.want)
		}
	}
	checkWallMesh(t, "niche", w)
}

func TestNicheInputErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Opening
	}{
		{"depth under 1 mm", Opening{1, 1.2, 0.4, 0.6, 0.0005}},
		{"depth within 1 mm of the thickness", Opening{1, 1.2, 0.4, 0.6, 0.0995}},
		{"depth past the thickness", Opening{1, 1.2, 0.4, 0.6, 0.2}},
		{"NaN depth", Opening{1, 1.2, 0.4, 0.6, math.NaN()}},
		{"negative depth", Opening{1, 1.2, 0.4, 0.6, -0.05}},
		{"floor-level niche", Opening{1, 0, 0.4, 0.6, 0.05}},
	} {
		_, err := BuildWall(doorEdge, 0, 2.60, 0.10, SideLeft, []Opening{tc.o})
		var pe *PlacementError
		if err == nil || errors.As(err, &pe) {
			t.Errorf("%s: want a plain input error, got %v", tc.name, err)
		}
	}
}

func TestNicheNearConcaveCorner(t *testing.T) {
	// L room wall 2 runs (4,2)→(2,2); its end line runs from u = 2.00 on the
	// measured face to u = 1.90 on the far face, so at w = 0.04 the wall
	// reaches u = 1.96. A 0.04 niche ending at 1.95 fits; a through opening
	// there cuts the corner.
	ops := make([][]Opening, len(lOutline))
	ops[2] = []Opening{{Offset: 1.80, Sill: 0.5, Width: 0.15, Height: 1.0, Depth: 0.04}}
	ws, err := BuildRoomWalls(lOutline, 0, 2.5, 0.10, SideLeft, ops)
	if err != nil {
		t.Fatal(err)
	}
	checkRoomMeshes(t, "niche at the corner", ws)

	ops[2][0].Depth = 0.09 // at w = 0.09 the wall reaches u = 1.91
	_, err = BuildRoomWalls(lOutline, 0, 2.5, 0.10, SideLeft, ops)
	wantRefusals(t, err, PlacementError{Wall: 2, Opening: 0, Reason: ReasonInCorner})
}

func TestNicheBesideThroughOpening(t *testing.T) {
	w, err := BuildWall(doorEdge, 0, 2.60, 0.10, SideRight, []Opening{{Offset: 0.20, Sill: 0, Width: 0.70, Height: 2.00}, niche})
	if err != nil {
		t.Fatal(err)
	}
	q := w.Quantities
	// measured 6.24 − 1.40 − 0.24; other 6.24 − 1.40; volume 0.624 − 0.14 − 0.0192
	if !near(q.MeasuredNet, 4.60, 1e-9) || !near(q.OtherNet, 4.84, 1e-9) || !near(q.NetVolume, 0.4648, 1e-9) {
		t.Errorf("quantities %+v", q)
	}
	checkWallMesh(t, "door and niche", w)
}
