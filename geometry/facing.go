package geometry

import (
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
)

// Exposure is what the Facing.Normal side of an element reaches.
type Exposure string

const (
	// ExposureExterior — the side reaches open air outside the building. This
	// is the only exposure that belongs on a compass elevation.
	ExposureExterior Exposure = "exterior"
	// ExposureEnclosed — the side reaches a void the building encloses with
	// nothing overhead: a courtyard, a lightwell. Weather-exposed, but on no
	// elevation, so counting it into one over-reports the facade.
	ExposureEnclosed Exposure = "enclosed"
	// ExposureInterior — no exposed side was found; an internal partition.
	ExposureInterior Exposure = "interior"
)

// Sign-certainty factors folded into Confidence. Confidence is one number
// covering how decisively the axis won and how sure the sign is; Exposure
// carries the "why", so a caller branches on the enum and thresholds on this.
const (
	signCertain   = 1.0 // exactly one side reached open air
	signCourtyard = 0.8 // neither side outdoors; one open to sky, one roofed
	signAmbiguous = 0.3 // both sides alike — the sign is a coin flip
)

// Facing is an element's outward direction in world space: the area-weighted
// dominant normal of its vertical faces, signed to point at the exposed side.
//
// The sign is resolved by probing outward from the element's BBox CENTRE, so an
// L-shaped or strongly curved element whose centre falls outside its own body
// probes from a point that is not in the element at all and degrades to low
// confidence.
type Facing struct {
	// Normal is unit length in world space and points at the exposed side.
	Normal [3]float64
	// FaceArea is the area of the faces pointing along Normal, in m². ONE side:
	// the outer face of a wall, not the sum of its two faces, so summing it over
	// the elements binned to one elevation gives that elevation's gross area.
	//
	// Gross, not net — openings are not subtracted here.
	FaceArea float64
	// Exposure is what the Normal side reaches.
	Exposure Exposure
	// Confidence is 0..1. Below ~0.5 the sign is a guess; a consumer that would
	// rather show "unclassified" than file a wall under the wrong elevation
	// thresholds here. In a quantity context a wrong bin is a wrong invoice.
	Confidence float64
}

// FacingOf returns the facing of e, or ok=false when the element has no
// dominant vertical face family (a column, a slab, a degenerate mesh).
//
// Outwardness is not a property of one element: with no neighbours both sides
// reach open air, so an element classified alone gets its axis,
// ExposureExterior, an ARBITRARY sign and low confidence. Prefer BuildFacings
// whenever the neighbours are available — it is what lets the sign be decided
// at all.
func FacingOf(e Element) (Facing, bool) {
	return facingWithin(e, nil)
}

