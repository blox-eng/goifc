package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
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

// derivedVolume builds a file whose element "w" is a wall and returns its
// derived Volume.
func derivedVolume(t *testing.T, data string) *float64 {
	t.Helper()
	f := ifcFile(t, data)
	r, err := model.Extract(f)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	q, ok := s.DerivedQuantities()["w"]
	if !ok {
		t.Fatal("no derived quantities for the wall")
	}
	return q.Volume
}

// An element that joins a real solid with an item meshed as its box has no
// volume to report: the box's volume is not the element's. One of real solids
// keeps its volume.
func TestDerivedVolumeSkipsAnElementWithABoxedItem(t *testing.T) {
	// A swept disk is not meshed; its box is 2 x 2 x 1 from (2,0,0).
	disk := "#20=IFCCARTESIANPOINT((2.,0.,0.));\n#21=IFCCARTESIANPOINT((4.,2.,1.));\n" +
		"#22=IFCPOLYLINE((#20,#21));\n#23=IFCSWEPTDISKSOLID(#22,0.05,$,$,$);\n"
	if v := derivedVolume(t, wallWith("#12,#23")+boxSolid(10)+disk); v != nil {
		t.Errorf("Volume = %v, want none for a solid joined with a box", *v)
	}
	if v := derivedVolume(t, wallWith("#12")+boxSolid(10)); v == nil || math.Abs(*v-1) > 1e-9 {
		t.Errorf("Volume = %v, want 1 for the solid alone", v)
	}
}
