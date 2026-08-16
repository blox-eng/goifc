package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
)

func TestFacingOfDeclinesNonFacadeGeometry(t *testing.T) {
	cases := map[string]Element{
		"square column": elemBox(v3{0, 0, 0}, v3{0.4, 0.4, 3}),
		"slab":          elemBox(v3{0, 0, 0}, v3{10, 10, 0.2}),
		"empty mesh":    {GlobalID: "e"},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := FacingOf(e); ok {
				t.Fatal("FacingOf accepted geometry with no facade")
			}
		})
	}
}

func TestFacingOfAloneIsExteriorAtLowConfidence(t *testing.T) {
	f, ok := FacingOf(elemBox(v3{0, 0, 0}, v3{10, 0.3, 3}))
	if !ok {
		t.Fatal("FacingOf declined a plain wall")
	}
	if f.Exposure != ExposureExterior {
		t.Fatalf("Exposure = %v, want %v for an element with no neighbours", f.Exposure, ExposureExterior)
	}
	if f.Confidence >= 0.5 {
		t.Fatalf("Confidence = %v, want low: a lone element's sign is arbitrary", f.Confidence)
	}
	if math.Abs(math.Abs(f.Normal[1])-1) > 1e-9 {
		t.Fatalf("Normal = %v, want the Y axis", f.Normal)
	}
}

func TestBuildFacingsSignsWallsOutward(t *testing.T) {
	// A closed 6x4 room. Every wall's normal must point AWAY from the room
	// centre (3,2) — that is the whole point of the sign.
	elems := roomWalls(6, 4, 0.3)
	got := BuildFacings(elems)
	if len(got) != 4 {
		t.Fatalf("got %d facings, want 4", len(got))
	}

	centres := map[string][2]float64{
		"s": {3, 0.15}, "n": {3, 3.85}, "w": {0.15, 2}, "e": {5.85, 2},
	}
	for id, f := range got {
		c := centres[id]
		away := [2]float64{c[0] - 3, c[1] - 2}
		if f.Normal[0]*away[0]+f.Normal[1]*away[1] <= 0 {
			t.Fatalf("wall %s normal %v points toward the room centre, not away", id, f.Normal)
		}
		if f.Exposure != ExposureExterior {
			t.Fatalf("wall %s Exposure = %v, want %v", id, f.Exposure, ExposureExterior)
		}
		if f.Confidence < 0.8 {
			t.Fatalf("wall %s Confidence = %v, want a confident answer", id, f.Confidence)
		}
	}
}

func TestBuildFacingsNormalsAreUnit(t *testing.T) {
	for id, f := range BuildFacings(roomWalls(6, 4, 0.3)) {
		l := math.Sqrt(f.Normal[0]*f.Normal[0] + f.Normal[1]*f.Normal[1] + f.Normal[2]*f.Normal[2])
		if math.Abs(l-1) > 1e-9 {
			t.Fatalf("%s normal length %v, want 1", id, l)
		}
	}
}

func TestBuildFacingsSkipsNonFacadeElements(t *testing.T) {
	elems := append(roomWalls(6, 4, 0.3), elemBox(v3{2, 2, 0}, v3{2.4, 2.4, 3}))
	elems[4].GlobalID = "col"
	if _, ok := BuildFacings(elems)["col"]; ok {
		t.Fatal("BuildFacings emitted a facing for a square column")
	}
}

// courtyardBuilding is an outer envelope enclosing a built band, which is
// roofed, wrapped around a sealed inner courtyard, which is not. The outer
// envelope is a 10x8 room (roomWalls); the inner ring walls off a 2x2
// courtyard centred at (5,4). Four slabs roof the band between the two rings
// but leave the courtyard's own footprint, x:[4,6] y:[3,5], uncovered.
//
// This is the fixture I-1 asks for: it is the only place in the suite that
// gives resolveSign's sideEnclosed branches (courtyard vs. covered band) two
// genuinely different sides to tell apart, so it is what proves the courtyard
// sign — not just the exposure — is not inverted.
func courtyardBuilding() []Element {
	elems := roomWalls(10, 8, 0.3)

	elems = append(elems,
		namedWall("is", v3{3.7, 2.7, 0}, v3{6.3, 3, 3}), // inner south, faces court +Y
		namedWall("in", v3{3.7, 5, 0}, v3{6.3, 5.3, 3}), // inner north, faces court -Y
		namedWall("iw", v3{3.7, 2.7, 0}, v3{4, 5.3, 3}), // inner west, faces court +X
		namedWall("ie", v3{6, 2.7, 0}, v3{6.3, 5.3, 3}), // inner east, faces court -X
	)

	// Roof the built band as a picture frame around the courtyard, so the
	// band reads sideCovered while the courtyard footprint stays open.
	elems = append(elems,
		namedWall("roofS", v3{0, 0, 3}, v3{10, 3, 3.2}),
		namedWall("roofN", v3{0, 5, 3}, v3{10, 8, 3.2}),
		namedWall("roofW", v3{0, 3, 3}, v3{4, 5, 3.2}),
		namedWall("roofE", v3{6, 3, 3}, v3{10, 5, 3.2}),
	)

	return elems
}

