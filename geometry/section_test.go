package geometry

import (
	"math"
	"reflect"
	"testing"

	"github.com/blox-eng/goifc/model"
)

// elemBox builds an Element from an axis-aligned box with identity placement
// (verts == world).
func elemBox(min, max v3) Element {
	var verts []float32
	w, tris := boxMeshWorld(min, max)
	for _, p := range w {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	return Element{GlobalID: "g", Verts: verts, Tris: tris,
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   [3]float64{min[0], min[1], min[2]}, BBoxMax: [3]float64{max[0], max[1], max[2]}}
}

// hollowBox builds an Element whose mesh is a concentric outer + inner box.
func hollowBox(outMin, outMax, inMin, inMax v3) Element {
	w1, t1 := boxMeshWorld(outMin, outMax)
	w2, t2 := boxMeshWorld(inMin, inMax)
	var verts []float32
	for _, p := range append(append([]v3{}, w1...), w2...) {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	return Element{GlobalID: "h", Verts: verts, Tris: tris,
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   [3]float64{outMin[0], outMin[1], outMin[2]}, BBoxMax: [3]float64{outMax[0], outMax[1], outMax[2]}}
}

func TestFootprintCutWhenPlaneCrosses(t *testing.T) {
	loops := Footprint(elemBox(v3{0, 0, 0}, v3{1, 1, 3}), 1.5)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d: %v", len(loops), loops)
	}
	if loops[0].Role != LoopCut {
		t.Fatalf("want LoopCut, got %q", loops[0].Role)
	}
}

func TestFootprintBelowWhenPlaneAbove(t *testing.T) {
	loops := Footprint(elemBox(v3{0, 0, 0}, v3{1, 1, 1}), 2.0)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d: %v", len(loops), loops)
	}
	if loops[0].Role != LoopSilhouette {
		t.Fatalf("want LoopSilhouette, got %q", loops[0].Role)
	}
}

func TestFootprintAabbFallback(t *testing.T) {
	e := Element{BBoxMin: [3]float64{0, 0, 0}, BBoxMax: [3]float64{2, 3, 0}}
	loops := Footprint(e, 1.0)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d: %v", len(loops), loops)
	}
	if loops[0].Role != LoopSilhouette {
		t.Fatalf("want LoopSilhouette, got %q", loops[0].Role)
	}
	if a := ringArea(loops[0].Points); math.Abs(a-6.0) > 1e-6 {
		t.Fatalf("want area 6.0, got %v", a)
	}
}

func TestFootprintHoleNesting(t *testing.T) {
	e := hollowBox(v3{0, 0, 0}, v3{4, 4, 4}, v3{1, 1, 0}, v3{3, 3, 4})
	loops := Footprint(e, 2.0)
	if len(loops) != 2 {
		t.Fatalf("want 2 cut loops, got %d: %v", len(loops), loops)
	}
	var outer, inner int
	for _, l := range loops {
		if l.Role != LoopCut {
			t.Fatalf("want LoopCut, got %q", l.Role)
		}
		if polygonArea2D(l.Points) > 0 {
			outer++
			if a := ringArea(l.Points); math.Abs(a-16.0) > 1e-6 {
				t.Fatalf("outer: want area 16.0, got %v", a)
			}
		} else {
			inner++
			if a := ringArea(l.Points); math.Abs(a-4.0) > 1e-6 {
				t.Fatalf("inner hole: want |area| 4.0, got %v", a)
			}
		}
	}
	if outer != 1 || inner != 1 {
		t.Fatalf("want 1 CCW outer + 1 CW hole, got outer=%d inner=%d", outer, inner)
	}
}

func TestFootprintDeterministic(t *testing.T) {
	e := hollowBox(v3{0, 0, 0}, v3{4, 4, 4}, v3{1, 1, 0}, v3{3, 3, 4})
	if !reflect.DeepEqual(Footprint(e, 2.0), Footprint(e, 2.0)) {
		t.Fatal("Footprint not deterministic")
	}
}

