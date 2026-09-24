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

// TestBoundedHalfSpace_NonRectangularBoundaryDeclines pins the fix for the
// inverted safety argument in clipTrianglesByBoundedPlane's old doc comment.
// The approximation replaces the polygon footprint with its own AABB in the
// polygon's local (u,v) frame. The kept set is
// (off-material) ∪ (material ∩ outside-footprint); since footprint ⊆ its
// AABB, outside(AABB) ⊆ outside(footprint), so approximating the footprint
// by its AABB removes MORE material than the true boolean does — the bound
// comes out TIGHTER than truth, i.e. it under-reports. That is the one
// failure this package cannot allow, so a boundary that is not its own AABB
// (a triangle here) must be declined and the element must fall back to the
// safe OBB, not silently clipped as a genuine tessellation.
//
// The fixture is a 2x2x3 box DIFFERENCEd with a triangular boundary
// (-1,-1),(3,-1),(-1,3) whose hypotenuse u+v=2 leaves the box's (2,2) corner
// OUTSIDE the triangle (u+v=4 > 2), so the true clip never touches that
// corner and the unclipped max-z (3) must survive.
func TestBoundedHalfSpace_NonRectangularBoundaryDeclines(t *testing.T) {
	f, r := loadFileAndModel(t, "clipped_by_triangular_halfspace.ifc")
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Elements) != 1 {
		t.Fatalf("elements = %d, want 1", len(s.Elements))
	}
	e := s.Elements[0]
	if e.Source != SourceOBB {
		t.Fatalf("Source = %q, want %q — a triangular boundary must decline to the safe fallback, not tessellate as a genuine clip", e.Source, SourceOBB)
	}
	if e.BBoxMax[2] < 3-1e-6 {
		t.Errorf("world max-z = %v, want the unclipped 3 — declining must not under-report the corner the triangle misses", e.BBoxMax[2])
	}
}

// TestBoundedHalfSpace_RectangularBoundaryStillClips is the positive
// companion to the decline test above: it pins that the area-vs-AABB gate
// only rejects boundaries that are NOT their own AABB, not every bounded
// half space — otherwise "always decline" would trivially satisfy the
// non-rectangular test above while silently undoing the whole PR.
func TestBoundedHalfSpace_RectangularBoundaryStillClips(t *testing.T) {
	f, r := loadFileAndModel(t, "clipped_by_bounded_halfspace.ifc")
	s, err := Build(f, r)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Elements[0]
	if e.Source == SourceOBB {
		t.Fatal("rectangular boundary declined to OBB, want it to clip")
	}
	if math.Abs(e.BBoxMax[2]-1.5) > 1e-6 {
		t.Errorf("world max-z = %v, want 1.5 — a rectangular boundary must still clip", e.BBoxMax[2])
	}
}

// TestBoundedHalfSpace_NearRectangularBoundaryDeclines pins the tightness of
// the area gate, not merely its presence. An area tolerance buys a LINEAR
// deviation of order sqrt(relEps) * L, because a corner cut of side d costs
// only d^2/2 of area: on this 20x20 boundary a 2.8 mm chamfer loses 3.9e-6 of
// 400, a ratio of ~1e-8. That sits comfortably under a 1e-6 relative
// tolerance, so the gate would have ADMITTED a boundary that is visibly not
// its own AABB, and the AABB clip would then have removed the corner the real
// footprint leaves intact — under-reporting the bound, which is the one
// failure this package cannot allow.
//
// The mutation replaces the (-10,-10) corner with the two ends of a 2.8 mm
// chamfer, leaving the AABB itself unchanged at 20x20, so the only thing
// under test is the area deficit.
func TestBoundedHalfSpace_NearRectangularBoundaryDeclines(t *testing.T) {
	src, err := os.ReadFile("testdata/synthetic/clipped_by_bounded_halfspace.ifc")
	if err != nil {
		t.Fatal(err)
	}
	mutated := string(src)
	for _, rep := range [][2]string{
		{"#57=IFCCARTESIANPOINT((-10.,-10.));", "#57=IFCCARTESIANPOINT((-9.9972,-10.));\n#157=IFCCARTESIANPOINT((-10.,-9.9972));"},
		{"#61=IFCPOLYLINE((#57,#58,#59,#60,#57));", "#61=IFCPOLYLINE((#57,#58,#59,#60,#157,#57));"},
	} {
		next := strings.Replace(mutated, rep[0], rep[1], 1)
		if next == mutated {
			t.Fatalf("fixture line %q not found — the fixture changed and this test did not", rep[0])
		}
		mutated = next
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
		t.Fatal(err)
	}
	if len(s.Elements) != 1 {
		t.Fatalf("elements = %d, want 1", len(s.Elements))
	}
	e := s.Elements[0]
	if e.Source != SourceOBB {
		t.Fatalf("Source = %q, want %q — a chamfered boundary is not its own AABB and must decline", e.Source, SourceOBB)
	}
	if e.BBoxMax[2] < 3-1e-6 {
		t.Errorf("world max-z = %v, want the unclipped 3 — declining must not under-report", e.BBoxMax[2])
	}
}

