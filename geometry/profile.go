package geometry

import (
	"math"

	"github.com/blox-eng/goifc/model"
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

// curvePoints reads a profile boundary curve's 2D points: a bare IfcPolyline,
// or an IfcCompositeCurve whose segments are polylines or circular arcs
// (Rhino/Grasshopper/ArchiCAD chain 2-point polylines; Revit adds nosing and
// fillet arcs as IfcTrimmedCurve over an IfcCircle). nil when any segment
// cannot be read: skipping one leaves a chord across it, a profile smaller
// than the solid, and that under-reported duplex_a's stair flights by the
// 10 mm nosing radius (#53). Declined, the solid falls back to a box, which
// collectPoints makes reach every conic's full extent.
func curvePoints(curve *step.Instance) [][2]float64 {
	if !curve.IsA("IfcCompositeCurve") {
		return polylinePoints(curve)
	}
	segsV, ok := curve.Get(attrCompositeSegments)
	if !ok || segsV.Kind != step.KindList {
		return nil
	}
	var out [][2]float64
	for _, sv := range segsV.List {
		if sv.Kind != step.KindRef || sv.Ref == nil || !sv.Ref.IsA("IfcCompositeCurveSegment") {
			return nil
		}
		parent, ok := sv.Ref.Ref(attrSegmentParentCurve)
		if !ok {
			return nil
		}
		var pts [][2]float64
		if parent.IsA("IfcTrimmedCurve") {
			pts = arcPoints(parent)
		} else {
			pts = polylinePoints(parent)
		}
		if len(pts) < 2 {
			return nil
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

// polylinePoints reads an IfcPolyline's 2D points. nil for any other curve
// type, or when any point is not a 2D or 3D coordinate.
func polylinePoints(curve *step.Instance) [][2]float64 {
	if !curve.IsA("IfcPolyline") {
		return nil
	}
	v, ok := curve.Get(attrPolylinePoints)
	if !ok || v.Kind != step.KindList {
		return nil
	}
	out := make([][2]float64, 0, len(v.List))
	for _, pv := range v.List {
		if pv.Kind != step.KindRef || pv.Ref == nil {
			return nil
		}
		c := floatsOf(pv.Ref, attrCoordinates)
		if len(c) < 2 {
			return nil
		}
		out = append(out, [2]float64{c[0], c[1]})
	}
	// IfcPolyline for a closed profile repeats the first point last; drop the dup.
	if n := len(out); n >= 2 && out[0] == out[n-1] {
		out = out[:n-1]
	}
	return out
}

const (
	attrTrimBasis    = 0 // IfcTrimmedCurve.BasisCurve
	attrTrim1        = 1
	attrTrim2        = 2
	attrTrimSense    = 3 // SenseAgreement: .T. runs trim1 to trim2 counter-clockwise
	attrTrimMaster   = 4 // MasterRepresentation: .CARTESIAN., .PARAMETER. or .UNSPECIFIED.
	attrConicPos     = 0 // IfcConic.Position
	attrCircleR      = 1 // IfcCircle.Radius
	attrEllipseSemi2 = 2 // IfcEllipse.SemiAxis2 (SemiAxis1 shares IfcCircle.Radius's slot)
)

// arcPoints tessellates an IfcTrimmedCurve over an IfcCircle in a 2D placement,
// from trim1 to trim2, raw units. nil for any other basis curve.
//
// The interior vertices sit on the CIRCUMSCRIBED polygon (radius r/cos(step/2)),
// not on the circle, so the polyline encloses the arc and every bound it adds is
// at or beyond the true one. Endpoints stay on the circle so the arc still meets
// its neighbouring segments. An inscribed polygon, the usual choice, cuts inside
// the arc and would under-report by up to r(1-cos(step/2)).
func arcPoints(trimmed *step.Instance) [][2]float64 {
	circle, ok := trimmed.Ref(attrTrimBasis)
	if !ok || !circle.IsA("IfcCircle") {
		return nil
	}
	pos, ok := circle.Ref(attrConicPos)
	if !ok || !pos.IsA("IfcAxis2Placement2D") {
		return nil
	}
	r := scalarAt(circle, attrCircleR)
	if !(r > 0) || math.IsInf(r, 0) {
		return nil
	}
	m := axisPlacement2D(pos)
	a1, ok1 := trimAngle(trimmed, attrTrim1, m)
	a2, ok2 := trimAngle(trimmed, attrTrim2, m)
	if !ok1 || !ok2 {
		return nil
	}
	sweep := math.Mod(a2-a1, 2*math.Pi)
	if sv, ok := trimmed.Get(attrTrimSense); ok && sv.Kind == step.KindBool && !sv.B {
		if sweep > 0 {
			sweep -= 2 * math.Pi
		}
	} else if sweep < 0 {
		sweep += 2 * math.Pi
	}
	if sweep == 0 || math.IsNaN(sweep) {
		return nil
	}
	n := int(math.Ceil(math.Abs(sweep) / (2 * math.Pi / circleSegments)))
	delta := sweep / float64(n)
	at := func(a, radius float64) [2]float64 {
		p := applyMat(m, v3{radius * math.Cos(a), radius * math.Sin(a), 0})
		return [2]float64{p[0], p[1]}
	}
	out := make([][2]float64, 0, n+2)
	out = append(out, at(a1, r))
	outer := r / math.Cos(delta/2)
	for k := 0; k < n; k++ {
		out = append(out, at(a1+(float64(k)+0.5)*delta, outer))
	}
	return append(out, at(a1+sweep, r))
}

// trimAngle reads one trim of a circular arc as an angle in radians about the
// circle's own frame m. A trim may carry a point, a parameter, or both; the
// curve's MasterRepresentation says which wins, and either stands in for a
// missing other. A parameter is a plane angle in the file's unit, so it cannot
// be read in a file that declares none.
func trimAngle(trimmed *step.Instance, attr int, m model.Mat4) (float64, bool) {
	v, ok := trimmed.Get(attr)
	if !ok || v.Kind != step.KindList {
		return 0, false
	}
	var point *step.Instance
	param, hasParam := 0.0, false
	for _, sel := range v.List {
		switch {
		case sel.Kind == step.KindRef && sel.Ref != nil && sel.Ref.IsA("IfcCartesianPoint"):
			point = sel.Ref
		case sel.Kind == step.KindTyped && sel.Str == "IFCPARAMETERVALUE" && len(sel.List) == 1:
			param, hasParam = numberOf(sel.List[0])
		}
	}
	usePoint := point != nil && (!hasParam || enumAt(trimmed, attrTrimMaster) != "PARAMETER")
	if usePoint {
		c := floatsOf(point, attrCoordinates)
		if len(c) < 2 {
			return 0, false
		}
		d := v3{c[0] - m[12], c[1] - m[13], 0}
		return math.Atan2(dotv(d, v3{m[4], m[5], 0}), dotv(d, v3{m[0], m[1], 0})), true
	}
	if !hasParam {
		return 0, false
	}
	scale, ok := model.PlaneAngleScale(trimmed.File())
	if !ok {
		return 0, false
	}
	a := param * scale
	return a, !math.IsInf(a, 0) && !math.IsNaN(a)
}

func numberOf(v step.Value) (float64, bool) {
	switch v.Kind {
	case step.KindFloat:
		return v.F, true
	case step.KindInt:
		return float64(v.I), true
	}
	return 0, false
}

func enumAt(inst *step.Instance, attr int) string {
	if v, ok := inst.Get(attr); ok && v.Kind == step.KindEnum {
		return v.Str
	}
	return ""
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