// axis-aligned box mesh [min,max] as world v3 verts + 12 tris.
func boxMeshWorld(min, max v3) ([]v3, []uint32) {
	c := [8]v3{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]}, {max[0], max[1], min[2]}, {min[0], max[1], min[2]},
		{min[0], min[1], max[2]}, {max[0], min[1], max[2]}, {max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	w := c[:]
	tris := []uint32{
		0, 2, 1, 0, 3, 2, 4, 5, 6, 4, 6, 7, // bottom, top
		0, 1, 5, 0, 5, 4, 1, 2, 6, 1, 6, 5, // sides
		2, 3, 7, 2, 7, 6, 3, 0, 4, 3, 4, 7,
	}
	return w, tris
}

func ringArea(r [][2]float64) float64 { // shoelace, abs
	var a float64
	for i := range r {
		j := (i + 1) % len(r)
		a += r[i][0]*r[j][1] - r[j][0]*r[i][1]
	}
	return math.Abs(a) / 2
}

func TestSectionRingsCubeMidCut(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 1})
	rings := sectionRings(w, tris, HorizontalPlane(0.5))
	if len(rings) != 1 {
		t.Fatalf("want 1 ring, got %d: %v", len(rings), rings)
	}
	if a := ringArea(rings[0]); math.Abs(a-1.0) > 1e-6 {
		t.Fatalf("want area 1.0, got %v", a)
	}
}

func TestSectionRingsPlaneMisses(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 1})
	rings := sectionRings(w, tris, HorizontalPlane(5.0))
	if len(rings) != 0 {
		t.Fatalf("want 0 rings, got %d: %v", len(rings), rings)
	}
}

func TestSectionRingsTwoDisjointBoxes(t *testing.T) {
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{3, 0, 0}, v3{4, 1, 2})
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	rings := sectionRings(w, tris, HorizontalPlane(1.0))
	if len(rings) != 2 {
		t.Fatalf("want 2 rings, got %d: %v", len(rings), rings)
	}
	for i, r := range rings {
		if a := ringArea(r); math.Abs(a-1.0) > 1e-6 {
			t.Fatalf("ring %d: want area 1.0, got %v", i, a)
		}
	}
}

func TestSectionRingsTJunction(t *testing.T) {
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{1, 0, 0}, v3{2, 1, 2}) // shares x=1 face
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	rings := sectionRings(w, tris, HorizontalPlane(1.0))
	if len(rings) == 0 {
		t.Fatalf("want >=1 ring, got 0")
	}
	var total float64
	for _, r := range rings {
		if len(r) < 3 {
			t.Fatalf("degenerate ring: %v", r)
		}
		total += ringArea(r)
	}
	if math.Abs(total-2.0) > 1e-6 {
		t.Fatalf("want total area 2.0, got %v (rings=%v)", total, rings)
	}
}

func TestSectionRingsOnPlaneFace(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 1})
	rings := sectionRings(w, tris, HorizontalPlane(1.0)) // top face coplanar with cut
	// Nothing spans the plane; the box merely touches it -> 0 section rings.
	if len(rings) != 0 {
		t.Fatalf("want 0 rings (coplanar top face skipped), got %d: %v", len(rings), rings)
	}
}

func TestSectionRingsCornerTouch(t *testing.T) {
	// Two boxes touching only along the x=1,y=1 vertical edge. The cut welds a
	// single point (1,1) shared by BOTH boxes -> degree-4 vertex that survives
	// parity cancellation (no full face is shared), forcing nextEdge's
	// smallest-turning-angle choice. A tangled figure-8 walk would fail here.
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{1, 1, 0}, v3{2, 2, 2})
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	rings := sectionRings(w, tris, HorizontalPlane(1.0))
	if len(rings) != 2 {
		t.Fatalf("want 2 rings (corner-touch resolved cleanly), got %d: %v", len(rings), rings)
	}
	var total float64
	for _, r := range rings {
		if len(r) < 3 {
			t.Fatalf("degenerate ring: %v", r)
		}
		if a := ringArea(r); math.Abs(a-1.0) > 1e-6 {
			t.Fatalf("want each ring area 1.0, got %v", a)
		}
		total += ringArea(r)
	}
	if math.Abs(total-2.0) > 1e-6 {
		t.Fatalf("want total area 2.0, got %v", total)
	}
}