// TestBoundedHalfSpace_FarFromOriginBoundaryStillClips guards the gate's
// NUMERICS, which tightening its tolerance made load-bearing. Area is
// translation invariant; a raw shoelace sum is not. A millimetre file placed
// on a site grid carries boundary coordinates near 1e6, whose pairwise
// products land near 1e12 while the area they cancel down to is ~1e2 — so the
// sum arrives carrying a relative error around 1e-6, six orders of magnitude
// coarser than the 1e-12 gate. The gate would then decline a perfectly good
// rectangle and the element would fall back to a box, silently losing the
// coverage this whole path exists to win.
//
// The mutation moves the polygon's Position by -1000000.13 in u and v and
// adds the same offset back to every boundary point, so the footprint occupies
// the same place in the world and the only thing that changes is the magnitude
// of the numbers the shoelace sum sees. The fractional part matters: on exact
// powers of ten the cancellation happens to come out clean, and the raw sum
// only misreports once the mantissa is actually crowded. The expected result is therefore
// identical to the unmutated fixture: a real clip, max-z 1.5.
func TestBoundedHalfSpace_FarFromOriginBoundaryStillClips(t *testing.T) {
	src, err := os.ReadFile("testdata/synthetic/clipped_by_bounded_halfspace.ifc")
	if err != nil {
		t.Fatal(err)
	}
	mutated := string(src)
	for _, rep := range [][2]string{
		{"#55=IFCCARTESIANPOINT((0.,0.,0.));", "#55=IFCCARTESIANPOINT((-1000000.13,-1000000.13,0.));"},
		{"#57=IFCCARTESIANPOINT((-10.,-10.));", "#57=IFCCARTESIANPOINT((999990.13,999990.13));"},
		{"#58=IFCCARTESIANPOINT((10.,-10.));", "#58=IFCCARTESIANPOINT((1000010.13,999990.13));"},
		{"#59=IFCCARTESIANPOINT((10.,10.));", "#59=IFCCARTESIANPOINT((1000010.13,1000010.13));"},
		{"#60=IFCCARTESIANPOINT((-10.,10.));", "#60=IFCCARTESIANPOINT((999990.13,1000010.13));"},
	} {
		next := strings.Replace(mutated, rep[0], rep[1], 1)
		if next == mutated {
			t.Fatalf("fixture line %q not found — the fixture changed and this test did not", rep[0])
		}
		mutated = next
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
		t.Fatal(err)
	}
	e := s.Elements[0]
	if e.Source == SourceOBB {
		t.Fatalf("Source = %q — a rectangle far from the origin declined; the gate is reading cancellation noise, not a real area deficit", e.Source)
	}
	if math.Abs(e.BBoxMax[2]-1.5) > 1e-6 {
		t.Errorf("world max-z = %v, want 1.5 — the same clip the unmutated fixture produces", e.BBoxMax[2])
	}
}
