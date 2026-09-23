package parity

import (
	"math"
	"testing"
)

func TestContains(t *testing.T) {
	inner := AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{1, 1, 1}}
	tests := []struct {
		name  string
		outer AABB
		want  bool
	}{
		{"identical", inner, true},
		{"strictly larger", AABB{Min: [3]float64{-1, -1, -1}, Max: [3]float64{2, 2, 2}}, true},
		{"short on max x", AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{0.5, 1, 1}}, false},
		{"short on min z", AABB{Min: [3]float64{0, 0, 0.5}, Max: [3]float64{1, 1, 1}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.outer, inner, 0); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tolerance absorbs float noise, not real under-reporting.
func TestContainsTolerance(t *testing.T) {
	inner := AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{1, 1, 1}}
	short := AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{1 - 1e-9, 1, 1}}
	if !Contains(short, inner, 1e-6) {
		t.Error("a 1 nm shortfall should pass a 1 um tolerance")
	}
	if Contains(short, inner, 0) {
		t.Error("a 1 nm shortfall should fail a zero tolerance")
	}
}

func TestVolume(t *testing.T) {
	if got := Volume(AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{2, 3, 4}}); got != 24 {
		t.Errorf("Volume() = %v, want 24", got)
	}
	// A planar element has zero extent on one axis.
	if got := Volume(AABB{Min: [3]float64{0, 0, 5}, Max: [3]float64{2, 3, 5}}); got != 0 {
		t.Errorf("Volume() of a planar box = %v, want 0", got)
	}
	// An inverted box is not negative volume.
	if got := Volume(AABB{Min: [3]float64{1, 1, 1}, Max: [3]float64{0, 0, 0}}); got != 0 {
		t.Errorf("Volume() of an inverted box = %v, want 0", got)
	}
	// A NaN extent must not propagate: one NaN ratio corrupts sort.Float64s
	// and every percentile the looseness table publishes.
	nan := AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{math.NaN(), 1, 1}}
	if got := Volume(nan); got != 0 {
		t.Errorf("Volume() of a NaN box = %v, want 0", got)
	}
}

// A non-finite bound compares false against every ordering test, so an
// unguarded Contains reports containment for a box that is not a box — Gate 1
// passing on exactly the elements it exists to catch.
func TestContainsRejectsNonFinite(t *testing.T) {
	real := AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{1, 1, 1}}
	huge := AABB{Min: [3]float64{-10, -10, -10}, Max: [3]float64{10, 10, 10}}
	bad := []struct {
		name string
		box  AABB
	}{
		{"NaN min", AABB{Min: [3]float64{math.NaN(), 0, 0}, Max: [3]float64{1, 1, 1}}},
		{"NaN max", AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{math.NaN(), 1, 1}}},
		{"+Inf max", AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{math.Inf(1), 1, 1}}},
		{"-Inf min", AABB{Min: [3]float64{math.Inf(-1), 0, 0}, Max: [3]float64{1, 1, 1}}},
	}
	for _, tt := range bad {
		t.Run(tt.name+" as outer", func(t *testing.T) {
			if Contains(tt.box, real, Tolerance) {
				t.Error("a non-finite goifc box must never satisfy containment")
			}
			if Finite(tt.box) {
				t.Error("Finite() must reject this box")
			}
		})
		t.Run(tt.name+" as inner", func(t *testing.T) {
			if Contains(huge, tt.box, Tolerance) {
				t.Error("a non-finite oracle box must never satisfy containment")
			}
		})
	}
	if !Finite(real) {
		t.Error("Finite() must accept a real box")
	}
}
