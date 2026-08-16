package geometry

import (
	"math"
	"testing"
)

// namedWall builds a wall box with an explicit GlobalID.
func namedWall(id string, min, max v3) Element {
	e := elemBox(min, max)
	e.GlobalID = id
	return e
}

func TestFourElevationPartition(t *testing.T) {
	// A closed rectangular room: every wall lands in exactly one of four
	// compass bins, all four bins are non-empty, and no wall is dropped.
	elems := []Element{
		namedWall("s", v3{0, 0, 0}, v3{6, 0.3, 3}),
		namedWall("n", v3{0, 3.7, 0}, v3{6, 4, 3}),
		namedWall("w", v3{0, 0, 0}, v3{0.3, 4, 3}),
		namedWall("e", v3{5.7, 0, 0}, v3{6, 4, 3}),
	}
	got := BuildFacings(elems)
	if len(got) != 4 {
		t.Fatalf("classified %d of 4 walls", len(got))
	}

	bins := map[string]int{}
	for id, f := range got {
		if f.Exposure != ExposureExterior {
			t.Fatalf("wall %s Exposure = %v, want exterior", id, f.Exposure)
		}
		if f.Confidence < 0.8 {
			t.Fatalf("wall %s Confidence = %v; the fill should resolve a closed room outright",
				id, f.Confidence)
		}
		bins[compass(f.Azimuth([2]float64{0, 1}))]++
	}
	for _, dir := range []string{"N", "E", "S", "W"} {
		if bins[dir] != 1 {
			t.Fatalf("bin %s holds %d walls, want exactly 1 (bins: %v)", dir, bins[dir], bins)
		}
	}
}

// compass buckets a bearing into the four cardinal elevations.
func compass(az float64) string {
	switch {
	case az < 45 || az >= 315:
		return "N"
	case az < 135:
		return "E"
	case az < 225:
		return "S"
	default:
		return "W"
	}
}

func TestNonConvexFootprintResolvesCorrectly(t *testing.T) {
	// A U-shaped plan, 10 wide x 8 deep, with a 4-wide recess open to the south.
	// The two walls flanking the recess face east and west INTO the recess. A
	// centroid rule misassigns them: they sit on the far side of the building
	// centre from the direction they actually face.
	//
	//   y=8  +----------------+
	//        |                |
	//   y=4  +-----+    +-----+
	//        |     |    |     |
	//   y=0  +-----+    +-----+
	//        0     3    7     10
	elems := []Element{
		namedWall("north", v3{0, 7.7, 0}, v3{10, 8, 3}),
		namedWall("west", v3{0, 0, 0}, v3{0.3, 8, 3}),
		namedWall("east", v3{9.7, 0, 0}, v3{10, 8, 3}),
		namedWall("southwest", v3{0, 0, 0}, v3{3, 0.3, 3}),
		namedWall("southeast", v3{7, 0, 0}, v3{10, 0.3, 3}),
		// The recess flanks: these are the walls a centroid rule gets wrong.
		namedWall("recessW", v3{2.7, 0, 0}, v3{3, 4, 3}),
		namedWall("recessE", v3{7, 0, 0}, v3{7.3, 4, 3}),
		namedWall("recessN", v3{3, 3.7, 0}, v3{7, 4, 3}),
	}
	got := BuildFacings(elems)

	// recessW is at x≈2.85, east of it is the open recess: it must face +X.
	// The building centroid is at x=5, EAST of the wall, so a centroid rule
	// would point it -X — exactly backwards.
	w, ok := got["recessW"]
	if !ok {
		t.Fatal("recessW was not classified")
	}
	if w.Normal[0] <= 0 {
		t.Fatalf("recessW normal %v, want +X into the recess", w.Normal)
	}

	// recessE is at x≈7.15, west of it is the open recess: it must face -X.
	e, ok := got["recessE"]
	if !ok {
		t.Fatal("recessE was not classified")
	}
	if e.Normal[0] >= 0 {
		t.Fatalf("recessE normal %v, want -X into the recess", e.Normal)
	}

	// The recess is open to the south, so it IS the outdoors, not a courtyard.
	for _, id := range []string{"recessW", "recessE"} {
		if got[id].Exposure != ExposureExterior {
			t.Fatalf("%s Exposure = %v, want exterior: the recess is open", id, got[id].Exposure)
		}
	}
}

