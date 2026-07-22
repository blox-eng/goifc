package geometry

import (
	"strings"
	"testing"

	"github.com/blox-eng/common/ifc/step"
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
