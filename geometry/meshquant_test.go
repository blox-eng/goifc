package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/common/ifc/model"
)

// unitCube returns a watertight, outward-wound unit cube [0,1]^3 as float32 verts
// (8 shared corners) + 12 triangles.
func unitCube() ([]float32, []uint32) {
	verts := []float32{
		0, 0, 0, // 0
		1, 0, 0, // 1
		1, 1, 0, // 2
		0, 1, 0, // 3
		0, 0, 1, // 4
		1, 0, 1, // 5
		1, 1, 1, // 6
		0, 1, 1, // 7
	}
	tris := []uint32{
		0, 2, 1, 0, 3, 2, // bottom (−Z)
		4, 5, 6, 4, 6, 7, // top (+Z)
		1, 2, 6, 1, 6, 5, // +X
		0, 4, 7, 0, 7, 3, // −X
		2, 3, 7, 2, 7, 6, // +Y
		0, 1, 5, 0, 5, 4, // −Y
	}
	return verts, tris
}

func TestMeshQuantities_UnitCube(t *testing.T) {
	verts, tris := unitCube()
	area, volume, perimeter := meshQuantities(verts, tris, model.Identity())

	approx := func(name string, got, want float64) {
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("%s = %.6f want %.6f", name, got, want)
		}
	}
	approx("volume", volume, 1.0)      // unit cube
	approx("max_side_area", area, 1.0) // each +axis face = 1×1
	approx("footprint_perimeter", perimeter, 4.0) // bottom square outline
}

// TestMeshQuantities_Placement: a 2×3×4 box translated + still axis-aligned. Volume
// 24, max side area = 3×4=12 (the +X face after mapping), footprint perimeter =
// 2(2+3)=10 (bottom 2×3 rectangle). Verifies world-transform + axis assignment.
func TestMeshQuantities_Box234(t *testing.T) {
	// scale the unit cube to 2(x)×3(y)×4(z)
	v, tris := unitCube()
	for i := 0; i < len(v); i += 3 {
		v[i] *= 2
		v[i+1] *= 3
		v[i+2] *= 4
	}
	// translate by (10, 20, 30) via placement
	m := model.Identity()
	m[12], m[13], m[14] = 10, 20, 30
	area, volume, perimeter := meshQuantities(v, tris, m)

	if math.Abs(volume-24) > 1e-4 {
		t.Errorf("volume = %.4f want 24", volume)
	}
	// side areas: +X face = y*z = 3*4 = 12; +Y = x*z = 2*4 = 8; +Z = x*y = 2*3 = 6. max = 12.
	if math.Abs(area-12) > 1e-4 {
		t.Errorf("max_side_area = %.4f want 12", area)
	}
	// footprint (bottom, 2×3) perimeter = 2*(2+3) = 10.
	if math.Abs(perimeter-10) > 1e-4 {
		t.Errorf("footprint_perimeter = %.4f want 10", perimeter)
	}
}
