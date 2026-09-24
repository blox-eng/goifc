package geometry

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// TestClipTrianglesByBoundedPlane_MaterialSideRemoved regression-tests the
// Task-7 review finding: clipTrianglesByBoundedPlane previously swapped the
// materialFrag/kept keepPositive arguments, so a triangle fully on the
// half-space's MATERIAL side (agreeInside) AND fully inside the polygon
// footprint — which IfcPolygonalBoundedHalfSpace defines as the region to
// REMOVE — survived the clip unclipped instead of being cut away.
//
// Setup: base plane z=0, normal +Z, AgreementFlag TRUE (agreeInside=true).
// DIFFERENCE(A, HalfSpace) removes the half-space's own volume from A; the
// oracle-validated plain-halfspace path (clipMeshByDifference's else branch)
// keeps the dot>=0 (z>=0) side when agreeInside=true, which means the
// half-space's OWN (material, to-remove) volume is the dot<=0 (z<0) side.
// Footprint is a 10x10 square centered on the origin in the plane's local
// (u,v). A triangle sitting entirely at z=-5 (material side) and entirely
// inside [-5,5]x[-5,5] (inside the footprint) must be entirely removed.
func TestClipTrianglesByBoundedPlane_MaterialSideRemoved(t *testing.T) {
	f, err := step.Parse(strings.NewReader(boundedHalfSpaceIFC))
	if err != nil {
		t.Fatal(err)
	}
	hs, ok := f.ByID(20)
	if !ok || !hs.IsA("IFCPOLYGONALBOUNDEDHALFSPACE") {
		t.Fatalf("fixture instance #20 = %v %v", hs, ok)
	}

	origin := v3{0, 0, 0}
	normal := v3{0, 0, 1}
	agreeInside := true // AgreementFlag=.T. → material (to-remove) side is z<0

	// One triangle, fully on the material side (z=-5) and fully inside the
	// [-5,5]x[-5,5] footprint — the spec says REMOVE it entirely.
	verts := []float32{
		-1, -1, -5,
		1, -1, -5,
		0, 1, -5,
	}
	tris := []uint32{0, 1, 2}

	_, keptTris, ok := clipTrianglesByBoundedPlane(verts, tris, origin, normal, agreeInside, hs)
	if !ok {
		t.Fatal("clipTrianglesByBoundedPlane declined (ok=false)")
	}
	if len(keptTris) != 0 {
		t.Errorf("triangle on material side AND inside footprint survived: %d indices kept, want 0", len(keptTris))
	}
}

// A triangle fully OFF the material side (z=+5) must survive unconditionally,
// regardless of the footprint — this is the "rest of the mesh is
// unconditionally kept" half of the union.
func TestClipTrianglesByBoundedPlane_OffMaterialSideKept(t *testing.T) {
	f, err := step.Parse(strings.NewReader(boundedHalfSpaceIFC))
	if err != nil {
		t.Fatal(err)
	}
	hs, _ := f.ByID(20)

	verts := []float32{
		-1, -1, 5,
		1, -1, 5,
		0, 1, 5,
	}
	tris := []uint32{0, 1, 2}

	_, keptTris, ok := clipTrianglesByBoundedPlane(verts, tris, v3{0, 0, 0}, v3{0, 0, 1}, true, hs)
	if !ok {
		t.Fatal("clipTrianglesByBoundedPlane declined (ok=false)")
	}
	if len(keptTris) == 0 {
		t.Error("triangle off the material side was removed, want kept")
	}
}

// A triangle on the material side but OUTSIDE the footprint must also
// survive — the footprint gate only removes material inside its bounds.
func TestClipTrianglesByBoundedPlane_MaterialSideOutsideFootprintKept(t *testing.T) {
	f, err := step.Parse(strings.NewReader(boundedHalfSpaceIFC))
	if err != nil {
		t.Fatal(err)
	}
	hs, _ := f.ByID(20)

	verts := []float32{
		100, 100, -5,
		101, 100, -5,
		100, 101, -5,
	}
	tris := []uint32{0, 1, 2}

	_, keptTris, ok := clipTrianglesByBoundedPlane(verts, tris, v3{0, 0, 0}, v3{0, 0, 1}, true, hs)
	if !ok {
		t.Fatal("clipTrianglesByBoundedPlane declined (ok=false)")
	}
	if len(keptTris) == 0 {
		t.Error("triangle on material side but outside footprint was removed, want kept")
	}
}

