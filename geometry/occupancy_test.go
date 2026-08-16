package geometry

import "testing"

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

func TestOccupancyOutsideReachesBorderNotRoom(t *testing.T) {
	g := buildOccupancy(roomWalls(6, 4, 0.3), 1.5)
	if g == nil {
		t.Fatal("buildOccupancy returned nil for a real slice")
	}
	// A point well outside the walls is open air.
	if got := g.probe(v3two(-1, -1), [2]float64{-1, 0}); got != sideOffGrid && got != sideOutside {
		t.Fatalf("outside the building = %v, want outside air", got)
	}
}

func TestOccupancyRoomInteriorIsNotOutside(t *testing.T) {
	g := buildOccupancy(roomWalls(6, 4, 0.3), 1.5)
	// Probe from just inside the south wall, heading north into the room: the
	// room is empty but must NOT read as open air.
	got := g.probe(v3two(3, 0.15), [2]float64{0, 1})
	if got == sideOutside || got == sideOffGrid {
		t.Fatalf("room interior = %v, want an enclosed state", got)
	}
}

func TestOccupancyEnclosedVoidIsUncovered(t *testing.T) {
	// A ring of walls with nothing overhead: the void inside is a courtyard.
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

func TestOccupancyNilWhenSliceMissesEverything(t *testing.T) {
	if g := buildOccupancy(roomWalls(6, 4, 0.3), 99); g != nil {
		t.Fatal("buildOccupancy returned a grid for a slice above all geometry")
	}
	if g := buildOccupancy(nil, 1.5); g != nil {
		t.Fatal("buildOccupancy returned a grid for no elements")
	}
}

func v3two(x, y float64) [2]float64 { return [2]float64{x, y} }
