package geometry

import (
	"errors"
	"math"
)

// BuildSlab builds a slab from a plan outline (meters, either winding, no
// holes): its top face is at z and it extends down by thickness. Quantities
// come from the numbers; the mesh is a closed shell that agrees with them.
// Invalid input returns an error and nothing else.
func BuildSlab(outline [][2]float64, z, thickness float64) (Slab, error) {
	if err := checkOutline(outline); err != nil {
		return Slab{}, err
	}
	if err := checkOrigin("z", z); err != nil {
		return Slab{}, err
	}
	if err := checkDim("thickness", thickness); err != nil {
		return Slab{}, err
	}
	ring := localRing(outline)
	cap, ok := triangulatePolygon(ring, newBudget())
	if !ok || len(cap) == 0 {
		return Slab{}, errors.New("outline could not be triangulated whole within the work budget")
	}

	n := uint32(len(ring))
	verts := make([]float32, 0, 6*n)
	for _, h := range []float64{0, -thickness} {
		for _, p := range ring {
			verts = append(verts, float32(p[0]), float32(p[1]), float32(h))
		}
	}
	// Ear clipping emits every triangle in one winding, so one sign orients
	// the whole cap: the largest triangle's. A small triangle's own sign
	// cannot: a straight-through point clips to a zero-area ear whose sign is
	// rounding noise.
	var big float64
	for i := 0; i+2 < len(cap); i += 3 {
		if c := cross2D(ring[cap[i]], ring[cap[i+1]], ring[cap[i+2]]); math.Abs(c) > math.Abs(big) {
			big = c
		}
	}
	ccw := polygonArea2D(ring) > 0
	tris := make([]uint32, 0, 2*len(cap)+6*int(n))
	for i := 0; i+2 < len(cap); i += 3 {
		a, b, c := cap[i], cap[i+1], cap[i+2]
		if big < 0 {
			b, c = c, b
		}
		tris = append(tris, a, b, c)       // top, facing +Z
		tris = append(tris, a+n, c+n, b+n) // bottom, facing -Z
	}
	for i := uint32(0); i < n; i++ {
		a, b := i, (i+1)%n
		if !ccw {
			a, b = b, a
		}
		tris = append(tris, a+n, b+n, b, a+n, b, a)
	}

	var perim float64
	for i := range ring {
		a, b := ring[i], ring[(i+1)%len(ring)]
		perim += math.Hypot(b[0]-a[0], b[1]-a[1])
	}
	area := math.Abs(polygonArea2D(ring))
	origin := [3]float64{outline[0][0], outline[0][1], z}
	return Slab{
		Element:    newElement(verts, tris, origin),
		Quantities: SlabQuantities{Area: area, Perimeter: perim, Volume: area * thickness},
		Top: Face{
			Origin:  origin,
			U:       [3]float64{1, 0, 0},
			V:       [3]float64{0, 1, 0},
			Normal:  [3]float64{0, 0, 1},
			Outline: ring,
		},
	}, nil
}
