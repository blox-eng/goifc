package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
)

// boxElement is a test-only Element whose mesh is one axis-aligned box, given
// in world coordinates with an identity placement.
func boxElement(gid string, min, max v3) Element {
	w, tris := boxMeshWorld(min, max)
	verts := make([]float32, 0, 3*len(w))
	for _, p := range w {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	return Element{
		GlobalID: gid,
		Verts:    verts,
		Tris:     tris,
		Placement: model.Mat4{
			1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
		BBoxMin: [3]float64{min[0], min[1], min[2]},
		BBoxMax: [3]float64{max[0], max[1], max[2]},
		Source:  SourceBrep,
	}
}

// loopArea is the absolute shoelace area of a loop.
func loopArea(l Loop) float64 {
	var twice float64
	for i := range l.Points {
		j := (i + 1) % len(l.Points)
		twice += l.Points[i][0]*l.Points[j][1] - l.Points[j][0]*l.Points[i][1]
	}
	return math.Abs(twice) / 2
}

// TestElevationPlaneKeepsUpPointingUp: a drawing's vertical axis must be the
// world's. PlaneFromNormal would also produce a valid basis, but its in-plane
// orientation is documented as unspecified and free to change between versions,
// so an elevation built on it would be rotated by an arbitrary amount.
func TestElevationPlaneKeepsUpPointingUp(t *testing.T) {
	for _, dir := range [][3]float64{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {1, 1, 0}} {
		p, ok := ElevationPlane(dir)
		if !ok {
			t.Fatalf("dir %v: not ok", dir)
		}
		if !p.Valid() {
			t.Errorf("dir %v: basis invalid", dir)
		}
		if p.V != [3]float64{0, 0, 1} {
			t.Errorf("dir %v: V = %v, want world up", dir, p.V)
		}
		if p.U[2] != 0 {
			t.Errorf("dir %v: U = %v, want it horizontal", dir, p.U)
		}
	}
}

// TestElevationPlaneRefusesAVerticalDirection: looking straight down is a plan,
// not an elevation, and there is no "up" left on the page to orient it by.
func TestElevationPlaneRefusesAVerticalDirection(t *testing.T) {
	for _, dir := range [][3]float64{{0, 0, 1}, {0, 0, -1}, {0, 0, 0}} {
		if _, ok := ElevationPlane(dir); ok {
			t.Errorf("dir %v: want ok=false", dir)
		}
	}
}

// TestSilhouetteOnBoxFromEachDirection: a 3 x 5 x 2 m box seen from each
// horizontal direction is a rectangle of the two dimensions across the view.
// Catches a swapped or mis-signed UV basis immediately, which is the error a
// projection API fails at first and most silently.
func TestSilhouetteOnBoxFromEachDirection(t *testing.T) {
	e := boxElement("box", v3{0, 0, 0}, v3{3, 5, 2})
	for _, tc := range []struct {
		dir  [3]float64
		want float64
	}{
		{[3]float64{1, 0, 0}, 5 * 2},
		{[3]float64{-1, 0, 0}, 5 * 2},
		{[3]float64{0, 1, 0}, 3 * 2},
		{[3]float64{0, -1, 0}, 3 * 2},
	} {
		p, ok := ElevationPlane(tc.dir)
		if !ok {
			t.Fatalf("dir %v: no plane", tc.dir)
		}
		loops := e.SilhouetteOn(p)
		if len(loops) != 1 {
			t.Fatalf("dir %v: want 1 loop, got %d", tc.dir, len(loops))
		}
		if got := loopArea(loops[0]); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("dir %v: area = %v, want %v", tc.dir, got, tc.want)
		}
		if loops[0].Role != LoopSilhouette {
			t.Errorf("dir %v: role = %q, want %q", tc.dir, loops[0].Role, LoopSilhouette)
		}
	}
}

// TestSilhouetteOnIsDeterministic: identical input must yield byte-identical
// output, so a consumer diffing drawings across imports sees no false churn.
func TestSilhouetteOnIsDeterministic(t *testing.T) {
	e := boxElement("box", v3{0, 0, 0}, v3{3, 5, 2})
	p, _ := ElevationPlane([3]float64{1, 0, 0})
	a, b := e.SilhouetteOn(p), e.SilhouetteOn(p)
	if len(a) != len(b) {
		t.Fatalf("loop count differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Role != b[i].Role || len(a[i].Points) != len(b[i].Points) {
			t.Fatalf("loop %d differs", i)
		}
		for j := range a[i].Points {
			if a[i].Points[j] != b[i].Points[j] {
				t.Fatalf("loop %d point %d differs", i, j)
			}
		}
	}
}

// TestSilhouetteOnRejectsABadPlane: no rings rather than wrongly-wound ones —
// a left-handed basis silently swaps outer rings and holes.
func TestSilhouetteOnRejectsABadPlane(t *testing.T) {
	e := boxElement("box", v3{0, 0, 0}, v3{3, 5, 2})
	left := Plane{U: [3]float64{1, 0, 0}, V: [3]float64{0, 0, 1}, N: [3]float64{0, 1, 0}}
	if left.Valid() {
		t.Fatal("fixture is no longer left-handed")
	}
	if loops := e.SilhouetteOn(left); loops != nil {
		t.Errorf("want nil for an invalid basis, got %d loops", len(loops))
	}
}

// TestSilhouetteOnEmptyMesh: an element with no mesh has no silhouette, and
// must not fall back to a bounding box the way FootprintOn does — a caller
// asking for a projection wants to know it got nothing.
func TestSilhouetteOnEmptyMesh(t *testing.T) {
	p, _ := ElevationPlane([3]float64{1, 0, 0})
	if loops := (Element{GlobalID: "empty"}).SilhouetteOn(p); loops != nil {
		t.Errorf("want nil, got %d loops", len(loops))
	}
}
