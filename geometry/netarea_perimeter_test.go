package geometry

import (
	"math"
	"testing"
)

func mustUnionPerimeter(t *testing.T, tris [][3][2]float64) float64 {
	t.Helper()
	_, p, ok := unionMeasure2D(tris)
	if !ok {
		t.Fatal("unionMeasure2D refused: the boundary did not close")
	}
	return p
}

// TestUnionPerimeter pins what "perimeter of a union" means, which is NOT the Σ
// of the parts' perimeters. Every expectation below is the length of the OUTLINE
// a pencil would trace around the covered region, derived by hand.
//
// The seam cases are the point: two footprints that merge share an edge interior
// to the union, and an interior edge belongs to the outline no more than shared
// area belongs to the deduction twice. Reveals are billed ALONG this outline, so
// a Σ would bill a seam that does not exist on the building.
func TestUnionPerimeter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rects []rect
		want  float64
	}{
		{"none", nil, 0},
		{"single", []rect{{0, 2, 0, 3}}, 10}, // 2*(2+3)
		{"disjoint", []rect{{0, 1, 0, 1}, {5, 6, 5, 6}}, 8},
		// THE SEAM CASE: two unit squares sharing a full edge are one 2x1
		// rectangle. Σ of the parts' perimeters is 8; the outline is 6.
		{"edge-touching in u", []rect{{0, 1, 0, 1}, {1, 2, 0, 1}}, 6},
		{"edge-touching in v", []rect{{0, 1, 0, 1}, {0, 1, 1, 2}}, 6},
		// A contained rect contributes no boundary — it is wholly interior.
		// Σ would add its 4.
		{"nested", []rect{{0, 2, 0, 2}, {0.5, 1.5, 0.5, 1.5}}, 8},
		// The same void exported three times is still one outline.
		{"identical", []rect{{0, 2, 0, 2}, {0, 2, 0, 2}, {0, 2, 0, 2}}, 8},
		// Three collinear rects merge into [0,4]x[0,1]: 2*(4+1).
		{"chain of three", []rect{{0, 2, 0, 1}, {1, 3, 0, 1}, {2, 4, 0, 1}}, 10},
		// Diagonally offset unit squares form a stepped outline. Tracing it:
		// 1 + 0.3 + 0.3 + 1 + 1 + 0.3 + 0.3 + 1 = 5.2.
		{"partial overlap", []rect{{0, 1, 0, 1}, {0.3, 1.3, 0.3, 1.3}}, 5.2},
		{"degenerate zero width", []rect{{1, 1, 0, 5}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mustUnionPerimeter(t, rectTris(nil, tc.rects...))
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("perimeter = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestUnionPerimeterEnclosesItsArea checks the two measures against each other
// instead of against a constant: among all shapes of a given area the disc has
// the shortest boundary, so perimeter² >= 4·pi·area holds for every union the
// engine can report. A perimeter that skipped seams it should have counted — or
// counted seams it should not — breaks this with no hand-computed expectation to
// argue with.
func TestUnionPerimeterEnclosesItsArea(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rects []rect
	}{
		{"single", []rect{{0, 2, 0, 3}}},
		{"disjoint", []rect{{0, 1, 0, 1}, {5, 6, 5, 6}}},
		{"merged", []rect{{0, 1, 0, 1}, {1, 2, 0, 1}}},
		{"chain of three", []rect{{0, 2, 0, 1}, {1, 3, 0, 1}, {2, 4, 0, 1}}},
		{"partial overlap", []rect{{0, 1, 0, 1}, {0.3, 1.3, 0.3, 1.3}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			area, perim, ok := unionMeasure2D(rectTris(nil, tc.rects...))
			if !ok {
				t.Fatal("unionMeasure2D refused")
			}
			if perim*perim < 4*math.Pi*area-1e-9 {
				t.Errorf("isoperimetric violation: perimeter %v encloses area %v", perim, area)
			}
		})
	}
}

// TestUnionMeasureAgreesWithUnionArea keeps the two entry points from drifting:
// unionArea2D delegates to unionMeasure2D, and a refactor that gave either its
// own boundary walk could return an area and a perimeter describing different
// shapes on real coordinates, where the walk is not guaranteed to classify
// identically twice.
func TestUnionMeasureAgreesWithUnionArea(t *testing.T) {
	tris := rectTris(nil, rect{0, 1, 0, 1}, rect{0.3, 1.3, 0.3, 1.3})
	viaArea, okA := unionArea2D(tris)
	viaMeasure, _, okM := unionMeasure2D(tris)
	if !okA || !okM {
		t.Fatal("union refused")
	}
	if math.Abs(viaArea-viaMeasure) > 1e-12 {
		t.Errorf("unionArea2D = %v but unionMeasure2D = %v", viaArea, viaMeasure)
	}
}