func TestBelowRingsCube(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{2, 3, 1})
	rings := belowRings(w, tris, HorizontalPlane(0))
	if len(rings) != 1 {
		t.Fatalf("want 1 silhouette ring, got %d", len(rings))
	}
	if got := ringArea(rings[0]); math.Abs(got-6.0) > 1e-6 { // 2*3
		t.Fatalf("want footprint area 6.0, got %v", got)
	}
}

func TestBelowRingsDeterministic(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{2, 3, 1})
	if !reflect.DeepEqual(belowRings(w, tris, HorizontalPlane(0)), belowRings(w, tris, HorizontalPlane(0))) {
		t.Fatal("belowRings not deterministic")
	}
}

func TestAabbRing(t *testing.T) {
	r := aabbRingOn([3]float64{1, 1, 0}, [3]float64{4, 3, 2}, HorizontalPlane(0))
	if got := ringArea(r); math.Abs(got-6.0) > 1e-6 {
		t.Fatalf("want 6.0, got %v", got)
	}
}

func TestRingSelfIntersects(t *testing.T) {
	// bowtie: edges (0,0)-(1,1) and (1,0)-(0,1) cross
	bowtie := [][2]float64{{0, 0}, {1, 1}, {1, 0}, {0, 1}}
	if !ringSelfIntersects(bowtie) {
		t.Fatalf("bowtie not detected")
	}
	square := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	if ringSelfIntersects(square) {
		t.Fatalf("simple square flagged as self-intersecting")
	}
}

func TestSectionRingsDeterministic(t *testing.T) {
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{1, 0, 0}, v3{2, 1, 2})
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	r1 := sectionRings(w, tris, HorizontalPlane(1.0))
	r2 := sectionRings(w, tris, HorizontalPlane(1.0))
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("non-deterministic output:\n r1=%v\n r2=%v", r1, r2)
	}
}

// planeYZ is the vertical plane at x = atX, viewed along -X: U=+Y, V=+Z, N=+X.
func planeYZ(atX float64) Plane {
	return Plane{
		Origin: [3]float64{atX, 0, 0},
		U:      [3]float64{0, 1, 0},
		V:      [3]float64{0, 0, 1},
		N:      [3]float64{1, 0, 0},
	}
}

func TestSectionOnVerticalCutHoleNesting(t *testing.T) {
	// Concentric boxes; a vertical cut through the middle yields the outer
	// boundary plus a nested hole — the same shape the horizontal path yields,
	// now on a plane the old code could not express.
	e := hollowBox(v3{0, 0, 0}, v3{4, 4, 4}, v3{1, 1, 1}, v3{3, 3, 3})
	loops := e.SectionOn(planeYZ(2))
	if len(loops) != 2 {
		t.Fatalf("want 2 loops (outer + hole), got %d: %v", len(loops), loops)
	}
	var outer, hole int
	for _, l := range loops {
		if l.Role != LoopCut {
			t.Fatalf("want LoopCut, got %q", l.Role)
		}
		if a := polygonArea2D(l.Points); a > 0 {
			outer++
		} else {
			hole++
		}
	}
	if outer != 1 || hole != 1 {
		t.Fatalf("want 1 CCW outer and 1 CW hole, got %d/%d", outer, hole)
	}
}

func TestSectionOnObliquePlaneArea(t *testing.T) {
	// A 45-degree plane cutting the unit cube at x+y=0.5 crosses two side FACES
	// and yields a sqrt(0.5) x 1 rectangle. Proves the UV projection is a real
	// projection: dropping Z would give a degenerate 0, dropping X would give
	// 0.5, and neither is sqrt(0.5).
	//
	// The plane is deliberately offset from the cube centre. Through the centre
	// it would be x+y=1, which passes exactly along the (1,0,z) and (0,1,z)
	// EDGES; triangles merely touching the plane do not span it (by design, see
	// TestSectionRingsOnPlaneFace), so no side segments are emitted and no ring
	// closes. That degenerate case is pinned separately below.
	e := elemBox(v3{0, 0, 0}, v3{1, 1, 1})
	p, ok := PlaneFromNormal([3]float64{0.25, 0.25, 0.5}, [3]float64{1, 1, 0})
	if !ok {
		t.Fatal("PlaneFromNormal failed")
	}
	loops := e.SectionOn(p)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d: %v", len(loops), loops)
	}
	got := math.Abs(polygonArea2D(loops[0].Points))
	want := math.Sqrt(0.5)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("area = %v, want sqrt(0.5) = %v", got, want)
	}
}

