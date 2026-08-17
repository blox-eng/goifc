package geometry

import (
	"math"
	"sort"
)

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

// sideAreaTol is the dot-product floor for counting a face as belonging to one
// side. A wall's end caps sit perpendicular to the facing direction at ~0 and
// are excluded; anything leaning even slightly toward it counts. It matches the
// tolerance IfcOpenShell's get_side_area applies to the same question.
const sideAreaTol = 0.01

// sideAreaDir sums the area of the triangles in the WORLD-space mesh (w, tris)
// whose normal points along dir, in m². It is sideArea generalized off the
// cardinal axes, and sideArea is now a thin wrapper over it.
//
// ONE side only. Unlike the axis vote it does NOT fold antipodes, so a
// free-standing wall reports its outer face rather than the sum of both. This
// is the facade quantity: summing it over the elements binned to one elevation
// gives that elevation's gross area.
//
// It reads triangle WINDING, which the axis vote deliberately does not. goifc
// orients its output outward — brep.go honours IfcFaceBound.Orientation and
// triangulate.go normalizes extrusion caps — but a source mesh wound inward
// throughout yields that element's INNER face instead, of equal magnitude on a
// plain wall and smaller on a stepped one. IfcOpenShell trusts winding here for
// the same reason: nothing else distinguishes the two faces of a wall.
func sideAreaDir(w []v3, tris []uint32, dir v3) float64 {
	l := math.Sqrt(dotv(dir, dir))
	if !(l > 0) {
		return 0
	}
	u := scalev(dir, 1/l)

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
		ln := math.Sqrt(dotv(n, n))
		if ln < 1e-15 {
			continue // degenerate triangle
		}
		if dotv(n, u)/ln <= sideAreaTol {
			continue // the far side, or edge-on
		}
		total += ln / 2
	}
	return total
}

// axisBucket accumulates triangle area for one unsigned direction family.
type axisBucket struct {
	dir  v3
	area float64
}

// canonAxis folds a direction and its antipode onto a single representative, so
// a wall's two large opposite faces vote TOGETHER instead of cancelling. The
// LARGEST-magnitude component decides the sign.
//
// Largest, not first: on a wall facing ±Y the X component is float32 vertex
// noise, and a rule that reads components in order lets that noise pick the
// sign. The wall's two faces then canonicalize to OPPOSITE representatives,
// land in antipodal buckets, split the vote below the dominance floor, and the
// element is silently reported as having no facade. The largest component is
// the one place jitter cannot reach — it is where the face actually points.
//
// Folding is the whole point of the axis stage: summing signed normals makes
// the winner a floating-point accident, because the inner and outer face of a
// wall are equal in area and opposite in direction.
func canonAxis(n v3) v3 {
	ax := 0
	for i := 1; i < 3; i++ {
		if math.Abs(n[i]) > math.Abs(n[ax]) {
			ax = i
		}
	}
	if n[ax] < 0 {
		return v3{-n[0], -n[1], -n[2]}
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
	var votes []axisBucket
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
		votes = append(votes, axisBucket{dir: canonAxis(u), area: triArea})
	}

	if total == 0 || len(votes) == 0 {
		return v3{}, 0, 0, false
	}

	// Sort the votes before bucketing them. Merging is greedy — a vote joins the
	// FIRST bucket within axisMergeCos — so where normals form a CHAIN finer
	// than the tolerance is wide (a curved wall, a faceted column: 0°, 4°, 8°
	// against a 5° tolerance) the bucket boundaries depend on which triangle
	// arrived first. Arriving 0,4,8 leaves two buckets and a 2:1 split; arriving
	// 4,0,8 merges all three into one. That moves share across
	// axisDominanceMin, so the same physical geometry could return a different
	// normal, or decline outright, purely on triangle order.
	//
	// Sorting on the canonical direction makes bucketing a function of the SET
	// of normals alone. It does not make the chain unambiguous — no
	// single-pass clustering can — it makes the answer reproducible.
	sort.Slice(votes, func(i, j int) bool {
		a, b := votes[i].dir, votes[j].dir
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		return a[2] < b[2]
	})

	var buckets []axisBucket
	// Iterate the slice, never a map: bucket identity must not depend on
	// iteration order or the returned normal stops being reproducible.
	for _, v := range votes {
		merged := false
		for j := range buckets {
			if dotv(buckets[j].dir, v.dir) >= axisMergeCos {
				buckets[j].area += v.area
				merged = true
				break
			}
		}
		if !merged {
			buckets = append(buckets, v)
		}
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
