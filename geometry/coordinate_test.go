package geometry

import (
	"math"
	"testing"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

func buildOne(t *testing.T, path string) Element {
	t.Helper()
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	r, err := model.Extract(f)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	s, err := Build(f, r)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(s.Elements) != 1 {
		t.Fatalf("want 1 element, got %d", len(s.Elements))
	}
	return s.Elements[0]
}

func closeVec(a, b [3]float64, eps float64) bool {
	for i := range a {
		if math.Abs(a[i]-b[i]) > eps {
			return false
		}
	}
	return true
}

func TestGate0_KnownWorldBox_Meters(t *testing.T) {
	e := buildOne(t, "testdata/synthetic/known_box.ifc")
	wantMin, wantMax := [3]float64{10, 20, 5}, [3]float64{11, 21, 6}
	if !closeVec(e.BBoxMin, wantMin, 1e-6) || !closeVec(e.BBoxMax, wantMax, 1e-6) {
		t.Errorf("world AABB = %v..%v, want %v..%v", e.BBoxMin, e.BBoxMax, wantMin, wantMax)
	}
}

func TestGate0_KnownWorldBox_Millimeters(t *testing.T) {
	e := buildOne(t, "testdata/synthetic/known_box_mm.ifc")
	wantMin, wantMax := [3]float64{10, 20, 5}, [3]float64{11, 21, 6}
	if !closeVec(e.BBoxMin, wantMin, 1e-6) || !closeVec(e.BBoxMax, wantMax, 1e-6) {
		t.Errorf("mm world AABB = %v..%v, want %v..%v (unit scaling wrong)", e.BBoxMin, e.BBoxMax, wantMin, wantMax)
	}
}
