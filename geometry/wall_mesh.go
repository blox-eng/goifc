package geometry

import (
	"math"
	"slices"
)

// gridSnap merges grid values closer than this, and snaps a mitre end this
// close to the measured face's end onto it. It must exceed the float32 step of
// a local vertex (6e-5 m at the 1,000 m bound), or two grid lines round onto
// one vertex and leave degenerate triangles. Opening edges are at least minDim
// (1 mm) from each other and from the ends, so only rounding noise merges.
const gridSnap = 1e-4

// meshWall meshes a wall in its own frame (u along the edge, w through the
// thickness from the measured face at w = 0, v up) as a closed shell wound
// outward. It returns (u, w, v) triples and triangle indices.
//
// Every face is cut on one global grid: U holds the face ends and every
// opening's u edges, V the base, top and every opening's v edges. A vertex is
// a grid crossing on the measured or the opposite side, so two faces that
// meet share every vertex along their common edge, and no vertex sits inside
// a neighbour's edge. Openings are checked first (checkOpenings), so each
// lies inside both faces, at least minDim from every end and from each other.
func meshWall(span wallSpan, height, thickness float64, ops []Opening) (pts [][3]float64, tris []uint32) {
	L, s, e, t := span.length, span.otherStart, span.otherEnd, thickness
	us := []float64{0, L, s, e}
	vs := []float64{0, height}
	for _, o := range ops {
		us = append(us, o.Offset, o.Offset+o.Width)
		vs = append(vs, o.Sill, o.Sill+o.Height)
	}
	U, V := snapSorted(us), snapSorted(vs)
	iu := func(x float64) int { return nearestIndex(U, x) }
	iv := func(x float64) int { return nearestIndex(V, x) }
	i0, iL, is, ie := iu(0), iu(L), iu(s), iu(e)
	top, bottom := len(V)-1, 0

	ids := map[[3]int]uint32{}
	vert := func(i, j, side int) uint32 {
		k := [3]int{i, j, side}
		if id, ok := ids[k]; ok {
			return id
		}
		id := uint32(len(pts))
		pts = append(pts, [3]float64{U[i], float64(side) * t, V[j]})
		ids[k] = id
		return id
	}
	poly := func(p ...uint32) { // a convex fan, in the given winding
		for k := 1; k+1 < len(p); k++ {
			tris = append(tris, p[0], p[k], p[k+1])
		}
	}
	inOpening := func(u, v float64) bool {
		for _, o := range ops {
			if u > o.Offset && u < o.Offset+o.Width && v > o.Sill && v < o.Sill+o.Height {
				return true
			}
		}
		return false
	}

	// The two faces: measured (w = 0, facing -w) and opposite (w = t, +w).
	for i := 0; i+1 < len(U); i++ {
		for j := 0; j+1 < len(V); j++ {
			if inOpening((U[i]+U[i+1])/2, (V[j]+V[j+1])/2) {
				continue
			}
			if i >= i0 && i+1 <= iL {
				poly(vert(i, j, 0), vert(i+1, j, 0), vert(i+1, j+1, 0), vert(i, j+1, 0))
			}
			if i >= is && i+1 <= ie {
				poly(vert(i, j, 1), vert(i, j+1, 1), vert(i+1, j+1, 1), vert(i+1, j, 1))
			}
		}
	}

	// Reveals: jambs face into the opening along u; the head faces down and
	// the sill up. A door (Sill == 0) has no sill strip.
	for _, o := range ops {
		a, b := iu(o.Offset), iu(o.Offset+o.Width)
		lo, hi := iv(o.Sill), iv(o.Sill+o.Height)
		for j := lo; j < hi; j++ {
			poly(vert(a, j, 0), vert(a, j, 1), vert(a, j+1, 1), vert(a, j+1, 0))
			poly(vert(b, j, 0), vert(b, j+1, 0), vert(b, j+1, 1), vert(b, j, 1))
		}
		for i := a; i < b; i++ {
			poly(vert(i, hi, 0), vert(i, hi, 1), vert(i+1, hi, 1), vert(i+1, hi, 0))
			if o.Sill > 0 {
				poly(vert(i, lo, 0), vert(i+1, lo, 0), vert(i+1, lo, 1), vert(i, lo, 1))
			}
		}
	}

	// Top (+v) and bottom (-v): the wall's plan is a trapezoid, its two
	// parallel sides the faces. Zip the grid points along the measured side
	// (w = 0) and the opposite side (w = t) into triangles. The bottom is cut
	// at every door, which has no bottom strip.
	cuts := []float64{math.Inf(-1)}
	for _, o := range ops {
		if o.Sill == 0 {
			cuts = append(cuts, o.Offset, o.Offset+o.Width)
		}
	}
	cuts = append(cuts, math.Inf(1))
	slices.Sort(cuts[1 : len(cuts)-1])
	zip(U, i0, iL, is, ie, func(a, b, c [2]int) { // top: (u, w) counter-clockwise faces +v
		tris = append(tris, vert(a[0], top, a[1]), vert(b[0], top, b[1]), vert(c[0], top, c[1]))
	})
	for k := 0; k+1 < len(cuts); k += 2 {
		lo, hi := cuts[k], cuts[k+1]
		clip := func(from, to int) (int, int) {
			for from < to && U[from] < lo-gridSnap {
				from++
			}
			for to > from && U[to] > hi+gridSnap {
				to--
			}
			return from, to
		}
		m0, m1 := clip(i0, iL)
		o0, o1 := clip(is, ie)
		zip(U, m0, m1, o0, o1, func(a, b, c [2]int) {
			tris = append(tris, vert(a[0], bottom, a[1]), vert(c[0], bottom, c[1]), vert(b[0], bottom, b[1]))
		})
	}

	// End caps: from the measured face's end to the opposite face's, full
	// height, cut at every V line both faces carry there.
	for j := 0; j+1 < len(V); j++ {
		poly(vert(i0, j, 0), vert(i0, j+1, 0), vert(is, j+1, 1), vert(is, j, 1))
		poly(vert(iL, j, 0), vert(ie, j, 1), vert(ie, j+1, 1), vert(iL, j+1, 0))
	}
	return pts, tris
}

