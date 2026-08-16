package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
)

// wallMesh returns world verts+tris for a box wall long in X, thin in Y.
func wallMesh(min, max v3) ([]v3, []uint32) { return boxMeshWorld(min, max) }

func TestDominantAxisSurvivesOppositeFaceCancellation(t *testing.T) {
	// A wall 10m in X, 0.3m in Y has two large opposite ±Y faces. Summing SIGNED
	// normals cancels them and the winner becomes noise; the unsigned vote must
	// return the Y axis decisively.
	w, tris := wallMesh(v3{0, 0, 0}, v3{10, 0.3, 3})
	dir, area, share, ok := dominantAxis(w, tris)
	if !ok {
		t.Fatal("dominantAxis declined a plain wall")
	}
	if math.Abs(math.Abs(dir[1])-1) > 1e-9 {
		t.Fatalf("dir = %v, want the Y axis", dir)
	}
	if share < 0.9 {
		t.Fatalf("share = %v, want a decisive Y win", share)
	}
	// Two 10x3 faces vote.
	if math.Abs(area-60) > 1e-6 {
		t.Fatalf("area = %v, want 60", area)
	}
}

func TestDominantAxisIndependentOfWinding(t *testing.T) {
	w, tris := wallMesh(v3{0, 0, 0}, v3{10, 0.3, 3})
	dirA, _, _, _ := dominantAxis(w, tris)

	flipped := make([]uint32, len(tris))
	copy(flipped, tris)
	for i := 0; i+2 < len(flipped); i += 3 {
		flipped[i+1], flipped[i+2] = flipped[i+2], flipped[i+1]
	}
	dirB, _, _, ok := dominantAxis(w, flipped)
	if !ok {
		t.Fatal("dominantAxis declined the reversed winding")
	}
	if dirA != dirB {
		t.Fatalf("winding changed the axis: %v vs %v", dirA, dirB)
	}
}

func TestDominantAxisExcludesRoofAndFloor(t *testing.T) {
	// A slab 10x10x0.2 is nearly all horizontal face area. With roof/floor faces
	// excluded, the four thin edges split evenly across X and Y, so no axis
	// reaches the dominance floor.
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{10, 10, 0.2})
	if _, _, _, ok := dominantAxis(w, tris); ok {
		t.Fatal("dominantAxis accepted a slab; it has no facade")
	}
}

func TestDominantAxisDeclinesSquareColumn(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{0.4, 0.4, 3})
	if _, _, share, ok := dominantAxis(w, tris); ok {
		t.Fatalf("dominantAxis accepted a square column (share %v)", share)
	}
}

func TestDominantAxisFollowsPlacementRotation(t *testing.T) {
	// The frame regression: a 90° rotation about Z must rotate the answer. A
	// function reading local Verts would return the same axis and be uniformly
	// wrong without ever failing.
	e := elemBox(v3{0, 0, 0}, v3{10, 0.3, 3})
	e.Placement = model.Mat4{0, 1, 0, 0, -1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

	w := worldPoints(e.Verts, e.Placement)
	dir, _, _, ok := dominantAxis(w, e.Tris)
	if !ok {
		t.Fatal("dominantAxis declined the rotated wall")
	}
	// Y axis rotated 90° about Z becomes the X axis.
	if math.Abs(math.Abs(dir[0])-1) > 1e-9 {
		t.Fatalf("dir = %v, want the X axis after a 90° rotation", dir)
	}
}

func TestDominantAxisIsDeterministic(t *testing.T) {
	w, tris := wallMesh(v3{0, 0, 0}, v3{10, 0.3, 3})
	first, _, _, _ := dominantAxis(w, tris)
	for i := 0; i < 20; i++ {
		got, _, _, _ := dominantAxis(w, tris)
		if got != first {
			t.Fatalf("run %d returned %v, want bit-identical %v", i, got, first)
		}
	}
}

func TestDominantAxisEmptyMesh(t *testing.T) {
	if _, _, _, ok := dominantAxis(nil, nil); ok {
		t.Fatal("dominantAxis accepted an empty mesh")
	}
}
