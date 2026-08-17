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

// wedgeWallMesh returns the two large faces of a wall 10m in X and 3m tall that
// thickens from 0.30m to 0.302m along its length. The taper is 0.006°, far
// below any tolerance here — but it gives BOTH faces a small NEGATIVE X normal
// component while their Y components stay opposite, which is the shape a real
// float32 vertex buffer produces on a nominally flat wall.
func wedgeWallMesh() ([]v3, []uint32) {
	const s = 0.001
	w := []v3{
		// Outer face, normal +Y.
		{0, 0.3, 0}, {10, 0.3 + s, 0}, {10, 0.3 + s, 3}, {0, 0.3, 3},
		// Inner face, normal -Y.
		{0, 0, 0}, {10, -s, 0}, {10, -s, 3}, {0, 0, 3},
	}
	tris := []uint32{
		0, 2, 1, 0, 3, 2,
		4, 5, 6, 4, 6, 7,
	}
	return w, tris
}

func TestDominantAxisFoldsFacesWithMatchingNormalJitter(t *testing.T) {
	// Both faces of this wall carry the same small -X noise, so a canonical sign
	// taken from the FIRST non-zero component flips both the same way and leaves
	// them antipodal: two buckets of 30 m², share 0.5, and the wall reports no
	// facade. The sign must come from the largest component instead.
	w, tris := wedgeWallMesh()

	dir, area, share, ok := dominantAxis(w, tris)
	if !ok {
		t.Fatalf("dominantAxis declined a wall with jittered normals (share %v)", share)
	}
	if math.Abs(math.Abs(dir[1])-1) > 1e-3 {
		t.Fatalf("dir = %v, want the Y axis", dir)
	}
	if share < 0.99 {
		t.Fatalf("share = %v, want both faces in one bucket", share)
	}
	if math.Abs(area-60) > 0.01 {
		t.Fatalf("area = %v, want both 10x3 faces voting (60)", area)
	}
}

func TestCanonAxisFoldsNearAntipodes(t *testing.T) {
	// Exact antipodes fold under any component-order rule, because negating a
	// vector flips its leading component too — which is why testing n against -n
	// proves nothing. The invariant that MATTERS is about near-antipodes: the two
	// faces of one wall are opposite to within tessellation noise, never exactly,
	// and they must still land close enough to share a bucket.
	pairs := [][2]v3{
		{{-1e-8, 1, 0}, {-1e-8, -1, 0}},     // matching X noise, opposite Y
		{{1e-8, 1, 0}, {2e-8, -1, 0}},       // matching X noise, differing magnitude
		{{1, 1e-8, 0}, {-1, 3e-8, 0}},       // an X-facing wall, noise in Y
		{{1e-9, 1e-9, 1}, {1e-9, 1e-9, -1}}, // a horizontal face pair
	}
	for _, p := range pairs {
		a, b := canonAxis(p[0]), canonAxis(p[1])
		la := math.Sqrt(dotv(a, a))
		lb := math.Sqrt(dotv(b, b))
		if la == 0 || lb == 0 {
			t.Fatalf("canonAxis zeroed %v or %v", p[0], p[1])
		}
		if cos := dotv(a, b) / (la * lb); cos < axisMergeCos {
			t.Fatalf("canonAxis(%v)=%v and canonAxis(%v)=%v fold apart (cos %v)",
				p[0], a, p[1], b, cos)
		}
	}
}

func TestDominantAxisEmptyMesh(t *testing.T) {
	if _, _, _, ok := dominantAxis(nil, nil); ok {
		t.Fatal("dominantAxis accepted an empty mesh")
	}
}
