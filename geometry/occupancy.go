package geometry

import (
	"math"
	"sort"
)

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
	return buildOccupancyWith(elems, z, newWorldCache(elems))
}

// bandGridDims sizes the grid for the model's cross-section at height z WITHOUT
// allocating it, returning the padded grid origin and its extent in cells.
//
// buildOccupancyWith uses it to build the grid; BuildFacings uses bandGridCells
// to learn what a band will cost BEFORE deciding how many bands may be in flight
// at once. Those two answers have to agree — a scheduler working from a copy of
// this arithmetic would silently stop bounding memory the moment either copy
// changed — so there is one implementation and both call it.
//
// ok is false when the slice misses all geometry, or when the extent would need
// more than occupancyMaxCells.
func bandGridDims(elems []Element, z float64) (originX, originY float64, nx, ny int, ok bool) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	any := false
	for i := range elems {
		e := &elems[i]
		if len(e.Tris) == 0 {
			continue
		}
		// One non-finite bbox must not disable the feature model-wide:
		// math.Min(x, NaN) is NaN, so folding it into the extent would poison
		// every band and drop every wall in the model to a coin-flip sign.
		if !finite3(e.BBoxMin) || !finite3(e.BBoxMax) {
			continue
		}
		// ONLY the elements the slice actually cuts. The grid exists to answer
		// "what is on either side of this element at this height", and that is
		// decided entirely by the cross-section — so the cross-section is what
		// the extent has to cover.
		//
		// Sizing it to everything ABOVE the slice instead lets one far-away
		// element that contributes no solid cells at all — a site mesh, a
		// georeferenced terrain surface — stretch the grid past
		// occupancyMaxCells. buildOccupancy then returns nil and EVERY element
		// on the band drops to an arbitrary sign at signAmbiguous confidence,
		// because of geometry that was never going to influence the answer.
		//
		// Elements above the slice still mark `covered` below; they just no
		// longer get a vote on how big the grid is. Their footprints outside it
		// fall away, which is correct — a roof 20 km away covers nothing here.
		if e.BBoxMin[2] > z || e.BBoxMax[2] < z {
			continue
		}
		minX, minY = math.Min(minX, e.BBoxMin[0]), math.Min(minY, e.BBoxMin[1])
		maxX, maxY = math.Max(maxX, e.BBoxMax[0]), math.Max(maxY, e.BBoxMax[1])
		any = true
	}
	if !any || !finite3(v3{minX, minY, 0}) || !finite3(v3{maxX, maxY, 0}) {
		return 0, 0, 0, 0, false
	}

	pad := occupancyPad * occupancyCell
	originX, originY = minX-pad, minY-pad
	nx = int((maxX-minX+2*pad)/occupancyCell) + 1
	ny = int((maxY-minY+2*pad)/occupancyCell) + 1
	// Bound each axis BEFORE multiplying. This library parses untrusted files,
	// so a file-supplied coordinate of 1e17 is reachable input: nx*ny overflows
	// int and wraps to a small positive number, sailing past a product-only
	// guard straight into a makeslice panic.
	// Divide, never multiply, for the product term. Each axis may legally reach
	// occupancyMaxCells (~4.2e7), so their product reaches ~1.8e15 — fine in a
	// 64-bit int, but on a 32-bit build (GOARCH=386, arm, wasm) `int` is 32 bits
	// and it wraps to a small positive number that sails past the cap, straight
	// into the makeslice panic this guard exists to prevent.
	if nx <= 0 || ny <= 0 || nx > occupancyMaxCells || ny > occupancyMaxCells ||
		nx > occupancyMaxCells/ny {
		return 0, 0, 0, 0, false
	}
	return originX, originY, nx, ny, true
}

// bandGridCells reports how many cells buildOccupancyWith would allocate per
// plane for the band at height z, or 0 when it would build no grid at all.
//
// A grid holds several []bool of this length, so this is the unit the aggregate
// memory budget in BuildFacings is denominated in.
func bandGridCells(elems []Element, z float64) int {
	_, _, nx, ny, ok := bandGridDims(elems, z)
	if !ok {
		return 0
	}
	return nx * ny
}

