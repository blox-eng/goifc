package model

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/step"
)

func trueNorthFile(t *testing.T, body string) *step.File {
	t.Helper()
	return parseString(t, "ISO-10303-21;\nHEADER;\nENDSEC;\nDATA;\n"+body+"ENDSEC;\nEND-ISO-10303-21;\n")
}

func TestTrueNorthReadsDirection(t *testing.T) {
	f := trueNorthFile(t, `#1=IFCDIRECTION((1.,0.));
#2=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#1);
`)
	got := TrueNorth(f)
	if math.Abs(got[0]-1) > 1e-12 || math.Abs(got[1]) > 1e-12 {
		t.Fatalf("TrueNorth = %v, want (1,0)", got)
	}
}

func TestTrueNorthNormalizes(t *testing.T) {
	f := trueNorthFile(t, `#1=IFCDIRECTION((3.,4.));
#2=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#1);
`)
	got := TrueNorth(f)
	if math.Abs(got[0]-0.6) > 1e-12 || math.Abs(got[1]-0.8) > 1e-12 {
		t.Fatalf("TrueNorth = %v, want (0.6,0.8)", got)
	}
}

func TestTrueNorthDefaults(t *testing.T) {
	cases := map[string]string{
		"absent attribute": `#2=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,$);
`,
		"zero length": `#1=IFCDIRECTION((0.,0.));
#2=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#1);
`,
		"one ratio": `#1=IFCDIRECTION((1.));
#2=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#1);
`,
		"no context at all": `#1=IFCPROJECT('x',$,'p',$,$,$,$,$,$);
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got := TrueNorth(trueNorthFile(t, body))
			if got != [2]float64{0, 1} {
				t.Fatalf("TrueNorth = %v, want (0,1)", got)
			}
		})
	}
}

func TestTrueNorthIgnoresSubContext(t *testing.T) {
	// A sub-context inherits TrueNorth from its parent and does not restate it;
	// reading one would yield the wrong direction and mask the real north.
	//
	// #2 is encoded as a STEP complex instance combining both the context and
	// sub-context parts. This is deliberate: step.File.ByType's exact-match
	// semantics already exclude an ordinary simple
	// IfcGeometricRepresentationSubContext instance, so a test built on that
	// shape would pass even with the IsA guard deleted. Registering #2 under
	// both part types (see step/parse.go finishComplexInstance) makes it
	// reachable through ByType("IfcGeometricRepresentationContext"), so this
	// test only passes if the guard actually filters it out.
	//
	// #2 also carries the lower express ID (2 < 5) and a decoy direction
	// (0,-1): if the guard were removed, lowest-ID selection would pick #2 and
	// TrueNorth would return (0,-1) instead of the real #5 north (1,0).
	f := trueNorthFile(t, `#1=IFCDIRECTION((0.,-1.));
#2=(IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#1)IFCGEOMETRICREPRESENTATIONSUBCONTEXT('Body','Model',*,*,*,*,#2,$,.MODEL_VIEW.,$));
#4=IFCDIRECTION((1.,0.));
#5=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#4);
`)
	got := TrueNorth(f)
	if math.Abs(got[0]-1) > 1e-12 || math.Abs(got[1]) > 1e-12 {
		t.Fatalf("TrueNorth = %v, want (1,0) from #5, not the filtered complex sub-context #2", got)
	}
}

func TestTrueNorthLowestIDWins(t *testing.T) {
	// Two top-level contexts, out of file order: #9 is written first but #3 has
	// the lower express ID and must win regardless. If the ID comparison were
	// reversed, dropped, or replaced with "first seen", this would return #9's
	// (0,-1) instead of #3's (1,0).
	f := trueNorthFile(t, `#7=IFCDIRECTION((0.,-1.));
#9=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#7);
#1=IFCDIRECTION((1.,0.));
#3=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#1);
`)
	got := TrueNorth(f)
	if math.Abs(got[0]-1) > 1e-12 || math.Abs(got[1]) > 1e-12 {
		t.Fatalf("TrueNorth = %v, want (1,0) from #3 (lowest ID), not #9", got)
	}
}