func TestBuildFacingsCourtyardWallsFaceEnclosedCourt(t *testing.T) {
	got := BuildFacings(courtyardBuilding())
	court := [2]float64{5, 4} // courtyard centre

	centres := map[string][2]float64{
		"is": {5, 2.85}, "in": {5, 5.15}, "iw": {3.85, 4}, "ie": {6.15, 4},
	}
	for _, id := range []string{"is", "in", "iw", "ie"} {
		f, ok := got[id]
		if !ok {
			t.Fatalf("inner wall %s has no facing", id)
		}
		if f.Exposure != ExposureEnclosed {
			t.Fatalf("inner wall %s Exposure = %v, want %v", id, f.Exposure, ExposureEnclosed)
		}
		// The load-bearing half: inverting resolveSign's flip changes only
		// the sign of Normal, not Exposure, so this is what actually catches
		// the courtyard arm being backwards.
		c := centres[id]
		toward := [2]float64{court[0] - c[0], court[1] - c[1]}
		if f.Normal[0]*toward[0]+f.Normal[1]*toward[1] <= 0 {
			t.Fatalf("inner wall %s normal %v does not point into the courtyard", id, f.Normal)
		}
	}
}

func TestBuildFacingsRoofedPartitionIsInterior(t *testing.T) {
	elems := roomWalls(6, 4, 0.3)
	elems = append(elems,
		namedWall("roof", v3{0, 0, 3}, v3{6, 4, 3.2}),
		// A partition splitting the roofed room in two: both sides land in the
		// same covered interior, so neither reaches open air or the sky.
		namedWall("p", v3{2.85, 0.3, 0}, v3{3.15, 3.7, 3}),
	)

	got := BuildFacings(elems)
	f, ok := got["p"]
	if !ok {
		t.Fatal("partition has no facing")
	}
	if f.Exposure != ExposureInterior {
		t.Fatalf("partition Exposure = %v, want %v", f.Exposure, ExposureInterior)
	}
}

// rotatedSouthWall builds the same south wall as roomWalls(6, 4, 0.3) — the
// world box (0,0,0)-(6,0.3,3) — but authored the way a real IFC element often
// is: a local mesh not aligned with world axes, lifted into place by
// Placement. Locally the wall is Y-long (0.3 x 6 x 3); Placement rotates it
// -90° about Z and translates so the transformed box lands exactly on the
// original's world AABB.
//
// This is what proves worldPoints(e.Verts, e.Placement) in facingWithin is
// load-bearing: every other fixture uses elemBox, which is identity
// placement, so a caller that quietly used local Verts directly would still
// pass every other test.
func rotatedSouthWall() Element {
	local, tris := boxMeshWorld(v3{0, 0, 0}, v3{0.3, 6, 3})
	var verts []float32
	for _, p := range local {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	// x' = y, y' = -x + 0.3, z' = z — a -90° rotation about Z plus the
	// translation needed to land on (0,0,0)-(6,0.3,3).
	placement := model.Mat4{
		0, -1, 0, 0,
		1, 0, 0, 0,
		0, 0, 1, 0,
		0, 0.3, 0, 1,
	}
	return Element{
		GlobalID:  "s",
		Verts:     verts,
		Tris:      tris,
		Placement: placement,
		BBoxMin:   [3]float64{0, 0, 0},
		BBoxMax:   [3]float64{6, 0.3, 3},
	}
}

func TestBuildFacingsNonIdentityPlacementMatchesWorldTwin(t *testing.T) {
	identity := roomWalls(6, 4, 0.3)
	rotated := append([]Element{rotatedSouthWall()}, identity[1:]...)

	wantFacings := BuildFacings(identity)
	gotFacings := BuildFacings(rotated)

	want, ok := wantFacings["s"]
	if !ok {
		t.Fatal("identity-placement south wall has no facing")
	}
	got, ok := gotFacings["s"]
	if !ok {
		t.Fatal("rotated-placement south wall has no facing")
	}

	const eps = 1e-6
	for i := range want.Normal {
		if math.Abs(want.Normal[i]-got.Normal[i]) > eps {
			t.Fatalf("Normal = %v, want %v (identity twin)", got.Normal, want.Normal)
		}
	}
	if math.Abs(want.VoteArea-got.VoteArea) > eps {
		t.Fatalf("VoteArea = %v, want %v", got.VoteArea, want.VoteArea)
	}
	if got.Exposure != want.Exposure {
		t.Fatalf("Exposure = %v, want %v", got.Exposure, want.Exposure)
	}
	if math.Abs(want.Confidence-got.Confidence) > eps {
		t.Fatalf("Confidence = %v, want %v", got.Confidence, want.Confidence)
	}
}

func TestAzimuth(t *testing.T) {
	north := [2]float64{0, 1}
	cases := []struct {
		name string
		n    [3]float64
		want float64
	}{
		{"north", [3]float64{0, 1, 0}, 0},
		{"east", [3]float64{1, 0, 0}, 90},
		{"south", [3]float64{0, -1, 0}, 180},
		{"west", [3]float64{-1, 0, 0}, 270},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Facing{Normal: c.n}.Azimuth(north)
			if math.Abs(got-c.want) > 1e-9 {
				t.Fatalf("Azimuth = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAzimuthShiftsWithTrueNorth(t *testing.T) {
	// Rotating north by 90° (north becomes +X) shifts every bearing by -90°.
	got := Facing{Normal: [3]float64{1, 0, 0}}.Azimuth([2]float64{1, 0})
	if math.Abs(got) > 1e-9 {
		t.Fatalf("Azimuth = %v, want 0 when the facing IS north", got)
	}
}

func TestAzimuthRangeAndDegenerate(t *testing.T) {
	for _, n := range [][3]float64{{0, 1, 0}, {1, 1, 0}, {-1, -0.001, 0}} {
		got := Facing{Normal: n}.Azimuth([2]float64{0, 1})
		if got < 0 || got >= 360 {
			t.Fatalf("Azimuth = %v, want [0,360)", got)
		}
	}
	// A purely vertical normal has no bearing; 0 is the documented answer.
	if got := (Facing{Normal: [3]float64{0, 0, 1}}).Azimuth([2]float64{0, 1}); got != 0 {
		t.Fatalf("Azimuth = %v, want 0 for a vertical normal", got)
	}
}
