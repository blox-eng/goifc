package geometry

import (
	"math"
	"testing"
)

// appendBoxes concatenates axis-aligned boxes into one mesh, offsetting each
// box's indices by the running vertex count.
func appendBoxes(boxes ...[2]v3) ([]v3, []uint32) {
	var w []v3
	var tris []uint32
	for _, b := range boxes {
		bw, bt := boxMeshWorld(b[0], b[1])
		base := uint32(len(w))
		w = append(w, bw...)
		for _, i := range bt {
			tris = append(tris, base+i)
		}
	}
	return w, tris
}

// TestUnionAreaOnSliversAtWorldCoordinates: two thin triangles sharing a
// diagonal, at the world coordinates and shape a real 29 MB ArchiCAD IFC2X3
// export produced. They do not overlap, so the union is their Σ.
//
// The coordinates are load-bearing and must not be "tidied" to small round
// numbers: the failure this pins needs a sub-edge midpoint that is NOT exactly
// representable on its own edge's line. At these magnitudes (~65 m, ~38 m) the
// midpoint of an edge lands a rounding step off the line, every orientation
// cross-product in strictlyInsideCCW becomes sign noise, and a sliver triangle
// reports its OWN boundary edge as strictly interior — so unionBoundary drops
// that edge as covered-on-both-sides. Small round coordinates land exactly on
// the line, evaluate to an exact zero, and never reproduce it.
func TestUnionAreaOnSliversAtWorldCoordinates(t *testing.T) {
	a := [2]float64{65.11808708855443, 37.66152506711301}
	b := [2]float64{65.01321996112344, 39.25472527427798}
	c := [2]float64{64.9903348621967, 39.22777561903379}
	d := [2]float64{65.14300437266617, 37.663557612277266}

	var tris [][3][2]float64
	tris = appendProjected(tris, a, b, c)
	tris = appendProjected(tris, b, a, d)

	var want float64
	for _, tr := range tris {
		want += math.Abs(signedArea2(tr)) / 2
	}
	if got := mustUnionArea(t, tris); math.Abs(got-want) > 1e-9 {
		t.Errorf("unionArea2D = %v, want %v (the two slivers are disjoint)", got, want)
	}
}

// TestUnionAreaIgnoresTrianglesDegenerateAtTheWeldQuantum: a real quad from the
// same export, delivered with two extra tessellation slivers along its shared
// edge. Those slivers have a nonzero exact area (~5e-8 m²) but two of their
// three vertices lie 2e-7 m apart — a fiftieth of the stitcher's 1e-5 m weld.
//
// A triangle the welding cannot represent must not reach the boundary tally.
// Welding collapses it to a segment, and the phantom directed edges that
// produces cancel the real quad's boundary edges, opening the outline.
func TestUnionAreaIgnoresTrianglesDegenerateAtTheWeldQuantum(t *testing.T) {
	p1 := [2]float64{43.51242924374534, 27.563393615916134}
	p2 := [2]float64{43.512429451215716, 27.56339612481004}
	p3 := [2]float64{43.47437802798363, 27.566542747808697}
	p4 := [2]float64{43.474377820513254, 27.56654023891479}
	p5 := [2]float64{43.563932418642864, 28.186210237627023}
	p6 := [2]float64{43.52588099541078, 28.18935686062568}

	var real [][3][2]float64
	real = appendProjected(real, p5, p3, p2)
	real = appendProjected(real, p3, p5, p6)
	var want float64
	for _, tr := range real {
		want += math.Abs(signedArea2(tr)) / 2
	}

	withSlivers := appendProjected(append([][3][2]float64(nil), real...), p1, p2, p3)
	withSlivers = appendProjected(withSlivers, p3, p4, p1)

	if got := mustUnionArea(t, withSlivers); math.Abs(got-want) > 1e-6 {
		t.Errorf("unionArea2D = %v, want %v — the slivers add no area they can represent", got, want)
	}
}

// TestUnionAreaWeldsPointsStraddlingAQuantumBoundary: a square cut into two
// triangles, where the two halves state their shared corner 9.85e-6 m apart —
// UNDER the 1e-5 m weld, so the shared diagonal must cancel as interior.
//
// The two v values are lifted from a real 29 MB ArchiCAD IFC2X3 export and are
// load-bearing: they straddle a rounding-bucket boundary (…881.32 and …882.30
// in quantum units). Welding by rounding to a bucket is therefore NOT the
// relation it claims to be — "within one quantum" is not transitive under it,
// and two points closer than the quantum land in different buckets. The
// diagonal then fails to cancel, the outline opens, and the area integral runs
// away with the model's distance from the origin.
func TestUnionAreaWeldsPointsStraddlingAQuantumBoundary(t *testing.T) {
	const vLow, vHigh = 25.298813159233607, 25.298823011843776
	if weldUV([2]float64{0, vLow}) == weldUV([2]float64{0, vHigh}) {
		t.Fatalf("fixture no longer straddles a bucket boundary")
	}
	a := [2]float64{45.0, vLow}
	aNudged := [2]float64{45.0, vHigh}
	b := [2]float64{46.0, vLow}
	c := [2]float64{46.0, 26.0}
	d := [2]float64{45.0, 26.0}

	var tris [][3][2]float64
	tris = appendProjected(tris, a, b, c)
	tris = appendProjected(tris, aNudged, c, d)

	want := 26.0 - vLow // 1 m wide square
	if got := mustUnionArea(t, tris); math.Abs(got-want) > 1e-4 {
		t.Errorf("unionArea2D = %v, want %v", got, want)
	}
}

