package geometry

import (
	"math"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/model"
)

func mustWall(t *testing.T, edge [2][2]float64, base, height, thickness float64, side Side, ops []Opening) Wall {
	t.Helper()
	w, err := BuildWall(edge, base, height, thickness, side, ops)
	if err != nil {
		t.Fatalf("BuildWall: %v", err)
	}
	return w
}

func mustShapes(t *testing.T, e Element) []previewShape {
	t.Helper()
	got, err := elementShapes(e, 0)
	if err != nil {
		t.Fatalf("elementShapes: %v", err)
	}
	return got
}

func classCount(shapes []previewShape) map[string]int {
	n := map[string]int{}
	for _, s := range shapes {
		n[s.class]++
	}
	return n
}

// shapeFacing returns the one shape whose normal is within 1e-6 of n.
func shapeFacing(t *testing.T, shapes []previewShape, n [3]float64) previewShape {
	t.Helper()
	var found []previewShape
	for _, s := range shapes {
		if math.Abs(s.normal[0]-n[0]) < 1e-6 && math.Abs(s.normal[1]-n[1]) < 1e-6 && math.Abs(s.normal[2]-n[2]) < 1e-6 {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d shapes face %v, want 1", len(found), n)
	}
	return found[0]
}

func TestPreviewClass(t *testing.T) {
	cases := []struct {
		n    [3]float64
		want string
	}{
		{[3]float64{0, 0, 1}, "top"},
		{[3]float64{-1, 0, 0}, "left"},
		{[3]float64{0, -1, 0}, "right"},
		{[3]float64{-0.6, -0.8, 0}, "right"},
		{[3]float64{-0.8, -0.6, 0}, "left"},
		{[3]float64{0, -0.8, 0.6}, "top"},
	}
	for _, c := range cases {
		if got := previewClass(c.n); got != c.want {
			t.Errorf("previewClass(%v) = %s, want %s", c.n, got, c.want)
		}
	}
}

func TestPreviewPoint(t *testing.T) {
	// A far (north-east) point draws higher, so its y is smaller.
	near, far := previewPoint([3]float64{0, 0, 0}), previewPoint([3]float64{1, 1, 0})
	if !(far[1] < near[1]) || math.Abs(far[0]-near[0]) > 1e-12 {
		t.Errorf("far %v, near %v: want far straight above near", far, near)
	}
	// Up draws up.
	if up := previewPoint([3]float64{0, 0, 1}); !(up[1] < 0) {
		t.Errorf("up %v: want y < 0", up)
	}
	// East draws right.
	if east := previewPoint([3]float64{1, 0, 0}); !(east[0] > 0) {
		t.Errorf("east %v: want x > 0", east)
	}
}

func TestElementShapes_SquareWallShowsThreeFaces(t *testing.T) {
	w := mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideRight, nil)
	got := mustShapes(t, w.Element)
	if n := classCount(got); len(got) != 3 || n["top"] != 1 || n["left"] != 1 || n["right"] != 1 {
		t.Fatalf("shapes %v, want one top, one left, one right", n)
	}
	for _, s := range got {
		if len(s.loops) != 1 || len(s.loops[0]) != 4 {
			t.Errorf("%s: loops %v, want one loop of 4 points", s.class, s.loops)
		}
	}
	south := shapeFacing(t, got, [3]float64{0, -1, 0})
	for _, p := range south.loops[0] {
		if math.Abs(p[1]) > 1e-6 {
			t.Errorf("south face point %v is off the plane y = 0", p)
		}
	}
}

func TestElementShapes_WindowMakesAHole(t *testing.T) {
	ops := []Opening{{Offset: 1, Sill: 0.9, Width: 1, Height: 1.2}}
	w := mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideRight, ops)
	got := mustShapes(t, w.Element)
	// top, south face, west end, the jamb at x = 2 (facing west), the sill.
	if n := classCount(got); len(got) != 5 || n["top"] != 2 || n["left"] != 2 || n["right"] != 1 {
		t.Fatalf("shapes %v (%d), want 2 top, 2 left, 1 right", n, len(got))
	}
	south := shapeFacing(t, got, [3]float64{0, -1, 0})
	if len(south.loops) != 2 {
		t.Fatalf("south face has %d loops, want 2", len(south.loops))
	}
	for _, l := range south.loops {
		if len(l) != 4 {
			t.Errorf("loop %v, want 4 points", l)
		}
	}
}

func TestElementShapes_DoorNotchesTheOutline(t *testing.T) {
	ops := []Opening{{Offset: 1, Sill: 0, Width: 0.9, Height: 2.1}}
	w := mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideRight, ops)
	south := shapeFacing(t, mustShapes(t, w.Element), [3]float64{0, -1, 0})
	if len(south.loops) != 1 || len(south.loops[0]) != 8 {
		t.Fatalf("south face loops %v, want one loop of 8 points", south.loops)
	}
}

