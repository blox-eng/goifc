package geometry

import (
	"fmt"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// NetArea is one host element's gross→net elevational reconciliation.
type NetArea struct {
	// Gross is the host's largest projected silhouette, m² — the outward faces
	// on the winning axis counted once per covered square metre.
	//
	// This is NOT DerivedQuantities Area, which is the ifcopenshell-parity
	// get_max_side_area Σ. The two agree on any host whose outward faces do not
	// hide one another (every prismatic wall) and diverge on a pilaster,
	// projecting bay or brise-soleil, where only the union is drawable. Parity
	// is that quantity's job; exactness is this one's.
	Gross            float64
	OpeningDeduction float64 // union of the opening footprints on the host axis, m² (0 when untrusted)
	// OpeningPerimeter is the boundary length of that SAME union, in metres
	// (0 when untrusted). Reveals — the returns around a window or door — are
	// billed per linear metre in facade trades, and the length they follow is
	// this outline, not the Σ of the individual voids' perimeters: where two
	// footprints merge, the seam between them is interior, and it belongs to
	// the outline no more than the shared area belongs to the deduction twice.
	OpeningPerimeter float64
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
// map. For each voided host it measures the gross silhouette on the host's
// winning projection axis, then deducts the UNION of the openings' footprints
// measured on that SAME axis (so wall and openings share a plane and bias
// cancels). Net is emitted only when every opening passes every trust gate (see
// below); an untrusted host carries gross + a reason and an absent (nil) Net.
//
// Gross and the deduction are BOTH unions from the same engine on the same
// plane, so Net is the exact net area of that projection — and equals the area
// of the host's Element.SilhouetteOn outline minus its openings, which is what
// makes the drawing and the number the same fact.
//
// Openings that overlap in the host plane are handled, not refused: the
// deduction is a union, so each covered square metre is deducted exactly once
// however many voids claim it.
//
// EXACTNESS ENVELOPE: an opening's footprint is its SILHOUETTE on the host's
// winning plane — every triangle of the void's solid projected there and
// unioned — so an arch, an L-shaped void or any other non-rectangular profile
// is measured at its true projected area rather than at a bounding box around
// it. Gross is the host's silhouette on that same plane, so gross and deduction
// share one projection AND one measure, and Net is the exact net area OF THAT
// PROJECTION.
//
// What that does NOT mean: for a host whose face is tilted relative to the
// winning axis, the projection foreshortens gross and deduction by the same
// factor, so Net stays internally consistent but under-states the true 3D face
// area. Consumers needing the on-face area of a tilted host must correct for
// the obliquity themselves.
//
// The only quantum in the path is the 1e-5 m endpoint weld the union boundary
// inherits from the ring stitcher; the coverage classification itself uses
// exact predicates and no tolerance.
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
		gross, axis, ok := maxSilhouetteAxis(hw, host.Tris)
		if !ok {
			// No gross to reconcile against. Emitting the Σ instead would look like
			// a measurement and net against a union deduction, mixing two measures.
			out[el.GlobalID] = NetArea{Reason: "host outline did not close"}
			continue
		}
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
		na.Reason = "degenerate host (no silhouette area)"
		return na
	}
	u, v := inPlaneAxes(axis)

	// Project every opening's SOLID onto the host's winning plane. Projecting all
	// of a solid's triangles and unioning them yields its silhouette there, which
	// is the void's true footprint — not a bounding box around it.
	var footprints [][3][2]float64
	for _, op := range openings {
		ov, otris, src := elementMesh(f, op.ID(), unitScale)
		if src == SourceOBB {
			// No real solid to measure — an OBB fallback is the opening's box, not
			// its footprint. Distrust the whole host rather than guess.
			na.Reason = "opening had no solid geometry"
			return na
		}
		// Meter-scale the translation so opening position and size share units.
		xf := scaleTransformTranslation(model.LocalPlacement(op), unitScale)
		ow := worldPoints(ov, xf)
		nv := uint32(len(ow))
		for t := 0; t+2 < len(otris); t += 3 {
			i0, i1, i2 := otris[t], otris[t+1], otris[t+2]
			if i0 >= nv || i1 >= nv || i2 >= nv {
				continue
			}
			footprints = appendProjected(footprints,
				[2]float64{ow[i0][u], ow[i0][v]},
				[2]float64{ow[i1][u], ow[i1][v]},
				[2]float64{ow[i2][u], ow[i2][v]})
		}
	}

	// UNION, not Σ: overlapping voids (a mullioned assembly, or a door and its
	// transom exported as two voids) have a perfectly well-defined combined
	// footprint. Summing footprints double-counts the intersection, which is a
	// limitation of the sum rather than of the model — so the deduction is the
	// exact area covered by the union, and each covered square metre is deducted
	// once no matter how many openings claim it.
	deduction, perimeter, ok := unionMeasure2D(footprints)
	if !ok {
		// Deducting a partial footprint would over-state Net by whatever the
		// boundary lost, and nothing downstream could tell.
		na.Reason = "opening outline did not close"
		return na
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
	na.OpeningPerimeter = perimeter
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
