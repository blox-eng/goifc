package geometry

import (
	"errors"
	"math"
	"testing"
)

// The #91 wall: 2.40 long, 2.60 high, 0.10 thick, with a door 0.80 × 2.00.
var (
	doorEdge = [2][2]float64{{0, 0}, {2.40, 0}}
	door     = Opening{Offset: 0.50, Sill: 0, Width: 0.80, Height: 2.00}
)

type wallCase struct {
	name               string
	edge               [2][2]float64
	height, thickness  float64
	ops                []Opening
	gross, net, reveal float64
	grossVol, netVol   float64
}

func wallOracle() []wallCase {
	return []wallCase{
		// gross 2.40·2.60; net 6.24 − 0.80·2.00; reveal 2·0.80 + 2·2.00 − 0.80 (no sill edge);
		// volume 6.24·0.10, net 0.624 − 1.60·0.10
		{"door", doorEdge, 2.60, 0.10, []Opening{door}, 6.24, 4.64, 4.80, 0.624, 0.464},
		// gross 3.00·2.70; net 8.10 − 1.20·1.20; reveal 2·1.20 + 2·1.20 (four sides);
		// volume 8.10·0.20, net 1.62 − 1.44·0.20
		{"window", [2][2]float64{{0, 0}, {3.00, 0}}, 2.70, 0.20,
			[]Opening{{Offset: 1.00, Sill: 0.90, Width: 1.20, Height: 1.20}}, 8.10, 6.66, 4.80, 1.62, 1.332},
		// no openings: net == gross, reveal 0
		{"solid", [2][2]float64{{0, 0}, {4.00, 0}}, 2.50, 0.25, nil, 10.0, 10.0, 0, 2.5, 2.5},
	}
}

func TestBuildWallOracle(t *testing.T) {
	for _, tc := range wallOracle() {
		w, err := BuildWall(tc.edge, 0, tc.height, tc.thickness, SideLeft, tc.ops)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		q := w.Quantities
		for _, c := range []struct {
			what      string
			got, want float64
		}{
			{"MeasuredGross", q.MeasuredGross, tc.gross},
			{"MeasuredNet", q.MeasuredNet, tc.net},
			{"OtherGross", q.OtherGross, tc.gross},
			{"OtherNet", q.OtherNet, tc.net},
			{"RevealPerimeter", q.RevealPerimeter, tc.reveal},
			{"GrossVolume", q.GrossVolume, tc.grossVol},
			{"NetVolume", q.NetVolume, tc.netVol},
			{"Height", q.Height, tc.height},
			{"Length", q.Length, tc.edge[1][0]},
		} {
			if !near(c.got, c.want, 1e-9) {
				t.Errorf("%s: %s = %v, want %v", tc.name, c.what, c.got, c.want)
			}
		}
	}
}

func wantRefusals(t *testing.T, err error, want ...PlacementError) {
	t.Helper()
	if err == nil {
		t.Fatalf("want refusals %+v, got nil", want)
	}
	var got []PlacementError
	var walk func(error)
	walk = func(e error) {
		if pe, ok := e.(*PlacementError); ok {
			got = append(got, *pe)
			return
		}
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, x := range j.Unwrap() {
				walk(x)
			}
		}
	}
	walk(err)
	if len(got) != len(want) {
		t.Fatalf("got refusals %+v (%v), want %+v", got, err, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("refusal %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	var pe *PlacementError
	if !errors.As(err, &pe) || *pe != want[0] {
		t.Errorf("errors.As gives %+v, want the first refusal %+v", pe, want[0])
	}
}

func TestBuildWallRefusesEachReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ops    []Opening
		index  int
		reason PlacementReason
	}{
		{"past the end", []Opening{{2.00, 0, 0.50, 2.0, 0}}, 0, ReasonTooWide},
		{"touches the end", []Opening{{1.60, 0, 0.80, 2.0, 0}}, 0, ReasonTooWide},
		{"touches the start", []Opening{{0, 0, 0.50, 2.0, 0}}, 0, ReasonTooWide},
		{"touches the top", []Opening{{0.50, 0.90, 0.80, 1.70, 0}}, 0, ReasonAboveWall},
		{"above the top", []Opening{{0.50, 0.90, 0.80, 2.00, 0}}, 0, ReasonAboveWall},
		{"shares an edge", []Opening{{0.20, 0, 0.80, 2.0, 0}, {1.00, 0, 0.80, 2.0, 0}}, 1, ReasonOverlaps},
		{"crosses", []Opening{{0.20, 0, 0.80, 2.0, 0}, {0.50, 1.0, 0.80, 1.0, 0}}, 1, ReasonOverlaps},
		{"touches at a corner", []Opening{{0.20, 0, 0.50, 1.0, 0}, {0.70, 1.0, 0.50, 1.0, 0}}, 1, ReasonOverlaps},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildWall(doorEdge, 0, 2.60, 0.10, SideLeft, tc.ops)
			wantRefusals(t, err, PlacementError{Wall: 0, Opening: tc.index, Reason: tc.reason})
		})
	}
}

