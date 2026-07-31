package geometry

import (
	"fmt"
	"math"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
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

// overlapEpsilon lets two openings' bounding rectangles share an edge without
// counting as an overlap (edge-touching mullions are not an overlap).
const overlapEpsilon = 1e-6

// overSubtractFraction is the share of gross above which the deduction is
// distrusted (openings claiming ≥95% of the wall face are treated as suspect).
const overSubtractFraction = 0.95

// NetAreas returns per-host (keyed by GlobalID) reconciliation for every element
// that has IfcRelVoidsElement openings. Hosts with NO voids are ABSENT from the
// map. For each voided host it measures the gross max-side-area on the host's
// winning projection axis, then deducts each opening's footprint measured on
// that SAME axis (so wall and openings share a plane and bias cancels). Net is
// emitted only when every opening passes every trust gate (see below); an
// untrusted host carries gross + a reason and an absent (nil) Net.
//
// EXACTNESS ENVELOPE: Net is EXACT only for a host whose elevational face is
// axis-aligned. Gross is a summed-face sideArea (rotation-invariant), while the
// opening deduction is an axis-aligned bounding SPAN — the two coincide only for
// axis-aligned rectangular geometry. A genuinely tilted host gets an APPROXIMATE
// net by design (the opening's axis-aligned span over-/under-counts its true
// in-plane footprint); consumers must not read Net as exact off-axis.
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

// reconcileHost applies the four trust gates for one voided host and returns its
// NetArea. All-or-nothing: the first gate any opening trips makes the whole host
// untrusted (Net nil, OpeningDeduction 0). Overlap and footprint are computed in
// world METERS — an opening's LocalPlacement translation is in RAW file units, so
// it MUST be meter-scaled (scaleTransformTranslation) before the overlap check,
// or overlap detection would compare millimetre positions against metre sizes.
func reconcileHost(f *step.File, openings []*step.Instance, gross float64, axis int, unitScale float64) NetArea {
	na := NetArea{Gross: gross}
	if gross <= 0 {
		// A degenerate host has no elevational face to net against; the
		// over-subtraction branch below would otherwise mislabel this "≥95%".
		na.Reason = "degenerate host (no max-side area)"
		return na
	}
	u, v := inPlaneAxes(axis)

	type rect struct{ uMin, uMax, vMin, vMax float64 }
	rects := make([]rect, 0, len(openings))
	var deduction float64
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
		deduction += (uMax - uMin) * (vMax - vMin)
	}

	// Overlap gate: two openings whose 2D bounding rectangles intersect would be
	// double-counted by a plain Σ footprint, so the deduction is untrustworthy.
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if intervalsOverlap(rects[i].uMin, rects[i].uMax, rects[j].uMin, rects[j].uMax) &&
				intervalsOverlap(rects[i].vMin, rects[i].vMax, rects[j].vMin, rects[j].vMax) {
				na.Reason = "openings overlap"
				return na
			}
		}
	}

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

// intervalsOverlap reports whether [aMin,aMax] and [bMin,bMax] overlap by more
// than overlapEpsilon (edge-touching is not an overlap).
func intervalsOverlap(aMin, aMax, bMin, bMax float64) bool {
	return aMin < bMax-overlapEpsilon && bMin < aMax-overlapEpsilon
}
