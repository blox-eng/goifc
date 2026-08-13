package geometry

// The frame rule, once: meshes are ELEMENT-LOCAL, placements and bounding
// boxes are WORLD. Element.Verts live in the element's own coordinate system;
// Element.Placement maps that system into world space; BBoxMin/BBoxMax are
// already world. Mixing a Verts-derived direction with a BBox-derived position
// is silently wrong — in the local frame most elements of a model are extruded
// along the SAME axis, so a facing classification built that way bins every
// element identically without a single error or degenerate mesh. WorldVerts
// and WorldNormal exist so the correct path is shorter than the wrong one.

// WorldVerts returns e.Verts transformed by e.Placement: world X,Y,Z triples in
// meters, IFC-native Z-up. Empty Verts returns nil.
//
// Allocates a fresh slice on every call; callers in a hot loop should hoist it.
// The transform runs in float64 and rounds to float32 at the end, so the result
// agrees with BBoxMin/BBoxMax to float32 precision, not exactly.
func (e Element) WorldVerts() []float32 {
	if len(e.Verts) == 0 {
		return nil
	}
	out := make([]float32, len(e.Verts))
	for i := 0; i+2 < len(e.Verts); i += 3 {
		w := applyMat(e.Placement, v3{float64(e.Verts[i]), float64(e.Verts[i+1]), float64(e.Verts[i+2])})
		out[i], out[i+1], out[i+2] = float32(w[0]), float32(w[1]), float32(w[2])
	}
	return out
}

// WorldNormal rotates a local DIRECTION (a face normal, an extrusion axis)
// into world space using only the 3x3 rotation part of e.Placement — a
// direction must not pick up the placement's translation. Use this, not
// applyMat/WorldVerts differencing, whenever the quantity is a direction.
//
// Magnitude is preserved, not normalized: IFC placements built from
// IfcAxis2Placement3D are orthonormal (see model.axis2placement's
// Gram-Schmidt), so a unit local direction comes back unit-length and a scaled
// one comes back scaled by the same factor.
func (e Element) WorldNormal(local [3]float64) [3]float64 {
	m := e.Placement
	// Column-major: m[col*4+row], so the basis vectors are columns 0,1,2 and
	// the translation in column 3 (m[12..14]) is deliberately left out.
	return [3]float64{
		m[0]*local[0] + m[4]*local[1] + m[8]*local[2],
		m[1]*local[0] + m[5]*local[1] + m[9]*local[2],
		m[2]*local[0] + m[6]*local[1] + m[10]*local[2],
	}
}