func TestBuildWallReturnsAllRefusals(t *testing.T) {
	ops := []Opening{{2.00, 0, 0.50, 2.0, 0}, door, {0.50, 0.5, 0.30, 2.50, 0}}
	_, err := BuildWall(doorEdge, 0, 2.60, 0.10, SideLeft, ops)
	wantRefusals(t, err,
		PlacementError{Wall: 0, Opening: 0, Reason: ReasonTooWide},
		PlacementError{Wall: 0, Opening: 2, Reason: ReasonAboveWall})
}

func TestBuildWallInputErrors(t *testing.T) {
	many := make([]Opening, maxOpenings+1)
	for i := range many {
		many[i] = Opening{Offset: 0.1 + float64(i)*0.1, Sill: 0.5, Width: 0.05, Height: 1}
	}
	for _, tc := range []struct {
		name string
		edge [2][2]float64
		h    float64
		side Side
		ops  []Opening
	}{
		{"NaN offset", doorEdge, 2.6, SideLeft, []Opening{{math.NaN(), 0, 0.8, 2, 0}}},
		{"zero width", doorEdge, 2.6, SideLeft, []Opening{{0.5, 0, 0, 2, 0}}},
		{"negative sill", doorEdge, 2.6, SideLeft, []Opening{{0.5, -0.1, 0.8, 2, 0}}},
		{"sill under 1 mm", doorEdge, 2.6, SideLeft, []Opening{{0.5, 0.0005, 0.8, 2, 0}}},
		{"height under 1 mm", doorEdge, 0.0005, SideLeft, nil},
		{"Inf height", doorEdge, math.Inf(1), SideLeft, nil},
		{"zero-length edge", [2][2]float64{{1, 1}, {1, 1}}, 2.6, SideLeft, nil},
		{"edge over 1,000 m", [2][2]float64{{0, 0}, {1001, 0}}, 2.6, SideLeft, nil},
		{"start 1.1e7 m out", [2][2]float64{{1.1e7, 0}, {1.1e7 + 2, 0}}, 2.6, SideLeft, nil},
		{"257 openings", [2][2]float64{{0, 0}, {30, 0}}, 2.6, SideLeft, many},
		{"Side(0)", doorEdge, 2.6, Side(0), nil},
		{"Side(3)", doorEdge, 2.6, Side(3), nil},
	} {
		_, err := BuildWall(tc.edge, 0, tc.h, 0.10, tc.side, tc.ops)
		var pe *PlacementError
		if err == nil || errors.As(err, &pe) {
			t.Errorf("%s: want a plain input error, got %v", tc.name, err)
		}
	}
	if _, err := BuildWall(doorEdge, math.NaN(), 2.6, 0.1, SideLeft, nil); err == nil {
		t.Error("NaN base: want an error")
	}
}

func TestSideText(t *testing.T) {
	for _, s := range []Side{SideLeft, SideRight} {
		b, err := s.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var back Side
		if err := back.UnmarshalText(b); err != nil || back != s {
			t.Errorf("%q round-trips to %v, %v", b, back, err)
		}
	}
	if b, _ := SideLeft.MarshalText(); string(b) != "left" {
		t.Errorf("SideLeft = %q, want left", b)
	}
	if b, _ := SideRight.MarshalText(); string(b) != "right" {
		t.Errorf("SideRight = %q, want right", b)
	}
	for _, bad := range []string{"Left", "", "centre"} {
		var s Side
		if err := s.UnmarshalText([]byte(bad)); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
	if _, err := Side(0).MarshalText(); err == nil {
		t.Error("Side(0): want an error")
	}
}
