package geometry

import (
	"math"
	"reflect"
	"testing"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// buildElevation parses a synthetic fixture and projects it along dir.
func buildElevation(t *testing.T, path string, dir [3]float64) (*Scene, *step.File, *model.Result, ElevationView) {
	t.Helper()
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	p, ok := ElevationPlane(dir)
	if !ok {
		t.Fatalf("no elevation plane for %v", dir)
	}
	return s, f, r, s.ElevationOn(f, r, p)
}

const twoWalls = "testdata/synthetic/elevation_two_walls.ifc"

// entityByID finds one entity in the view.
func entityByID(t *testing.T, v ElevationView, gid string) ElevationEntity {
	t.Helper()
	for _, e := range v.Entities {
		if e.GlobalID == gid {
			return e
		}
	}
	t.Fatalf("no entity %s in view of %d", gid, len(v.Entities))
	return ElevationEntity{}
}

const (
	wallA = "0GUIDwallA000000000022"
	wallB = "0GUIDwallB000000000072"
)

// TestElevationOutlineIsTheWallFace: two parallel walls, 4 m long, 3 m and 4 m
// tall. Each outline is that face and nothing else — a wrong UV basis would
// yield the 0.2 m thickness somewhere in the rectangle.
func TestElevationOutlineIsTheWallFace(t *testing.T) {
	_, _, _, v := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	for _, tc := range []struct {
		gid  string
		want float64
	}{{wallA, 4 * 3}, {wallB, 4 * 4}} {
		e := entityByID(t, v, tc.gid)
		if len(e.Outline) != 1 {
			t.Fatalf("%s: want 1 outline loop, got %d", tc.gid, len(e.Outline))
		}
		if got := loopArea(e.Outline[0]); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: outline area = %v, want %v", tc.gid, got, tc.want)
		}
		if polygonArea2D(e.Outline[0].Points) <= 0 {
			t.Errorf("%s: outer ring is not CCW", tc.gid)
		}
		if e.IFCClass != "IFCWALL" {
			t.Errorf("%s: IFCClass = %q", tc.gid, e.IFCClass)
		}
	}
}

// TestElevationOpeningIsANestedHole: the window is projected onto the SAME
// plane as its host and wound CW, so an even-odd or nonzero fill renders it as
// a cutout rather than as a second filled rectangle on top of the wall.
func TestElevationOpeningIsANestedHole(t *testing.T) {
	_, _, _, v := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	e := entityByID(t, v, wallA)
	if len(e.Openings) != 1 {
		t.Fatalf("want 1 opening loop, got %d", len(e.Openings))
	}
	if got := loopArea(e.Openings[0]); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("opening area = %v, want 1.0", got)
	}
	if polygonArea2D(e.Openings[0].Points) >= 0 {
		t.Error("opening inside the outline must be wound CW to read as a hole")
	}
	if b := entityByID(t, v, wallB); len(b.Openings) != 0 {
		t.Errorf("wall B has no voids; want 0 openings, got %d", len(b.Openings))
	}
}

// TestElevationAreaAgreesWithNetArea is THE invariant: what the drawing shows
// and what the quantity reports are the same fact, measured by the same engine
// on the same plane. Drift here means one of the two is lying, and a consumer
// hit-testing a region of the elevation to read its net area would get a number
// that does not match the shape it clicked.
//
// Wall A is square to this view, so its NetAreas winning axis IS this plane.
// That precondition is the whole content of the invariant — see ElevationView
// on reconciling a host whose winning axis lies elsewhere.
func TestElevationAreaAgreesWithNetArea(t *testing.T) {
	s, f, r, v := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	na, ok := s.NetAreas(f, r)[wallA]
	if !ok || !na.Trusted {
		t.Fatalf("wall A must be a trusted net area, got %+v", na)
	}
	e := entityByID(t, v, wallA)

	var drawn float64
	for _, l := range e.Outline {
		drawn += loopArea(l)
	}
	for _, l := range e.Openings {
		drawn -= loopArea(l)
	}
	if math.Abs(drawn-*na.Net) > 1e-9 {
		t.Errorf("outline minus openings = %v, but NetAreas reports Net = %v", drawn, *na.Net)
	}
}

// TestElevationSortsNearestFirst: wall B stands 5 m beyond wall A, so a viewer
// on the +Y side meets B first. Depth is a SIGNED position along the view
// normal, not an absolute distance, so the assertion is on the ordering and on
// the 5 m gap between them.
func TestElevationSortsNearestFirst(t *testing.T) {
	_, _, _, v := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	if len(v.Entities) != 2 {
		t.Fatalf("want 2 entities, got %d", len(v.Entities))
	}
	if v.Entities[0].GlobalID != wallB {
		t.Errorf("nearest is %s, want wall B", v.Entities[0].GlobalID)
	}
	if v.Entities[0].Depth >= v.Entities[1].Depth {
		t.Errorf("depths %v, %v are not ascending", v.Entities[0].Depth, v.Entities[1].Depth)
	}
	if gap := v.Entities[1].Depth - v.Entities[0].Depth; math.Abs(gap-5) > 1e-6 {
		t.Errorf("depth gap = %v, want the walls' 5 m separation", gap)
	}
}

