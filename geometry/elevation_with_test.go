package geometry

import (
	"testing"
)

// elevationWithPlanes is the north/east pair the sharing tests project onto.
func elevationWithPlanes(t *testing.T) []Plane {
	t.Helper()
	return []Plane{
		mustElevationPlane(t, [3]float64{0, 1, 0}),
		mustElevationPlane(t, [3]float64{1, 0, 0}),
	}
}

// TestElevationsWith_MatchesElevations pins the equivalence that makes sharing
// safe: handed the classification Elevations would have built for itself,
// ElevationsWith must draw exactly the same thing. If the two ever diverge, a
// caller that shares one pass to also total the facade gets sheets that
// disagree with the sheets every other caller sees, and nothing in the output
// says so.
func TestElevationsWith_MatchesElevations(t *testing.T) {
	s, f, r, _ := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	planes := elevationWithPlanes(t)

	want := s.Elevations(f, r, planes)
	got := s.ElevationsWith(f, r, planes, BuildFacings(s.Elements))

	if len(got) != len(want) {
		t.Fatalf("ElevationsWith = %d views, Elevations = %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i].Entities) != len(want[i].Entities) {
			t.Fatalf("view %d: %d entities, want %d", i, len(got[i].Entities), len(want[i].Entities))
		}
		for j := range want[i].Entities {
			if got[i].Entities[j].GlobalID != want[i].Entities[j].GlobalID {
				t.Fatalf("view %d entity %d: GlobalID %q, want %q",
					i, j, got[i].Entities[j].GlobalID, want[i].Entities[j].GlobalID)
			}
			if got[i].Entities[j].Depth != want[i].Entities[j].Depth {
				t.Fatalf("view %d entity %d: Depth %v, want %v",
					i, j, got[i].Entities[j].Depth, want[i].Entities[j].Depth)
			}
		}
		if got[i].Bounds != want[i].Bounds {
			t.Fatalf("view %d: Bounds %v, want %v", i, got[i].Bounds, want[i].Bounds)
		}
	}
}

// TestElevationsWith_TakesAnEmptyClassificationAtFaceValue guards the one way
// this API can silently cost exactly what it exists to save. An empty map is a
// positive claim — "nothing is classified" — and treating it as "classify it
// for me" would restore the BuildFacings pass the caller came here to skip,
// invisibly, because the drawing would still look right.
//
// An unclassified scene has no outward-facing elements to draw, so any entity
// here means the map was rebuilt behind the caller's back. nil is covered
// alongside empty: the two read identically on lookup, and only the shared
// loop's lazy-build signal distinguishes them.
func TestElevationsWith_TakesAnEmptyClassificationAtFaceValue(t *testing.T) {
	s, f, r, _ := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	planes := elevationWithPlanes(t)

	if n := len(s.Elevations(f, r, planes)[0].Entities); n == 0 {
		t.Fatal("fixture draws nothing even when classified; the assertions below cannot discriminate")
	}

	for _, tc := range []struct {
		name    string
		facings map[string]Facing
	}{
		{"empty", map[string]Facing{}},
		{"nil", nil},
	} {
		for i, v := range s.ElevationsWith(f, r, planes, tc.facings) {
			if len(v.Entities) != 0 {
				t.Errorf("%s classification, view %d: drew %d entities, want 0 — "+
					"the map was rebuilt rather than taken at face value",
					tc.name, i, len(v.Entities))
			}
		}
	}
}
