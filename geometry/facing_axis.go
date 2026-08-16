package geometry

import "math"

// verticalCosLimit excludes roof and floor faces from the axis vote: a face
// whose normal is within 15° of vertical (|n.z| > cos 75°) is horizontal
// building fabric, not facade.
var verticalCosLimit = math.Cos(75 * math.Pi / 180)

// axisMergeCos is the angular tolerance for folding two face normals into one
// axis bucket, ~5°. Tessellation jitter and float32 vertex rounding move a
// nominally flat face's normals by well under this; a real chamfer or a curved
// wall segment moves them by more and correctly votes separately.
var axisMergeCos = math.Cos(5 * math.Pi / 180)

// axisDominanceMin is the share of vertical face area the winning axis must
// hold to be called a facing. A square column splits ~50/50 across two axes and
// has no facade, so declining is the correct answer rather than a coin flip.
const axisDominanceMin = 0.6

// axisBucket accumulates triangle area for one unsigned direction family.
type axisBucket struct {
	dir  v3
	area float64
}

// canonAxis folds a direction and its antipode onto a single representative, so
// a wall's two large opposite faces vote TOGETHER instead of cancelling. The
// first component whose magnitude clears the epsilon decides the sign.
//
// Folding is the whole point of the axis stage: summing signed normals makes
// the winner a floating-point accident, because the inner and outer face of a
// wall are equal in area and opposite in direction.
func canonAxis(n v3) v3 {
	const axisEps = 1e-9
	for i := 0; i < 3; i++ {
		if n[i] > axisEps {
			return n
		}
		if n[i] < -axisEps {
			return v3{-n[0], -n[1], -n[2]}
		}
	}
	return n
}

// dominantAxis returns the unsigned dominant direction of the vertical faces in
// the WORLD-space mesh (w, tris), the face area voting for it, and that area's
// share of all vertical face area.
//
// The returned direction is canonical, NOT outward: its sign carries no
// meaning. Resolving which of ±dir points away from the building is a separate
// question that needs the element's neighbours (see occupancy.go).
//
// ok=false means the element has no facade — no vertical faces at all, or no
// axis reaching axisDominanceMin.
func dominantAxis(w []v3, tris []uint32) (dir v3, area, share float64, ok bool) {
	var buckets []axisBucket
	var total float64

	for i := 0; i+2 < len(tris); i += 3 {
		ia, ib, ic := tris[i], tris[i+1], tris[i+2]
		if int(ia) >= len(w) || int(ib) >= len(w) || int(ic) >= len(w) {
			continue
		}
		a, b, c := w[ia], w[ib], w[ic]
		n := crossv(subv(b, a), subv(c, a))
		if !finite3(n) {
			continue
		}
		l := math.Sqrt(dotv(n, n))
		if l < 1e-15 {
			continue // degenerate triangle
		}
		u := scalev(n, 1/l)
		if math.Abs(u[2]) > verticalCosLimit {
			continue // roof or floor
		}
		triArea := l / 2
		total += triArea

		cu := canonAxis(u)
		merged := false
		// Iterate the slice, never a map: bucket identity must not depend on
		// iteration order or the returned normal stops being reproducible.
		for j := range buckets {
			if dotv(buckets[j].dir, cu) >= axisMergeCos {
				buckets[j].area += triArea
				merged = true
				break
			}
		}
		if !merged {
			buckets = append(buckets, axisBucket{dir: cu, area: triArea})
		}
	}

	if total == 0 || len(buckets) == 0 {
		return v3{}, 0, 0, false
	}

	best := 0
	for j := range buckets {
		// Strictly greater, so a tie keeps the earlier bucket.
		if buckets[j].area > buckets[best].area {
			best = j
		}
	}
	share = buckets[best].area / total
	if share < axisDominanceMin {
		return v3{}, 0, share, false
	}
	return buckets[best].dir, buckets[best].area, share, true
}