// snapSorted sorts xs and drops values within gridSnap of the one before.
func snapSorted(xs []float64) []float64 {
	slices.Sort(xs)
	out := xs[:1]
	for _, x := range xs[1:] {
		if x-out[len(out)-1] > gridSnap {
			out = append(out, x)
		}
	}
	return out
}

// nearestIndex returns the index of the grid value nearest x.
func nearestIndex(grid []float64, x float64) int {
	i, _ := slices.BinarySearch(grid, x)
	switch {
	case i == len(grid):
		return i - 1
	case i > 0 && math.Abs(grid[i-1]-x) < math.Abs(grid[i]-x):
		return i - 1
	}
	return i
}

// placeWall maps wall-frame points into element-local coordinates (relative
// to the edge start, z up from the base) and keeps the winding outward when
// (u, g, z) is a left-handed frame.
func placeWall(f wallFrame, pts [][3]float64, tris []uint32) ([]float32, []uint32) {
	verts := make([]float32, 0, 3*len(pts))
	for _, p := range pts {
		x := p[0]*f.u[0] + p[1]*f.g[0]
		y := p[0]*f.u[1] + p[1]*f.g[1]
		verts = append(verts, float32(x), float32(y), float32(p[2]))
	}
	if f.u[0]*f.g[1]-f.u[1]*f.g[0] < 0 {
		for k := 0; k+2 < len(tris); k += 3 {
			tris[k+1], tris[k+2] = tris[k+2], tris[k+1]
		}
	}
	return verts, tris
}

// zip triangulates the convex region between two parallel chains of grid
// points: the measured side U[m0..m1] at w = 0 and the opposite side
// U[o0..o1] at w = t. It walks both chains in u, always advancing the one
// whose next point comes first, and emits each triangle counter-clockwise in
// (u, w) as pairs (grid index, side).
func zip(U []float64, m0, m1, o0, o1 int, emit func(a, b, c [2]int)) {
	i, j := m0, o0
	for i < m1 || j < o1 {
		if j == o1 || (i < m1 && U[i+1] <= U[j+1]) {
			emit([2]int{i, 0}, [2]int{i + 1, 0}, [2]int{j, 1})
			i++
		} else {
			emit([2]int{i, 0}, [2]int{j + 1, 1}, [2]int{j, 1})
			j++
		}
	}
}
