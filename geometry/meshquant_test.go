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
	approx("volume", volume, 1.0)                 // unit cube
	approx("max_side_area", area, 1.0)            // each +axis face = 1×1
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

// boxWH scales the unit cube to dims (x,y,z) — helper for maxSideAreaAxis
// argmax/tie-break cases below.
func boxWH(x, y, z float32) ([]float32, []uint32) {
	v, tris := unitCube()
	for i := 0; i < len(v); i += 3 {
		v[i] *= x
		v[i+1] *= y
		v[i+2] *= z
	}
	return v, tris
}

func TestMaxSideAreaAxis(t *testing.T) {
	tests := []struct {
		name     string
		dims     [3]float32 // x, y, z box dimensions (unitCube scaled)
		wantArea float64
		wantAxis int
	}{
		// side areas: +X=y*z, +Y=x*z, +Z=x*y. The winning face's plane drops the
		// coordinate normal to it: +X wins -> axis 0 (YZ), +Y wins -> axis 1 (XZ),
		// +Z wins -> axis 2 (XY).
		{name: "argmax_x_face_wins", dims: [3]float32{2, 3, 4}, wantArea: 12, wantAxis: 0},
		{name: "argmax_y_face_wins", dims: [3]float32{4, 2, 3}, wantArea: 12, wantAxis: 1},
		{name: "argmax_z_face_wins_flat_slab", dims: [3]float32{4, 3, 2}, wantArea: 12, wantAxis: 2},
		// unit cube: all three side areas are exactly 1 (tied 3-way). Lowest index
		// (0) must win, deterministically.
		{name: "tie_break_lowest_axis_wins", dims: [3]float32{1, 1, 1}, wantArea: 1, wantAxis: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verts, tris := boxWH(tt.dims[0], tt.dims[1], tt.dims[2])
			area, axis := maxSideAreaAxis(toV3(verts), tris)
			if math.Abs(area-tt.wantArea) > 1e-6 {
				t.Errorf("area = %.6f want %.6f", area, tt.wantArea)
			}
			if axis != tt.wantAxis {
				t.Errorf("axis = %d want %d", axis, tt.wantAxis)
			}
		})
	}
}

// TestMaxSideAreaAxis_NoRegression confirms maxSideAreaAxis's area matches the
// scalar maxSideArea/meshQuantities value on the same fixture used by
// TestMeshQuantities_Box234 — the axis addition must not change the emitted
// area.
func TestMaxSideAreaAxis_NoRegression(t *testing.T) {
	v, tris := unitCube()
	for i := 0; i < len(v); i += 3 {
		v[i] *= 2
		v[i+1] *= 3
		v[i+2] *= 4
	}
	m := model.Identity()
	m[12], m[13], m[14] = 10, 20, 30

	wantArea, _, _ := meshQuantities(v, tris, m)

	w := make([]v3, len(v)/3)
	for i := range w {
		w[i] = applyMat(m, v3{float64(v[3*i]), float64(v[3*i+1]), float64(v[3*i+2])})
	}
	gotArea, gotAxis := maxSideAreaAxis(w, tris)

	if math.Abs(gotArea-wantArea) > 1e-9 {
		t.Errorf("maxSideAreaAxis area = %.9f, meshQuantities area = %.9f: must match exactly", gotArea, wantArea)
	}
	if gotAxis != 0 { // +X face (area 12) wins, X dropped -> axis 0
		t.Errorf("axis = %d want 0", gotAxis)
	}
}

// toV3 converts flat float32 verts to the []v3 shape maxSideAreaAxis takes
// (mirrors the conversion meshQuantities does internally after applying the
// placement — here the identity, so raw verts == world verts).
func toV3(verts []float32) []v3 {
	w := make([]v3, len(verts)/3)
	for i := range w {
		w[i] = v3{float64(verts[3*i]), float64(verts[3*i+1]), float64(verts[3*i+2])}
	}
	return w
}