// BuildFacings classifies every element against the others, keyed by GlobalID.
// Elements with no facade are absent from the result rather than present with a
// zero value, so a caller cannot mistake "declined" for "faces +X".
//
// Elements sharing a GlobalID collapse to the last one classified; that is a
// malformed model, and de-duplicating silently would hide it.
func BuildFacings(elems []Element) map[string]Facing {
	out := make(map[string]Facing, len(elems))

	// Vote on axes FIRST, then build grids only for the elements that got one.
	// Rasterizing the whole model for a band whose every element is a slab, a
	// column or a piece of furniture is pure waste — the sign is never asked
	// for — and on a real model those elements are most of the file.
	bands := map[int64][]voted{}
	var keys []int64

	// One transform of each element's vertices for the whole run, shared by the
	// axis vote, every band's grid, and the final sign. See worldCache: without
	// it an element is re-transformed once per band beneath it.
	wc := newWorldCache(elems)

	for i := range elems {
		e := elems[i]
		dir, share, ok := axisOf(e, wc.at(i))
		if !ok {
			continue
		}
		key, keyed := sliceKey(e)
		if !keyed {
			out[e.GlobalID] = signFacing(e, dir, share, nil, wc.at(i))
			continue
		}
		if _, seen := bands[key]; !seen {
			keys = append(keys, key)
		}
		bands[key] = append(bands[key], voted{i, e, dir, share})
	}

	// Sorted, never a map range: an iteration order must not be able to reach
	// the output, even where today's arithmetic happens not to care.
	slices.Sort(keys)

	// Bands are independent — each builds its own grid at a height its KEY
	// denotes, reads only the shared read-only inputs, and decides only its own
	// members — so they are computed in parallel. On a real building this is the
	// dominant cost of the whole import: the rasterization, not the vertex
	// transform, and it repeats per band.
	//
	// Concurrency is BOUNDED rather than one goroutine per band, because a grid
	// is the large allocation here. The serial version kept exactly one live at a
	// time and said so; this keeps at most facingBandWorkers live.
	//
	// That is NOT the memory regression it looks like. Measured on kb645.ifc
	// (1,922 elements, 152 bands), peak heap across the call went DOWN, 692MB
	// serial to 676MB here, while total allocation fell from 9.9GB to 6.7GB. The
	// cache retains 55MB, and eliminating 88x redundant transform churn more than
	// pays for both it and the extra grids. The bound is insurance against a
	// pathological model, not a tax being levied on the normal one.
	//
	// Unbounded is still wrong: it would put one grid per distinct mid-height in
	// flight at once, which is the blow-up the serial comment existed to prevent.
	wc.fill() // read-only from here, so the workers may share it

	results := make([]map[string]Facing, len(keys))
	workers := runtime.GOMAXPROCS(0)
	if workers > facingBandWorkers {
		workers = facingBandWorkers
	}
	if workers > len(keys) {
		workers = len(keys)
	}
	// Bound the AGGREGATE grid memory, not just the grid COUNT. A count alone is
	// only a memory ceiling if every grid is small: occupancyMaxCells lets one
	// band allocate ~40M cells across several []bool, so eight of those in flight
	// is roughly eight times what the serial version could ever hold. On kb645
	// this never binds — its grids are tiny and the clamp leaves workers alone —
	// but the models that reach the cap are precisely the georeferenced,
	// site-scale ones the cap was written for, so the ceiling has to hold there
	// too, not just on a well-behaved building.
	//
	// Sized off the LARGEST band rather than the model's overall extent: a band's
	// grid covers only the elements its slice cuts, so the overall extent
	// over-estimates badly on exactly those far-flung models and would serialize
	// runs that never needed it.
	if maxCells := maxBandGridCells(elems, keys); maxCells > 0 {
		if allowed := facingBandCellBudget / maxCells; allowed < workers {
			workers = allowed
		}
	}
	if workers < 1 {
		workers = 1
	}

	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				k := int(next.Add(1) - 1)
				if k >= len(keys) {
					return
				}
				key := keys[k]
				// Built at the height the KEY denotes, never at some member
				// element's exact mid-height: two elements can differ by most of
				// a cell and still share a key, so cutting at one arrival's
				// height would make the plane — and every sign on the band —
				// depend on input order.
				g := buildOccupancyWith(elems, float64(key)*occupancyCell, wc)
				band := make(map[string]Facing, len(bands[key]))
				for _, v := range bands[key] {
					band[v.e.GlobalID] = signFacing(v.e, v.dir, v.share, g, wc.at(v.i))
				}
				results[k] = band
				// g dies here, so at most `workers` grids are ever live.
			}
		}()
	}
	wg.Wait()

	// Merged in sorted key order, never in completion order. Bands hold disjoint
	// elements so no key normally collides — but a malformed file CAN repeat a
	// GlobalId across two bands, and then the winner is decided by merge order.
	// Serial iteration made that the sorted one; completion order would make it
	// the scheduler's.
	for _, band := range results {
		for id, f := range band {
			out[id] = f
		}
	}
	return out
}

// facingBandWorkers caps how many occupancy grids BuildFacings keeps in flight.
// A grid is the large allocation in this package (up to occupancyMaxCells), so
// this is a memory ceiling first and a parallelism knob second: peak grid memory
// is this many, whatever the model's band count.
//
// It is only HALF the ceiling — see facingBandCellBudget for the other half.
const facingBandWorkers = 8

