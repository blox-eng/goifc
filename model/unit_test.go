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

func unitFile(units, list string) string {
	return "ISO-10303-21;\nHEADER;\nFILE_SCHEMA(('IFC4'));\nENDSEC;\nDATA;\n" + units +
		"#10=IFCUNITASSIGNMENT((" + list + "));\nENDSEC;\nEND-ISO-10303-21;\n"
}

// A trimmed circle's parameters are plane angles in the file's unit, so the
// arc a Revit (degree) file means cannot be read without it.
func TestPlaneAngleScale(t *testing.T) {
	const radian = "#15=IFCSIUNIT(*,.PLANEANGLEUNIT.,$,.RADIAN.);\n"
	const degree = "#13=IFCDIMENSIONALEXPONENTS(0,0,0,0,0,0,0);\n" +
		"#14=IFCMEASUREWITHUNIT(IFCPLANEANGLEMEASURE(0.017453292519943295),#15);\n" +
		"#12=IFCCONVERSIONBASEDUNIT(#13,.PLANEANGLEUNIT.,'DEGREE',#14);\n"
	const metre = "#11=IFCSIUNIT(*,.LENGTHUNIT.,$,.METRE.);\n"
	for _, tc := range []struct {
		name, units, list string
		want              float64
		ok                bool
	}{
		{"radian", metre + radian, "#11,#15", 1, true},
		// The degree's ConversionFactor nests a radian IfcSIUnit that is not
		// in the assignment; only the top-level unit counts.
		{"degree", metre + radian + degree, "#11,#12", math.Pi / 180, true},
		{"none", metre, "#11", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := PlaneAngleScale(parseString(t, unitFile(tc.units, tc.list)))
			if ok != tc.ok || math.Abs(got-tc.want) > 1e-15 {
				t.Errorf("PlaneAngleScale = %v, %v; want %v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
