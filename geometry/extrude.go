package geometry

import "github.com/blox-eng/goifc/step"

const (
	attrSweptArea     = 0
	attrSolidPosition = 1
	attrExtrudedDir   = 2
	attrExtrudeDepth  = 3
)

// extrudeSolid tessellates an IfcExtrudedAreaSolid into an element-local mesh in
// RAW file units. Caller scales to meters. ok=false when the profile is not parseable.
func extrudeSolid(solid *step.Instance) (verts []float32, tris []uint32, ok bool) {
	prof, ok := solid.Ref(attrSweptArea)
	if !ok {
		return nil, nil, false
	}
	poly := profilePolygon(prof)
	if len(poly) < 3 {
		return nil, nil, false
	}
	depth := scalarAt(solid, attrExtrudeDepth)
	if depth == 0 {
		return nil, nil, false
	}
	// Extrude direction (solid-local), default +Z.
	dir := v3{0, 0, 1}
	if d, ok := solid.Ref(attrExtrudedDir); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			dir = normv(v3{c[0], c[1], c[2]}) // IFC dir need not be unit; ifcopenshell normalizes
		}
	}
	// Solid position (IfcAxis2Placement3D) places the profile plane in the solid frame.
	place := identityMat()
	if pos, ok := solid.Ref(attrSolidPosition); ok {
		place = axisPlacement3D(pos)
	}

	n := len(poly)
	// Bottom ring (z=0) then top ring (z=depth along dir), both placed by `place`.
	local := make([]v3, 0, 2*n)
	for _, p := range poly {
		local = append(local, applyMat(place, v3{p[0], p[1], 0}))
	}
	for _, p := range poly {
		local = append(local, applyMat(place, v3{p[0] + dir[0]*depth, p[1] + dir[1]*depth, dir[2] * depth}))
	}
	verts = make([]float32, 0, 3*len(local))
	for _, p := range local {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	// Side walls.
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		b0, b1, t0, t1 := uint32(i), uint32(j), uint32(n+i), uint32(n+j)
		tris = append(tris, b0, b1, t1, b0, t1, t0)
	}
	// Caps: ear-clip the 2D profile polygon ONCE (handles concave footprints —
	// a naive fan inverts on L-shaped walls/slabs), emit bottom (reversed
	// winding, downward normal) and top rings from the same triangulation.
	capTris := triangulatePolygon(poly)
	for t := 0; t+2 < len(capTris); t += 3 {
		a, b, c := capTris[t], capTris[t+1], capTris[t+2]
		tris = append(tris, a, c, b)                               // bottom
		tris = append(tris, uint32(n)+a, uint32(n)+b, uint32(n)+c) // top
	}
	return verts, tris, true
}
