package geometry

import (
	"math"
	"testing"
)

// Regression for a review finding: profilePolygon returned an arbitrary
// profile's polygon in its as-parsed winding, but triangulatePolygon
// internally CCW-normalizes for the caps while extrudeSolid built side walls
// straight from that order — a CW input polygon left side walls facing inward
// (culled) while caps faced outward. ensureCCW must normalize any input to CCW
// without altering the vertex set.
func TestEnsureCCW_ReversesClockwisePolygon(t *testing.T) {
	// A unit square wound clockwise.
	cw := [][2]float64{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
	if polygonArea2D(cw) >= 0 {
		t.Fatalf("test fixture is not clockwise: area=%f", polygonArea2D(cw))
	}
	out := ensureCCW(cw)
	if polygonArea2D(out) <= 0 {
		t.Errorf("ensureCCW did not produce a CCW polygon: area=%f", polygonArea2D(out))
	}
	if len(out) != len(cw) {
		t.Fatalf("ensureCCW changed vertex count: got %d, want %d", len(out), len(cw))
	}
	seen := make(map[[2]float64]bool, len(cw))
	for _, p := range cw {
		seen[p] = true
	}
	for _, p := range out {
		if !seen[p] {
			t.Errorf("ensureCCW introduced/changed a vertex: %v not in original set", p)
		}
	}
}

// A CCW polygon must be returned unchanged (same order), not needlessly reversed.
func TestEnsureCCW_LeavesCounterClockwiseUnchanged(t *testing.T) {
	ccw := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	if polygonArea2D(ccw) <= 0 {
		t.Fatalf("test fixture is not CCW: area=%f", polygonArea2D(ccw))
	}
	out := ensureCCW(ccw)
	for i := range ccw {
		if out[i] != ccw[i] {
			t.Errorf("ensureCCW reordered an already-CCW polygon at index %d: got %v, want %v", i, out[i], ccw[i])
		}
	}
}

// An L-shaped (concave) hexagon: a naive fan would produce an inverted/overlapping
// triangle. Ear-clipping must yield exactly n-2 = 4 triangles, all inside the polygon.
func TestTriangulate_ConcaveL(t *testing.T) {
	L := [][2]float64{{0, 0}, {2, 0}, {2, 1}, {1, 1}, {1, 2}, {0, 2}}
	tris := triangulatePolygon(L)
	if len(tris) != (len(L)-2)*3 {
		t.Fatalf("got %d indices, want %d (n-2 triangles)", len(tris), (len(L)-2)*3)
	}
	// Total triangulated area must equal the polygon area (no overlap/inversion).
	var area float64
	for i := 0; i+2 < len(tris); i += 3 {
		a, b, c := L[tris[i]], L[tris[i+1]], L[tris[i+2]]
		area += math.Abs(cross2D(a, b, c)) / 2
	}
	want := math.Abs(polygonArea2D(L)) // = 3.0
	if math.Abs(area-want) > 1e-9 {
		t.Errorf("triangulated area = %f, want %f (overlap or inversion)", area, want)
	}
	// Every emitted triangle must share CCW winding (positive signed area). The
	// oracle gate is AABB-only and blind to inside-out faces; a single-sided viewer
	// material culls wrongly-wound faces while all automated gates stay green.
	for i := 0; i+2 < len(tris); i += 3 {
		if cross2D(L[tris[i]], L[tris[i+1]], L[tris[i+2]]) <= 0 {
			t.Errorf("triangle %d is not CCW (inside-out)", i/3)
		}
	}
}

// faceTrisAgreeWithNormal returns true iff every triangle triangulateFace emits
// is wound so its geometric normal points the SAME way as the loop's Newell
// normal (positive dot). This is the outward-consistency invariant brep meshes
// need; the pre-fix projection is sign-blind and fails it for negative-facing faces.
func faceTrisAgreeWithNormal(t *testing.T, loop []v3) bool {
	t.Helper()
	// Newell normal of the loop (same computation triangulateFace uses).
	var n v3
	for i := range loop {
		j := (i + 1) % len(loop)
		n[0] += (loop[i][1] - loop[j][1]) * (loop[i][2] + loop[j][2])
		n[1] += (loop[i][2] - loop[j][2]) * (loop[i][0] + loop[j][0])
		n[2] += (loop[i][0] - loop[j][0]) * (loop[i][1] + loop[j][1])
	}
	tris := triangulateFace(loop)
	if len(tris) < 3 {
		t.Fatalf("triangulateFace returned %d indices, want >=3", len(tris))
	}
	for i := 0; i+2 < len(tris); i += 3 {
		a, b, c := loop[tris[i]], loop[tris[i+1]], loop[tris[i+2]]
		e1 := v3{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
		e2 := v3{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
		if dotv(crossv(e1, e2), n) < 0 {
			return false
		}
	}
	return true
}

// A +Z-facing square (CCW seen from +Z) and its mirror -Z-facing square (CCW
// seen from -Z) must BOTH triangulate to outward-agreeing winding. Pre-fix, the
// -Z case comes back inward.
func TestTriangulateFace_WindingMatchesNormalBothSigns(t *testing.T) {
	up := []v3{{0, 0, 0}, {1, 0, 0}, {1, 1, 0}, {0, 1, 0}} // Newell normal +Z
	if !faceTrisAgreeWithNormal(t, up) {
		t.Error("+Z face: triangles disagree with Newell normal")
	}
	down := []v3{{0, 0, 0}, {0, 1, 0}, {1, 1, 0}, {1, 0, 0}} // reversed → Newell normal -Z
	if !faceTrisAgreeWithNormal(t, down) {
		t.Error("-Z face: triangles disagree with Newell normal (the sign-blind projection bug)")
	}
	// Sanity: the two loops really do have opposite-Z Newell normals.
	nz := func(loop []v3) float64 {
		var z float64
		for i := range loop {
			j := (i + 1) % len(loop)
			z += (loop[i][0] - loop[j][0]) * (loop[i][1] + loop[j][1])
		}
		return z
	}
	if !(nz(up) > 0 && nz(down) < 0) {
		t.Fatalf("test setup wrong: nz(up)=%v nz(down)=%v", nz(up), nz(down))
	}
	_ = math.Abs
}

// A face whose dominant normal axis is X and points negative must also agree.
func TestTriangulateFace_NegativeXFace(t *testing.T) {
	// Square in the x=0 plane, wound so Newell normal points -X.
	loop := []v3{{0, 0, 0}, {0, 0, 1}, {0, 1, 1}, {0, 1, 0}}
	if !faceTrisAgreeWithNormal(t, loop) {
		t.Error("-X face: triangles disagree with Newell normal")
	}
}
