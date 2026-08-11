package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/step"
)

// Regression for a review finding: axisPlacement2D read RefDirection via the
// 3D placement's attribute index (2), which is out of range on
// IfcAxis2Placement2D ([Location=0, RefDirection=1]) — silently dropping any
// rotation on a 2D profile Position. A +90 degree RefDirection (0.,1.) must
// rotate the local x-axis (1,0) to world (0,1).
func TestAxisPlacement2D_RefDirectionRotation(t *testing.T) {
	const src = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('axis2d','',(''),(''),'','','');
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCCARTESIANPOINT((0.,0.));
#2=IFCDIRECTION((0.,1.));
#3=IFCAXIS2PLACEMENT2D(#1,#2);
ENDSEC;
END-ISO-10303-21;
`
	f, err := step.ParseBytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	inst, ok := f.ByID(3)
	if !ok {
		t.Fatal("instance #3 not found")
	}
	m := axisPlacement2D(inst)
	got := applyMat(m, v3{1, 0, 0})
	want := v3{0, 1, 0}
	if math.Abs(got[0]-want[0]) > 1e-9 || math.Abs(got[1]-want[1]) > 1e-9 {
		t.Errorf("axisPlacement2D rotation wrong: got (%f,%f), want (%f,%f)", got[0], got[1], want[0], want[1])
	}
}
