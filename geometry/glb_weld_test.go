package geometry

import "testing"

// TestWeldMesh dedupes a quad stored as two triangles with 6 separate vertex
// copies (4 distinct corners) → 4 unique positions, indices preserving both tris.
func TestWeldMesh(t *testing.T) {
	verts := []float32{
		0, 0, 0, 1, 0, 0, 1, 1, 0, // tri 0
		0, 0, 0, 1, 1, 0, 0, 1, 0, // tri 1 (shares 2 corners with tri 0)
	}
	tris := []uint32{0, 1, 2, 3, 4, 5}

	positions, indices := weldMesh(verts, tris)

	if len(positions) != 4 {
		t.Fatalf("welded positions = %d want 4: %v", len(positions), positions)
	}
	if len(indices) != 6 {
		t.Fatalf("indices = %d want 6", len(indices))
	}
	// every index must be in range of the deduped positions
	for _, ix := range indices {
		if int(ix) >= len(positions) {
			t.Fatalf("index %d out of range (%d positions)", ix, len(positions))
		}
	}
	// the two triangles must reference the same welded corner for the shared verts:
	// tri0 v0 (0,0,0) == tri1 v0 (0,0,0); tri0 v2 (1,1,0) == tri1 v1 (1,1,0).
	if indices[0] != indices[3] {
		t.Errorf("shared vertex (0,0,0) not welded: %d vs %d", indices[0], indices[3])
	}
	if indices[2] != indices[4] {
		t.Errorf("shared vertex (1,1,0) not welded: %d vs %d", indices[2], indices[4])
	}
}

// TestWeldMeshSkipsOutOfRangeTriangle ensures a triangle referencing a
// vertex index past the end of verts is dropped whole, rather than
// silently remapped to vertex 0 (a corrupt degenerate triangle).
func TestWeldMeshSkipsOutOfRangeTriangle(t *testing.T) {
	verts := []float32{
		0, 0, 0, 1, 0, 0, 1, 1, 0, // one valid triangle, 3 verts
	}
	outOfRange := uint32(len(verts) / 3) // one past the last valid vertex index
	tris := []uint32{
		outOfRange, outOfRange + 1, outOfRange + 2, // malformed triangle, all OOB
		0, 1, 2, // valid triangle
	}

	positions, indices := weldMesh(verts, tris)

	if len(indices) != 3 {
		t.Fatalf("indices = %d want 3 (malformed triangle must be dropped whole): %v", len(indices), indices)
	}
	for _, ix := range indices {
		if int(ix) >= len(positions) {
			t.Fatalf("index %d out of range (%d positions)", ix, len(positions))
		}
	}
}
