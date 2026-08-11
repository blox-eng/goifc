package model

import "github.com/blox-eng/goifc/step"

// IfcSIUnit: [Dimensions, UnitType, Prefix, Name]
const (
	attrUnitType = 1
	attrPrefix   = 2
	attrSIName   = 3
)

// IfcConversionBasedUnit: [Dimensions, UnitType, Name, ConversionFactor] — UnitType
// is at the same index as IfcSIUnit's, so attrUnitType is reused.
const attrConversionFactor = 3

// IfcMeasureWithUnit: [ValueComponent, UnitComponent]
const (
	attrMeasureValue = 0
	attrMeasureUnit  = 1
)

// IfcProject: attr index 8 = UnitsInContext (-> IfcUnitAssignment).
const attrProjectUnitsInContext = 8

// IfcUnitAssignment: attr index 0 = Units (a list of unit refs).
const attrUnitAssignmentUnits = 0

var siPrefix = map[string]float64{
	"EXA": 1e18, "PETA": 1e15, "TERA": 1e12, "GIGA": 1e9, "MEGA": 1e6, "KILO": 1e3,
	"HECTO": 1e2, "DECA": 1e1, "DECI": 1e-1, "CENTI": 1e-2, "MILLI": 1e-3,
	"MICRO": 1e-6, "NANO": 1e-9, "PICO": 1e-12, "FEMTO": 1e-15, "ATTO": 1e-18,
}

// unitAssignment locates the file's IfcUnitAssignment: prefers
// IfcProject.UnitsInContext (real files always have an IfcProject), falling back
// to the first bare IfcUnitAssignment in the file (our synthetic fixtures have no
// IfcProject).
func unitAssignment(f *step.File) *step.Instance {
	for _, p := range f.ByType("IfcProject") {
		if ua, ok := p.Ref(attrProjectUnitsInContext); ok && ua.IsA("IfcUnitAssignment") {
			return ua
		}
	}
	if uas := f.ByType("IfcUnitAssignment"); len(uas) > 0 {
		return uas[0]
	}
	return nil
}

// topLevelLengthUnit returns the top-level LENGTHUNIT member of the file's
// IfcUnitAssignment.Units, if any.
func topLevelLengthUnit(f *step.File) (*step.Instance, bool) {
	ua := unitAssignment(f)
	if ua == nil {
		return nil, false
	}
	units, ok := ua.Get(attrUnitAssignmentUnits)
	if !ok || units.Kind != step.KindList {
		return nil, false
	}
	for _, u := range units.List {
		if u.Kind != step.KindRef || u.Ref == nil {
			continue
		}
		if enumEq(u.Ref, attrUnitType, "LENGTHUNIT") {
			return u.Ref, true
		}
	}
	return nil, false
}

// siScale returns an IfcSIUnit's scale relative to its base unit (prefix
// multiplier; METRE with no prefix = 1.0).
func siScale(u *step.Instance) float64 {
	scale := 1.0
	if p, ok := u.Get(attrPrefix); ok && p.Kind == step.KindEnum {
		if m, hit := siPrefix[p.Str]; hit {
			scale = m
		}
	}
	return scale
}

// maxConversionUnitDepth caps IfcConversionBasedUnit chain recursion. Real
// files chain 1-2 deep (e.g. inch -> foot -> metre); this is panic-safety
// symmetry with containerOf's depth guard, protecting against a
// cyclic/malformed ConversionFactor chain stack-overflowing.
const maxConversionUnitDepth = 8

// conversionScale resolves an IfcConversionBasedUnit's scale to meters, porting
// ifcopenshell.util.unit.calculate_unit_scale's loop: multiply by
// ConversionFactor.ValueComponent, then recurse/resolve ConversionFactor.
// UnitComponent (an IfcConversionBasedUnit chains further; an IfcSIUnit
// terminates, contributing its own prefix). ok is false if the factor can't be
// resolved (unrealistic/malformed file).
func conversionScale(u *step.Instance) (float64, bool) {
	return conversionScaleDepth(u, maxConversionUnitDepth)
}

func conversionScaleDepth(u *step.Instance, depth int) (float64, bool) {
	if depth <= 0 {
		return 0, false
	}
	cf, ok := u.Ref(attrConversionFactor)
	if !ok || !cf.IsA("IfcMeasureWithUnit") {
		return 0, false
	}
	valAttr, ok := cf.Get(attrMeasureValue)
	if !ok {
		return 0, false
	}
	value, ok := measureFloat(valAttr)
	if !ok {
		return 0, false
	}
	base, ok := cf.Ref(attrMeasureUnit)
	if !ok {
		return 0, false
	}
	switch {
	case base.IsA("IfcSIUnit"):
		return value * siScale(base), true
	case base.IsA("IfcConversionBasedUnit"):
		baseScale, ok := conversionScaleDepth(base, depth-1)
		if !ok {
			return 0, false
		}
		return value * baseScale, true
	default:
		return 0, false
	}
}

// measureFloat unwraps a STEP value that holds a numeric IfcMeasure: either a
// bare KindFloat/KindInt, or a KindTyped wrapper (e.g. IFCLENGTHMEASURE(0.3048))
// whose sole inner element is the numeric literal.
func measureFloat(v step.Value) (float64, bool) {
	switch v.Kind {
	case step.KindFloat:
		return v.F, true
	case step.KindInt:
		return float64(v.I), true
	case step.KindTyped:
		if len(v.List) != 1 {
			return 0, false
		}
		return measureFloat(v.List[0])
	default:
		return 0, false
	}
}

// UnitScale returns meters per the file's length unit (ports
// ifcopenshell.util.unit.calculate_unit_scale for the LENGTHUNIT). It resolves
// the TOP-LEVEL length unit of IfcProject.UnitsInContext (or the first bare
// IfcUnitAssignment) — never a global scan, which would find an IfcSIUnit nested
// inside an IfcConversionBasedUnit's ConversionFactor and silently misreport a
// foot/inch file as 1.0. Returns 1.0 when no resolvable LENGTHUNIT is present.
func UnitScale(f *step.File) float64 {
	u, ok := topLevelLengthUnit(f)
	if !ok {
		return 1.0
	}
	switch {
	case u.IsA("IfcSIUnit"):
		return siScale(u) // METRE base = 1m; prefix multiplies
	case u.IsA("IfcConversionBasedUnit"):
		if scale, ok := conversionScale(u); ok {
			return scale
		}
	}
	return 1.0
}

// UnitIsUnhandled reports whether the file's top-level length unit is neither a
// resolvable IfcSIUnit nor a resolvable IfcConversionBasedUnit (e.g. no length
// unit at all, or a conversion factor that couldn't be resolved). A proper
// IfcConversionBasedUnit (foot/inch) is now correctly scaled by UnitScale, so it
// no longer counts as unhandled.
func UnitIsUnhandled(f *step.File) bool {
	u, ok := topLevelLengthUnit(f)
	if !ok {
		return false // no length unit at all: UnitScale's silent-metres default, not new behavior
	}
	if u.IsA("IfcSIUnit") {
		return false
	}
	if u.IsA("IfcConversionBasedUnit") {
		_, resolved := conversionScale(u)
		return !resolved
	}
	return true // exotic/unresolvable unit type
}

func enumEq(inst *step.Instance, idx int, label string) bool {
	v, ok := inst.Get(idx)
	return ok && v.Kind == step.KindEnum && v.Str == label
}
