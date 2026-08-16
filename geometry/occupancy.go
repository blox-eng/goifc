package geometry

import "math"

// occupancyCell is the slice grid resolution in metres. It is a constant rather
// than a caller knob because it is the one number that decides this method's
// failure mode, and a caller has no way to pick it better than the library can:
// a gap narrower than a cell seals, an element thinner than a cell vanishes.
// 10 cm sits below any real wall thickness and any real doorway width.
const occupancyCell = 0.1

// occupancyPad is the ring of empty cells added around the model's extent, so
// the flood fill always starts from cells that are genuinely outside the
// building rather than from a border cell a wall happens to touch.
const occupancyPad = 3

// occupancyMaxCells caps the grid so a georeferenced or site-scale model cannot
// allocate unboundedly. Beyond it the slice is refused (nil) and every element
// on it degrades to low confidence, which is the honest failure.
const occupancyMaxCells = 40 << 20

// sideState is what a probe found on one side of an element.
type sideState int

const (
	// sideSolid — the probe never left building fabric.
	sideSolid sideState = iota
	// sideOutside — the probe reached air connected to the world border.
	sideOutside
	// sideOffGrid — the probe walked off the grid, which is outside air.
	sideOffGrid
	// sideEnclosed — air the building encloses, with nothing overhead: a
	// courtyard, a lightwell. Weather-exposed but on no elevation.
	sideEnclosed
	// sideCovered — air the building encloses, with geometry overhead: a room.
	sideCovered
)

// isOpenAir reports whether s means the outdoors proper.
func (s sideState) isOpenAir() bool { return s == sideOutside || s == sideOffGrid }

// occupancy is one horizontal slice of a model, rasterized.
//
// solid   — cells the building's cross-section occupies at this height.
// outside — empty cells reachable from the grid border, i.e. open air.
// covered — cells with geometry ABOVE the slice. This is what separates a
//
//	courtyard (enclosed, uncovered) from a room (enclosed, covered).
//	At a single height the two are identical: both are voids ringed by
//	walls. Only what is overhead tells them apart.
type occupancy struct {
	minX, minY float64
	nx, ny     int
	solid      []bool
	outside    []bool
	covered    []bool
}

// buildOccupancy rasterizes the model's cross-section at height z, floods open
// air inward from the border, and marks what has geometry overhead.
//
// Returns nil when the slice misses all geometry, or when the extent would need
// more than occupancyMaxCells.
func buildOccupancy(elems []Element, z float64) *occupancy {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	any := false
	for i := range elems {
		e := &elems[i]
		if len(e.Tris) == 0 {
			continue
		}
		// Include everything at or above the slice: geometry below cannot be
		// overhead, and geometry crossing z contributes solid cells.
		if e.BBoxMax[2] < z {
			continue
		}
		minX, minY = math.Min(minX, e.BBoxMin[0]), math.Min(minY, e.BBoxMin[1])
		maxX, maxY = math.Max(maxX, e.BBoxMax[0]), math.Max(maxY, e.BBoxMax[1])
		any = true
	}
	if !any || !finite3(v3{minX, minY, 0}) || !finite3(v3{maxX, maxY, 0}) {
		return nil
	}

	pad := occupancyPad * occupancyCell
	g := &occupancy{minX: minX - pad, minY: minY - pad}
	g.nx = int((maxX-minX+2*pad)/occupancyCell) + 1
	g.ny = int((maxY-minY+2*pad)/occupancyCell) + 1
	if g.nx <= 0 || g.ny <= 0 || g.nx*g.ny > occupancyMaxCells {
		return nil
	}
	g.solid = make([]bool, g.nx*g.ny)
	g.covered = make([]bool, g.nx*g.ny)

	plane := HorizontalPlane(z)
	sliced := false
	for i := range elems {
		e := &elems[i]
		if len(e.Tris) == 0 {
			continue
		}
		w := worldPoints(e.Verts, e.Placement)

		// Solid: the actual cross-section at z. Rings, not bounding boxes — an
		// L-shaped element's box would fill its own concavity and could seal a
		// courtyard that is really open.
		if e.BBoxMin[2] <= z && e.BBoxMax[2] >= z {
			for _, ring := range sectionRings(w, e.Tris, plane) {
				g.markRing(ring)
				sliced = true
			}
		}

		// Covered: the XY footprint of anything wholly above the slice.
		if e.BBoxMin[2] > z {
			g.markFootprint(w, e.Tris)
		}
	}
	if !sliced {
		return nil
	}

	g.floodOutside()
	return g
}

// idx returns the flat index of cell (ix,iy).
func (g *occupancy) idx(ix, iy int) int { return iy*g.nx + ix }

// cellOf maps a world XY point to a cell, reporting in=false when it lies off
// the grid.
func (g *occupancy) cellOf(x, y float64) (ix, iy int, in bool) {
	ix = int((x - g.minX) / occupancyCell)
	iy = int((y - g.minY) / occupancyCell)
	if ix < 0 || iy < 0 || ix >= g.nx || iy >= g.ny {
		return 0, 0, false
	}
	return ix, iy, true
}

// markRing rasterizes a closed ring's edges as solid. Edges rather than the
// filled interior: the ring is a barrier to the flood fill, and filling would
// also fill the inside of a hollow element.
func (g *occupancy) markRing(ring [][2]float64) {
	for i := range ring {
		a := ring[i]
		b := ring[(i+1)%len(ring)]
		g.markSegment(a, b)
	}
}

