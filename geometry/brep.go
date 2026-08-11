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
)

// shellBasedSurfaceModelMesh unions the faces of every shell (IfcClosedShell
// or IfcOpenShell) in an IfcShellBasedSurfaceModel's SbsmBoundary set, raw
// units. Used by multi-shell family instances (e.g. a door's frame/leaf/
// hardware, each its own shell) that mix this representation type with plain
// IfcFacetedBrep siblings.
func shellBasedSurfaceModelMesh(sbsm *step.Instance) (verts []float32, tris []uint32, ok bool) {
	boundaryV, has := sbsm.Get(attrSbsmBoundary)
	if !has || boundaryV.Kind != step.KindList {
		return nil, nil, false
	}
	for _, sv := range boundaryV.List {
		if sv.Kind != step.KindRef || sv.Ref == nil {
			continue
		}
		if v, t, shellOK := brepMesh(sv.Ref); shellOK {
			appendMesh(&verts, &tris, v, t)
		}
	}
	return verts, tris, len(tris) > 0
}

// brepMesh tessellates every planar face of an IfcFacetedBrep (or a bare
// IfcClosedShell/IfcConnectedFaceSet), raw units. Inner bounds (holes) are
// ignored in v1 (walls solid).
func brepMesh(brep *step.Instance) (verts []float32, tris []uint32, ok bool) {
	shell := brep
	if brep.IsA("IfcFacetedBrep") {
		s, has := brep.Ref(attrBrepOuter)
		if !has {
			return nil, nil, false
		}
		shell = s
	}
	facesV, has := shell.Get(attrShellFaces)
	if !has || facesV.Kind != step.KindList {
		return nil, nil, false
	}
	for _, fv := range facesV.List {
		if fv.Kind != step.KindRef || fv.Ref == nil || !fv.Ref.IsA("IfcFace") {
			continue
		}
		loop := faceOuterLoop(fv.Ref)
		if len(loop) < 3 {
			continue
		}
		base := uint32(len(verts) / 3)
		for _, p := range loop {
			verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
		}
		// Ear-clip (not fan) — brep faces can be concave.
		for _, off := range triangulateFace(loop) {
			tris = append(tris, base+off)
		}
	}
	return verts, tris, len(tris) > 0
}

// faceOuterLoop returns the polygon of a face's outer bound (first IfcFaceOuterBound,
// else first bound). Points are IfcPolyLoop.Polygon coordinates, raw units.
func faceOuterLoop(face *step.Instance) []v3 {
	boundsV, ok := face.Get(attrFaceBounds)
	if !ok || boundsV.Kind != step.KindList {
		return nil
	}
	var fallback []v3
	for _, bv := range boundsV.List {
		if bv.Kind != step.KindRef || bv.Ref == nil {
			continue
		}
		loop, ok := bv.Ref.Ref(attrBoundLoop)
		if !ok || !loop.IsA("IfcPolyLoop") {
			continue
		}
		pts := loopPoints(loop)
		// IfcFaceBound.Orientation=.F. means the loop vertices run opposite to the
		// face normal (IFC spec); reverse them so the loop winding — which
		// triangulateFace derives the facet normal from via Newell's method —
		// matches the intended outward facing. Ignoring this ships those facets
		// inside-out (inward normals / backface-culled), and the AABB oracle is
		// blind to it since the vertex set is identical.
		if o, ok := bv.Ref.Get(attrBoundOrientation); ok && o.Kind == step.KindBool && !o.B {
			reverseV3(pts)
		}
		if bv.Ref.IsA("IfcFaceOuterBound") {
			return pts
		}
		if fallback == nil {
			fallback = pts
		}
	}
	return fallback
}

func loopPoints(loop *step.Instance) []v3 {
	v, ok := loop.Get(attrLoopPolygon)
	if !ok || v.Kind != step.KindList {
		return nil
	}
	var out []v3
	for _, pv := range v.List {
		if pv.Kind != step.KindRef || pv.Ref == nil {
			continue
		}
		c := floatsOf(pv.Ref, attrCoordinates)
		if len(c) >= 3 {
			out = append(out, v3{c[0], c[1], c[2]})
		}
	}
	return out
}