// TestUnionAreaIsTranslationInvariant: the same shape measured near the origin
// and 500 km away must give the same area. Green's theorem is exact about any
// point for a CLOSED boundary, so a result that moves with the input's world
// position is the tell that the boundary did not close — and the further out
// the model is georeferenced, the more a defect is magnified.
func TestUnionAreaIsTranslationInvariant(t *testing.T) {
	local := rectTris(nil, rect{0, 2, 0, 3}, rect{1, 4, 1, 2})
	want := mustUnionArea(t, local)

	for _, off := range [][2]float64{{65, 38}, {5000, 4000}, {500000, 4700000}} {
		moved := make([][3][2]float64, len(local))
		for i, tr := range local {
			for j, p := range tr {
				moved[i][j] = [2]float64{p[0] + off[0], p[1] + off[1]}
			}
		}
		if got := mustUnionArea(t, moved); math.Abs(got-want) > 1e-6 {
			t.Errorf("offset %v: unionArea2D = %v, want %v", off, got, want)
		}
	}
}

// TestMaxSilhouetteAxisCountsOverlappingFacesOnce: a wall with a pilaster
// projecting from its face. Both the wall face and the pilaster face point +X,
// and the pilaster's face sits directly in front of part of the wall's, so a Σ
// counts that overlapped strip twice. The elevational area you can draw, render
// or measure is what you SEE — the union — and the pilaster hides wall rather
// than adding face.
func TestMaxSilhouetteAxisCountsOverlappingFacesOnce(t *testing.T) {
	w, tris := appendBoxes(
		[2]v3{{0, 0, 0}, {0.2, 4, 3}},   // wall: +X face 4 x 3 = 12
		[2]v3{{0.2, 1, 0}, {0.5, 2, 3}}, // pilaster: +X face 1 x 3 = 3, in front of the wall
	)

	// The Σ this replaces, stated so a change to it cannot pass unnoticed.
	if sum, axis := maxSideAreaAxis(w, tris); math.Abs(sum-15) > 1e-9 || axis != 0 {
		t.Fatalf("maxSideAreaAxis = (%v, %d), want (15, 0) — fixture no longer states the Σ case", sum, axis)
	}

	area, axis := mustMaxSilhouette(t, w, tris)
	if axis != 0 {
		t.Errorf("axis = %d, want 0 (the YZ projection, X dropped)", axis)
	}
	if math.Abs(area-12) > 1e-9 {
		t.Errorf("area = %v, want 12 — the pilaster hides wall, it does not add face", area)
	}
}

// TestMaxSilhouetteAxisPicksTheAxisByUnionNotBySum: a brise-soleil — five fins
// standing off a plate. Along X the fins line up one behind another, so their Σ
// is large but they all project onto the same square; along Y the plate is
// genuinely broad. Σ picks X (5.1) and reads the fins five times over; the union
// picks Y (2.1), which is the projection that actually shows the most.
func TestMaxSilhouetteAxisPicksTheAxisByUnionNotBySum(t *testing.T) {
	boxes := [][2]v3{{{0, 1, 0}, {2, 1.1, 1}}} // plate: +Y face 2 x 1 = 2
	for i := 0; i < 5; i++ {                   // fins: +X face 1 x 1 = 1 each, all on the same YZ square
		x := float64(i) * 0.5
		boxes = append(boxes, [2]v3{{x, 0, 0}, {x + 0.1, 1, 1}})
	}
	w, tris := appendBoxes(boxes...)

	if sum, axis := maxSideAreaAxis(w, tris); math.Abs(sum-5.1) > 1e-9 || axis != 0 {
		t.Fatalf("maxSideAreaAxis = (%v, %d), want (5.1, 0) — fixture no longer states the Σ case", sum, axis)
	}

	area, axis := mustMaxSilhouette(t, w, tris)
	if axis != 1 {
		t.Errorf("axis = %d, want 1 (the XZ projection, Y dropped)", axis)
	}
	if math.Abs(area-2.1) > 1e-9 {
		t.Errorf("area = %v, want 2.1", area)
	}
}

// TestMaxSilhouetteAxisMatchesTheSumOnAPlainBox pins the two measures together
// where they must agree: a solid with no self-overlap along any axis has nothing
// for a Σ to double-count, so switching gross to a union changes no simple host.
func TestMaxSilhouetteAxisMatchesTheSumOnAPlainBox(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{0.2, 4, 3})
	sum, sumAxis := maxSideAreaAxis(w, tris)
	area, axis := mustMaxSilhouette(t, w, tris)
	if axis != sumAxis {
		t.Errorf("axis = %d, want %d (same winning projection as the Σ)", axis, sumAxis)
	}
	if math.Abs(area-sum) > 1e-9 {
		t.Errorf("area = %v, want %v (the Σ, since no face hides another)", area, sum)
	}
}
