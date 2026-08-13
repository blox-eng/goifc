package geometry

import (
	"math"
	"reflect"
	"testing"
)

func TestHorizontalPlaneIsRightHanded(t *testing.T) {
	p := HorizontalPlane(2.5)
	if !p.Valid() {
		t.Fatalf("HorizontalPlane must be a valid right-handed basis: %+v", p)
	}
	if p.Origin != [3]float64{0, 0, 2.5} {
		t.Fatalf("origin = %v, want {0,0,2.5}", p.Origin)
	}
	if p.U != [3]float64{1, 0, 0} || p.V != [3]float64{0, 1, 0} || p.N != [3]float64{0, 0, 1} {
		t.Fatalf("basis = %v/%v/%v, want +X/+Y/+Z", p.U, p.V, p.N)
	}
}

func TestPlaneValidRejectsLeftHanded(t *testing.T) {
	// U=+X, V=+Y, N=-Z reads as a reasonable "look up instead of down" plane and
	// is left-handed: it would silently swap outer rings and holes.
	p := Plane{Origin: [3]float64{0, 0, 0}, U: [3]float64{1, 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, -1}}
	if p.Valid() {
		t.Fatal("left-handed basis must be rejected")
	}
}

func TestPlaneValidRejectsNonOrthonormal(t *testing.T) {
	cases := map[string]Plane{
		"non-unit U":        {U: [3]float64{2, 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, 1}},
		"non-perpendicular": {U: [3]float64{1, 0, 0}, V: [3]float64{1, 1, 0}, N: [3]float64{0, 0, 1}},
		"zero N":            {U: [3]float64{1, 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, 0}},
		"NaN origin":        {Origin: [3]float64{math.NaN(), 0, 0}, U: [3]float64{1, 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, 1}},
		// A NaN component makes the ADJACENT unit-length check useless — every
		// comparison against NaN is false — so finite3 is the only line standing
		// between a NaN basis and NaN ring coordinates.
		"NaN in U": {U: [3]float64{math.NaN(), 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, 1}},
		"NaN in N": {U: [3]float64{1, 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, math.NaN()}},
		"Inf in V": {U: [3]float64{1, 0, 0}, V: [3]float64{0, math.Inf(1), 0}, N: [3]float64{0, 0, 1}},
	}
	for name, p := range cases {
		if p.Valid() {
			t.Errorf("%s: must be rejected", name)
		}
	}
}

func TestPlaneFromNormalProducesValidBasis(t *testing.T) {
	for _, n := range [][3]float64{
		{0, 0, 1}, {1, 0, 0}, {0, 1, 0}, {0, 0, -1}, {-1, 0, 0},
		{1, 1, 0}, {1, 2, 3}, {0.001, 1, 0},
	} {
		p, ok := PlaneFromNormal([3]float64{1, 2, 3}, n)
		if !ok {
			t.Fatalf("n=%v: want ok", n)
		}
		if !p.Valid() {
			t.Fatalf("n=%v: basis invalid: %+v", n, p)
		}
		want := normv(n)
		for i := 0; i < 3; i++ {
			if math.Abs(p.N[i]-want[i]) > 1e-12 {
				t.Fatalf("n=%v: N=%v, want normalized %v", n, p.N, want)
			}
		}
		if p.Origin != [3]float64{1, 2, 3} {
			t.Fatalf("n=%v: origin not preserved: %v", n, p.Origin)
		}
	}
}

func TestPlaneFromNormalRejectsDegenerate(t *testing.T) {
	for _, n := range [][3]float64{
		{0, 0, 0}, {math.NaN(), 0, 1}, {math.Inf(1), 0, 0},
	} {
		if _, ok := PlaneFromNormal([3]float64{0, 0, 0}, n); ok {
			t.Errorf("n=%v: want ok=false", n)
		}
	}
	if _, ok := PlaneFromNormal([3]float64{math.NaN(), 0, 0}, [3]float64{0, 0, 1}); ok {
		t.Error("non-finite origin: want ok=false")
	}
}

func TestPlaneFromNormalDeterministic(t *testing.T) {
	a, _ := PlaneFromNormal([3]float64{0, 0, 0}, [3]float64{1, 2, 3})
	b, _ := PlaneFromNormal([3]float64{0, 0, 0}, [3]float64{1, 2, 3})
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("not deterministic: %+v vs %+v", a, b)
	}
}

func TestProjectUVAndSignedDist(t *testing.T) {
	p := HorizontalPlane(2)
	if got := signedDist(p, v3{5, 6, 7}); math.Abs(got-5) > 1e-12 {
		t.Fatalf("signedDist = %v, want 5", got)
	}
	if got := projectUV(p, v3{5, 6, 7}); got != [2]float64{5, 6} {
		t.Fatalf("projectUV = %v, want {5,6}", got)
	}
}

// UV is plane-LOCAL, not world-absolute. Every other plane in this suite has
// zero in-plane origin components (HorizontalPlane's origin is {0,0,cutZ}), so
// dropping the origin subtraction in projectUV is invisible to them; this plane
// is offset along U and V so it is not.
func TestProjectUVIsOriginRelative(t *testing.T) {
	p := Plane{Origin: [3]float64{1, 2, 0}, U: [3]float64{1, 0, 0}, V: [3]float64{0, 1, 0}, N: [3]float64{0, 0, 1}}
	if !p.Valid() {
		t.Fatal("fixture plane must be valid")
	}
	if got := projectUV(p, v3{5, 6, 7}); got != [2]float64{4, 4} {
		t.Fatalf("projectUV = %v, want {4,4}; {5,6} means the origin was not subtracted", got)
	}
}

// A finite n whose squared length overflows leaves the normalized vector zero,
// which cleared the length gate and returned an all-zero basis with ok=true.
// ok=true must imply Valid, or a caller who checks only ok gets silent nil rings.
func TestPlaneFromNormalRejectsOverflowingNormal(t *testing.T) {
	for _, n := range [][3]float64{
		{1e200, 1e200, 1e200}, {1e200, 0, 0}, {0, 1e300, 1e300},
	} {
		p, ok := PlaneFromNormal([3]float64{0, 0, 0}, n)
		if ok && !p.Valid() {
			t.Errorf("n=%v: ok=true but basis invalid: %+v", n, p)
		}
	}
}
