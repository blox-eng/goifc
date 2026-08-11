package geometry

import (
	"math"

	"github.com/blox-eng/goifc/model"
)

// meshQuantities computes tier-2 quantities from an element's WORLD-space proxy
// mesh, porting ifcopenshell.util.shape so the values track the Python
// import_emit baseline (which measured real world-coord meshes):
//
//	area      = get_max_side_area        (max over X/Y/Z of Σ actual area of tris facing +axis)
//	volume    = get_volume               (|Σ signed tetra|, unconditional)
//	perimeter = get_footprint_perimeter  (boundary edge length of downward −Z faces)
//
// Verts are element-local; placement lifts them to world — orientation matters for
// area (which axis a face faces) and perimeter (which faces point down). Returns
// zeros for a degenerate mesh.
func meshQuantities(verts []float32, tris []uint32, placement model.Mat4) (area, volume, perimeter float64) {
	if len(tris) < 3 || len(verts) < 9 {
		return 0, 0, 0
	}
	w := worldPoints(verts, placement)
	return maxSideArea(w, tris), meshVolumeWorld(w, tris), footprintPerimeter(w, tris)
}

func distv(a, b v3) float64 { d := subv(a, b); return math.Sqrt(dotv(d, d)) }

// sideArea sums the ACTUAL areas of triangles whose unit normal points toward
// +axis (dot > 0.01, matching ifcopenshell get_side_area at angle 90°).
func sideArea(w []v3, tris []uint32, axis int) float64 {
	nv := uint32(len(w))
	var total float64
	for t := 0; t+2 < len(tris); t += 3 {
		i0, i1, i2 := tris[t], tris[t+1], tris[t+2]
		if i0 >= nv || i1 >= nv || i2 >= nv {
			continue
		}
		n := crossv(subv(w[i1], w[i0]), subv(w[i2], w[i0]))
		l := math.Sqrt(dotv(n, n))
		if l == 0 {
			continue
		}
		if n[axis]/l > 0.01 {
			total += 0.5 * l
		}
	}
	return total
}

func maxSideArea(w []v3, tris []uint32) float64 {
	area, _ := maxSideAreaAxis(w, tris)
	return area
}

// maxSideAreaAxis is maxSideArea extended to also report which projection won,
// for callers (e.g. net-area opening subtraction) that need to measure
// openings on the same plane as their host's max-side pick.
//
// axis is the DROPPED coordinate index of the winning projection — i.e. the
// same axis index sideArea was called with, since sideArea(axis) sums the
// faces whose normal points toward +axis, which is exactly the plane
// perpendicular to axis (the plane you'd project onto by dropping that
// coordinate): 0 = YZ plane (X dropped), 1 = XZ plane (Y dropped), 2 = XY
// plane (Z dropped).
//
// Ties are broken by lowest axis index, deterministically, so a host and its
// openings — measured separately — always agree on the winning plane.
func maxSideAreaAxis(w []v3, tris []uint32) (area float64, axis int) {
	areas := [3]float64{sideArea(w, tris, 0), sideArea(w, tris, 1), sideArea(w, tris, 2)}
	area, axis = areas[0], 0
	for i := 1; i < 3; i++ {
		if areas[i] > area {
			area, axis = areas[i], i
		}
	}
	return area, axis
}

// meshVolumeWorld is ifcopenshell get_volume: |Σ signed tetrahedra|, computed
// unconditionally (no manifold gate — matches the Python baseline). Kept separate
// from meshVolume (which gates on isClosedManifold for the model's stricter
// authored-vs-derived contract).
func meshVolumeWorld(w []v3, tris []uint32) float64 {
	nv := uint32(len(w))
	var vol float64
	for t := 0; t+2 < len(tris); t += 3 {
		i0, i1, i2 := tris[t], tris[t+1], tris[t+2]
		if i0 >= nv || i1 >= nv || i2 >= nv {
			continue
		}
		vol += dotv(w[i0], crossv(w[i1], w[i2]))
	}
	return math.Abs(vol / 6)
}

// footprintPerimeter is ifcopenshell get_footprint_perimeter: the total length of
// the boundary (unshared) edges of the downward-facing (−Z normal) faces. Go brep
// meshes emit a fresh vertex block per face (no shared indices, unlike
// ifcopenshell), so vertices are welded by quantized WORLD position first — else
// every edge looks unshared and the perimeter blows up.
func footprintPerimeter(w []v3, tris []uint32) float64 {
	nv := uint32(len(w))
	const q = 1e5 // 1e-5 m weld quantum
	weld := make(map[[3]int64]int)
	pos := make(map[int]v3)
	id := func(i uint32) int {
		k := [3]int64{
			int64(math.Round(w[i][0] * q)),
			int64(math.Round(w[i][1] * q)),
			int64(math.Round(w[i][2] * q)),
		}
		v, ok := weld[k]
		if !ok {
			v = len(weld)
			weld[k] = v
			pos[v] = w[i]
		}
		return v
	}
	type edge [2]int
	all := make(map[edge]bool)
	shared := make(map[edge]bool)
	for t := 0; t+2 < len(tris); t += 3 {
		i0, i1, i2 := tris[t], tris[t+1], tris[t+2]
		if i0 >= nv || i1 >= nv || i2 >= nv {
			continue
		}
		n := crossv(subv(w[i1], w[i0]), subv(w[i2], w[i0]))
		l := math.Sqrt(dotv(n, n))
		if l == 0 || n[2]/l >= -1e-6 { // keep only downward faces
			continue
		}
		a, b, c := id(i0), id(i1), id(i2)
		for _, e := range [3]edge{{a, b}, {b, c}, {c, a}} {
			rev := edge{e[1], e[0]}
			if all[e] || all[rev] {
				shared[e], shared[rev] = true, true
			} else {
				all[e] = true
			}
		}
	}
	var per float64
	for e := range all {
		if !shared[e] {
			per += distv(pos[e[0]], pos[e[1]])
		}
	}
	return per
}
