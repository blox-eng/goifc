package geometry

import (
	"fmt"
	"math"
	"sort"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// NetArea is one host element's gross→net elevational reconciliation.
type NetArea struct {
	Gross            float64  // host max-side-area, m² (== DerivedQuantities Area)
	OpeningDeduction float64  // Σ opening footprint on the host axis, m² (0 when untrusted)
	Net              *float64 // net area, m² — populated ONLY when Trusted; nil otherwise.
	// Never fabricated (matches the engine's pos()/nil-means-absent contract).
	Trusted bool
	Reason  string // when !Trusted, why (short, human-readable)
}

// overSubtractFraction is the share of gross above which the deduction is
// distrusted (openings claiming ≥95% of the wall face are treated as suspect).
const overSubtractFraction = 0.95

// NetAreas returns per-host (keyed by GlobalID) reconciliation for every element
// that has IfcRelVoidsElement openings. Hosts with NO voids are ABSENT from the
// map. For each voided host it measures the gross max-side-area on the host's
// winning projection axis, then deducts the UNION of the openings' footprints
// measured on that SAME axis (so wall and openings share a plane and bias
// cancels). Net is emitted only when every opening passes every trust gate (see
// below); an untrusted host carries gross + a reason and an absent (nil) Net.
//
// Openings that overlap in the host plane are handled, not refused: the
// deduction is a union, so each covered square metre is deducted exactly once
// however many voids claim it.
//
// EXACTNESS ENVELOPE: Net is EXACT only for a host whose elevational face is
// axis-aligned AND whose openings project to rectangles. Gross is a summed-face
// sideArea (rotation-invariant), while each opening's footprint is an
// axis-aligned bounding SPAN — the two coincide only for axis-aligned
// rectangular geometry. A tilted host, or an opening whose true profile is not
// a rectangle (an arch, an L-shaped void), gets an APPROXIMATE net by design:
// the bounding span over-counts the void, so Net is an UNDER-estimate. Taking
// the union removes double-counting between openings; it does not tighten any
// single opening's span. Consumers must not read Net as exact off-axis.
//
// SIDE EFFECT: this appends the aggregated orphan-fill warning to s.Warnings.
// Intended to be called ONCE per Scene — calling it repeatedly duplicates that
// warning and re-does the work.
//
// INVARIANT (load-bearing): the host mesh in s.Elements must contain NO opening
// (IfcRelVoidsElement) geometry — gross is then the SOLID elevational area and
// OpeningDeduction is the ONLY netting applied. This holds today because
// clip.go's clipMeshByDifference subtracts ONLY an IfcHalfSpaceSolid second
// operand (plane/half-space cuts for roof-lines and miter joins); it never bakes
// a solid void into the host mesh. If clipMeshByDifference is ever extended to
// subtract a SOLID second operand, a voided host would be netted TWICE — once in
// its mesh, once here (double-subtraction). The un-voided invariant is guarded
// by TestNetAreas_RectWindow, which asserts Gross == the full solid wall area.
func (s *Scene) NetAreas(f *step.File, r *model.Result) map[string]NetArea {
	out := make(map[string]NetArea)
	meshByGID := make(map[string]*Element, len(s.Elements))
	for i := range s.Elements {
		meshByGID[s.Elements[i].GlobalID] = &s.Elements[i]
	}
	for i := range r.Elements {
		el := &r.Elements[i]
		inst, ok := f.ByID(el.ExpressID)
		if !ok {
			continue
		}
		openings := model.OpeningsOf(f, inst)
		if len(openings) == 0 {
			continue // no voids → absent from the map
		}
		host, ok := meshByGID[el.GlobalID]
		if !ok || len(host.Tris) == 0 {
			s.Warnings = append(s.Warnings, "net area: no host mesh for "+el.GlobalID)
			continue
		}
		hw := worldPoints(host.Verts, host.Placement)
		gross, axis := maxSideAreaAxis(hw, host.Tris)
		out[el.GlobalID] = reconcileHost(f, openings, gross, axis, r.UnitScale)
	}
	// One aggregated warning for filled-but-void-less openings (never per host —
	// IFC2X3 door/window fills would otherwise flood it). Detection lives in the
	// model package to keep IfcRelFills/IfcRelVoids rel-walking out of geometry.
	if orphans := model.OrphanFillOpenings(f); len(orphans) > 0 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("%d openings fill a void but void no host", len(orphans)))
	}
	return out
}

