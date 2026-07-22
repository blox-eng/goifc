package model

import (
	"math"
	"testing"
)

func TestUnitScaleMillimetre(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/units_mm.ifc"))
	if s := UnitScale(f); math.Abs(s-0.001) > 1e-12 {
		t.Fatalf("mm scale = %v, want 0.001", s)
	}
}

func TestUnitScaleDefaultsToOne(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\nENDSEC;\nEND-ISO-10303-21;\n")
	if s := UnitScale(f); s != 1.0 {
		t.Fatalf("no unit assignment scale = %v, want 1.0", s)
	}
}

// units_imperial.ifc is a realistic foot IfcConversionBasedUnit: it embeds an
// inner IfcSIUnit(.LENGTHUNIT.,.METRE.) inside its ConversionFactor, which used
// to trip a global ByType("IfcSIUnit") scan into finding that INNER unit and
// reporting scale=1.0 with no warning. UnitScale must resolve the TOP-LEVEL
// length unit instead and correctly compute foot = 0.3048m.
func TestUnitScaleConversionBasedFoot(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/units_imperial.ifc"))
	if s := UnitScale(f); math.Abs(s-0.3048) > 1e-6 {
		t.Fatalf("UnitScale = %v, want 0.3048 for a foot IfcConversionBasedUnit", s)
	}
	if UnitIsUnhandled(f) {
		t.Fatalf("UnitIsUnhandled = true, want false: a resolvable IfcConversionBasedUnit is now handled (no warning)")
	}
}

func TestUnitIsUnhandledFalseForSI(t *testing.T) {
	f := parseString(t, mustRead(t, "testdata/synthetic/units_mm.ifc"))
	if UnitIsUnhandled(f) {
		t.Fatalf("UnitIsUnhandled = true, want false for a recognized IfcSIUnit LENGTHUNIT")
	}
}

func TestUnitIsUnhandledFalseForNoUnits(t *testing.T) {
	f := parseString(t, "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\nENDSEC;\nEND-ISO-10303-21;\n")
	if UnitIsUnhandled(f) {
		t.Fatalf("UnitIsUnhandled = true, want false when no unit assignment is present at all")
	}
}