func TestElementShapes_FarOriginMatchesShiftedNear(t *testing.T) {
	ops := []Opening{{Offset: 1, Sill: 0.9, Width: 1, Height: 1.2}}
	near := mustShapes(t, mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideRight, ops).Element)
	far := mustShapes(t, mustWall(t, [2][2]float64{{1e6, 1e6}, {1e6 + 3, 1e6}}, 0, 2.5, 0.2, SideRight, ops).Element)
	if len(near) != len(far) {
		t.Fatalf("%d shapes near, %d far", len(near), len(far))
	}
	for i := range near {
		if len(near[i].loops) != len(far[i].loops) {
			t.Fatalf("shape %d: %d loops near, %d far", i, len(near[i].loops), len(far[i].loops))
		}
		for j := range near[i].loops {
			for k, p := range near[i].loops[j] {
				q := far[i].loops[j][k]
				if math.Abs(q[0]-1e6-p[0]) > 1e-3 || math.Abs(q[1]-1e6-p[1]) > 1e-3 || math.Abs(q[2]-p[2]) > 1e-3 {
					t.Fatalf("shape %d loop %d point %d: far %v, near %v", i, j, k, q, p)
				}
			}
		}
	}
}

func TestElementShapes_RefusesBadMesh(t *testing.T) {
	nan := Element{Verts: []float32{float32(math.NaN()), 0, 0, 1, 0, 0, 0, 1, 0}, Tris: []uint32{0, 1, 2}, Placement: model.Identity()}
	if _, err := elementShapes(nan, 0); err == nil || !strings.Contains(err.Error(), "vertex") {
		t.Errorf("NaN vertex: err %v, want one naming the vertex", err)
	}
	bad := model.Identity()
	bad[12] = math.Inf(1)
	inf := Element{Verts: []float32{0, 0, 0, 1, 0, 0, 0, 1, 0}, Tris: []uint32{0, 1, 2}, Placement: bad}
	if _, err := elementShapes(inf, 0); err == nil || !strings.Contains(err.Error(), "placement") {
		t.Errorf("Inf placement: err %v, want one naming the placement", err)
	}
	idx := Element{Verts: []float32{0, 0, 0, 1, 0, 0, 0, 1, 0}, Tris: []uint32{0, 1, 99}, Placement: model.Identity()}
	if _, err := elementShapes(idx, 0); err == nil || !strings.Contains(err.Error(), "index") {
		t.Errorf("index 99: err %v, want one naming the index", err)
	}
}

func mustOpenings(t *testing.T, w Wall) ([]previewShape, []previewShape) {
	t.Helper()
	shapes := mustShapes(t, w.Element)
	return shapes, wallOpenings(w, shapes, 0)
}

var previewWindow = []Opening{{Offset: 1, Sill: 0.9, Width: 1, Height: 1.2}}

func TestWallOpenings_OnTheMeasuredFaceWhenItFacesTheViewer(t *testing.T) {
	_, got := mustOpenings(t, mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideRight, previewWindow))
	if len(got) != 1 || got[0].class != "opening" || len(got[0].loops) != 1 || len(got[0].loops[0]) != 4 {
		t.Fatalf("openings %v, want one opening of 4 points", got)
	}
	for _, p := range got[0].loops[0] {
		if math.Abs(p[1]) > 1e-9 {
			t.Errorf("corner %v is off the measured plane y = 0", p)
		}
	}
}

func TestWallOpenings_OnTheOtherFaceWhenMeasuredInside(t *testing.T) {
	// The same solid twice, y in [-0.2, 0]: measured on the north face, then on the south face.
	_, inside := mustOpenings(t, mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideLeft, previewWindow))
	_, outside := mustOpenings(t, mustWall(t, [2][2]float64{{0, -0.2}, {3, -0.2}}, 0, 2.5, 0.2, SideRight, previewWindow))
	if len(inside) != 1 || len(outside) != 1 {
		t.Fatalf("%d openings measured inside, %d outside, want 1 each", len(inside), len(outside))
	}
	for k, p := range inside[0].loops[0] {
		q := outside[0].loops[0][k]
		if math.Abs(p[0]-q[0]) > 1e-3 || math.Abs(p[1]-q[1]) > 1e-3 || math.Abs(p[2]-q[2]) > 1e-3 {
			t.Errorf("corner %d: %v measured inside, %v outside", k, p, q)
		}
	}
}

func TestWallOpenings_NicheOnAHiddenFaceIsNotDrawn(t *testing.T) {
	niche := []Opening{{Offset: 1, Sill: 0.9, Width: 1, Height: 1.2, Depth: 0.05}}
	if _, got := mustOpenings(t, mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideLeft, niche)); len(got) != 0 {
		t.Errorf("openings %v, want none", got)
	}
}

func TestWallOpenings_OnFaceInFrontOfFaceSeq(t *testing.T) {
	shapes, got := mustOpenings(t, mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideLeft, previewWindow))
	south := shapeFacing(t, shapes, [3]float64{0, -1, 0})
	if len(got) != 1 || got[0].seq != south.seq || got[0].elem != south.elem {
		t.Fatalf("opening %v, want seq %d (the south face)", got, south.seq)
	}
}
