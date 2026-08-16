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
	// reading one would yield (0,1) and mask the real north.
	f := trueNorthFile(t, `#1=IFCDIRECTION((1.,0.));
#2=IFCGEOMETRICREPRESENTATIONCONTEXT($,'Model',3,1.E-5,$,#1);
#3=IFCGEOMETRICREPRESENTATIONSUBCONTEXT('Body','Model',*,*,*,*,#2,$,.MODEL_VIEW.,$);
`)
	if got := TrueNorth(f); math.Abs(got[0]-1) > 1e-12 {
		t.Fatalf("TrueNorth = %v, want (1,0)", got)
	}
}
