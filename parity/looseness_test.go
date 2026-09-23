package parity

import (
	"math"
	"testing"
)

// A planar oracle box has zero volume, so a naive ratio is +Inf and poisons
// every percentile after it. Such elements are counted, not divided by.
func TestRatiosExcludeDegenerateOracleBoxes(t *testing.T) {
	unit := AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{1, 1, 1}}
	planar := AABB{Min: [3]float64{0, 0, 5}, Max: [3]float64{2, 3, 5}}
	pairs := []boxPair{
		{Got: unit, Want: unit},   // ratio 1
		{Got: unit, Want: planar}, // degenerate, skipped
	}

	l := summarize(pairs)
	if l.Compared != 1 {
		t.Errorf("Compared = %d, want 1", l.Compared)
	}
	if l.Degenerate != 1 {
		t.Errorf("Degenerate = %d, want 1", l.Degenerate)
	}
	for _, v := range []float64{l.P50, l.P90, l.Max} {
		if math.IsInf(v, 0) || math.IsNaN(v) {
			t.Errorf("percentile is %v; a degenerate box leaked into the distribution", v)
		}
	}
	if l.Max != 1 {
		t.Errorf("Max = %v, want 1", l.Max)
	}
}

func TestRatiosPercentiles(t *testing.T) {
	unit := AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{1, 1, 1}}
	box := func(s float64) AABB {
		return AABB{Min: [3]float64{0, 0, 0}, Max: [3]float64{s, s, s}}
	}
	// ratios: 1, 8, 27, 64 (cubes of 1..4)
	pairs := []boxPair{
		{Got: box(1), Want: unit},
		{Got: box(2), Want: unit},
		{Got: box(3), Want: unit},
		{Got: box(4), Want: unit},
	}
	l := summarize(pairs)
	if l.Compared != 4 {
		t.Fatalf("Compared = %d, want 4", l.Compared)
	}
	if l.Max != 64 {
		t.Errorf("Max = %v, want 64", l.Max)
	}
	if l.P50 < 8 || l.P50 > 27 {
		t.Errorf("P50 = %v, want between 8 and 27", l.P50)
	}
}
