package geometry

import "math"

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
type Facing struct {
	// Normal is unit length in world space and points at the exposed side.
	Normal [3]float64
	// FaceArea is the m² of vertical face voting for this direction.
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
	// One grid per distinct slice height, so a storey is rasterized once rather
	// than once per wall on it.
	grids := map[int64]*occupancy{}

	for i := range elems {
		e := elems[i]
		key := sliceKey(e)
		g, built := grids[key]
		if !built {
			g = buildOccupancy(elems, sliceHeight(e))
			grids[key] = g
		}
		if f, ok := facingWithin(e, g); ok {
			out[e.GlobalID] = f
		}
	}
	return out
}

// sliceHeight is the height a facing is judged at: the element's mid-height,
// which for a wall is clear of both the floor slab it sits on and the ceiling
// over it.
func sliceHeight(e Element) float64 { return (e.BBoxMin[2] + e.BBoxMax[2]) / 2 }

// sliceKey quantizes a slice height to occupancyCell so elements on one storey
// share a grid. Quantizing to the grid resolution is exactly as precise as the
// grid itself, so this costs no accuracy.
func sliceKey(e Element) int64 {
	return int64(math.Round(sliceHeight(e) / occupancyCell))
}

// facingWithin resolves e's facing against grid g. A nil g means no neighbour
// context — the axis still resolves, the sign does not.
func facingWithin(e Element, g *occupancy) (Facing, bool) {
	if len(e.Tris) == 0 {
		return Facing{}, false
	}
	w := worldPoints(e.Verts, e.Placement)
	dir, area, share, ok := dominantAxis(w, e.Tris)
	if !ok {
		return Facing{}, false
	}

	f := Facing{Normal: dir, FaceArea: area}

	if g == nil {
		f.Exposure = ExposureExterior
		f.Confidence = share * signAmbiguous
		return f, true
	}

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
	return f, true
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