// buildOccupancyWith is buildOccupancy over a caller-owned worldCache, shared
// across every band in one BuildFacings run so a vertex is transformed once per
// model rather than once per band it sits beneath.
func buildOccupancyWith(elems []Element, z float64, wc *worldCache) *occupancy {
	originX, originY, nx, ny, ok := bandGridDims(elems, z)
	if !ok {
		return nil
	}
	g := &occupancy{minX: originX, minY: originY, nx: nx, ny: ny}
	g.solid = make([]bool, g.nx*g.ny)
	g.covered = make([]bool, g.nx*g.ny)

	plane := HorizontalPlane(z)
	sliced := false
	for i := range elems {
		e := &elems[i]
		if len(e.Tris) == 0 {
			continue
		}
		// Test the z range before transforming. An element wholly below the
		// slice marks nothing, yet transforming it costs a full vertex
		// allocation — and BuildFacings builds one grid per band, so that waste
		// multiplies by the band count.
		crosses := e.BBoxMin[2] <= z && e.BBoxMax[2] >= z
		above := e.BBoxMax[2] > z
		if !crosses && !above {
			continue
		}
		w := wc.at(i)

		// Solid: the actual cross-section at z, filled per element (even-odd
		// over that element's own rings, so a genuinely hollow element keeps
		// its hole) — not bounding boxes, which would fill an L-shaped
		// element's concavity and could seal a courtyard that is really open.
		if crosses {
			rings := sectionRings(w, e.Tris, plane)
			if len(rings) > 0 {
				g.markFilled(rings)
				sliced = true
			}
		}

		// Covered: the XY footprint of anything above the slice — including the
		// upper part of an element straddling it, such as a slab whose underside
		// sits below z. markFootprint clips at z, so the part below the cut
		// contributes nothing.
		//
		// For a PRISMATIC element straddling the slice this costs nothing: its
		// covered cells are exactly its solid cells, which probe walks over.
		// Where the element widens ABOVE the cut — a sloped roof, a cantilever —
		// covered spills past solid by that overhang and a cell just beside the
		// element can read covered rather than open. At 10 cm cells that is a
		// real but small bias toward reporting interior, and it is honest: there
		// genuinely is something overhead there.
		if above {
			g.markFootprint(w, e.Tris, z)
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
//
// math.Floor, not an int conversion: truncation rounds toward zero, so a point
// in [minX-cell, minX) would land on cell 0 and be reported as inside.
func (g *occupancy) cellOf(x, y float64) (ix, iy int, in bool) {
	ix = int(math.Floor((x - g.minX) / occupancyCell))
	iy = int(math.Floor((y - g.minY) / occupancyCell))
	if ix < 0 || iy < 0 || ix >= g.nx || iy >= g.ny {
		return 0, 0, false
	}
	return ix, iy, true
}

// markFilled marks one element's cross-section as solid: every ring's edges
// (so a feature thinner than one cell still blocks the flood fill), plus an
// even-odd scanline fill over ALL of the element's rings together. Even-odd
// over the whole set, rather than filling each ring independently, is what
// keeps a genuinely hollow element hollow: even-odd cancels a hole against its
// outer ring regardless of the winding sectionRings happens to emit.
//
// Filled per element, not over the union of every element's rings: two
// elements' rings must never be allowed to cancel one another out.
func (g *occupancy) markFilled(rings [][][2]float64) {
	for _, ring := range rings {
		g.markRing(ring)
	}
	g.fillRingsEvenOdd(rings)
}

// markRing rasterizes a closed ring's edges as solid.
func (g *occupancy) markRing(ring [][2]float64) {
	for i := range ring {
		a := ring[i]
		b := ring[(i+1)%len(ring)]
		g.markSegment(a, b)
	}
}

// fillRingsEvenOdd fills the interior of a set of rings (an element's
// cross-section, holes included) using a horizontal-scanline, even-odd point
// test at each grid row's cell centres.
func (g *occupancy) fillRingsEvenOdd(rings [][][2]float64) {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, ring := range rings {
		for _, p := range ring {
			minY = math.Min(minY, p[1])
			maxY = math.Max(maxY, p[1])
		}
	}
	if !(maxY > minY) {
		return
	}
	iy0, iy1 := g.clampRow(minY), g.clampRow(maxY)

	var xs []float64
	for iy := iy0; iy <= iy1; iy++ {
		y := g.minY + (float64(iy)+0.5)*occupancyCell
		xs = xs[:0]
		for _, ring := range rings {
			for i := range ring {
				a := ring[i]
				b := ring[(i+1)%len(ring)]
				// Half-open on the lower endpoint so a vertex lying exactly on
				// the scanline is never counted by both of its edges.
				if (a[1] <= y) == (b[1] <= y) {
					continue
				}
				t := (y - a[1]) / (b[1] - a[1])
				xs = append(xs, a[0]+t*(b[0]-a[0]))
			}
		}
		sort.Float64s(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			x0, x1 := xs[i], xs[i+1]
			ix0, ix1 := g.clampCol(x0), g.clampCol(x1)
			for ix := ix0; ix <= ix1; ix++ {
				px := g.minX + (float64(ix)+0.5)*occupancyCell
				if px >= x0 && px <= x1 {
					g.solid[g.idx(ix, iy)] = true
				}
			}
		}
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

// markFootprint marks the filled XY projection of the part of a mesh ABOVE
// height z as covered. Filled, not outlined: "is there a roof over this point"
// is a question about the interior of the projection.
//
// Clipped at z, not projected whole. The question is what is OVERHEAD, and
// geometry below the cut is underfoot: projecting a wall's wider footing, or
// the plinth a facade stands on, marks the ground beside it as roofed. A
// courtyard ringed by such walls then reads covered — a room — and its walls
// report ExposureInterior instead of ExposureEnclosed.
func (g *occupancy) markFootprint(w []v3, tris []uint32, z float64) {
	for i := 0; i+2 < len(tris); i += 3 {
		ia, ib, ic := tris[i], tris[i+1], tris[i+2]
		if int(ia) >= len(w) || int(ib) >= len(w) || int(ic) >= len(w) {
			continue
		}
		poly := clipAboveZ(w[ia], w[ib], w[ic], z)
		// Fan-triangulate the clipped polygon: the half-space clip of a triangle
		// is convex and has at most 4 corners, so a fan from the first is exact.
		for k := 1; k+1 < len(poly); k++ {
			g.fillTriangle2D(
				[2]float64{poly[0][0], poly[0][1]},
				[2]float64{poly[k][0], poly[k][1]},
				[2]float64{poly[k+1][0], poly[k+1][1]},
			)
		}
	}
}

// clipAboveZ returns the part of triangle abc at or above height z, as a convex
// polygon of 0, 3 or 4 corners. Sutherland-Hodgman against one half-space.
//
// A vertex at exactly z counts as inside, so a triangle lying flat ON the cut
// is kept whole rather than collapsing to a degenerate sliver.
func clipAboveZ(a, b, c v3, z float64) []v3 {
	src := [3]v3{a, b, c}
	out := make([]v3, 0, 4)
	for i := 0; i < 3; i++ {
		p, q := src[i], src[(i+1)%3]
		pin, qin := p[2] >= z, q[2] >= z
		if pin {
			out = append(out, p)
		}
		// Exactly one endpoint inside means the edge crosses z, so q[2] != p[2]
		// and the division below is safe.
		if pin != qin {
			t := (z - p[2]) / (q[2] - p[2])
			out = append(out, v3{
				p[0] + t*(q[0]-p[0]),
				p[1] + t*(q[1]-p[1]),
				z,
			})
		}
	}
	return out
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
	area := cross2D(a, b, c)
	if math.Abs(area) < 1e-15 {
		return // degenerate in plan
	}
	x0, y0 := g.clampCol(minX), g.clampRow(minY)
	x1, y1 := g.clampCol(maxX), g.clampRow(maxY)
	for iy := y0; iy <= y1; iy++ {
		for ix := x0; ix <= x1; ix++ {
			px := g.minX + (float64(ix)+0.5)*occupancyCell
			py := g.minY + (float64(iy)+0.5)*occupancyCell
			p := [2]float64{px, py}
			w0 := cross2D(b, c, p) / area
			w1 := cross2D(c, a, p) / area
			w2 := cross2D(a, b, p) / area
			if w0 >= 0 && w1 >= 0 && w2 >= 0 {
				g.covered[g.idx(ix, iy)] = true
			}
		}
	}
}

// clampCol maps a world X to a column index clamped into the grid, and clampRow
// a world Y to a row. Separate axes because most callers want one of them, and
// a combined helper made them pass a dummy coordinate for the other.
func (g *occupancy) clampCol(x float64) int {
	return clampIndex(int(math.Floor((x-g.minX)/occupancyCell)), g.nx)
}

func (g *occupancy) clampRow(y float64) int {
	return clampIndex(int(math.Floor((y-g.minY)/occupancyCell)), g.ny)
}

// clampIndex clamps i into [0, n).
func clampIndex(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// floodOutside depth-first fills every empty cell reachable from the grid
// border. The reached set is open air; empty cells it never reaches are voids
// the building encloses. Depth or breadth makes no difference to the reached
// set, and popping from the tail needs no head index.
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