func TestSectionOnPlaneContainingEdgesIsKnownGap(t *testing.T) {
	// x+y=1 does not graze the cube — it BISECTS it, passing through the
	// interior (the cube centre lies exactly on it) with the two vertical
	// edges (1,0,z) and (0,1,z) as the long sides of the cut. The true section
	// is a real 1 x sqrt(2) rectangle, area sqrt(2).
	//
	// SectionOn returns nil instead, because a triangle only emits a crossing
	// segment when it has a vertex STRICTLY above and a vertex STRICTLY below
	// the plane. The top and bottom faces do span it and emit their segments
	// correctly, but the side faces merely touch it along an edge (every side
	// vertex is on-plane or one-sided), so they contribute nothing and the two
	// spanning segments never connect into a closed ring.
	//
	// This pins CURRENT behaviour so a change is noticed — it does NOT assert
	// that nil is the right answer. It is a known gap: a caller cutting a
	// diagonal section through a rectangular column here is told the plane
	// missed when it in fact bisected the solid. Fixing it belongs with the
	// strict-sign span test in sectionRings/triCrossing, not here.
	e := elemBox(v3{0, 0, 0}, v3{1, 1, 1})
	p, ok := PlaneFromNormal([3]float64{0.5, 0.5, 0.5}, [3]float64{1, 1, 0})
	if !ok {
		t.Fatal("PlaneFromNormal failed")
	}
	if got := e.SectionOn(p); got != nil {
		t.Fatalf("edge-grazing plane: want nil, got %v", got)
	}
}

func TestSectionOnRotationInvariance(t *testing.T) {
	// The test that catches a wrong basis: the same box cut horizontally, and
	// rotated 90 degrees about X then cut with the correspondingly rotated
	// plane, must give the same ring area.
	e := elemBox(v3{0, 0, 0}, v3{1, 2, 3})
	flat := e.SectionOn(HorizontalPlane(1.5))
	if len(flat) != 1 {
		t.Fatalf("horizontal: want 1 loop, got %d", len(flat))
	}

	// Rotate 90 degrees about X (column-major): local +Y -> world +Z,
	// local +Z -> world -Y.
	rot := e
	rot.Placement = model.Mat4{
		1, 0, 0, 0,
		0, 0, 1, 0,
		0, -1, 0, 0,
		0, 0, 0, 1,
	}
	// The plane's origin and normal rotate the same way: (0,0,1.5) -> (0,-1.5,0)
	// and +Z -> -Y.
	p, ok := PlaneFromNormal([3]float64{0, -1.5, 0}, [3]float64{0, -1, 0})
	if !ok {
		t.Fatal("PlaneFromNormal failed")
	}
	turned := rot.SectionOn(p)
	if len(turned) != 1 {
		t.Fatalf("rotated: want 1 loop, got %d: %v", len(turned), turned)
	}

	a := math.Abs(polygonArea2D(flat[0].Points))
	b := math.Abs(polygonArea2D(turned[0].Points))
	if math.Abs(a-b) > 1e-9 {
		t.Fatalf("rotation-variant: horizontal area %v vs rotated %v", a, b)
	}
}

func TestSectionOnRejectsInvalidBasis(t *testing.T) {
	e := hollowBox(v3{0, 0, 0}, v3{4, 4, 4}, v3{1, 1, 1}, v3{3, 3, 3})
	lefty := Plane{Origin: [3]float64{2, 0, 0}, U: [3]float64{0, 1, 0}, V: [3]float64{0, 0, 1}, N: [3]float64{-1, 0, 0}}
	if got := e.SectionOn(lefty); got != nil {
		t.Fatalf("left-handed basis: want nil, got %v", got)
	}
	if got := FootprintOn(e, lefty); got != nil {
		t.Fatalf("left-handed basis: FootprintOn want nil, got %v", got)
	}
	skew := Plane{Origin: [3]float64{2, 0, 0}, U: [3]float64{0, 1, 0}, V: [3]float64{0, 1, 1}, N: [3]float64{1, 0, 0}}
	if got := e.SectionOn(skew); got != nil {
		t.Fatalf("non-orthonormal basis: want nil, got %v", got)
	}
}

