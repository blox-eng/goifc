package geometry

import (
	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

const (
	attrMapSource      = 0 // IfcMappedItem.MappingSource -> IfcRepresentationMap
	attrMapTarget      = 1 // IfcMappedItem.MappingTarget -> IfcCartesianTransformationOperator3D
	attrMapOrigin      = 0 // IfcRepresentationMap.MappingOrigin (IfcAxis2Placement3D)
	attrMappedRep      = 1 // IfcRepresentationMap.MappedRepresentation (IfcShapeRepresentation)
	attrCTOAxis1       = 0
	attrCTOAxis2       = 1
	attrCTOLocalOrigin = 2
	attrCTOScale       = 3
	attrCTOAxis3       = 4
	attrCTOScale2      = 5 // IfcCartesianTransformationOperator3DNonUniform.Scale2 (Y)
	attrCTOScale3      = 6 // IfcCartesianTransformationOperator3DNonUniform.Scale3 (Z)
)

// mappedItemMesh tessellates the mapped representation and composes
// target * origin onto its element-local vertices (raw units, scaled to meters).
// depth bounds recursion into nested IfcMappedItem chains (see maxMapDepth) —
// a cyclic or pathologically-nested mapped structure in an untrusted IFC
// upload returns gracefully (ok=false) instead of overflowing the stack.
func mappedItemMesh(item *step.Instance, unitScale float64, depth int) (verts []float32, tris []uint32, src GeomSource, ok bool) {
	if depth >= maxMapDepth {
		return nil, nil, SourceOBB, false
	}
	repMap, has := item.Ref(attrMapSource)
	if !has {
		return nil, nil, SourceOBB, false
	}
	mappedRep, has := repMap.Ref(attrMappedRep)
	if !has {
		return nil, nil, SourceOBB, false
	}
	xform := model.Identity()
	if origin, has := repMap.Ref(attrMapOrigin); has {
		xform = axisPlacement3D(origin)
	}
	if target, has := item.Ref(attrMapTarget); has {
		xform = transformOperator3D(target).Mul(xform) // target ∘ origin
	}
	src = SourceOBB
	itemsV, has := mappedRep.Get(attrRepresentationItems)
	if !has || itemsV.Kind != step.KindList {
		return nil, nil, SourceOBB, false
	}
	// mv is already in meters; apply the (unitless-rotation + raw-translation)
	// mapping transform, whose translation is raw units → scale it too. Loop-invariant
	// (same xform for every item), so compute once rather than per iteration.
	x := scaleTransformTranslation(xform, unitScale)
	for _, iv := range itemsV.List {
		if iv.Kind != step.KindRef || iv.Ref == nil {
			continue
		}
		mv, mt, ms := tessellateItemDepth(iv.Ref, unitScale, depth+1) // recurse into A/B/C in scaled meters
		if len(mv) == 0 {
			continue
		}
		tv := transformVerts(mv, x)
		appendMesh(&verts, &tris, tv, mt)
		src = promoteSource(src, ms)
	}
	return verts, tris, src, len(tris) > 0
}

// transformOperator3D builds a 4x4 from IfcCartesianTransformationOperator3D.
func transformOperator3D(op *step.Instance) model.Mat4 {
	x := v3{1, 0, 0}
	if d, ok := op.Ref(attrCTOAxis1); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			x = normv(v3{c[0], c[1], c[2]})
		}
	}
	z := v3{0, 0, 1}
	if d, ok := op.Ref(attrCTOAxis3); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			z = normv(v3{c[0], c[1], c[2]})
		}
	}
	// Axis2 (Y) is deliberately NOT read: the IFC IfcCartesianTransformationOperator
	// derivation fixes Y = Z×X (see orthonormalize below), so a supplied Axis2 is
	// not consulted — matching ifcopenshell's kernel.
	scale := 1.0
	if sv, ok := op.Get(attrCTOScale); ok && sv.Kind == step.KindFloat {
		scale = sv.F
	}
	// IfcCartesianTransformationOperator3DNonUniform adds Scale2 (Y, attr 5) and
	// Scale3 (Z, attr 6), each defaulting to Scale when unset — a plain uniform
	// operator (or a NonUniform instance omitting them) keeps all three axes equal.
	// Missing this bites hard: a beam/column swept along a 1mm-deep parametric
	// profile and stretched via Scale3 (seen up to ~5000x in real Rhino exports)
	// renders as a ~1mm sliver instead of a multi-meter member if only the
	// uniform Scale is applied.
	scaleY, scaleZ := scale, scale
	if op.IsA("IfcCartesianTransformationOperator3DNonUniform") {
		if sv, ok := op.Get(attrCTOScale2); ok && sv.Kind == step.KindFloat {
			scaleY = sv.F
		}
		if sv, ok := op.Get(attrCTOScale3); ok && sv.Kind == step.KindFloat {
			scaleZ = sv.F
		}
	}
	// Orthonormalize per IFC IfcCartesianTransformationOperator derive: z primary,
	// x made ⟂ z, y = z×x. A raw independent-normalize (Axis1/2/3 as-is) skews any
	// rotated family instance whose Axis2 is defaulted.
	x, y, z := orthonormalXZ(x, z)
	m := model.Identity()
	m[0], m[1], m[2] = x[0]*scale, x[1]*scale, x[2]*scale
	m[4], m[5], m[6] = y[0]*scaleY, y[1]*scaleY, y[2]*scaleY
	m[8], m[9], m[10] = z[0]*scaleZ, z[1]*scaleZ, z[2]*scaleZ
	if loc, ok := op.Ref(attrCTOLocalOrigin); ok {
		if c := floatsOf(loc, attrCoordinates); len(c) == 3 {
			m[12], m[13], m[14] = c[0], c[1], c[2]
		}
	}
	return m
}

// scaleTransformTranslation returns m with its translation column scaled (rep-unit
// translations → meters); the rotation/scale 3x3 is unitless and untouched.
func scaleTransformTranslation(m model.Mat4, s float64) model.Mat4 {
	m[12] *= s
	m[13] *= s
	m[14] *= s
	return m
}

func transformVerts(verts []float32, m model.Mat4) []float32 {
	out := make([]float32, len(verts))
	for i := 0; i+2 < len(verts); i += 3 {
		w := applyMat(m, v3{float64(verts[i]), float64(verts[i+1]), float64(verts[i+2])})
		out[i], out[i+1], out[i+2] = float32(w[0]), float32(w[1]), float32(w[2])
	}
	return out
}
