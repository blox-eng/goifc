package geometry

import (
	"math"

	"github.com/blox-eng/goifc/step"
)

const (
	attrBoolOperator      = 0 // IfcBooleanResult/IfcBooleanClippingResult.Operator
	attrBoolFirstOperand  = 1
	attrBoolSecondOperand = 2

	attrHSBaseSurface     = 0 // IfcHalfSpaceSolid.BaseSurface
	attrHSAgreementFlag   = 1 // IfcHalfSpaceSolid.AgreementFlag
	attrHSPosition        = 2 // IfcPolygonalBoundedHalfSpace.Position (subtype-only)
	attrHSPolygonBoundary = 3 // IfcPolygonalBoundedHalfSpace.PolygonalBoundary (subtype-only)

	attrPlanePosition = 0 // IfcPlane.Position
)

// clipMeshByDifference tessellates DIFFERENCE(A, B) where A is any first
// operand (recursed through tessellateItemDepth, so it transparently handles
// an extrude/brep/mapped/nested-boolean first operand) and B is an
// IfcHalfSpaceSolid — plain (the common "cut by an infinite plane" pattern
// used for roof-line/gable-end clips) or bounded by a 2D polygon footprint
// (IfcPolygonalBoundedHalfSpace, Revit's usual wall/slab/beam miter-join cut).
// Returns ok=false for a non-DIFFERENCE operator or a non-planar base surface,
// letting the caller fall back to the (safe, conservative-superset) OBB path.
func clipMeshByDifference(item *step.Instance, unitScale float64, depth int) ([]float32, []uint32, GeomSource, bool) {
	if depth >= maxMapDepth {
		return nil, nil, SourceOBB, false
	}
	opV, ok := item.Get(attrBoolOperator)
	if !ok || opV.Kind != step.KindEnum || opV.Str != "DIFFERENCE" {
		return nil, nil, SourceOBB, false
	}
	first, ok := item.Ref(attrBoolFirstOperand)
	if !ok {
		return nil, nil, SourceOBB, false
	}
	second, ok := item.Ref(attrBoolSecondOperand)
	if !ok {
		return nil, nil, SourceOBB, false
	}
	if !second.IsA("IfcHalfSpaceSolid") {
		return nil, nil, SourceOBB, false
	}
	origin, normal, agreeInside, ok := halfSpacePlane(second)
	if !ok {
		return nil, nil, SourceOBB, false
	}
	// Recurse in RAW file units (scale=1) so the plane (built from raw
	// IfcCartesianPoint/IfcDirection values) and the mesh stay in the same
	// unscaled frame for clipping; scale to meters once, at the end.
	verts, tris, src := tessellateItemDepth(first, 1.0, depth+1)
	if len(verts) == 0 || len(tris) == 0 {
		return nil, nil, SourceOBB, false
	}
	// Empirically (verified against the oracle on a real gable-end roof clip,
	// AgreementFlag=.F.): the kept side is the one whose signed distance to the
	// base plane has the SAME sign convention as AgreementFlag itself — i.e.
	// keep the >=0 side when AgreementFlag is TRUE, the <=0 side when FALSE.
	if second.IsA("IfcPolygonalBoundedHalfSpace") {
		verts, tris, ok = clipTrianglesByBoundedPlane(verts, tris, origin, normal, agreeInside, second)
		if !ok {
			return nil, nil, SourceOBB, false
		}
	} else {
		verts, tris = clipTrianglesByPlane(verts, tris, origin, normal, agreeInside)
	}
	if len(tris) == 0 {
		return nil, nil, SourceOBB, false // fully consumed — treat as unsupported rather than empty
	}
	return scaleVerts(verts, unitScale), tris, src, true
}

// clipTrianglesByBoundedPlane clips by DIFFERENCE(A, PolygonalBoundedHalfSpace).
// The removed region is "on the base-plane material side AND within the 2D
// polygon footprint" — an AND of two conditions — so the kept region is "off
// the material side OR outside the footprint," a UNION, not an intersection
// (a plain chained clipTrianglesByPlane, which always computes an
// intersection of kept fragments, would be silently wrong here: it would also
// remove material outside the footprint that a real clip leaves untouched).
//
// This computes it correctly, approximating the polygon footprint by its own
// axis-aligned bounding box in the polygon's local (u,v) plane — EXACT when
// the boundary is itself a rectangle (the overwhelmingly common Revit case:
// wall/slab/beam end-miter cuts export as a rectangular "cookie cutter"), and
// a safe superset (keeps slightly more than truth) for any other convex or
// concave boundary, since footprint ⊆ its own AABB.
func clipTrianglesByBoundedPlane(verts []float32, tris []uint32, origin, normal v3, agreeInside bool, hs *step.Instance) ([]float32, []uint32, bool) {
	polyPos, ok := hs.Ref(attrHSPosition)
	if !ok {
		return nil, nil, false
	}
	boundary, ok := hs.Ref(attrHSPolygonBoundary)
	if !ok {
		return nil, nil, false
	}
	poly := curvePoints(boundary)
	if len(poly) < 3 {
		return nil, nil, false
	}
	polyOrigin, polyX, polyY, _ := planeFrame(polyPos)
	uMin, uMax, vMin, vMax := poly[0][0], poly[0][0], poly[0][1], poly[0][1]
	for _, p := range poly[1:] {
		uMin, uMax = math.Min(uMin, p[0]), math.Max(uMax, p[0])
		vMin, vMax = math.Min(vMin, p[1]), math.Max(vMax, p[1])
	}
	// materialFrag = the (candidate-for-removal) piece on the half-space's
	// material side; the rest of the mesh is unconditionally kept. Per
	// IfcHalfSpaceSolid.AgreementFlag (buildingSMART spec; ifcopenshell kernel:
	// orientation = !AgreementFlag) and this package's own oracle-validated
	// plain-halfspace path below (keepPositive = agreeInside for the KEPT
	// side), the material (to-remove) side is the OPPOSITE of agreeInside.
	materialFrag, materialTris := clipTrianglesByPlane(verts, tris, origin, normal, !agreeInside)
	kept, keptTris := clipTrianglesByPlane(verts, tris, origin, normal, agreeInside)
	// From materialFrag, keep whatever falls OUTSIDE the footprint AABB — a
	// union of "outside u", "outside v" fragments (each an independent plane
	// clip against one AABB face, so pieces may legitimately overlap; harmless
	// for bbox/rendering purposes, just a few redundant triangles).
	for _, edge := range [4]struct {
		originU, originV float64
		normal           v3
	}{
		{uMin, 0, scalev(polyX, -1)}, // u < uMin kept
		{uMax, 0, polyX},             // u > uMax kept
		{0, vMin, scalev(polyY, -1)}, // v < vMin kept
		{0, vMax, polyY},             // v > vMax kept
	} {
		edgeOrigin := addv(polyOrigin, addv(scalev(polyX, edge.originU), scalev(polyY, edge.originV)))
		ov, ot := clipTrianglesByPlane(materialFrag, materialTris, edgeOrigin, edge.normal, true)
		kept = append(kept, ov...)
		base := uint32(len(kept)-len(ov)) / 3
		for _, idx := range ot {
			keptTris = append(keptTris, base+idx)
		}
	}
	return kept, keptTris, true
}

