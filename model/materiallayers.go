package model

import "github.com/blox-eng/goifc/step"

// IfcMaterialLayer: [Material,LayerThickness,IsVentilated,Name,Description,Category,Priority]
const (
	attrLayerThickness    = 1
	attrLayerIsVentilated = 2
	attrLayerCategory     = 5
)

// IfcMaterialLayerSetUsage: [ForLayerSet,LayerSetDirection,DirectionSense,OffsetFromReferenceLine,ReferenceExtent]
const (
	attrLayerSetDirection = 1
	attrDirectionSense    = 2
)

// IfcMaterial: [Name,Description,Category] (IFC4); [Name] (IFC2X3). Name is index
// 0 in both, so reading it is schema-agnostic.
const attrMaterialName = 0

// MaterialLayer is one IfcMaterialLayer with its per-layer attributes preserved.
// materialLeaves (element.go) walks the same list but flattens each layer to its
// IfcMaterial, which is exactly the information loss MaterialLayer exists to stop.
//
// Every field is absent-safe: a nil pointer or an empty string means the
// attribute was $ (unset) or, for IsVentilated, .U. (EXPRESS LOGICAL unknown).
// A missing attribute is never coerced to a zero — a phantom 0 mm layer or a
// phantom "solid" reads as measured fact downstream.
//
// IfcMaterialLayer.Priority (0-100, joint/corner interpenetration) is read by
// nothing here on purpose — it is named here so callers know it exists rather
// than silently losing it.
type MaterialLayer struct {
	// MaterialName is IfcMaterial.Name verbatim; "" when the layer's Material is
	// unset. NOT a catalog key — base.materials is priced and has no external_id.
	MaterialName string
	// ThicknessMm is LayerThickness converted to millimetres; nil when unset.
	ThicknessMm *float64
	// IsVentilated is THREE-VALUED: true (.T., air gap exchanging with outside),
	// false (.F., solid), nil (.U. unknown, or the attribute absent). TRUE and nil
	// must never be collapsed — a gap without exchange is not a solid layer.
	IsVentilated *bool
	// Category is IfcMaterialLayer.Category verbatim; "" when unset. IFC's
	// recommended keywords (LoadBearing / Insulation / Inner finish / Outer
	// finish) are coarser than our LayerRole vocabulary, and this field is 0%
	// populated in every model measured for this milestone. Callers map it or
	// leave role null — they never infer a role from MaterialName.
	Category string
}

// LayerSet is an IfcMaterialLayerSet as read for one instance, in declared
// EXPRESS LIST order.
type LayerSet struct {
	Layers []MaterialLayer
	// Direction is IfcMaterialLayerSetUsage.LayerSetDirection (AXIS1/AXIS2/AXIS3);
	// "" when the instance carries a bare IfcMaterialLayerSet with no usage. This
	// package emits the IFC label verbatim — mapping it onto a product vocabulary
	// is the consumer's job, not the parser's.
	Direction string
	// Sense is IfcMaterialLayerSetUsage.DirectionSense (POSITIVE/NEGATIVE); ""
	// when absent. It is read so it is not lost silently, and deliberately does
	// NOT reorder Layers: the sense says which way the stack runs from the
	// reference line, which is not enough on its own to say which end is
	// "outside" (that needs the element's placement too). Reordering on a guess
	// would invert a build-up; preserving the declared order does not.
	Sense string
}

// MaterialLayers returns the layer set associated with inst, preserving each
// layer's attributes and the declared list order. scale converts raw file length
// units to metres (model.UnitScale) — thicknesses come back in millimetres.
//
// inst may be an IfcTypeObject (which typically carries a bare
// IfcMaterialLayerSet) or an occurrence (which typically carries an
// IfcMaterialLayerSetUsage wrapping the same set). Both reach the same layers;
// only the occurrence path yields Direction/Sense.
//
// An instance with no material association, or one associated with something
// that is not a layer set (IfcMaterialList, a profile set, a bare IfcMaterial),
// yields a zero LayerSet. Those shapes carry no ordered build-up.
func MaterialLayers(f *step.File, inst *step.Instance, scale float64) LayerSet {
	for _, rel := range f.Inverse(inst) {
		if !rel.IsA("IfcRelAssociatesMaterial") {
			continue
		}
		m, ok := rel.Ref(attrRel5) // RelatingMaterial
		if !ok {
			continue
		}
		out := LayerSet{}
		set := m
		if m.IsA("IfcMaterialLayerSetUsage") {
			out.Direction = enumAt(m, attrLayerSetDirection)
			out.Sense = enumAt(m, attrDirectionSense)
			s, ok := m.Ref(attrForLayerSet)
			if !ok {
				continue
			}
			set = s
		}
		if !set.IsA("IfcMaterialLayerSet") {
			continue
		}
		v, ok := set.Get(attrMaterialLayers)
		if !ok || v.Kind != step.KindList {
			continue
		}
		for _, item := range v.List {
			if item.Kind != step.KindRef || item.Ref == nil || !item.Ref.IsA("IfcMaterialLayer") {
				continue
			}
			out.Layers = append(out.Layers, readMaterialLayer(item.Ref, scale))
		}
		if len(out.Layers) > 0 {
			return out
		}
	}
	return LayerSet{}
}

func readMaterialLayer(l *step.Instance, scale float64) MaterialLayer {
	out := MaterialLayer{Category: strVal(l, attrLayerCategory)}
	if mat, ok := l.Ref(attrLayerMaterial); ok {
		out.MaterialName = strVal(mat, attrMaterialName)
	}
	if raw, ok := floatAt(l, attrLayerThickness); ok {
		mm := raw * scale * 1000
		out.ThicknessMm = &mm
	}
	// KindBool is .T./.F. only. .U. parses as KindLogical and $ as KindNull, and
	// both leave IsVentilated nil — the UNKNOWN third state.
	if v, ok := l.Get(attrLayerIsVentilated); ok && v.Kind == step.KindBool {
		b := v.B
		out.IsVentilated = &b
	}
	return out
}

func enumAt(inst *step.Instance, idx int) string {
	v, ok := inst.Get(idx)
	if !ok || v.Kind != step.KindEnum {
		return ""
	}
	return v.Str
}