func TestSectionOnPlaneMisses(t *testing.T) {
	e := elemBox(v3{0, 0, 0}, v3{1, 1, 1})
	if got := e.SectionOn(planeYZ(50)); got != nil {
		t.Fatalf("plane clear of the mesh: want nil, got %v", got)
	}
}

func TestFootprintOnSilhouetteInvariantUnderFlip(t *testing.T) {
	// For a CLOSED solid the parity boundary of the front-facing set equals that
	// of the back-facing set, so flipping N changes nothing. Pinned so a future
	// reader does not "fix" a flip that was never wrong.
	e := elemBox(v3{0, 0, 0}, v3{1, 2, 3})
	fwd := FootprintOn(e, planeYZ(-10))
	back, ok := PlaneFromNormal([3]float64{-10, 0, 0}, [3]float64{-1, 0, 0})
	if !ok {
		t.Fatal("PlaneFromNormal failed")
	}
	rev := FootprintOn(e, back)
	if len(fwd) != 1 || len(rev) != 1 {
		t.Fatalf("want 1 silhouette loop each, got %d and %d", len(fwd), len(rev))
	}
	a := math.Abs(polygonArea2D(fwd[0].Points))
	b := math.Abs(polygonArea2D(rev[0].Points))
	if math.Abs(a-b) > 1e-9 {
		t.Fatalf("silhouette area changed with N sign: %v vs %v", a, b)
	}
}

func TestFootprintOnAabbFallbackUsesPlaneFrame(t *testing.T) {
	// An element with no usable mesh, cut on a vertical plane, must fall back to
	// its AABB projected into UV — not the world-XY rectangle.
	e := Element{
		GlobalID: "empty",
		BBoxMin:  [3]float64{0, 0, 0},
		BBoxMax:  [3]float64{1, 2, 3},
	}
	loops := FootprintOn(e, planeYZ(0))
	if len(loops) != 1 {
		t.Fatalf("want 1 fallback loop, got %d", len(loops))
	}
	// U=+Y spans 0..2, V=+Z spans 0..3 -> area 6. The world-XY rectangle would
	// be 1 x 2 = 2.
	if got := math.Abs(polygonArea2D(loops[0].Points)); math.Abs(got-6) > 1e-12 {
		t.Fatalf("fallback area = %v, want 6 (UV frame); 2 means world XY", got)
	}
}

func TestSectionOnDeterministic(t *testing.T) {
	e := hollowBox(v3{0, 0, 0}, v3{4, 4, 4}, v3{1, 1, 1}, v3{3, 3, 3})
	p, ok := PlaneFromNormal([3]float64{2, 2, 2}, [3]float64{1, 2, 3})
	if !ok {
		t.Fatal("PlaneFromNormal failed")
	}
	if !reflect.DeepEqual(e.SectionOn(p), e.SectionOn(p)) {
		t.Fatal("SectionOn not deterministic on an oblique plane")
	}
}

func TestLoopRoleWireValuesUnchanged(t *testing.T) {
	// These strings are persisted in drawing data and matched as literals by
	// renderers. Changing them invalidates drawings already on disk.
	if string(LoopCut) != "cut" {
		t.Fatalf("LoopCut = %q, want \"cut\"", LoopCut)
	}
	if string(LoopSilhouette) != "below" {
		t.Fatalf("LoopSilhouette = %q, want \"below\"", LoopSilhouette)
	}
}

// belowRings keeps the faces OPPOSING p.N. Every other silhouette test uses a
// closed box, where the front-facing and back-facing parity boundaries are
// identical, so inverting the predicate is invisible to them. An open mesh is
// the only input that can tell the two sets apart.
func TestBelowRingsKeepsOnlyOpposingFaces(t *testing.T) {
	// A 2x2 quad facing -Z at z=0, and a 1x1 quad facing +Z at z=1. Looking
	// along N=+Z, only the -Z-facing quad opposes N, so the area must be 4.
	w := []v3{
		{0, 0, 0}, {2, 0, 0}, {2, 2, 0}, {0, 2, 0},
		{0, 0, 1}, {1, 0, 1}, {1, 1, 1}, {0, 1, 1},
	}
	tris := []uint32{0, 2, 1, 0, 3, 2, 4, 5, 6, 4, 6, 7}
	rings := belowRings(w, tris, HorizontalPlane(0))
	if len(rings) != 1 {
		t.Fatalf("want 1 silhouette ring, got %d: %v", len(rings), rings)
	}
	if a := ringArea(rings[0]); math.Abs(a-4) > 1e-9 {
		t.Fatalf("silhouette area = %v, want 4 (the -Z-facing quad); 1 means the facing test is inverted", a)
	}
}

