package geometry

import (
	"math"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

func identityMat() model.Mat4 { return model.Identity() }

// orthonormalXZ derives an orthonormal (x, y, z) basis from a primary axis z and
// a reference direction x, per the IFC IfcAxis2Placement3D /
// IfcCartesianTransformationOperator convention: z is fixed, x is made
// perpendicular to z, and y = z×x. When the supplied x is zero or parallel to z
// (degenerate or malformed input), an arbitrary perpendicular is substituted so
// the basis never collapses to a zero-scaled, non-invertible frame (which would
// silently flatten the element to a point / produce an invalid glTF matrix).
func orthonormalXZ(x, z v3) (ox, oy, oz v3) {
	z = normv(z)
	x = subv(x, scalev(z, dotv(x, z)))
	if dotv(x, x) < 1e-20 {
		x = perpendicular(z)
	}
	x = normv(x)
	return x, crossv(z, x), z
}

// perpendicular returns an arbitrary unit vector orthogonal to unit vector z, by
// crossing z with whichever principal axis it is least aligned with.
func perpendicular(z v3) v3 {
	switch {
	case math.Abs(z[0]) <= math.Abs(z[1]) && math.Abs(z[0]) <= math.Abs(z[2]):
		return normv(crossv(z, v3{1, 0, 0}))
	case math.Abs(z[1]) <= math.Abs(z[2]):
		return normv(crossv(z, v3{0, 1, 0}))
	default:
		return normv(crossv(z, v3{0, 0, 1}))
	}
}

func axisPlacement3D(a *step.Instance) model.Mat4 {
	m := model.Identity()
	if loc, ok := a.Ref(attrAxisLocation); ok {
		c := floatsOf(loc, attrCoordinates)
		for i := 0; i < len(c) && i < 3; i++ {
			m[12+i] = c[i]
		}
	}
	z := v3{0, 0, 1}
	if d, ok := a.Ref(attrAxisZ); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			z = v3{c[0], c[1], c[2]}
		}
	}
	x := v3{1, 0, 0}
	if d, ok := a.Ref(attrAxisX); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			x = v3{c[0], c[1], c[2]}
		}
	}
	x, y, z := orthonormalXZ(x, z)
	m[0], m[1], m[2] = x[0], x[1], x[2]
	m[4], m[5], m[6] = y[0], y[1], y[2]
	m[8], m[9], m[10] = z[0], z[1], z[2]
	return m
}

// axisPlacement2D: Location + RefDirection (X); Z fixed +Z.
func axisPlacement2D(a *step.Instance) model.Mat4 {
	m := model.Identity()
	if loc, ok := a.Ref(attrAxisLocation); ok {
		c := floatsOf(loc, attrCoordinates)
		for i := 0; i < len(c) && i < 2; i++ {
			m[12+i] = c[i]
		}
	}
	x := v3{1, 0, 0}
	if d, ok := a.Ref(attrAxis2DRefDir); ok {
		if c := floatsOf(d, attrCoordinates); len(c) >= 2 {
			x = normv(v3{c[0], c[1], 0})
		}
	}
	y := v3{-x[1], x[0], 0}
	m[0], m[1] = x[0], x[1]
	m[4], m[5] = y[0], y[1]
	return m
}

const (
	attrAxisLocation = 0
	attrAxisZ        = 1
	attrAxisX        = 2 // IfcAxis2Placement3D.RefDirection

	// attrAxis2DRefDir is IfcAxis2Placement2D.RefDirection — index 1, since the
	// 2D placement has no Axis attribute (Location=0, RefDirection=1), unlike
	// the 3D placement's [Location=0, Axis=1, RefDirection=2].
	attrAxis2DRefDir = 1
)

func dotv(a, b v3) float64      { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func subv(a, b v3) v3           { return v3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func addv(a, b v3) v3           { return v3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func scalev(a v3, s float64) v3 { return v3{a[0] * s, a[1] * s, a[2] * s} }
func crossv(a, b v3) v3 {
	return v3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func normv(a v3) v3 {
	n := dotv(a, a)
	if n == 0 {
		return a
	}
	s := 1 / math.Sqrt(n)
	return scalev(a, s)
}