// markSegment marks every cell a segment passes through, sampling at half a
// cell so no cell is stepped over.
func (g *occupancy) markSegment(a, b [2]float64) {
	dx, dy := b[0]-a[0], b[1]-a[1]
	l := math.Hypot(dx, dy)
	if !(l > 0) || math.IsInf(l, 0) {
		if ix, iy, in := g.cellOf(a[0], a[1]); in {
			g.solid[g.idx(ix, iy)] = true
		}
		return
	}
	steps := int(l/(occupancyCell/2)) + 1
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		if ix, iy, in := g.cellOf(a[0]+dx*t, a[1]+dy*t); in {
			g.solid[g.idx(ix, iy)] = true
		}
	}
}

// markFootprint marks the filled XY projection of a mesh as covered. Filled,
// not outlined: "is there a roof over this point" is a question about the
// interior of the projection.
func (g *occupancy) markFootprint(w []v3, tris []uint32) {
	for i := 0; i+2 < len(tris); i += 3 {
		ia, ib, ic := tris[i], tris[i+1], tris[i+2]
		if int(ia) >= len(w) || int(ib) >= len(w) || int(ic) >= len(w) {
			continue
		}
		g.fillTriangle2D(
			[2]float64{w[ia][0], w[ia][1]},
			[2]float64{w[ib][0], w[ib][1]},
			[2]float64{w[ic][0], w[ic][1]},
		)
	}
}

// fillTriangle2D marks every cell whose centre lies inside triangle abc.
func (g *occupancy) fillTriangle2D(a, b, c [2]float64) {
	minX := math.Min(a[0], math.Min(b[0], c[0]))
	maxX := math.Max(a[0], math.Max(b[0], c[0]))
	minY := math.Min(a[1], math.Min(b[1], c[1]))
	maxY := math.Max(a[1], math.Max(b[1], c[1]))
	if math.IsNaN(minX) || math.IsNaN(minY) || math.IsInf(maxX, 0) || math.IsInf(maxY, 0) {
		return
	}
	area := edge2D(a, b, c)
	if math.Abs(area) < 1e-15 {
		return // degenerate in plan
	}
	x0, y0, _ := g.clampCell(minX, minY)
	x1, y1, _ := g.clampCell(maxX, maxY)
	for iy := y0; iy <= y1; iy++ {
		for ix := x0; ix <= x1; ix++ {
			px := g.minX + (float64(ix)+0.5)*occupancyCell
			py := g.minY + (float64(iy)+0.5)*occupancyCell
			p := [2]float64{px, py}
			w0 := edge2D(b, c, p) / area
			w1 := edge2D(c, a, p) / area
			w2 := edge2D(a, b, p) / area
			if w0 >= 0 && w1 >= 0 && w2 >= 0 {
				g.covered[g.idx(ix, iy)] = true
			}
		}
	}
}

// edge2D is twice the signed area of triangle abc in plan.
func edge2D(a, b, c [2]float64) float64 {
	return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
}

// clampCell maps a point to a cell index clamped into the grid.
func (g *occupancy) clampCell(x, y float64) (ix, iy int, in bool) {
	ix = int((x - g.minX) / occupancyCell)
	iy = int((y - g.minY) / occupancyCell)
	if ix < 0 {
		ix = 0
	}
	if iy < 0 {
		iy = 0
	}
	if ix >= g.nx {
		ix = g.nx - 1
	}
	if iy >= g.ny {
		iy = g.ny - 1
	}
	return ix, iy, true
}

// floodOutside breadth-first fills every empty cell reachable from the grid
// border. The reached set is open air; empty cells it never reaches are voids
// the building encloses.
func (g *occupancy) floodOutside() {
	g.outside = make([]bool, g.nx*g.ny)
	queue := make([]int, 0, g.nx+g.ny)

	push := func(ix, iy int) {
		i := g.idx(ix, iy)
		if g.solid[i] || g.outside[i] {
			return
		}
		g.outside[i] = true
		queue = append(queue, i)
	}
	for ix := 0; ix < g.nx; ix++ {
		push(ix, 0)
		push(ix, g.ny-1)
	}
	for iy := 0; iy < g.ny; iy++ {
		push(0, iy)
		push(g.nx-1, iy)
	}

	// 4-connected on purpose: an 8-connected fill leaks diagonally through a
	// wall that meets another only at a corner, which would report a sealed
	// room as outdoors.
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		ix, iy := i%g.nx, i/g.nx
		if ix > 0 {
			push(ix-1, iy)
		}
		if ix+1 < g.nx {
			push(ix+1, iy)
		}
		if iy > 0 {
			push(ix, iy-1)
		}
		if iy+1 < g.ny {
			push(ix, iy+1)
		}
	}
}

// probe walks from a world XY point along dir, steps over the solid fabric it
// starts in, and reports the first air it reaches.
func (g *occupancy) probe(from [2]float64, dir [2]float64) sideState {
	l := math.Hypot(dir[0], dir[1])
	if !(l > 0) {
		return sideSolid
	}
	ux, uy := dir[0]/l, dir[1]/l
	step := occupancyCell / 2
	maxSteps := 2 * (g.nx + g.ny)

	for s := 1; s <= maxSteps; s++ {
		x := from[0] + ux*float64(s)*step
		y := from[1] + uy*float64(s)*step
		ix, iy, in := g.cellOf(x, y)
		if !in {
			return sideOffGrid
		}
		i := g.idx(ix, iy)
		if g.solid[i] {
			continue // still inside fabric; keep walking
		}
		switch {
		case g.outside[i]:
			return sideOutside
		case g.covered[i]:
			return sideCovered
		default:
			return sideEnclosed
		}
	}
	return sideSolid
}
