package geometry

import "github.com/blox-eng/goifc/step"

const (
	attrBrepOuter        = 0 // IfcFacetedBrep.Outer
	attrShellFaces       = 0 // IfcClosedShell/IfcConnectedFaceSet.CfsFaces
	attrFaceBounds       = 0 // IfcFace.Bounds
	attrBoundLoop        = 0 // IfcFaceBound.Bound
	attrBoundOrientation = 1 // IfcFaceBound.Orientation
	attrLoopPolygon      = 0 // IfcPolyLoop.Polygon
	attrSbsmBoundary     = 0 // IfcShellBasedSurfaceModel.SbsmBoundary
	attrFbsmFaces        = 0 // IfcFaceBasedSurfaceModel.FbsmFaces
)

// surfaceModelMesh unions the faces of every shell or face set in a surface
// model's boundary attribute, raw units. Any member brepMesh cannot read in
// full declines the whole model, for the reason brepMesh gives.
//
// Shared by IfcShellBasedSurfaceModel (SbsmBoundary, a SET of IfcShell) and
// IfcFaceBasedSurfaceModel (FbsmFaces, a SET of IfcConnectedFaceSet). Two
// different schema entities with the same shape: a set of things brepMesh
// already tessellates. attr is a parameter rather than assumed, because the
// two constants agreeing at 0 is a fact about the schema, not a rule.
//
// Used by multi-shell family instances (a door's frame/leaf/hardware, each its
// own shell) that mix these representation types with plain IfcFacetedBrep
// siblings in the same element.
func surfaceModelMesh(m *step.Instance, attr int, b *budget) (verts []float32, tris []uint32, ok bool) {
	boundaryV, has := m.Get(attr)
	if !has || boundaryV.Kind() != step.KindList {
		return nil, nil, false
	}
	for _, sv := range boundaryV.List() {
		if sv.Kind() != step.KindRef || sv.Ref() == nil {
			return nil, nil, false
		}
		v, t, shellOK := brepMesh(sv.Ref(), b)
		if !shellOK {
			return nil, nil, false
		}
		appendMesh(&verts, &tris, v, t)
	}
	return verts, tris, len(tris) > 0
}

// brepMesh tessellates every planar face of an IfcFacetedBrep (or a bare
// IfcClosedShell/IfcConnectedFaceSet), raw units. Inner bounds (holes) are
// ignored in v1 (walls solid).
//
// A face it cannot read or triangulate within b declines the whole shell rather
// than being skipped: the faces that did parse make a mesh smaller than the
// solid, and shipping it would under-report the element's bounds where the OBB
// fallback over-reports them.
func brepMesh(brep *step.Instance, b *budget) (verts []float32, tris []uint32, ok bool) {
	shell := brep
	if brep.IsA("IfcFacetedBrep") {
		s, has := brep.Ref(attrBrepOuter)
		if !has {
			return nil, nil, false
		}
		shell = s
	}
	facesV, has := shell.Get(attrShellFaces)
	if !has || facesV.Kind() != step.KindList {
		return nil, nil, false
	}
	for _, fv := range facesV.List() {
		if fv.Kind() != step.KindRef || fv.Ref() == nil || !fv.Ref().IsA("IfcFace") {
			return nil, nil, false
		}
		loop := faceOuterLoop(fv.Ref())
		if len(loop) < 3 || !b.spend(int64(len(loop))) {
			return nil, nil, false
		}
		// Ear-clip (not fan) — brep faces can be concave.
		faceTris, ok := triangulateFace(loop, b)
		if !ok {
			return nil, nil, false
		}
		base := uint32(len(verts) / 3)
		for _, p := range loop {
			verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
		}
		for _, off := range faceTris {
			tris = append(tris, base+off)
		}
	}
	return verts, tris, len(tris) > 0
}

// faceOuterLoop returns the polygon of a face's outer bound (first IfcFaceOuterBound,
// else first bound). Points are IfcPolyLoop.Polygon coordinates, raw units. nil
// when any bound is unreadable: skipping an unreadable outer bound would promote
// a hole to the face's outline.
func faceOuterLoop(face *step.Instance) []v3 {
	boundsV, ok := face.Get(attrFaceBounds)
	if !ok || boundsV.Kind() != step.KindList {
		return nil
	}
	var fallback []v3
	for _, bv := range boundsV.List() {
		if bv.Kind() != step.KindRef || bv.Ref() == nil {
			return nil
		}
		loop, ok := bv.Ref().Ref(attrBoundLoop)
		if !ok || !loop.IsA("IfcPolyLoop") {
			return nil
		}
		pts := loopPoints(loop)
		if pts == nil {
			return nil
		}
		// IfcFaceBound.Orientation=.F. means the loop vertices run opposite to the
		// face normal (IFC spec); reverse them so the loop winding — which
		// triangulateFace derives the facet normal from via Newell's method —
		// matches the intended outward facing. Ignoring this ships those facets
		// inside-out (inward normals / backface-culled), and the AABB cross-check
		// is blind to it since the vertex set is identical.
		if o, ok := bv.Ref().Get(attrBoundOrientation); ok && o.Kind() == step.KindBool && !o.Bool() {
			reverseV3(pts)
		}
		if bv.Ref().IsA("IfcFaceOuterBound") {
			return pts
		}
		if fallback == nil {
			fallback = pts
		}
	}
	return fallback
}

// loopPoints returns an IfcPolyLoop's points, or nil if any is not a 3D point.
func loopPoints(loop *step.Instance) []v3 {
	v, ok := loop.Get(attrLoopPolygon)
	if !ok || v.Kind() != step.KindList {
		return nil
	}
	out := make([]v3, 0, len(v.List()))
	for _, pv := range v.List() {
		if pv.Kind() != step.KindRef || pv.Ref() == nil {
			return nil
		}
		c := floatsOf(pv.Ref(), attrCoordinates)
		if len(c) < 3 {
			return nil
		}
		out = append(out, v3{c[0], c[1], c[2]})
	}
	return out
}
