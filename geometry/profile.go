package geometry

import (
	"math"

	"github.com/blox-eng/goifc/step"
)

const (
	attrProfilePosition = 2 // IfcParameterizedProfileDef.Position
	attrRectXDim        = 3
	attrRectYDim        = 4
	attrCircleRadius    = 3
	attrOuterCurve      = 2 // IfcArbitraryClosedProfileDef.OuterCurve
	attrPolylinePoints  = 0
)

const circleSegments = 24

// profilePolygon returns the profile's outer boundary as 2D points in the
// profile's own coordinate system (before IfcExtrudedAreaSolid.Position),
// raw units. nil when the profile type is not supported (caller falls back to OBB).
func profilePolygon(p *step.Instance) [][2]float64 {
	switch {
	case p.IsA("IfcRectangleProfileDef"):
		xs := scalarAt(p, attrRectXDim)
		ys := scalarAt(p, attrRectYDim)
		hx, hy := xs/2, ys/2
		poly := [][2]float64{{-hx, -hy}, {hx, -hy}, {hx, hy}, {-hx, hy}}
		return ensureCCW(placePolygon(p, poly))
	case p.IsA("IfcCircleProfileDef"):
		rr := scalarAt(p, attrCircleRadius)
		poly := make([][2]float64, 0, circleSegments)
		for i := 0; i < circleSegments; i++ {
			a := 2 * math.Pi * float64(i) / circleSegments
			poly = append(poly, [2]float64{rr * math.Cos(a), rr * math.Sin(a)})
		}
		return ensureCCW(placePolygon(p, poly))
	case p.IsA("IfcArbitraryClosedProfileDef"), p.IsA("IfcArbitraryProfileDefWithVoids"):
		curve, ok := p.Ref(attrOuterCurve)
		if !ok {
			return nil
		}
		// already in profile coords; no Position on arbitrary profiles. Winding is
		// whatever the authoring tool emitted — normalize so side walls (built from
		// this order) and caps (which triangulatePolygon CCW-normalizes internally)
		// agree on facing direction.
		return ensureCCW(curvePoints(curve))
	}
	return nil
}

// placePolygon applies IfcRectangle/CircleProfileDef.Position (IfcAxis2Placement2D)
// to a polygon defined about the profile origin.
func placePolygon(p *step.Instance, poly [][2]float64) [][2]float64 {
	pos, ok := p.Ref(attrProfilePosition)
	if !ok {
		return poly
	}
	m := axisPlacement2D(pos)
	out := make([][2]float64, len(poly))
	for i, q := range poly {
		w := applyMat(m, v3{q[0], q[1], 0})
		out[i] = [2]float64{w[0], w[1]}
	}
	return out
}

const (
	attrCompositeSegments  = 0 // IfcCompositeCurve.Segments
	attrSegmentSameSense   = 1 // IfcCompositeCurveSegment.SameSense
	attrSegmentParentCurve = 2 // IfcCompositeCurveSegment.ParentCurve
)

// curvePoints reads a profile boundary curve's 2D points. Supports a bare
// IfcPolyline directly, or an IfcCompositeCurve of mostly-straight-line
// segments (common from Rhino/Grasshopper/ArchiCAD IFC exporters, which build
// closed profile boundaries as a chain of 2-point
// IfcCompositeCurveSegment→IfcPolyline pieces rather than one IfcPolyline).
// A segment whose ParentCurve isn't an IfcPolyline (e.g. a small fillet/
// nosing radius via IfcTrimmedCurve/IfcCircle) is DROPPED — not fatal to the
// whole curve — leaving a short straight "chord" across that corner instead
// of the true arc. This is deliberately lenient: earlier this returned nil
// for the WHOLE profile on any single unsupported segment, which starves
// extrudeSolid entirely and forces the OBB fallback — and that fallback's
// raw collectPoints walk over the arc's IfcCircle/IfcAxis2Placement subtree
// can pick up the arc CENTER point (which need not lie anywhere near the
// actual profile boundary), silently blowing up the element's bbox far past
// its real extent. A polygon missing one small filleted corner is a much
// smaller, safer error than an unbounded stray point.
func curvePoints(curve *step.Instance) [][2]float64 {
	if curve.IsA("IfcCompositeCurve") {
		segsV, ok := curve.Get(attrCompositeSegments)
		if !ok || segsV.Kind != step.KindList {
			return nil
		}
		var out [][2]float64
		for _, sv := range segsV.List {
			if sv.Kind != step.KindRef || sv.Ref == nil || !sv.Ref.IsA("IfcCompositeCurveSegment") {
				continue
			}
			parent, ok := sv.Ref.Ref(attrSegmentParentCurve)
			if !ok {
				continue
			}
			pts := polylinePoints(parent)
			if len(pts) < 2 {
				continue // unsupported segment type (arc etc.) — drop, don't abort
			}
			sameSense := true
			if ssV, ok := sv.Ref.Get(attrSegmentSameSense); ok && ssV.Kind == step.KindBool {
				sameSense = ssV.B
			}
			if !sameSense {
				for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
					pts[i], pts[j] = pts[j], pts[i]
				}
			}
			// Consecutive segments share their join point; drop the duplicate.
			if len(out) > 0 && out[len(out)-1] == pts[0] {
				pts = pts[1:]
			}
			out = append(out, pts...)
		}
		if n := len(out); n >= 2 && out[0] == out[n-1] {
			out = out[:n-1]
		}
		return out
	}
	return polylinePoints(curve)
}

// polylinePoints reads an IfcPolyline's 2D points (returns nil for any other
// curve type — composite curves are handled by curvePoints).
func polylinePoints(curve *step.Instance) [][2]float64 {
	if !curve.IsA("IfcPolyline") {
		return nil
	}
	v, ok := curve.Get(attrPolylinePoints)
	if !ok || v.Kind != step.KindList {
		return nil
	}
	var out [][2]float64
	for _, pv := range v.List {
		if pv.Kind != step.KindRef || pv.Ref == nil {
			continue
		}
		c := floatsOf(pv.Ref, attrCoordinates)
		if len(c) >= 2 {
			out = append(out, [2]float64{c[0], c[1]})
		}
	}
	// IfcPolyline for a closed profile repeats the first point last; drop the dup.
	if n := len(out); n >= 2 && out[0] == out[n-1] {
		out = out[:n-1]
	}
	return out
}

func scalarAt(inst *step.Instance, attr int) float64 {
	v, ok := inst.Get(attr)
	if !ok {
		return 0
	}
	switch v.Kind {
	case step.KindFloat:
		return v.F
	case step.KindInt:
		return float64(v.I)
	}
	return 0
}
