package geometry

import (
	"testing"

	"github.com/blox-eng/goifc/step"
)

const (
	boxedOpeningFixture = "testdata/synthetic/elevation_boxed_opening.ifc"
	boxedWallA          = "0GUIDwallA000000000024" // window + a box
	boxedWallB          = "0GUIDwallB000000000043" // window + a box, mapped
	boxedWallC          = "0GUIDwallC000000000063" // window + a real solid
)

// An opening that unions a real solid with an item meshed as its box is not
// a footprint to deduct: the box would count as part of the void.
func TestNetAreasDistrustsAnOpeningWithABoxedItem(t *testing.T) {
	_, m := buildNetAreas(t, boxedOpeningFixture)
	for _, gid := range []string{boxedWallA, boxedWallB} {
		t.Run(gid, func(t *testing.T) {
			assertUntrusted(t, m[gid], "solid geometry")
		})
	}
}

// The same opening with the box replaced by a real solid is deducted whole:
// 12 − (1 + 0.25).
func TestNetAreasTrustsAnOpeningOfRealSolids(t *testing.T) {
	_, m := buildNetAreas(t, boxedOpeningFixture)
	na := m[boxedWallC]
	if !na.Trusted {
		t.Fatalf("want Trusted, got reason %q", na.Reason)
	}
	closeAbs(t, "Gross", &na.Gross, 12.0, 1e-6)
	closeAbs(t, "OpeningDeduction", &na.OpeningDeduction, 1.25, 1e-6)
	closeAbs(t, "Net", na.Net, 10.75, 1e-6)
}

// An opening with a boxed item is not drawn as a hole; one of real solids is.
func TestElevationSkipsAnOpeningWithABoxedItem(t *testing.T) {
	_, _, _, v := buildElevation(t, boxedOpeningFixture, [3]float64{0, 1, 0})
	for _, gid := range []string{boxedWallA, boxedWallB} {
		if e := entityByID(t, v, gid); len(e.Openings) != 0 {
			t.Errorf("%s: an opening with a boxed item was drawn as %d hole(s); want none", gid, len(e.Openings))
		}
	}
	if e := entityByID(t, v, boxedWallC); len(e.Openings) == 0 {
		t.Errorf("%s: an opening of real solids was not drawn", boxedWallC)
	}
}

// A mapped representation's boxed item marks every occurrence, cached or not.
func TestABoxedItemMarksAMappedOccurrence(t *testing.T) {
	f, err := step.ParseFile(boxedOpeningFixture)
	if err != nil {
		t.Fatal(err)
	}
	item, _ := f.ByID(50)
	for _, c := range []*meshCache{nil, {}} {
		for range 2 {
			if m := tessellateItemDepth(item, 1, 0, c, newBudget()); !m.boxed || m.src != SourceExtrude {
				t.Errorf("cache=%v: boxed=%v src=%s, want a boxed extrude", c != nil, m.boxed, m.src)
			}
		}
	}
}
