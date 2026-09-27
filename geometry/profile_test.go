package geometry

import (
	"strings"
	"testing"
)

// dProfile extrudes a D-shaped profile 1 m along +Z: the square x in [0,2],
// y in [-1,1], whose left side is replaced by segment #106, which must run
// from (0,1) to (0,-1) through x=-1 (a unit semicircle about the origin). The
// element sits at (10, 20, 5), so its true world min x is 9.
func dProfile(arc string) string {
	return "#100=IFCCARTESIANPOINT((0.,-1.));\n#101=IFCCARTESIANPOINT((2.,-1.));\n" +
		"#102=IFCCARTESIANPOINT((2.,1.));\n#103=IFCCARTESIANPOINT((0.,1.));\n" +
		"#104=IFCPOLYLINE((#100,#101,#102,#103));\n" +
		"#105=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#104);\n" +
		"#110=IFCCARTESIANPOINT((0.,0.));\n#111=IFCDIRECTION((1.,0.));\n" +
		"#112=IFCAXIS2PLACEMENT2D(#110,#111);\n" +
		arc +
		"#120=IFCCOMPOSITECURVE((#105,#106),.F.);\n" +
		"#121=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#120);\n" +
		"#122=IFCDIRECTION((0.,0.,1.));\n" +
		"#61=IFCEXTRUDEDAREASOLID(#121,#21,#122,1.);\n"
}

const (
	unitCircle = "#113=IFCCIRCLE(#112,1.);\n"
	degrees    = "#12=IFCCONVERSIONBASEDUNIT(#13,.PLANEANGLEUNIT.,'DEGREE',#14);\n" +
		"#13=IFCDIMENSIONALEXPONENTS(0,0,0,0,0,0,0);\n" +
		"#14=IFCMEASUREWITHUNIT(IFCPLANEANGLEMEASURE(0.017453292519943295),#15);\n" +
		"#15=IFCSIUNIT(*,.PLANEANGLEUNIT.,$,.RADIAN.);\n"
	radians = "#12=IFCSIUNIT(*,.PLANEANGLEUNIT.,$,.RADIAN.);\n"
)

// A composite-curve profile used to drop every segment that was not an
// IfcPolyline and extrude what was left. Revit exports stair nosings that way —
// a 10 mm IfcTrimmedCurve arc — so duplex_a's stair flights came out 9.9 mm
// short (#53). An arc must bound the profile from above; a segment that cannot
// be read must decline the profile, and the box it falls back to must still
// reach the arc.
func TestExtrude_ArcSegmentBoundsFromAbove(t *testing.T) {
	for _, tc := range []struct {
		name, unit, arc string
		want            GeomSource
	}{
		{"degree parameters", degrees, unitCircle +
			"#106=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#107);\n" +
			"#107=IFCTRIMMEDCURVE(#113,(IFCPARAMETERVALUE(90.)),(IFCPARAMETERVALUE(270.)),.T.,.PARAMETER.);\n", SourceExtrude},
		{"radian parameters", radians, unitCircle +
			"#106=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#107);\n" +
			"#107=IFCTRIMMEDCURVE(#113,(IFCPARAMETERVALUE(1.5707963267948966)),(IFCPARAMETERVALUE(4.71238898038469)),.T.,.PARAMETER.);\n", SourceExtrude},
		{"cartesian trims", "", unitCircle +
			"#106=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#107);\n" +
			"#107=IFCTRIMMEDCURVE(#113,(#103),(#100),.T.,.CARTESIAN.);\n", SourceExtrude},
		// Clockwise from 270 to 90 passes through 180 but runs from (0,-1) to
		// (0,1), so the segment's reversed sense turns it back around.
		{"sense disagreement, reversed segment", degrees, unitCircle +
			"#106=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.F.,#107);\n" +
			"#107=IFCTRIMMEDCURVE(#113,(IFCPARAMETERVALUE(270.)),(IFCPARAMETERVALUE(90.)),.F.,.PARAMETER.);\n", SourceExtrude},
		{"parameters with no angle unit", "", unitCircle +
			"#106=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#107);\n" +
			"#107=IFCTRIMMEDCURVE(#113,(IFCPARAMETERVALUE(90.)),(IFCPARAMETERVALUE(270.)),.T.,.PARAMETER.);\n", SourceOBB},
		{"arc of an ellipse", degrees, "#113=IFCELLIPSE(#112,1.,1.);\n" +
			"#106=IFCCOMPOSITECURVESEGMENT(.CONTINUOUS.,.T.,#107);\n" +
			"#107=IFCTRIMMEDCURVE(#113,(IFCPARAMETERVALUE(90.)),(IFCPARAMETERVALUE(270.)),.T.,.PARAMETER.);\n", SourceOBB},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := dProfile(tc.arc)
			if tc.unit != "" {
				data = tc.unit + data
			}
			e := buildAngled(t, data, tc.unit != "")
			if e.Source != tc.want {
				t.Errorf("source = %q, want %q", e.Source, tc.want)
			}
			if e.BBoxMin[0] > 9+1e-9 {
				t.Errorf("world min x = %v, want <= 9: the bound stops short of the arc", e.BBoxMin[0])
			}
			if tc.want == SourceExtrude && e.BBoxMin[0] < 9-0.01 {
				t.Errorf("world min x = %v, want within 1 cm of 9", e.BBoxMin[0])
			}
			if e.BBoxMax[0] < 12 {
				t.Errorf("world max x = %v, want >= 12", e.BBoxMax[0])
			}
		})
	}
}

// buildAngled builds the fixture with #12 listed in the unit assignment when
// hasUnit is set.
func buildAngled(t *testing.T, data string, hasUnit bool) Element {
	t.Helper()
	if !hasUnit {
		return buildFaceSet(t, data)
	}
	return buildFaceSetWith(t, strings.Replace(faceSetIFC(data),
		"#10=IFCUNITASSIGNMENT((#11));", "#10=IFCUNITASSIGNMENT((#11,#12));", 1))
}
