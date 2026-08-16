package geometry

import (
	"math"
	"testing"
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
