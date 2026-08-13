package geometry

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// rotZ builds a column-major placement: rotation of deg about Z, then
// translation by tx,ty,tz.
func rotZ(deg float64, tx, ty, tz float64) model.Mat4 {
	c, s := math.Cos(deg*math.Pi/180), math.Sin(deg*math.Pi/180)
	m := model.Identity()
	m[0], m[1] = c, s  // column 0 = rotated X axis
	m[4], m[5] = -s, c // column 1 = rotated Y axis
	m[12], m[13], m[14] = tx, ty, tz
	return m
}

func closeF32(a, b float32) bool {
	d := math.Abs(float64(a) - float64(b))
	return d <= 1e-6*math.Max(1, math.Abs(float64(a)))
}

func TestWorldVerts_KnownTransform(t *testing.T) {
	// Unit-square corners, rotated 90° about Z then translated by (10,20,5).
	// (x,y) -> (-y,x), so (1,0) -> (0,1) and (0,1) -> (-1,0).
	e := Element{
		Verts:     []float32{0, 0, 0, 1, 0, 0, 0, 1, 0, 1, 1, 2},
		Placement: rotZ(90, 10, 20, 5),
	}
	want := []float32{10, 20, 5, 10, 21, 5, 9, 20, 5, 9, 21, 7}
	got := e.WorldVerts()
	if len(got) != len(want) {
		t.Fatalf("WorldVerts len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !closeF32(got[i], want[i]) {
			t.Errorf("WorldVerts[%d] = %v, want %v (full: %v)", i, got[i], want[i], got)
			break
		}
	}
}

func TestWorldVerts_IdentityIsUnchanged(t *testing.T) {
	in := []float32{1, 2, 3, -4, 5.5, 6}
	e := Element{Verts: in, Placement: model.Identity()}
	got := e.WorldVerts()
	for i := range in {
		if !closeF32(got[i], in[i]) {
			t.Fatalf("identity placement changed verts: %v -> %v", in, got)
		}
	}
	if &got[0] == &in[0] {
		t.Error("WorldVerts returned the caller's slice; it must allocate")
	}
}

func TestWorldVerts_EmptyIsEmptyNotPanic(t *testing.T) {
	if got := (Element{Placement: rotZ(30, 1, 2, 3)}).WorldVerts(); len(got) != 0 {
		t.Errorf("WorldVerts of empty Verts = %v, want empty", got)
	}
	if got := (Element{Verts: []float32{}}).WorldVerts(); len(got) != 0 {
		t.Errorf("WorldVerts of zero-length Verts = %v, want empty", got)
	}
	// A partial trailing triple is dropped, not emitted half-transformed with a
	// fabricated zero component.
	e := Element{Verts: []float32{1, 2, 3, 9, 9}, Placement: model.Identity()}
	if got := e.WorldVerts(); len(got) != 3 {
		t.Errorf("WorldVerts of a partial trailing triple = %v, want 3 floats", got)
	}
	if got := (Element{Verts: []float32{1, 2}}).WorldVerts(); len(got) != 0 {
		t.Errorf("WorldVerts of a sub-triple slice = %v, want empty", got)
	}
}

func TestWorldNormal_IgnoresTranslation(t *testing.T) {
	local := [3]float64{0, 1, 0}
	base := Element{Placement: rotZ(0, 0, 0, 0)}
	moved := Element{Placement: rotZ(0, 1234, -56, 7.89)}
	if base.WorldNormal(local) != moved.WorldNormal(local) {
		t.Errorf("translation changed the normal: %v vs %v", base.WorldNormal(local), moved.WorldNormal(local))
	}
	if got := moved.WorldNormal(local); !closeVec(got, [3]float64{0, 1, 0}, 1e-12) {
		t.Errorf("WorldNormal under pure translation = %v, want %v", got, local)
	}
}

func TestWorldNormal_RotatesByExactly90(t *testing.T) {
	e := Element{Placement: rotZ(90, 500, -500, 3)}
	// +X local becomes +Y world; +Y local becomes -X world.
	if got := e.WorldNormal([3]float64{1, 0, 0}); !closeVec(got, [3]float64{0, 1, 0}, 1e-12) {
		t.Errorf("WorldNormal(+X) = %v, want (0,1,0)", got)
	}
	if got := e.WorldNormal([3]float64{0, 1, 0}); !closeVec(got, [3]float64{-1, 0, 0}, 1e-12) {
		t.Errorf("WorldNormal(+Y) = %v, want (-1,0,0)", got)
	}
	// Z is the rotation axis and is untouched.
	if got := e.WorldNormal([3]float64{0, 0, 1}); !closeVec(got, [3]float64{0, 0, 1}, 1e-12) {
		t.Errorf("WorldNormal(+Z) = %v, want (0,0,1)", got)
	}
}