func TestEnclosedCourtyardIsNotAnElevation(t *testing.T) {
	// A building ring with a sealed courtyard and a roof over the built fabric
	// but NOT over the court. The court-facing walls must report enclosed, so a
	// consumer summing elevations does not count them.
	var elems []Element
	// Outer shell 12x12.
	elems = append(elems,
		namedWall("outS", v3{0, 0, 0}, v3{12, 0.3, 3}),
		namedWall("outN", v3{0, 11.7, 0}, v3{12, 12, 3}),
		namedWall("outW", v3{0, 0, 0}, v3{0.3, 12, 3}),
		namedWall("outE", v3{11.7, 0, 0}, v3{12, 12, 3}),
	)
	// Court ring 4..8 in both axes.
	elems = append(elems,
		namedWall("crtS", v3{4, 4, 0}, v3{8, 4.3, 3}),
		namedWall("crtN", v3{4, 7.7, 0}, v3{8, 8, 3}),
		namedWall("crtW", v3{4, 4, 0}, v3{4.3, 8, 3}),
		namedWall("crtE", v3{7.7, 4, 0}, v3{8, 8, 3}),
	)
	// Roof over the built ring only: four slabs, none over 4..8 x 4..8.
	elems = append(elems,
		namedWall("roofS", v3{0, 0, 3}, v3{12, 4, 3.2}),
		namedWall("roofN", v3{0, 8, 3}, v3{12, 12, 3.2}),
		namedWall("roofW", v3{0, 4, 3}, v3{4, 8, 3.2}),
		namedWall("roofE", v3{8, 4, 3}, v3{12, 8, 3.2}),
	)
	got := BuildFacings(elems)

	// courtyardCentre is where the court-facing walls must point their Normal:
	// inverting the sign of the courtyard changes the Normal, not the
	// Exposure, so an exposure-only assertion cannot catch a fully inverted
	// courtyard sign. For each court wall, its Normal's horizontal dot
	// product with the vector from the wall's own BBox centre toward the
	// courtyard centre must be positive.
	courtyardCentre := v3{6, 6, 0}
	for _, id := range []string{"crtS", "crtN", "crtW", "crtE"} {
		f, ok := got[id]
		if !ok {
			t.Fatalf("%s was not classified", id)
		}
		if f.Exposure != ExposureEnclosed {
			t.Fatalf("%s Exposure = %v, want enclosed — a sealed court is on no elevation",
				id, f.Exposure)
		}

		e := elementByID(elems, id)
		center := v3{
			(e.BBoxMin[0] + e.BBoxMax[0]) / 2,
			(e.BBoxMin[1] + e.BBoxMax[1]) / 2,
			(e.BBoxMin[2] + e.BBoxMax[2]) / 2,
		}
		toCourt := v3{courtyardCentre[0] - center[0], courtyardCentre[1] - center[1], 0}
		dot := f.Normal[0]*toCourt[0] + f.Normal[1]*toCourt[1]
		if dot <= 0 {
			t.Fatalf("%s Normal %v does not point into the courtyard from centre %v (dot=%v)",
				id, f.Normal, center, dot)
		}
	}
	for _, id := range []string{"outS", "outN", "outW", "outE"} {
		if f := got[id]; f.Exposure != ExposureExterior {
			t.Fatalf("%s Exposure = %v, want exterior", id, f.Exposure)
		}
	}
}

// elementByID finds the element with the given GlobalID, panicking if absent
// — a test helper, so a lookup miss is a bug in the fixture, not a case to
// handle gracefully.
func elementByID(elems []Element, id string) Element {
	for _, e := range elems {
		if e.GlobalID == id {
			return e
		}
	}
	panic("no element with GlobalID " + id)
}

func TestBuildFacingsIsDeterministic(t *testing.T) {
	elems := []Element{
		namedWall("s", v3{0, 0, 0}, v3{6, 0.3, 3}),
		namedWall("n", v3{0, 3.7, 0}, v3{6, 4, 3}),
		namedWall("w", v3{0, 0, 0}, v3{0.3, 4, 3}),
		namedWall("e", v3{5.7, 0, 0}, v3{6, 4, 3}),
	}
	first := BuildFacings(elems)
	for run := 0; run < 25; run++ {
		got := BuildFacings(elems)
		if len(got) != len(first) {
			t.Fatalf("run %d classified %d elements, want %d", run, len(got), len(first))
		}
		for id, f := range first {
			g := got[id]
			// Bit-identical, not approximately equal: any drift here means an
			// ordering dependency, and it will surface as a wall changing
			// elevation between two runs on the same file.
			if g.Normal != f.Normal || g.FaceArea != f.FaceArea ||
				g.Exposure != f.Exposure || g.Confidence != f.Confidence {
				t.Fatalf("run %d, %s: %+v != %+v", run, id, g, f)
			}
		}
	}
}

func TestInteriorPartitionIsNotExterior(t *testing.T) {
	// A partition inside a roofed room reports interior, so it never lands in
	// an elevation bin.
	elems := []Element{
		namedWall("outS", v3{0, 0, 0}, v3{8, 0.3, 3}),
		namedWall("outN", v3{0, 7.7, 0}, v3{8, 8, 3}),
		namedWall("outW", v3{0, 0, 0}, v3{0.3, 8, 3}),
		namedWall("outE", v3{7.7, 0, 0}, v3{8, 8, 3}),
		namedWall("roof", v3{0, 0, 3}, v3{8, 8, 3.2}),
		namedWall("part", v3{3.9, 0.3, 0}, v3{4.1, 7.7, 3}),
	}
	got := BuildFacings(elems)
	p, ok := got["part"]
	if !ok {
		t.Fatal("the partition was not classified")
	}
	if p.Exposure != ExposureInterior {
		t.Fatalf("partition Exposure = %v, want interior", p.Exposure)
	}
	if math.Abs(math.Abs(p.Normal[0])-1) > 1e-9 {
		t.Fatalf("partition Normal = %v, want the X axis", p.Normal)
	}
}
