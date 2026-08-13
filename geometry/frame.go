package geometry

// This file holds the local->world accessors. The frame rule they exist to
// enforce is stated once, in the package doc.

// WorldVerts returns e.Verts transformed by e.Placement: world X,Y,Z triples in
// meters, IFC-native Z-up. Verts holding fewer than 3 floats returns nil; a
// trailing partial triple is dropped rather than emitted half-transformed.
//
// Allocates a fresh slice on every call; callers in a hot loop should hoist it.
// The transform runs in float64 and rounds to float32 on the way out, so the
// result agrees with BBoxMin/BBoxMax to float32 precision, not exactly — that
// gap grows with distance from the origin, so code needing exact world
// positions on a georeferenced model should transform through Placement in
// float64 itself.
func (e Element) WorldVerts() []float32 {
	if len(e.Verts) < 3 {
		return nil
	}
	return transformVerts(e.Verts[:len(e.Verts)/3*3], e.Placement)
}

// WorldNormal rotates a local DIRECTION (a face normal, an extrusion axis)
// into world space using only the 3x3 rotation part of e.Placement — a
// direction must not pick up the placement's translation. Use this, not
// differencing two WorldVerts, whenever the quantity is a direction.
//
// Magnitude is preserved, not normalized: a placement composed from
// IfcAxis2Placement3D is orthonormal and right-handed, so a unit local
// direction comes back unit-length and a scaled one comes back scaled by the
// same factor. That orthonormality is also why rotating the direction is the
// correct transform here and no inverse-transpose is needed.
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