// TestElevationBoundsCoverEveryLoop: the union of both walls is 4 m wide and
// as tall as the taller wall.
func TestElevationBoundsCoverEveryLoop(t *testing.T) {
	_, _, _, v := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	want := [2][2]float64{{-2, 0}, {2, 4}}
	if v.Bounds != want {
		t.Errorf("Bounds = %v, want %v", v.Bounds, want)
	}
}

// TestElevationFollowsTheFacingSign pins the membership rule AND the limit that
// comes with it. Both walls here are freestanding, so both sides reach open air
// and BuildFacings resolves the outward sign arbitrarily at low Confidence —
// which puts both on ONE of the two opposite elevations and neither on the
// other. That is honest rather than correct, and Confidence is the signal a
// consumer thresholds on before binning a wall to a compass elevation, because
// in a quantity context a wrong bin is a wrong invoice.
func TestElevationFollowsTheFacingSign(t *testing.T) {
	s, f, r, front := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	back := s.ElevationOn(f, r, mustElevationPlane(t, [3]float64{0, -1, 0}))

	if len(front.Entities)+len(back.Entities) != 2 {
		t.Errorf("each wall belongs to exactly one of the two opposite views, got %d + %d",
			len(front.Entities), len(back.Entities))
	}
	for gid, fa := range BuildFacings(s.Elements) {
		if fa.Confidence >= 0.5 {
			t.Errorf("%s: Confidence %v — fixture no longer states the ambiguous-sign case", gid, fa.Confidence)
		}
	}
}

// TestElevationIsDeterministic: identical input yields an identical view, so a
// consumer diffing drawings across imports sees no false churn.
func TestElevationIsDeterministic(t *testing.T) {
	s, f, r, a := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	b := s.ElevationOn(f, r, mustElevationPlane(t, [3]float64{0, 1, 0}))
	if !reflect.DeepEqual(a, b) {
		t.Error("ElevationOn is not deterministic")
	}
}

// TestElevationOnInvalidPlane: no entities rather than wrongly-wound ones.
func TestElevationOnInvalidPlane(t *testing.T) {
	s, f, r, _ := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	v := s.ElevationOn(f, r, Plane{})
	if len(v.Entities) != 0 {
		t.Errorf("want no entities for an invalid basis, got %d", len(v.Entities))
	}
}

func mustElevationPlane(t *testing.T, dir [3]float64) Plane {
	t.Helper()
	p, ok := ElevationPlane(dir)
	if !ok {
		t.Fatalf("no elevation plane for %v", dir)
	}
	return p
}

func TestElevations_MatchesElevationOnPerPlane(t *testing.T) {
	// buildElevation is the fixture helper already in this file; twoWalls is its
	// const. The fourth return (a view) is not needed here — we build the planes
	// explicitly below so the singular and plural calls see the same ones.
	s, f, r, _ := buildElevation(t, twoWalls, [3]float64{0, 1, 0})

	north, ok := ElevationPlane([3]float64{0, 1, 0})
	if !ok {
		t.Fatal("ElevationPlane(north): ok=false")
	}
	east, ok := ElevationPlane([3]float64{1, 0, 0})
	if !ok {
		t.Fatal("ElevationPlane(east): ok=false")
	}
	planes := []Plane{north, east}

	got := s.Elevations(f, r, planes)
	if len(got) != len(planes) {
		t.Fatalf("Elevations = %d views, want %d", len(got), len(planes))
	}
	for i, p := range planes {
		want := s.ElevationOn(f, r, p)
		if len(got[i].Entities) != len(want.Entities) {
			t.Fatalf("view %d: %d entities, want %d", i, len(got[i].Entities), len(want.Entities))
		}
		for j := range want.Entities {
			if got[i].Entities[j].GlobalID != want.Entities[j].GlobalID {
				t.Fatalf("view %d entity %d: GlobalID %q, want %q",
					i, j, got[i].Entities[j].GlobalID, want.Entities[j].GlobalID)
			}
			if got[i].Entities[j].Depth != want.Entities[j].Depth {
				t.Fatalf("view %d entity %d: Depth %v, want %v",
					i, j, got[i].Entities[j].Depth, want.Entities[j].Depth)
			}
		}
		if got[i].Bounds != want.Bounds {
			t.Fatalf("view %d: Bounds %v, want %v", i, got[i].Bounds, want.Bounds)
		}
	}
}

func TestElevations_NoPlanesIsEmpty(t *testing.T) {
	s, f, r, _ := buildElevation(t, twoWalls, [3]float64{0, 1, 0})
	if got := s.Elevations(f, r, nil); len(got) != 0 {
		t.Fatalf("Elevations(nil planes) = %d views, want 0", len(got))
	}
}
