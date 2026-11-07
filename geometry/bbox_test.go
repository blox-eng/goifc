package geometry

import (
	"fmt"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/step"
)

// TestCollectPoints_DepthBound proves collectPoints' walk terminates on a
// deeply-nested (but acyclic, so the seen-set alone doesn't help) forward-ref
// chain instead of recursing until it stack-overflows the worker — the same
// untrusted-input hardening maxMapDepth already provides for mapped items.
// The chain is built well past maxWalkDepth; a real IFC representation
// subgraph is only a handful of levels deep.
func TestCollectPoints_DepthBound(t *testing.T) {
	const chainLen = maxWalkDepth + 1000

	var b strings.Builder
	b.WriteString("ISO-10303-21;\nHEADER;\n")
	b.WriteString("FILE_DESCRIPTION((''),'2;1');\n")
	b.WriteString("FILE_NAME('deep.ifc','',(''),(''),'','','');\n")
	b.WriteString("FILE_SCHEMA(('IFC4'));\n")
	b.WriteString("ENDSEC;\nDATA;\n")

	// #1 is the root; each IFCLOCALPLACEMENT's first (PlacementRelTo) attribute
	// chains to the next instance, terminating at a real IFCCARTESIANPOINT far
	// past maxWalkDepth. collectPoints doesn't validate entity semantics, so a
	// simple ref chain via a real IFC entity type exercises the same forward-ref
	// walk as a pathological representation subgraph would.
	for id := 1; id <= chainLen; id++ {
		fmt.Fprintf(&b, "#%d=IFCLOCALPLACEMENT(#%d,$);\n", id, id+1)
	}
	fmt.Fprintf(&b, "#%d=IFCCARTESIANPOINT((1.,2.,3.));\n", chainLen+1)
	b.WriteString("ENDSEC;\nEND-ISO-10303-21;\n")

	f, err := step.ParseBytes([]byte(b.String()))
	if err != nil {
		t.Fatalf("parse deep chain: %v", err)
	}
	root, ok := f.ByID(1)
	if !ok {
		t.Fatal("root instance #1 missing")
	}

	pts := collectPoints(root, nil)

	// The point past maxWalkDepth must NOT be reached — proves the bound
	// actually stopped the walk (rather than the chain just happening to be
	// short enough to fully traverse).
	if len(pts) != 0 {
		t.Errorf("collectPoints found %d points, want 0 (the real point sits beyond maxWalkDepth=%d)", len(pts), maxWalkDepth)
	}
}

// TestCollectPointsPlacesAnExtrudedProfile proves the box walk keeps an
// extruded solid's profile points out of the box once the solid has added its
// placed points (#110). The profile is a thin strip along the diagonal, and
// the solid's Position turns it onto the X axis. The raw profile points sit
// in the profile's own frame, and they widened the Y extent to 10.
func TestCollectPointsPlacesAnExtrudedProfile(t *testing.T) {
	for _, tc := range []struct{ name, points string }{
		{"exact rings", "#11,#12,#13,#14,#11"},
		{"approximation", "#11,#13"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := ifcFile(t, `#1=IFCEXTRUDEDAREASOLID(#2,#5,#8,1.);
#2=IFCARBITRARYCLOSEDPROFILEDEF(.AREA.,$,#3);
#3=IFCPOLYLINE((`+tc.points+`));
#5=IFCAXIS2PLACEMENT3D(#6,#7,#9);
#6=IFCCARTESIANPOINT((0.,0.,0.));
#7=IFCDIRECTION((0.,0.,1.));
#8=IFCDIRECTION((0.,0.,1.));
#9=IFCDIRECTION((1.,-1.,0.));
#11=IFCCARTESIANPOINT((0.,0.));
#12=IFCCARTESIANPOINT((0.07,-0.07));
#13=IFCCARTESIANPOINT((10.,10.));
#14=IFCCARTESIANPOINT((9.93,10.07));
`)
			solid, _ := f.ByID(1)
			b := pointsBox(collectPoints(solid, nil))
			if !b.ok {
				t.Fatal("collectPoints found no points")
			}
			if dx := b.hi[0] - b.lo[0]; dx < 14.1 || dx > 14.2 {
				t.Errorf("X extent %.3f, want the strip's length 14.142", dx)
			}
			if dy := b.hi[1] - b.lo[1]; dy > 0.2 {
				t.Errorf("Y extent %.3f, want under 0.2: raw profile points leaked into the box", dy)
			}
		})
	}
}