// boundedHalfSpaceIFC is a minimal synthetic fixture: #20 is an
// IfcPolygonalBoundedHalfSpace whose Position (#4) is the world XY plane
// (origin at 0,0,0, Z axis +Z, X axis +X) and whose PolygonalBoundary (#14)
// is a 10x10 square centered on the origin, local u/v in [-5,5]. BaseSurface
// and AgreementFlag ($) are left null — clipTrianglesByBoundedPlane never
// reads them (the caller supplies origin/normal/agreeInside directly); only
// halfSpacePlane (exercised by clipMeshByDifference, tested elsewhere) needs
// them populated.
const boundedHalfSpaceIFC = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('x.ifc','2026-07-22',(''),(''),'','','');
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCCARTESIANPOINT((0.,0.,0.));
#2=IFCDIRECTION((0.,0.,1.));
#3=IFCDIRECTION((1.,0.,0.));
#4=IFCAXIS2PLACEMENT3D(#1,#2,#3);
#10=IFCCARTESIANPOINT((-5.,-5.));
#11=IFCCARTESIANPOINT((5.,-5.));
#12=IFCCARTESIANPOINT((5.,5.));
#13=IFCCARTESIANPOINT((-5.,5.));
#14=IFCPOLYLINE((#10,#11,#12,#13));
#20=IFCPOLYGONALBOUNDEDHALFSPACE($,$,#4,#14);
ENDSEC;
END-ISO-10303-21;
`

// clip.go gated its second operand on IsA("IfcHalfSpaceSolid"), and IsA is
// exact-keyword only — there is no EXPRESS schema behind it. So the SUBTYPE
// IfcPolygonalBoundedHalfSpace never passed, and the bounded-clipping path
// below it was dead code on all 11 corpus instances.
// https://github.com/blox-eng/goifc/issues/52
func TestBoundedHalfSpace_ClipIsReachedThroughDispatch(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/clipped_by_bounded_halfspace.ifc")
	if err != nil {
		t.Fatal(err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Elements[0]
	if e.Source == SourceOBB {
		t.Fatalf("source = %q — the clip declined and the element fell back to a box", e.Source)
	}
	// The half space's material (removed) side is above z=1.5, so the bottom
	// half survives. A max-z of 3 means no clip happened; a MIN-z of 1.5 means
	// the AgreementFlag sign is inverted and the wrong half was removed —
	// which is exactly the risk the design doc flags, so fail loudly.
	if math.Abs(e.BBoxMax[2]-1.5) > 1e-6 {
		t.Errorf("world max-z = %v, want 1.5 (min-z = %v)", e.BBoxMax[2], e.BBoxMin[2])
	}
	if math.Abs(e.BBoxMin[2]) > 1e-6 {
		t.Errorf("world min-z = %v, want 0", e.BBoxMin[2])
	}
}

// Once the gate opens, malformed half spaces reach halfSpacePlane and
// clipTrianglesByBoundedPlane for the first time. Each must decline into the
// safe OBB fallback rather than produce a wrong plane or index past a short
// ring.
func TestBoundedHalfSpace_MalformedDeclinesToBox(t *testing.T) {
	src, err := os.ReadFile("testdata/synthetic/clipped_by_bounded_halfspace.ifc")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, old, new string }{
		{"base surface is not a plane", "#54=IFCPLANE(#53);", "#54=IFCCARTESIANPOINT((0.,0.,1.5));"},
		{"boundary has two points", "#61=IFCPOLYLINE((#57,#58,#59,#60,#57));", "#61=IFCPOLYLINE((#57,#58));"},
		{"position is absent", "#62=IFCPOLYGONALBOUNDEDHALFSPACE(#54,.F.,#56,#61);", "#62=IFCPOLYGONALBOUNDEDHALFSPACE(#54,.F.,$,#61);"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := strings.Replace(string(src), tc.old, tc.new, 1)
			if mutated == string(src) {
				t.Fatalf("fixture line %q not found — the fixture changed and this test did not", tc.old)
			}
			f, err := step.Parse(strings.NewReader(mutated))
			if err != nil {
				t.Fatal(err)
			}
			r, err := model.Extract(f)
			if err != nil {
				t.Fatal(err)
			}
			s, err := Build(f, r)
			if err != nil {
				t.Fatalf("Build errored on a malformed half space: %v — it must degrade, not fail", err)
			}
			if len(s.Elements) != 1 {
				t.Fatalf("elements = %d, want 1", len(s.Elements))
			}
			// The box is the conservative superset: larger than truth is safe,
			// smaller is the one failure a quantities consumer cannot defend
			// against.
			if s.Elements[0].BBoxMax[2] < 3-1e-6 {
				t.Errorf("world max-z = %v, want the unclipped 3 — a declined clip must not shrink the bound", s.Elements[0].BBoxMax[2])
			}
		})
	}
}