// facingBandCellBudget caps the total occupancy-grid cells in flight across all
// band workers.
//
// Deliberately equal to occupancyMaxCells, the most a SINGLE grid was ever
// allowed to allocate: the aggregate can therefore never exceed what the serial
// version already held at its own worst case, so band parallelism cannot raise
// peak grid memory above the pre-existing ceiling — it only decides how that
// fixed budget is divided. A model whose bands are small (every real building
// measured) spends it on facingBandWorkers grids at once; a model whose bands
// approach the cap spends it on one, and runs exactly as it used to.
const facingBandCellBudget = occupancyMaxCells

// maxBandGridCells returns the largest per-plane cell count any of these bands
// will allocate, so BuildFacings can size its worker pool to the budget before
// the first grid exists.
//
// This is a bbox pass per band with no vertex transform, which is nothing beside
// the rasterization it is sizing — measured at well under 1% of the stage.
func maxBandGridCells(elems []Element, keys []int64) int {
	most := 0
	for _, key := range keys {
		if c := bandGridCells(elems, float64(key)*occupancyCell); c > most {
			most = c
		}
	}
	return most
}

// voted is an element whose axis has been decided but whose sign has not.
type voted struct {
	i     int // index into the elems slice, so the shared worldCache can be asked
	e     Element
	dir   v3
	share float64
}

// sliceHeight is the height a facing is judged at: the element's mid-height,
// which for a wall is clear of both the floor slab it sits on and the ceiling
// over it.
func sliceHeight(e Element) float64 { return (e.BBoxMin[2] + e.BBoxMax[2]) / 2 }

// sliceKeyMax bounds a key so the float-to-int64 conversion below is defined.
// A model reaching it is 9e16 m across at 10 cm; its grid would be refused by
// occupancyMaxCells anyway.
const sliceKeyMax = 1 << 62

// sliceKey quantizes a slice height to occupancyCell so elements on one storey
// share a grid. Quantizing to the grid resolution is exactly as precise as the
// grid itself, so this costs no accuracy.
//
// ok=false for an element whose bbox is not finite. Such an element declines
// on its own — no grid, arbitrary sign, low confidence — rather than taking its
// whole band down with it. Its bbox is also the probe origin, so there is no
// meaningful answer to give it.
func sliceKey(e Element) (int64, bool) {
	if !finite3(e.BBoxMin) || !finite3(e.BBoxMax) {
		return 0, false
	}
	q := math.Round(sliceHeight(e) / occupancyCell)
	if math.Abs(q) >= sliceKeyMax {
		return 0, false
	}
	return int64(q), true
}

// axisOf runs the unsigned axis vote for e in world space. ok=false means the
// element has no facade, and so needs no neighbour context at all.
// w is e's world points, supplied by the caller so a run that asks about the
// same element repeatedly transforms it once.
func axisOf(e Element, w []v3) (dir v3, share float64, ok bool) {
	if len(e.Tris) == 0 {
		return v3{}, 0, false
	}
	// The vote area is deliberately dropped: it folds antipodes, so it is a vote
	// weight and never a quantity. FaceArea is measured separately, on one side,
	// once the sign is known.
	dir, _, share, ok = dominantAxis(w, e.Tris)
	return dir, share, ok
}

// facingWithin resolves e's facing against grid g. A nil g means no neighbour
// context — the axis still resolves, the sign does not.
// Single-element path: no run to amortise over, so it transforms once here and
// hands the same slice to both steps rather than taking a cache.
func facingWithin(e Element, g *occupancy) (Facing, bool) {
	w := worldPoints(e.Verts, e.Placement)
	dir, share, ok := axisOf(e, w)
	if !ok {
		return Facing{}, false
	}
	return signFacing(e, dir, share, g, w), true
}