// An element with no mesh never had its bounds measured, so it can still carry
// worldAABB's empty sentinel. Projecting an infinity through a basis multiplies
// it by a zero component and yields NaN, so the fallback must decline instead.
func TestFootprintOnNonFiniteAabbYieldsNoRings(t *testing.T) {
	e := Element{
		BBoxMin: [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)},
		BBoxMax: [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)},
	}
	for _, p := range []Plane{HorizontalPlane(1), planeYZ(0)} {
		if got := FootprintOn(e, p); got != nil {
			t.Fatalf("non-finite AABB: want nil, got %v", got)
		}
	}
}

// A non-finite basis must yield NO rings, not rings full of NaN coordinates.
// Plane.Valid rejects these today via both finite3 and the handedness check;
// this pins the outcome callers depend on regardless of which check catches it.
func TestSectionOnRejectsNonFiniteBasis(t *testing.T) {
	e := elemBox(v3{0, 0, 0}, v3{1, 1, 1})
	for name, bad := range map[string]Plane{
		"NaN in U": {Origin: [3]float64{0, 0, 0.5}, U: [3]float64{math.NaN(), 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, 1}},
		"Inf in N": {Origin: [3]float64{0, 0, 0.5}, U: [3]float64{1, 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, math.Inf(1)}},
	} {
		if got := e.SectionOn(bad); got != nil {
			t.Errorf("%s: SectionOn want nil, got %v", name, got)
		}
		if got := FootprintOn(e, bad); got != nil {
			t.Errorf("%s: FootprintOn want nil, got %v", name, got)
		}
	}
}

// An open mesh has no front/back symmetry, so which way p.N points decides
// whether FootprintOn returns the real outline or falls through to the bounding
// box. A triangle is used because its outline (area 2) and its AABB rectangle
// (area 4) differ — a quad would hide the substitution behind equal areas.
func TestFootprintOnOpenMeshIsDirectionDependent(t *testing.T) {
	e := Element{
		GlobalID:  "open",
		Verts:     []float32{0, 0, 0, 2, 0, 0, 0, 2, 0},
		Tris:      []uint32{0, 2, 1}, // wound to face -Z
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   [3]float64{0, 0, 0}, BBoxMax: [3]float64{2, 2, 0},
	}

	// N=+Z opposes the face: the true triangular outline.
	facing := FootprintOn(e, HorizontalPlane(5))
	if len(facing) != 1 {
		t.Fatalf("N=+Z: want 1 loop, got %d", len(facing))
	}
	if a := ringArea(facing[0].Points); math.Abs(a-2) > 1e-9 {
		t.Fatalf("N=+Z: area = %v, want 2 (the triangle itself)", a)
	}

	// N=-Z opposes nothing, so the silhouette branch yields nothing and the
	// bounding-box fallback substitutes a rectangle — tagged the same way.
	flipped := Plane{Origin: [3]float64{0, 0, 5}, U: [3]float64{1, 0, 0}, V: [3]float64{0, -1, 0}, N: [3]float64{0, 0, -1}}
	if !flipped.Valid() {
		t.Fatal("flipped fixture must be a valid right-handed basis")
	}
	away := FootprintOn(e, flipped)
	if len(away) != 1 {
		t.Fatalf("N=-Z: want 1 loop, got %d", len(away))
	}
	if a := ringArea(away[0].Points); math.Abs(a-4) > 1e-9 {
		t.Fatalf("N=-Z: area = %v, want 4 (the AABB rectangle fallback)", a)
	}
	if away[0].Role != LoopSilhouette {
		t.Fatalf("N=-Z: role = %q, want %q — the fallback is indistinguishable by Role", away[0].Role, LoopSilhouette)
	}
}