// reconcileHost applies the three trust gates for one voided host and returns its
// NetArea. All-or-nothing: the first gate any opening trips makes the whole host
// untrusted (Net nil, OpeningDeduction 0). Footprints are computed in world
// METERS — an opening's LocalPlacement translation is in RAW file units, so it
// MUST be meter-scaled (scaleTransformTranslation) before the union, or a
// millimetre file's openings land ~1000× away from the host and the union
// silently degrades to a plain Σ (guarded by TestNetAreas_MillimetreOverlap).
func reconcileHost(f *step.File, openings []*step.Instance, gross float64, axis int, unitScale float64) NetArea {
	na := NetArea{Gross: gross}
	if gross <= 0 {
		// A degenerate host has no elevational face to net against; the
		// over-subtraction branch below would otherwise mislabel this "≥95%".
		na.Reason = "degenerate host (no max-side area)"
		return na
	}
	u, v := inPlaneAxes(axis)

	rects := make([]rect, 0, len(openings))
	for _, op := range openings {
		ov, _, src := elementMesh(f, op.ID(), unitScale)
		if src == SourceOBB {
			// No real solid to measure — an OBB fallback is the opening's box, not
			// its footprint. Distrust the whole host rather than guess.
			na.Reason = "opening had no solid geometry"
			return na
		}
		// Meter-scale the translation so opening position and size share units.
		xf := scaleTransformTranslation(model.LocalPlacement(op), unitScale)
		ow := worldPoints(ov, xf)
		uMin, uMax, vMin, vMax := spanRect(ow, u, v)
		rects = append(rects, rect{uMin, uMax, vMin, vMax})
	}

	// UNION, not Σ: overlapping voids (a mullioned assembly, or a door and its
	// transom exported as two voids) have a perfectly well-defined combined
	// footprint. Summing rectangles double-counts the intersection, which is a
	// limitation of the sum rather than of the model — so the deduction is the
	// exact area covered by the union, and each covered square metre is deducted
	// once no matter how many openings claim it.
	deduction := unionArea(rects)

	// Over-subtraction gate: openings claiming ≥95% of gross are implausible
	// (bad geometry or a mostly-glass curtain wall) — don't emit a near-zero net.
	if deduction >= overSubtractFraction*gross {
		na.Reason = "openings ≥95% of gross"
		return na
	}

	net := gross - deduction
	na.Net = &net
	na.OpeningDeduction = deduction
	na.Trusted = true
	return na
}

// inPlaneAxes returns the two coordinate indices that remain after dropping the
// winning projection axis (0→{1,2}, 1→{0,2}, 2→{0,1}).
func inPlaneAxes(axis int) (u, v int) {
	switch axis {
	case 0:
		return 1, 2
	case 1:
		return 0, 2
	default:
		return 0, 1
	}
}

// spanRect returns the min/max of pts on the two in-plane coordinate indices.
func spanRect(pts []v3, u, v int) (uMin, uMax, vMin, vMax float64) {
	uMin, vMin = math.Inf(1), math.Inf(1)
	uMax, vMax = math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		uMin, uMax = math.Min(uMin, p[u]), math.Max(uMax, p[u])
		vMin, vMax = math.Min(vMin, p[v]), math.Max(vMax, p[v])
	}
	return uMin, uMax, vMin, vMax
}

// rect is one opening's axis-aligned footprint in the host's in-plane (u,v)
// coordinates, in world meters.
type rect struct{ uMin, uMax, vMin, vMax float64 }

// unionArea returns the EXACT area covered by the union of axis-aligned
// rectangles — each covered point counted once however many rects contain it.
//
// Vertical-slab sweep: the u coordinates of every rect edge cut the plane into
// slabs within which the set of covering rects cannot change, so each slab's
// contribution is its width times the union LENGTH of the v-intervals covering
// it. Exact (no sampling): a slab is classified by its midpoint, which is
// strictly inside every rect that spans it and strictly outside every rect that
// does not, because no rect edge falls in a slab's interior by construction.
//
// O(n² log n) for n rects. n is the opening count of ONE host, so it is small.
func unionArea(rects []rect) float64 {
	if len(rects) == 0 {
		return 0
	}
	us := make([]float64, 0, 2*len(rects))
	for _, r := range rects {
		us = append(us, r.uMin, r.uMax)
	}
	sort.Float64s(us)

	var total float64
	spans := make([][2]float64, 0, len(rects))
	for i := 0; i+1 < len(us); i++ {
		width := us[i+1] - us[i]
		if width <= 0 {
			continue // duplicate coordinate: zero-width slab
		}
		mid := us[i] + width/2
		spans = spans[:0]
		for _, r := range rects {
			if r.uMin < mid && mid < r.uMax {
				spans = append(spans, [2]float64{r.vMin, r.vMax})
			}
		}
		total += width * unionLength(spans)
	}
	return total
}

// unionLength returns the total length covered by the union of 1D intervals.
// It sorts in place, so the caller must not rely on spans' order afterwards.
func unionLength(spans [][2]float64) float64 {
	if len(spans) == 0 {
		return 0
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a][0] < spans[b][0] })
	var total float64
	lo, hi := spans[0][0], spans[0][1]
	for _, s := range spans[1:] {
		if s[0] > hi { // gap: bank the run just closed and open a new one
			total += hi - lo
			lo, hi = s[0], s[1]
			continue
		}
		if s[1] > hi {
			hi = s[1]
		}
	}
	return total + (hi - lo)
}
