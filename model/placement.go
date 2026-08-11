package model

import (
	"math"

	"github.com/blox-eng/goifc/step"
)

// LocalPlacement returns the world transform of inst's ObjectPlacement, composing
// the IfcLocalPlacement.PlacementRelTo chain (child placed relative to parent).
// Ports ifcopenshell.util.placement.get_local_placement. Units are the file's
// native length unit (Extract scales to meters). Missing/!IfcLocalPlacement -> identity.
func LocalPlacement(inst *step.Instance) Mat4 {
	place, ok := inst.Ref(attrObjectPlacement)
	if !ok {
		return Identity()
	}
	return localPlacementMatrix(place)
}

func localPlacementMatrix(place *step.Instance) Mat4 {
	if place == nil || !place.IsA("IfcLocalPlacement") {
		return Identity()
	}
	// world = parent(PlacementRelTo) * relative(RelativePlacement)
	parent := Identity()
	if p, ok := place.Ref(attrPlacementRelTo); ok {
		parent = localPlacementMatrix(p)
	}
	rel := Identity()
	if a, ok := place.Ref(attrRelativePlacement); ok {
		rel = axis2placement(a)
	}
	return parent.Mul(rel)
}

// axis2placement builds a 4x4 from IfcAxis2Placement3D (Location + optional Z axis
// + optional X ref direction). 2D and defaulted axes fall back to world axes.
func axis2placement(a *step.Instance) Mat4 {
	m := Identity()
	if loc, ok := a.Ref(attrAxisLocation); ok {
		c := coords(loc)
		for i := 0; i < len(c) && i < 3; i++ {
			m[12+i] = c[i]
		}
	}
	z := []float64{0, 0, 1}
	if d, ok := a.Ref(attrAxisZ); ok {
		if v := coords(d); len(v) == 3 {
			z = normalize(v)
		}
	}
	x := []float64{1, 0, 0}
	if d, ok := a.Ref(attrAxisX); ok {
		if v := coords(d); len(v) == 3 {
			x = v
		}
	}
	// Gram-Schmidt: make x orthonormal to z, y = z × x.
	x = normalize(sub(x, scale(z, dot(x, z))))
	y := cross(z, x)
	// Columns 0,1,2 are the basis vectors x,y,z (column-major).
	m[0], m[1], m[2] = x[0], x[1], x[2]
	m[4], m[5], m[6] = y[0], y[1], y[2]
	m[8], m[9], m[10] = z[0], z[1], z[2]
	return m
}

// coords extracts the float list from an IfcCartesianPoint / IfcDirection.
func coords(inst *step.Instance) []float64 {
	v, ok := inst.Get(attrCoordinates)
	if !ok || v.Kind != step.KindList {
		return nil
	}
	out := make([]float64, 0, len(v.List))
	for _, e := range v.List {
		switch e.Kind {
		case step.KindFloat:
			out = append(out, e.F)
		case step.KindInt:
			out = append(out, float64(e.I))
		}
	}
	return out
}

func dot(a, b []float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func sub(a, b []float64) []float64 { return []float64{a[0]-b[0], a[1]-b[1], a[2]-b[2]} }
func scale(a []float64, s float64) []float64 { return []float64{a[0]*s, a[1]*s, a[2]*s} }
func cross(a, b []float64) []float64 {
	return []float64{a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2], a[0]*b[1]-a[1]*b[0]}
}
func normalize(a []float64) []float64 {
	n := dot(a, a)
	if n == 0 {
		return a
	}
	s := 1 / math.Sqrt(n)
	return scale(a, s)
}