// halfSpacePlane reads an IfcHalfSpaceSolid's base plane as (origin, normal,
// agreementFlag). Only a planar IfcPlane base surface is supported.
func halfSpacePlane(hs *step.Instance) (origin, normal v3, agreeInside bool, ok bool) {
	surf, has := hs.Ref(attrHSBaseSurface)
	if !has || !surf.IsA("IfcPlane") {
		return v3{}, v3{}, false, false
	}
	pos, has := surf.Ref(attrPlanePosition)
	if !has {
		return v3{}, v3{}, false, false
	}
	origin, _, _, normal = planeFrame(pos)
	agreeInside = true
	if av, has := hs.Get(attrHSAgreementFlag); has && av.Kind == step.KindBool {
		agreeInside = av.B
	}
	return origin, normal, agreeInside, true
}

// planeFrame reads an IfcAxis2Placement3D as (origin, x, y, z) basis vectors —
// the same derivation as axisPlacement3D, just returned as vectors instead of
// packed into a Mat4, since clipping only needs a point and a normal.
func planeFrame(a *step.Instance) (origin, x, y, z v3) {
	if loc, ok := a.Ref(attrAxisLocation); ok {
		c := floatsOf(loc, attrCoordinates)
		for i := 0; i < len(c) && i < 3; i++ {
			origin[i] = c[i]
		}
	}
	z = v3{0, 0, 1}
	if d, ok := a.Ref(attrAxisZ); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			z = v3{c[0], c[1], c[2]}
		}
	}
	x = v3{1, 0, 0}
	if d, ok := a.Ref(attrAxisX); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			x = v3{c[0], c[1], c[2]}
		}
	}
	x, y, z = orthonormalXZ(x, z)
	return origin, x, y, z
}

// clipTrianglesByPlane clips a triangle mesh against a plane (origin, normal)
// via per-triangle Sutherland-Hodgman, keeping the side where
// dot(p-origin,normal) >= 0 when keepPositive, else <= 0. A clipped triangle
// yields a convex polygon (3 or 4 verts) with a NEW vertex at each edge that
// crosses the plane — this is what recovers the true cut boundary (e.g. a
// roof ridge line) that a plain point-classification approach would miss
// entirely, since no such vertex exists in the unclipped source mesh.
func clipTrianglesByPlane(verts []float32, tris []uint32, origin, normal v3, keepPositive bool) ([]float32, []uint32) {
	at := func(i uint32) v3 {
		return v3{float64(verts[3*i]), float64(verts[3*i+1]), float64(verts[3*i+2])}
	}
	side := func(p v3) float64 {
		d := dotv(subv(p, origin), normal)
		if !keepPositive {
			d = -d
		}
		return d
	}
	var outVerts []float32
	var outTris []uint32
	emit := func(p v3) uint32 {
		idx := uint32(len(outVerts) / 3)
		outVerts = append(outVerts, float32(p[0]), float32(p[1]), float32(p[2]))
		return idx
	}
	for t := 0; t+2 < len(tris); t += 3 {
		pts := [3]v3{at(tris[t]), at(tris[t+1]), at(tris[t+2])}
		d := [3]float64{side(pts[0]), side(pts[1]), side(pts[2])}
		var poly []v3
		for i := 0; i < 3; i++ {
			j := (i + 1) % 3
			if d[i] >= 0 {
				poly = append(poly, pts[i])
			}
			if (d[i] >= 0) != (d[j] >= 0) {
				tt := d[i] / (d[i] - d[j])
				poly = append(poly, v3{
					pts[i][0] + tt*(pts[j][0]-pts[i][0]),
					pts[i][1] + tt*(pts[j][1]-pts[i][1]),
					pts[i][2] + tt*(pts[j][2]-pts[i][2]),
				})
			}
		}
		if len(poly) < 3 {
			continue
		}
		base := emit(poly[0])
		for i := 1; i+1 < len(poly); i++ {
			v1 := emit(poly[i])
			v2 := emit(poly[i+1])
			outTris = append(outTris, base, v1, v2)
		}
	}
	return outVerts, outTris
}
