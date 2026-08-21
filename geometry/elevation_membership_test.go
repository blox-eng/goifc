package geometry

import (
	"math"
	"testing"
)

// compassPlanes is the four-sheet set a facade drawing is made of.
func compassPlanes(t *testing.T) []Plane {
	t.Helper()
	return []Plane{
		mustElevationPlane(t, [3]float64{0, 1, 0}),  // north
		mustElevationPlane(t, [3]float64{1, 0, 0}),  // east
		mustElevationPlane(t, [3]float64{0, -1, 0}), // south
		mustElevationPlane(t, [3]float64{-1, 0, 0}), // west
	}
}

// nudged returns n rotated by deg about Z — the hair of off-axis that a real
// placement transform leaves behind, and that a synthetic fixture never has.
func nudged(n [3]float64, deg float64) [3]float64 {
	rad := deg * math.Pi / 180
	c, s := math.Cos(rad), math.Sin(rad)
	return [3]float64{n[0]*c - n[1]*s, n[0]*s + n[1]*c, n[2]}
}

// TestElevations_AWallIsNotOnThePerpendicularSheet is goifc#28.
//
// A wall cannot face both north and east. On kb645 all 87 of 87 ETICS hosts
// landed on two perpendicular sheets, the secondary carrying 10-50% of the
// dominant sheet's area — because SilhouetteOn projects the whole SOLID, so an
// edge-on wall still draws its thickness at full height however small the dot
// product that admitted it.
//
// The exactly-axis-aligned synthetic fixtures cannot catch this: their dot with
// a perpendicular plane is exactly 0, which even the old `<= 0` test excluded.
// Real placement transforms leave a normal a hair off-axis, the dot goes barely
// positive, and the wall is admitted. So the nudge here is the bug's actual
// precondition, not a contrivance to make the test fail.
func TestElevations_AWallIsNotOnThePerpendicularSheet(t *testing.T) {
	s, f, r, _ := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	planes := compassPlanes(t)

	facings := BuildFacings(s.Elements)
	if len(facings) == 0 {
		t.Fatal("fixture classifies nothing; the assertions below cannot discriminate")
	}
	for gid, fc := range facings {
		fc.Normal = nudged(fc.Normal, 0.01)
		facings[gid] = fc
	}

	sheets := map[string]int{}
	for _, v := range s.ElevationsWith(f, r, planes, facings) {
		for _, e := range v.Entities {
			sheets[e.GlobalID]++
		}
	}

	if len(sheets) == 0 {
		t.Fatal("no entity landed on any sheet; a wall must be drawn somewhere")
	}
	for _, gid := range []string{wallA, wallB} {
		if got := sheets[gid]; got != 1 {
			t.Errorf("%s appears on %d sheets, want exactly 1", gid, got)
		}
	}
}

// TestElevationFacing_Threshold pins the rule itself, at the boundary.
//
// `>=`, not `>`: at a true 45 degrees two planes sit exactly AT the threshold,
// and `>` would admit the wall to NEITHER, deleting it from the drawing. A
// genuinely diagonal wall belongs on both sheets; that is the documented tie.
func TestElevationFacing_Threshold(t *testing.T) {
	north := [3]float64{0, 1, 0}
	for _, tc := range []struct {
		name string
		deg  float64
		want bool
	}{
		{"head on", 0, true},
		{"just inside the cut", 44.9, true},
		{"exactly diagonal is a tie, not a deletion", 45, true},
		{"just outside the cut", 45.1, false},
		{"a hair off perpendicular is the #28 case", 89.99, false},
		{"perpendicular", 90, false},
		{"facing away", 180, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := facesSheet(nudged(north, tc.deg), north); got != tc.want {
				t.Errorf("facesSheet at %v deg = %v, want %v", tc.deg, got, tc.want)
			}
		})
	}
}
