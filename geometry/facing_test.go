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

func TestFaceAreaIsOneSideAndElevationsSumToTheFacade(t *testing.T) {
	// A 6x4 room, 0.3m walls, 3m tall. Each wall presents exactly ONE outward
	// face: 6x3 south and north, 4x3 west and east — 60 m² of facade in total.
	//
	// The axis vote folds antipodes, so the area that WON the vote counts each
	// wall's inner face too and totals 120. That number is a vote weight and was
	// never summable per elevation; FaceArea is measured on one side, which is
	// what makes this sum meaningful at all.
	facings := BuildFacings(roomWalls(6, 4, 0.3))

	want := map[string]float64{"s": 18, "n": 18, "w": 12, "e": 12}
	var total float64
	for _, id := range []string{"s", "n", "w", "e"} {
		f, ok := facings[id]
		if !ok {
			t.Fatalf("wall %q has no facing", id)
		}
		if math.Abs(f.FaceArea-want[id]) > 1e-6 {
			t.Fatalf("wall %q FaceArea = %v, want %v (one side, not both)",
				id, f.FaceArea, want[id])
		}
		total += f.FaceArea
	}
	if math.Abs(total-60) > 1e-6 {
		t.Fatalf("the four elevations sum to %v m², want the 60 m² facade", total)
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
	if math.Abs(want.FaceArea-got.FaceArea) > eps {
		t.Fatalf("FaceArea = %v, want %v", got.FaceArea, want.FaceArea)
	}
	if got.Exposure != want.Exposure {
		t.Fatalf("Exposure = %v, want %v", got.Exposure, want.Exposure)
	}
	if math.Abs(want.Confidence-got.Confidence) > eps {
		t.Fatalf("Confidence = %v, want %v", got.Confidence, want.Confidence)
	}
}

func TestAzimuthRejectsNaNInputs(t *testing.T) {
	// A NaN bearing passes every range check downstream — it is neither < 0 nor
	// >= 360 — so it propagates silently into whatever bins on it.
	nan := math.NaN()

	f := Facing{Normal: [3]float64{0, 1, 0}}
	if got := f.Azimuth([2]float64{nan, nan}); got != 0 {
		t.Fatalf("NaN true north gave bearing %v, want the (0,1) fallback", got)
	}
	bad := Facing{Normal: [3]float64{nan, 1, 0}}
	if got := bad.Azimuth([2]float64{0, 1}); got != 0 {
		t.Fatalf("NaN normal gave bearing %v, want 0", got)
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

// prismElem extrudes a counter-clockwise plan polygon to height h, wound
// outward, so each side's area lands on the side its normal names.
func prismElem(id string, plan [][2]float64, h float64) Element {
	n := len(plan)
	var w []v3
	for _, p := range plan {
		w = append(w, v3{p[0], p[1], 0})
	}
	for _, p := range plan {
		w = append(w, v3{p[0], p[1], h})
	}
	var tris []uint32
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		bi, bj, ti, tj := uint32(i), uint32(j), uint32(i+n), uint32(j+n)
		tris = append(tris, bi, bj, tj, bi, tj, ti)
	}
	for i := 1; i+1 < n; i++ {
		tris = append(tris, 0, uint32(i+1), uint32(i))
		tris = append(tris, uint32(n), uint32(n+i), uint32(n+i+1))
	}
	var verts []float32
	min, max := w[0], w[0]
	for _, p := range w {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
		for k := 0; k < 3; k++ {
			min[k], max[k] = math.Min(min[k], p[k]), math.Max(max[k], p[k])
		}
	}
	return Element{GlobalID: id, Verts: verts, Tris: tris,
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   [3]float64(min), BBoxMax: [3]float64(max)}
}

func TestBackAreaIsTheFarSideOfAMitredRoom(t *testing.T) {
	// A 6x4 room, 0.3m walls, 3m tall, mitred at the corners: every wall's
	// outer face runs the full side and its inner face is 0.6m shorter. The
	// far side is where a layer behind the structure goes, so it must read
	// the inner length, not a copy of the outer one.
	const tk, h = 0.3, 3.0
	walls := []Element{
		prismElem("s", [][2]float64{{0, 0}, {6, 0}, {6 - tk, tk}, {tk, tk}}, h),
		prismElem("n", [][2]float64{{tk, 4 - tk}, {6 - tk, 4 - tk}, {6, 4}, {0, 4}}, h),
		prismElem("w", [][2]float64{{0, 0}, {tk, tk}, {tk, 4 - tk}, {0, 4}}, h),
		prismElem("e", [][2]float64{{6, 0}, {6, 4}, {6 - tk, 4 - tk}, {6 - tk, tk}}, h),
	}
	facings := BuildFacings(walls)
	want := map[string][2]float64{"s": {18, 16.2}, "n": {18, 16.2}, "w": {12, 10.2}, "e": {12, 10.2}}
	for id, w := range want {
		f, ok := facings[id]
		if !ok {
			t.Fatalf("wall %q has no facing", id)
		}
		if f.Exposure != ExposureExterior {
			t.Fatalf("wall %q exposure = %q, want exterior", id, f.Exposure)
		}
		if math.Abs(f.FaceArea-w[0]) > 1e-6 || math.Abs(f.BackArea-w[1]) > 1e-6 {
			t.Fatalf("wall %q FaceArea/BackArea = %v/%v, want %v/%v", id, f.FaceArea, f.BackArea, w[0], w[1])
		}
	}
}