func TestWorldNormal_PreservesMagnitude(t *testing.T) {
	e := Element{Placement: rotZ(37, 9, 9, 9)}
	unit := e.WorldNormal([3]float64{1, 0, 0})
	scaled := e.WorldNormal([3]float64{3, 0, 0})
	for i := range unit {
		if math.Abs(scaled[i]-3*unit[i]) > 1e-12 {
			t.Fatalf("scaled direction = %v, want 3x %v", scaled, unit)
		}
	}
	if n := math.Sqrt(unit[0]*unit[0] + unit[1]*unit[1] + unit[2]*unit[2]); math.Abs(n-1) > 1e-12 {
		t.Errorf("unit local direction returned length %v, want 1 (placement should be orthonormal)", n)
	}
}

// TestWorldVerts_AABBEqualsBBox is the invariant this whole frame distinction
// exists for: BBoxMin/BBoxMax are the world AABB, so recomputing that AABB
// from WorldVerts() must reproduce them — for every geometry source. If
// WorldVerts ever lost the placement (or BBox stopped being world), the two
// diverge here instead of silently downstream.
func TestWorldVerts_AABBEqualsBBox(t *testing.T) {
	paths, err := filepath.Glob("testdata/synthetic/*.ifc")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "../testdata/two_storey_spanning.ifc")

	seen := map[GeomSource]int{}
	rotated := 0
	for _, path := range paths {
		f, err := step.ParseFile(path)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		r, err := model.Extract(f)
		if err != nil {
			t.Fatalf("extract %s: %v", path, err)
		}
		s, err := Build(f, r)
		if err != nil {
			t.Fatalf("build %s: %v", path, err)
		}
		for _, e := range s.Elements {
			if len(e.Verts) == 0 {
				continue
			}
			seen[e.Source]++
			if isRotated(e.Placement) {
				rotated++
			}
			min, max := aabbOf(e.WorldVerts())
			if !closeVecRel(min, e.BBoxMin) || !closeVecRel(max, e.BBoxMax) {
				t.Errorf("%s %s (%s): AABB(WorldVerts) = %v..%v, BBox = %v..%v",
					filepath.Base(path), e.GlobalID, e.Source, min, max, e.BBoxMin, e.BBoxMax)
			}
		}
	}

	for _, src := range []GeomSource{SourceExtrude, SourceBrep, SourceOBB} {
		if seen[src] == 0 {
			t.Errorf("corpus exercised no %s elements — the invariant is untested for that source", src)
		}
	}
	// A translation-only placement is not enough: it commutes with the AABB, so
	// the invariant would hold even if WorldVerts dropped the rotation. Only a
	// rotated placement makes this test able to fail.
	if rotated == 0 {
		t.Error("corpus has no ROTATED placement — the invariant holds vacuously")
	}
}

// isRotated reports whether m's 3x3 part differs from identity, i.e. whether
// the placement does anything a bare translation would not.
func isRotated(m model.Mat4) bool {
	id := model.Identity()
	for _, i := range []int{0, 1, 2, 4, 5, 6, 8, 9, 10} {
		if math.Abs(m[i]-id[i]) > 1e-12 {
			return true
		}
	}
	return false
}

// closeVecRel compares world coordinates with a tolerance that scales with
// magnitude. WorldVerts rounds to float32, whose quantum grows with distance
// from the origin (~6e-5 m at 1 km, ~8e-3 m at 100 km), and georeferenced IFC
// models routinely sit at such offsets. A fixed absolute epsilon would make
// the next real-world fixture fail for a rounding reason that has nothing to
// do with the frame invariant this test guards.
func closeVecRel(a, b [3]float64) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-6*math.Max(1, math.Abs(b[i])) {
			return false
		}
	}
	return true
}

// TestWorldNormal_ParsedPlacement ties WorldNormal to a placement the PARSER
// produced, not a hand-built matrix: obb_revolved.ifc is placed at 45 degrees
// about Z, so a local +X must come back on the diagonal, still unit-length.
func TestWorldNormal_ParsedPlacement(t *testing.T) {
	e := buildOne(t, "testdata/synthetic/obb_revolved.ifc")
	if !isRotated(e.Placement) {
		t.Fatalf("fixture placement is not rotated: %v", e.Placement)
	}
	h := math.Sqrt2 / 2
	if got := e.WorldNormal([3]float64{1, 0, 0}); !closeVec(got, [3]float64{h, h, 0}, 1e-9) {
		t.Errorf("WorldNormal(+X) = %v, want (%v,%v,0)", got, h, h)
	}
	if got := e.WorldNormal([3]float64{0, 1, 0}); !closeVec(got, [3]float64{-h, h, 0}, 1e-9) {
		t.Errorf("WorldNormal(+Y) = %v, want (%v,%v,0)", got, -h, h)
	}
	n := e.WorldNormal([3]float64{1, 0, 0})
	if l := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2]); math.Abs(l-1) > 1e-9 {
		t.Errorf("parsed placement is not orthonormal: |WorldNormal(+X)| = %v, want 1", l)
	}
}

func aabbOf(verts []float32) (min, max [3]float64) {
	min = [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	max = [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i := 0; i+2 < len(verts); i += 3 {
		for k := 0; k < 3; k++ {
			v := float64(verts[i+k])
			min[k] = math.Min(min[k], v)
			max[k] = math.Max(max[k], v)
		}
	}
	return min, max
}
