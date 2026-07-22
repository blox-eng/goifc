package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/common/ifc/step"
)

// TestCollectPoints_ExtrudeLadderTerminates proves the collectPoints ->
// extrudedAreaApproxPoints -> collectPoints ladder is bounded. A crafted
// IfcExtrudedAreaSolid whose SweptArea references itself makes extrudeSolid
// fail (a solid is not a profile), so the OBB fallback calls
// extrudedAreaApproxPoints, which re-enters collectPoints on the same solid.
// Before the maxApproxLadder bound this recursed with a fresh seen-set every
// hop until the Go stack overflowed (unrecoverable, kills the import worker).
// Reaching the assertion at all proves it now terminates.
func TestCollectPoints_ExtrudeLadderTerminates(t *testing.T) {
	// #1.SweptArea = #1 (self-reference); Depth present so the branch is taken.
	const src = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('ladder','',(''),(''),'','','');
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCEXTRUDEDAREASOLID(#1,$,$,1.);
ENDSEC;
END-ISO-10303-21;
`
	f, err := step.ParseBytes([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	solid, ok := f.ByID(1)
	if !ok {
		t.Fatal("instance #1 not found")
	}
	// Must return (not stack-overflow); a self-referential solid carries no
	// CartesianPoints, so the collected set is empty.
	if pts := collectPoints(solid); len(pts) != 0 {
		t.Errorf("collectPoints on a self-referential solid returned %d points, want 0", len(pts))
	}
}

// TestFaceOuterLoop_OrientationReversesLoop proves IfcFaceBound.Orientation=.F.
// reverses the returned loop, so triangulateFace derives an outward-facing
// normal. Ignoring the flag ships inside-out facets, and the AABB oracle is
// blind to it (identical vertex set).
func TestFaceOuterLoop_OrientationReversesLoop(t *testing.T) {
	// PolyLoop declared order: (0,0,0),(1,0,0),(1,1,0). Orientation=.F. must
	// reverse it to (1,1,0),(1,0,0),(0,0,0).
	const src = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('face','',(''),(''),'','','');
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCCARTESIANPOINT((0.,0.,0.));
#2=IFCCARTESIANPOINT((1.,0.,0.));
#3=IFCCARTESIANPOINT((1.,1.,0.));
#4=IFCPOLYLOOP((#1,#2,#3));
#5=IFCFACEOUTERBOUND(#4,.F.);
#6=IFCFACE((#5));
ENDSEC;
END-ISO-10303-21;
`
	f, err := step.ParseBytes([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	face, ok := f.ByID(6)
	if !ok {
		t.Fatal("instance #6 not found")
	}
	loop := faceOuterLoop(face)
	want := []v3{{1, 1, 0}, {1, 0, 0}, {0, 0, 0}}
	if len(loop) != len(want) {
		t.Fatalf("loop has %d points, want %d", len(loop), len(want))
	}
	for i := range want {
		if loop[i] != want[i] {
			t.Errorf("orientation .F. point %d = %v, want %v (loop not reversed)", i, loop[i], want[i])
		}
	}
}

// TestTriangulateFace_WindingMatchesNormal closes the brep dominant-path gap:
// every triangle triangulateFace emits must be wound consistently with the
// face's own Newell normal. The AABB oracle cannot see an inside-out brep face.
func TestTriangulateFace_WindingMatchesNormal(t *testing.T) {
	// A concave (L-shaped) planar face in z=0, CCW → Newell normal +Z.
	loop := []v3{{0, 0, 0}, {2, 0, 0}, {2, 1, 0}, {1, 1, 0}, {1, 2, 0}, {0, 2, 0}}
	tris := triangulateFace(loop)
	if len(tris) != (len(loop)-2)*3 {
		t.Fatalf("got %d indices, want %d", len(tris), (len(loop)-2)*3)
	}
	var nx, ny, nz float64
	for i := range loop {
		j := (i + 1) % len(loop)
		nx += (loop[i][1] - loop[j][1]) * (loop[i][2] + loop[j][2])
		ny += (loop[i][2] - loop[j][2]) * (loop[i][0] + loop[j][0])
		nz += (loop[i][0] - loop[j][0]) * (loop[i][1] + loop[j][1])
	}
	newell := v3{nx, ny, nz}
	for i := 0; i+2 < len(tris); i += 3 {
		a, b, c := loop[tris[i]], loop[tris[i+1]], loop[tris[i+2]]
		n := crossv(subv(b, a), subv(c, a))
		if dotv(n, newell) <= 0 {
			t.Errorf("triangle %d wound opposite the face normal (inside-out)", i/3)
		}
	}
}

// TestAxisPlacement3D_DegenerateRefDirection proves a RefDirection parallel to
// the Axis (the Gram-Schmidt X collapses to zero) yields an orthonormal basis
// via an arbitrary perpendicular, not a zero-scaled non-invertible frame that
// would silently flatten the element to a point.
func TestAxisPlacement3D_DegenerateRefDirection(t *testing.T) {
	const src = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('axis3d','',(''),(''),'','','');
FILE_SCHEMA(('IFC4'));
ENDSEC;
DATA;
#1=IFCCARTESIANPOINT((0.,0.,0.));
#2=IFCDIRECTION((0.,0.,1.));
#3=IFCDIRECTION((0.,0.,1.));
#4=IFCAXIS2PLACEMENT3D(#1,#2,#3);
ENDSEC;
END-ISO-10303-21;
`
	f, err := step.ParseBytes([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	inst, ok := f.ByID(4)
	if !ok {
		t.Fatal("instance #4 not found")
	}
	m := axisPlacement3D(inst)
	x := applyMat(m, v3{1, 0, 0}) // basis column 0 (translation is zero here)
	mag := math.Sqrt(dotv(x, x))
	if math.IsNaN(mag) || math.Abs(mag-1) > 1e-9 {
		t.Errorf("degenerate placement produced non-unit x-axis %v (|x|=%f) — basis collapsed", x, mag)
	}
}
