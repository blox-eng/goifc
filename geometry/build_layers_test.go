package geometry

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

var square4 = [][2]float64{{0, 0}, {4, 0}, {4, 4}, {0, 4}}

func TestLayersInputErrors(t *testing.T) {
	seventeen := make([]float64, 17)
	for i := range seventeen {
		seventeen[i] = 0.01
	}
	for _, tc := range []struct {
		name   string
		layers []float64
		want   string
	}{
		{"nil", nil, "no layers"},
		{"empty", []float64{}, "no layers"},
		{"too many", seventeen, "16"},
		{"thin layer among thick", []float64{0.2, 0.0005, 0.2}, "layer 1"},
		{"NaN", []float64{0.2, math.NaN()}, "layer 1"},
		{"Inf", []float64{math.Inf(1)}, "layer 0"},
		{"sum too thick", []float64{600, 600}, "thickness"},
	} {
		_, err := BuildSlabLayers(square4, 3.0, tc.layers)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
}

func TestBuildSlabLayersOracle(t *testing.T) {
	slabs, err := BuildSlabLayers(square4, 3.0, []float64{0.05, 0.20})
	if err != nil {
		t.Fatal(err)
	}
	if len(slabs) != 2 {
		t.Fatalf("got %d slabs, want 2", len(slabs))
	}
	for k, want := range []struct{ vol, top float64 }{{0.8, 3.0}, {3.2, 2.95}} {
		s := slabs[k]
		q := s.Quantities
		if !near(q.Area, 16, 1e-9) || !near(q.Perimeter, 16, 1e-9) || !near(q.Volume, want.vol, 1e-9) {
			t.Errorf("layer %d: quantities %+v, want area 16, perimeter 16, volume %v", k, q, want.vol)
		}
		if !near(s.Top.Origin[2], want.top, 1e-12) {
			t.Errorf("layer %d: top at z %v, want %v", k, s.Top.Origin[2], want.top)
		}
		th := []float64{0.05, 0.20}[k]
		meshAgrees(t, s.Element, q.Volume, 2*q.Area+q.Perimeter*th, 0, q.Area, q.Perimeter, q.Volume)
	}
}

func TestBuildSlabLayers_OneLayerIsBuildSlab(t *testing.T) {
	want, err := BuildSlab(square4, 3.0, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	got, err := BuildSlabLayers(square4, 3.0, []float64{0.25})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Errorf("one layer differs from BuildSlab:\n got %+v\nwant %+v", got, want)
	}
}
