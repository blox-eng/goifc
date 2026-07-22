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