// signFacing resolves which of ±dir points at the exposed side, against grid g.
// A nil g means no neighbour context: the axis stands, the sign is arbitrary
// and the confidence says so.
// w is e's world points, supplied by the caller for the same reason as axisOf.
func signFacing(e Element, dir v3, share float64, g *occupancy, w []v3) Facing {
	f := Facing{Normal: dir}

	if g == nil {
		f.Exposure = ExposureExterior
		f.Confidence = share * signAmbiguous
	} else {
		cx := (e.BBoxMin[0] + e.BBoxMax[0]) / 2
		cy := (e.BBoxMin[1] + e.BBoxMax[1]) / 2
		from := [2]float64{cx, cy}

		pos := g.probe(from, [2]float64{dir[0], dir[1]})
		neg := g.probe(from, [2]float64{-dir[0], -dir[1]})

		exposure, flip, factor := resolveSign(pos, neg)
		if flip {
			f.Normal = v3{-dir[0], -dir[1], -dir[2]}
		}
		f.Exposure = exposure
		f.Confidence = share * factor
	}

	// Measured against the RESOLVED normal, never the canonical one: the whole
	// point of FaceArea is that it names the side the element actually presents,
	// so it has to be computed after the sign is known.
	f.FaceArea = sideAreaDir(w, e.Tris, f.Normal)
	return f
}

// resolveSign turns the two probe results into an exposure, whether to flip the
// canonical direction, and how much to trust the sign.
//
// Open air wins outright. Failing that, a side open to the sky beats a roofed
// one — that is a courtyard wall, and it is the case a centroid or ray rule
// gets confidently wrong. When both sides look alike the sign is a coin flip
// and says so.
func resolveSign(pos, neg sideState) (Exposure, bool, float64) {
	switch {
	case pos.isOpenAir() && !neg.isOpenAir():
		return ExposureExterior, false, signCertain
	case neg.isOpenAir() && !pos.isOpenAir():
		return ExposureExterior, true, signCertain
	case pos.isOpenAir() && neg.isOpenAir():
		// Freestanding: both sides are outdoors, so the exposure is honest but
		// the sign is arbitrary.
		return ExposureExterior, false, signAmbiguous
	case pos == sideEnclosed && neg != sideEnclosed:
		return ExposureEnclosed, false, signCourtyard
	case neg == sideEnclosed && pos != sideEnclosed:
		return ExposureEnclosed, true, signCourtyard
	case pos == sideEnclosed && neg == sideEnclosed:
		return ExposureEnclosed, false, signAmbiguous
	default:
		// Both sides roofed or solid: an internal partition.
		return ExposureInterior, false, signAmbiguous
	}
}

// Azimuth returns the compass bearing of f in degrees CLOCKWISE from trueNorth,
// in [0, 360). Pass model.TrueNorth(file) for a real bearing, or {0,1} for a
// model-space one.
//
// Only the XY part of the normal has a bearing. A normal with no horizontal
// component — which FacingOf never returns, since it excludes near-vertical
// faces — has no bearing at all and yields 0.
func (f Facing) Azimuth(trueNorth [2]float64) float64 {
	// Negated comparisons, so a NaN coordinate takes the same branch as a
	// degenerate one. Written the other way round every NaN test is false, the
	// guards wave it through, and Atan2 returns a NaN bearing that no downstream
	// range check catches — NaN is neither < 0 nor >= 360.
	nx, ny := f.Normal[0], f.Normal[1]
	if !(math.Hypot(nx, ny) > 1e-12) {
		return 0
	}
	tx, ty := trueNorth[0], trueNorth[1]
	if !(math.Hypot(tx, ty) > 1e-12) {
		tx, ty = 0, 1
	}
	// Clockwise from north: the component along north is the cosine, the
	// component along north-rotated-90°-clockwise is the sine.
	along := nx*tx + ny*ty
	right := nx*ty - ny*tx
	deg := math.Atan2(right, along) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	// Atan2 can land exactly on 360 after the wrap for a hair-negative angle.
	if deg >= 360 {
		deg = 0
	}
	return deg
}
