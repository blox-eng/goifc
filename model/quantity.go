package model

import (
	"github.com/blox-eng/common/ifc/step"
)

// IfcQuantity{Area,Volume,Length,...}: [Name,Description,Unit,<Value>,Formula] → value idx 3.
const attrQuantityValue = 3

var (
	areaNames = []string{"NetSideArea", "NetArea", "GrossSideArea", "GrossArea", "Area"}
	volNames  = []string{"NetVolume", "GrossVolume", "Volume"}
)

// QtoQuantities reads tier-1 quantities from IfcElementQuantity sets attached to
// inst via IfcRelDefinesByProperties, merged with quantities inherited from
// inst's TYPE (HasPropertySets), matching ifcopenshell's
// get_psets(should_inherit=True) order: type quantities are seeded first, then
// occurrence quantities override on name collision (occurrence-internal
// collisions keep decompose.py's first-wins-per-set semantics). Lengths scale
// by `scale`, areas by scale^2, volumes by scale^3 (values are stored in file
// length units). ok is false when no quantity set yields an area (the tier-1
// gate, matching decompose.py).
func QtoQuantities(f *step.File, inst *step.Instance, scale float64) (Quantities, bool) {
	flat := map[string]float64{}
	if typ := GetType(f, inst); typ != nil {
		for _, qtoSet := range typePropertySets(typ, true) {
			for name, v := range qtoSet {
				if val, ok := v.(float64); ok {
					flat[name] = val
				}
			}
		}
	}
	occurrence := map[string]float64{}
	for _, rel := range f.Inverse(inst) {
		if !rel.IsA("IfcRelDefinesByProperties") {
			continue
		}
		def, ok := rel.Ref(attrRel5)
		if !ok || !def.IsA("IfcElementQuantity") {
			continue
		}
		qv, ok := def.Get(attrQuantities)
		if !ok || qv.Kind != step.KindList {
			continue
		}
		for _, q := range qv.List {
			if q.Kind != step.KindRef || q.Ref == nil {
				continue
			}
			name := strVal(q.Ref, attrPropName)
			val, has := floatAt(q.Ref, attrQuantityValue)
			if name == "" || !has {
				continue
			}
			if _, exists := occurrence[name]; !exists {
				occurrence[name] = val // first-wins across occurrence quantity sets, matching decompose.py's setdefault
			}
		}
	}
	for name, val := range occurrence {
		flat[name] = val // occurrence overrides type, matching get_psets inheritance order
	}
	if len(flat) == 0 {
		return Quantities{}, false
	}
	pick := func(names []string, power int) *float64 {
		for _, n := range names {
			if v, ok := flat[n]; ok {
				scaled := v * pow(scale, power)
				return &scaled
			}
		}
		return nil
	}
	q := Quantities{
		Area:      pick(areaNames, 2),
		Volume:    pick(volNames, 3),
		Length:    pick([]string{"Length"}, 1),
		Width:     pick([]string{"Width", "Thickness"}, 1),
		Height:    pick([]string{"Height"}, 1),
		Perimeter: pick([]string{"Perimeter"}, 1),
	}
	return q, q.Area != nil
}

func strVal(inst *step.Instance, idx int) string {
	v, ok := inst.Get(idx)
	if !ok || v.Kind != step.KindString {
		return ""
	}
	return v.Str
}

func floatAt(inst *step.Instance, idx int) (float64, bool) {
	v, ok := inst.Get(idx)
	if !ok {
		return 0, false
	}
	switch v.Kind {
	case step.KindFloat:
		return v.F, true
	case step.KindInt:
		return float64(v.I), true
	}
	return 0, false
}

func pow(base float64, exp int) float64 {
	r := 1.0
	for i := 0; i < exp; i++ {
		r *= base
	}
	return r
}
