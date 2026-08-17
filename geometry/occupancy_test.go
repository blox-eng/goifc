package geometry

import (
	"math"
	"testing"
)

// steppedPlinth is ONE element shaped like a stem standing on a wider pad: a
// 4x4 pad from z=0 to 1, and a 1x1 stem from z=1 to 3 rising out of it. Sliced
// at 1.5 m, only the stem is cross-section — the pad is entirely underfoot.
func steppedPlinth() Element {
	e := elemBox(v3{0, 0, 0}, v3{4, 4, 3}) // placement and bbox
	padW, padT := boxMeshWorld(v3{0, 0, 0}, v3{4, 4, 1})
	stemW, stemT := boxMeshWorld(v3{1.5, 1.5, 1}, v3{2.5, 2.5, 3})

	var verts []float32
	for _, p := range append(append([]v3{}, padW...), stemW...) {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	off := uint32(len(padW))
	tris := append([]uint32{}, padT...)
	for _, i := range stemT {
		tris = append(tris, i+off)
	}
	e.GlobalID, e.Verts, e.Tris = "plinth", verts, tris
	return e
}

func TestMarkFootprintIgnoresGeometryBelowTheCut(t *testing.T) {
	g := buildOccupancy([]Element{steppedPlinth()}, 1.5)
	if g == nil {
		t.Fatal("buildOccupancy returned nil for an element the slice cuts")
	}

	// (0.5, 0.5) sits over the pad but well clear of the stem. The pad is half a
	// metre BELOW the cut, so nothing is overhead there. Projecting the whole
	// mesh marks it roofed, which is what turns a courtyard into a room.
	ix, iy, in := g.cellOf(0.5, 0.5)
	if !in {
		t.Fatal("a point over the pad fell off the grid")
	}
	if g.covered[g.idx(ix, iy)] {
		t.Fatal("a pad below the cut marked the ground beside the stem as covered")
	}

	// The stem genuinely is overhead at its own footprint.
	ix, iy, in = g.cellOf(2, 2)
	if !in {
		t.Fatal("the stem centre fell off the grid")
	}
	if !g.covered[g.idx(ix, iy)] {
		t.Fatal("geometry above the cut is not marked covered")
	}
}

func TestBuildOccupancyExtentFollowsTheCrossSectionOnly(t *testing.T) {
	// A 1 m cube 20 km away, sitting entirely ABOVE the slice, contributes no
	// solid cells whatsoever. Sizing the grid to reach it needs ~4e10 cells,
	// past occupancyMaxCells — so buildOccupancy refuses the slice and every
	// wall on the band falls back to an arbitrary sign at low confidence,
	// because of geometry that could never have influenced the answer.
	elems := roomWalls(6, 4, 0.3)
	far := elemBox(v3{20000, 20000, 5}, v3{20001, 20001, 6})
	far.GlobalID = "far"
	elems = append(elems, far)

	g := buildOccupancy(elems, 1.5)
	if g == nil {
		t.Fatal("buildOccupancy refused a 6x4 room because of a distant element above it")
	}
	if g.nx > 200 || g.ny > 200 {
		t.Fatalf("grid is %dx%d cells, want it sized to the room", g.nx, g.ny)
	}

	// The walls must still resolve decisively, which is the point of the fix.
	facings := BuildFacings(elems)
	for _, id := range []string{"s", "n", "w", "e"} {
		f, ok := facings[id]
		if !ok {
			t.Fatalf("wall %q has no facing", id)
		}
		if f.Exposure != ExposureExterior {
			t.Fatalf("wall %q Exposure = %v, want exterior", id, f.Exposure)
		}
		if f.Confidence < 0.5 {
			t.Fatalf("wall %q Confidence = %v, want a resolved sign", id, f.Confidence)
		}
	}
}

// roomWalls returns four walls enclosing [0,w]x[0,d] with the given thickness,
// each 3m tall. Each wall gets a distinct GlobalID (south/north/west/east)
// because a later task keys results by GlobalID; elemBox alone would give
// them all the same id.
func roomWalls(w, d, t float64) []Element {
	south := elemBox(v3{0, 0, 0}, v3{w, t, 3})
	south.GlobalID = "s"
	north := elemBox(v3{0, d - t, 0}, v3{w, d, 3})
	north.GlobalID = "n"
	west := elemBox(v3{0, 0, 0}, v3{t, d, 3})
	west.GlobalID = "w"
	east := elemBox(v3{w - t, 0, 0}, v3{w, d, 3})
	east.GlobalID = "e"
	return []Element{south, north, west, east}
}

// roomWallsWithGap is roomWalls but the south wall is split into two segments
// leaving a doorway [gapStart, gapStart+gapWidth) open, wider than one cell.
func roomWallsWithGap(w, d, t, gapStart, gapWidth float64) []Element {
	south1 := elemBox(v3{0, 0, 0}, v3{gapStart, t, 3})
	south1.GlobalID = "s1"
	south2 := elemBox(v3{gapStart + gapWidth, 0, 0}, v3{w, t, 3})
	south2.GlobalID = "s2"
	north := elemBox(v3{0, d - t, 0}, v3{w, d, 3})
	north.GlobalID = "n"
	west := elemBox(v3{0, 0, 0}, v3{t, d, 3})
	west.GlobalID = "w"
	east := elemBox(v3{w - t, 0, 0}, v3{w, d, 3})
	east.GlobalID = "e"
	return []Element{south1, south2, north, west, east}
}

func TestOccupancyOutsideReachesBorderNotRoom(t *testing.T) {
	g := buildOccupancy(roomWalls(6, 4, 0.3), 1.5)
	if g == nil {
		t.Fatal("buildOccupancy returned nil for a real slice")
	}
	// A point well outside the walls is open air.
	if got := g.probe(v3two(-1, -1), [2]float64{-1, 0}); got != sideOffGrid && got != sideOutside {
		t.Fatalf("outside the building = %v, want outside air", got)
	}
	// Probing from inside the south wall, heading further outward (away from
	// the room), must escape to open air — not stall inside the wall's own
	// fabric. This is the case C-1 broke: with only ring edges marked solid,
	// a probe starting inside the wall's body never left its unmarked
	// interior and reported the room's own state instead of the outdoors.
	if got := g.probe(v3two(3, 0.15), [2]float64{0, -1}); got != sideOutside && got != sideOffGrid {
		t.Fatalf("probe outward through the south wall = %v, want outside", got)
	}
}

func TestOccupancyEnclosedVoidIsUncovered(t *testing.T) {
	// A ring of walls with nothing overhead: the void inside is a courtyard.
	// Probe from just inside the south wall, heading north into the room: the
	// room is empty but must read as an enclosed void, not open air.
	g := buildOccupancy(roomWalls(6, 4, 0.3), 1.5)
	if got := g.probe(v3two(3, 0.15), [2]float64{0, 1}); got != sideEnclosed {
		t.Fatalf("uncovered void = %v, want sideEnclosed", got)
	}
}

func TestOccupancyCoveredVoidIsRoom(t *testing.T) {
	// Same ring, plus a slab spanning it above the slice: now it is a room.
	elems := append(roomWalls(6, 4, 0.3), elemBox(v3{0, 0, 3}, v3{6, 4, 3.2}))
	g := buildOccupancy(elems, 1.5)
	if got := g.probe(v3two(3, 0.15), [2]float64{0, 1}); got != sideCovered {
		t.Fatalf("roofed void = %v, want sideCovered", got)
	}
}

func TestOccupancyStraddlingSlabStillCovers(t *testing.T) {
	// A slab that straddles the slice height (spans below AND above z) must
	// still mark the room below it as covered: it plainly has a roof, even
	// though the slab's own BBoxMin sits below z.
	elems := append(roomWalls(6, 4, 0.3), elemBox(v3{0, 0, 1.4}, v3{6, 4, 3.2}))
	g := buildOccupancy(elems, 1.0)
	if got := g.probe(v3two(3, 0.15), [2]float64{0, 1}); got != sideCovered {
		t.Fatalf("room under a straddling slab = %v, want sideCovered", got)
	}
}

func TestOccupancyDoorwayConnectsRoomToOutside(t *testing.T) {
	// A doorway gap wider than one cell must flood-connect the room to the
	// outside: without it (roomWalls, sealed) the same probe reads
	// sideEnclosed, as TestOccupancyEnclosedVoidIsUncovered checks.
	elems := roomWallsWithGap(6, 4, 0.3, 2, 0.5)
	g := buildOccupancy(elems, 1.5)
	// Probe from inside the north wall, heading south into the room — far
	// from the doorway in x, but the flood fill floods the whole connected
	// room, not just the cells nearest the gap.
	got := g.probe(v3two(3, 3.85), [2]float64{0, -1})
	if got != sideOutside {
		t.Fatalf("room with an open doorway = %v, want sideOutside", got)
	}
}

func TestOccupancyNilWhenSliceMissesEverything(t *testing.T) {
	if g := buildOccupancy(roomWalls(6, 4, 0.3), 99); g != nil {
		t.Fatal("buildOccupancy returned a grid for a slice above all geometry")
	}
	if g := buildOccupancy(nil, 1.5); g != nil {
		t.Fatal("buildOccupancy returned a grid for no elements")
	}
}

func TestOccupancyRefusesHugeExtentWithoutPanicking(t *testing.T) {
	// A file-supplied coordinate this large makes nx*ny overflow int. The wrap
	// is not monotonic in the coordinate — whether the wrapped product happens
	// to land above or below occupancyMaxCells depends on the other axis too —
	// so a product-only guard rejects most magnitudes and panics with
	// `makeslice: len out of range` on the rest (2e17 here). This library reads
	// untrusted files, so the sweep is the test, not one lucky number.
	for _, m := range []float64{1e15, 1e16, 5e16, 1e17, 2e17, 3e17, 7e17, 1e18} {
		elems := append(roomWalls(6, 4, 0.3), elemBox(v3{0, 0, 0}, v3{m, 0.3, 3}))
		if g := buildOccupancy(elems, 1.5); g != nil {
			t.Fatalf("buildOccupancy returned a %dx%d grid for a %g m extent", g.nx, g.ny, m)
		}
	}
}

// nanWall is a wall whose bbox never got measured, which is what a degenerate
// or unplaced element looks like coming out of the assembler.
func nanWall(id string) Element {
	e := elemBox(v3{2, 1, 0}, v3{3, 1.3, 3})
	e.GlobalID = id
	nan := math.NaN()
	e.BBoxMin = [3]float64{nan, nan, nan}
	e.BBoxMax = [3]float64{nan, nan, nan}
	return e
}

func TestOccupancyIgnoresNonFiniteBBoxes(t *testing.T) {
	// math.Min(x, NaN) is NaN, so folding one unmeasured element into the
	// extent used to return nil for EVERY band of the model.
	elems := append(roomWalls(6, 4, 0.3), nanWall("nan"))
	g := buildOccupancy(elems, 1.5)
	if g == nil {
		t.Fatal("one NaN bbox disabled the whole slice")
	}
	if got := g.probe(v3two(3, 0.15), [2]float64{0, 1}); got != sideEnclosed {
		t.Fatalf("room interior = %v, want sideEnclosed — the healthy geometry still rasterized", got)
	}
}

func TestBuildFacingsNaNElementDoesNotDegradeNeighbours(t *testing.T) {
	healthy := BuildFacings(roomWalls(6, 4, 0.3))
	withNaN := BuildFacings(append(roomWalls(6, 4, 0.3), nanWall("nan")))

	for id, want := range healthy {
		got, ok := withNaN[id]
		if !ok {
			t.Fatalf("%s vanished once a NaN element joined the model", id)
		}
		if got != want {
			t.Fatalf("%s: %+v != %+v — a NaN neighbour changed a healthy wall", id, got, want)
		}
	}
	// The NaN element itself declines to a coin flip rather than claiming an
	// elevation it cannot have probed for.
	if f, ok := withNaN["nan"]; ok && f.Confidence >= 0.5 {
		t.Fatalf("NaN element Confidence = %v, want low", f.Confidence)
	}
}

func v3two(x, y float64) [2]float64 { return [2]float64{x, y} }
