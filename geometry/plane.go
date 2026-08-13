package geometry

import "math"

// planeBasisEps is the tolerance for the orthonormality checks in Plane.Valid.
// Generous on purpose: the failure this guard exists to stop is a LEFT-handed
// basis silently swapping outer rings and holes, and that is caught by a sign
// test which no magnitude tolerance can weaken. Meanwhile a basis hand-built
// from single-precision directions lands around 1e-7, and rejecting it would
// surface as an empty drawing indistinguishable from a plane that missed.
const planeBasisEps = 1e-6

// Plane is a cutting plane: a point on it plus an orthonormal basis. U and V
// span the plane and define the 2D coordinates of emitted rings; N is the
// normal.
//
// The basis MUST be orthonormal and RIGHT-HANDED (N == U x V). This is
// load-bearing, not decorative: ring winding, hole classification via
// nestEvenOdd, and representativePoint's interior nudge all assume it. A
// left-handed basis silently swaps outer rings and holes rather than failing,
// so SectionOn and FootprintOn validate it and emit no rings when it is wrong.
// Prefer PlaneFromNormal over hand-building a basis.
type Plane struct {
	Origin  [3]float64
	U, V, N [3]float64
}

// HorizontalPlane returns the z = cutZ plane with U=+X, V=+Y, N=+Z — the plane
// Footprint cuts.
func HorizontalPlane(cutZ float64) Plane {
	return Plane{
		Origin: [3]float64{0, 0, cutZ},
		U:      [3]float64{1, 0, 0},
		V:      [3]float64{0, 1, 0},
		N:      [3]float64{0, 0, 1},
	}
}

// PlaneFromNormal derives a right-handed orthonormal basis for the plane through
// origin with normal n. U is seeded from the world axis least parallel to n, so
// the choice is deterministic and never degenerate. Returns ok=false for a
// non-finite origin, or an n that is non-finite, shorter than 1e-12, or large
// enough that normalizing it overflows.
//
// ok=true guarantees Valid reports true for the returned plane, so a caller
// that checks ok need not check Valid as well.
//
// The resulting U (and therefore V) is deterministic for a given n but
// otherwise UNSPECIFIED: it is NOT guaranteed to match HorizontalPlane's frame
// for a +Z normal, or any other particular in-plane orientation. It may also
// change between versions of this package. A caller that needs a specific U/V
// orientation (e.g. to match HorizontalPlane, or an external convention) must
// build the Plane itself rather than rely on this function's choice.
func PlaneFromNormal(origin, n [3]float64) (Plane, bool) {
	if !finite3(origin) || !finite3(n) {
		return Plane{}, false
	}
	l := math.Sqrt(dotv(n, n))
	if l < 1e-12 {
		return Plane{}, false
	}
	nn := v3{n[0] / l, n[1] / l, n[2] / l}

	// Seed with the world axis least parallel to nn; ties resolve to the lowest
	// index, so the result never depends on iteration order.
	ax := 0
	for i := 1; i < 3; i++ {
		if math.Abs(nn[i]) < math.Abs(nn[ax]) {
			ax = i
		}
	}
	var seed v3
	seed[ax] = 1

	u := normv(crossv(seed, nn))
	// v = n x u completes a right-handed frame: u x v == u x (n x u) == n,
	// since u is unit and perpendicular to n.
	v := crossv(nn, u)
	p := Plane{Origin: origin, U: u, V: v, N: nn}
	// dot(n,n) overflows to +Inf for a huge-but-finite n, which clears the length
	// gate above and leaves nn the zero vector. Rather than enumerate every such
	// case, confirm the basis actually built: ok=true has to mean the plane is
	// usable, or a caller who checks only ok gets silently empty rings later.
	if !p.Valid() {
		return Plane{}, false
	}
	return p, true
}

// Valid reports whether p's basis is finite, orthonormal to planeBasisEps, and
// right-handed (N == U x V). Everything downstream of the projection assumes
// all three. SectionOn and FootprintOn return no rings when a plane fails this
// check, so a caller that hand-builds a Plane can call Valid up front to tell
// a bad basis apart from a plane that genuinely missed the mesh.
func (p Plane) Valid() bool {
	if !finite3(p.Origin) {
		return false
	}
	for _, a := range [3]v3{p.U, p.V, p.N} {
		if !finite3(a) || math.Abs(dotv(a, a)-1) > planeBasisEps {
			return false
		}
	}
	if math.Abs(dotv(p.U, p.V)) > planeBasisEps ||
		math.Abs(dotv(p.U, p.N)) > planeBasisEps ||
		math.Abs(dotv(p.V, p.N)) > planeBasisEps {
		return false
	}
	return dotv(p.N, crossv(p.U, p.V)) > 0
}

// projectUV returns q's coordinates in p's UV frame, meters.
func projectUV(p Plane, q v3) [2]float64 {
	d := subv(q, p.Origin)
	return [2]float64{dotv(d, p.U), dotv(d, p.V)}
}

// signedDist returns q's signed distance from p along p.N, meters. This is the
// generalization of the old "vertex z minus cut height" term.
func signedDist(p Plane, q v3) float64 { return dotv(subv(q, p.Origin), p.N) }

// finite3 reports whether every component of a is finite (no NaN, no Inf).
func finite3(a v3) bool {
	for _, c := range a {
		if math.IsNaN(c) || math.IsInf(c, 0) {
			return false
		}
	}
	return true
}
