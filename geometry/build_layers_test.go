package geometry

import (
	"errors"
	"fmt"
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

// layers3 is plaster, brick and insulation: 0.29 m in all.
var (
	layers3     = []float64{0.015, 0.175, 0.10}
	edge3       = [2][2]float64{{0, 0}, {3, 0}}
	window3     = Opening{Offset: 1.00, Sill: 0.90, Width: 1.20, Height: 1.20}
	niche3      = Opening{Offset: 1.0, Sill: 1.0, Width: 0.5, Height: 0.5, Depth: 0.10}
	layers3Sum  = 0.015 + 0.175 + 0.10
	wallHeight3 = 2.50
)

func buildLayers3(t *testing.T, ops ...Opening) []Wall {
	t.Helper()
	ws, err := BuildWallLayers(edge3, 0, wallHeight3, layers3, SideLeft, ops)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != len(layers3) {
		t.Fatalf("got %d layers, want %d", len(ws), len(layers3))
	}
	return ws
}

type q struct {
	what      string
	got, want float64
}

func checkQ(t *testing.T, label string, cs ...q) {
	t.Helper()
	for _, c := range cs {
		if !near(c.got, c.want, 1e-9) {
			t.Errorf("%s: %s = %v, want %v", label, c.what, c.got, c.want)
		}
	}
}

func TestBuildWallLayersOracle(t *testing.T) {
	ws := buildLayers3(t, window3)
	origins := [][3]float64{{0, 0, 0}, {0, -0.015, 0}, {0, -0.19, 0}}
	for k, w := range ws {
		wq := w.Quantities
		th := layers3[k]
		checkQ(t, fmt.Sprintf("layer %d", k),
			q{"Length", wq.Length, 3.00},
			q{"MeasuredGross", wq.MeasuredGross, 7.50},
			q{"OtherGross", wq.OtherGross, 7.50},
			q{"MeasuredNet", wq.MeasuredNet, 6.06},
			q{"OtherNet", wq.OtherNet, 6.06},
			q{"RevealPerimeter", wq.RevealPerimeter, 4.80},
			q{"GrossVolume", wq.GrossVolume, 7.5 * th},
			q{"NetVolume", wq.NetVolume, 6.06 * th},
			q{"Origin x", w.Measured.Origin[0], origins[k][0]},
			q{"Origin y", w.Measured.Origin[1], origins[k][1]},
			q{"Origin z", w.Measured.Origin[2], origins[k][2]},
		)
		if w.Measured.Normal != [3]float64{0, 1, 0} {
			t.Errorf("layer %d: normal %v, want (0, 1, 0)", k, w.Measured.Normal)
		}
	}
}

func TestBuildWallLayers_NicheInsideALayer(t *testing.T) {
	ws := buildLayers3(t, niche3)
	if ops := ws[0].Measured.Openings; len(ops) != 1 || ops[0].Depth != 0 {
		t.Errorf("layer 0 openings %+v, want the niche cut through", ops)
	}
	if ops := ws[1].Measured.Openings; len(ops) != 1 || !near(ops[0].Depth, 0.085, 1e-12) {
		t.Errorf("layer 1 openings %+v, want the niche at depth 0.085", ops)
	}
	if ops := ws[2].Measured.Openings; len(ops) != 0 {
		t.Errorf("layer 2 openings %+v, want none", ops)
	}
	q0, q1, q2 := ws[0].Quantities, ws[1].Quantities, ws[2].Quantities
	checkQ(t, "layer 0", q{"MeasuredNet", q0.MeasuredNet, 7.25}, q{"OtherNet", q0.OtherNet, 7.25}, q{"NicheBack", q0.NicheBack, 0})
	checkQ(t, "layer 1", q{"MeasuredNet", q1.MeasuredNet, 7.25}, q{"OtherNet", q1.OtherNet, 7.50}, q{"NicheBack", q1.NicheBack, 0.25})
	checkQ(t, "layer 2", q{"MeasuredNet", q2.MeasuredNet, 7.50}, q{"OtherNet", q2.OtherNet, 7.50}, q{"NicheBack", q2.NicheBack, 0})
}

func TestBuildWallLayers_NicheOnBoundary(t *testing.T) {
	n := niche3
	n.Depth = 0.015 + 0.175
	ws := buildLayers3(t, n)
	for k := range 2 {
		if ops := ws[k].Measured.Openings; len(ops) != 1 || ops[0].Depth != 0 {
			t.Errorf("layer %d openings %+v, want the niche cut through", k, ops)
		}
	}
	if ops := ws[2].Measured.Openings; len(ops) != 0 {
		t.Errorf("layer 2 openings %+v, want none", ops)
	}
	q2 := ws[2].Quantities
	checkQ(t, "layer 2", q{"MeasuredNet", q2.MeasuredNet, 7.25}, q{"OtherNet", q2.OtherNet, 7.50},
		q{"NicheBack", q2.NicheBack, 0.25}, q{"NetVolume", q2.NetVolume, 0.75})
}

func TestBuildWallLayers_LayerSliver(t *testing.T) {
	for _, d := range []float64{0.0155, 0.1895} {
		n := niche3
		n.Depth = d
		_, err := BuildWallLayers(edge3, 0, wallHeight3, layers3, SideLeft, []Opening{n})
		var pe *PlacementError
		if !errors.As(err, &pe) || *pe != (PlacementError{Wall: 0, Opening: 0, Reason: ReasonLayerSliver}) {
			t.Errorf("depth %v: err = %v, want wall 0 opening 0: layer_sliver", d, err)
		}
	}
}

// layerInvariants checks that layers add up to the whole wall.
func layerInvariants(t *testing.T, label string, layers []Wall, whole Wall, thick []float64) {
	t.Helper()
	wq := whole.Quantities
	var vol, backs float64
	for k, w := range layers {
		lq := w.Quantities
		vol += lq.NetVolume
		backs += lq.NicheBack
		if k+1 < len(layers) {
			checkQ(t, fmt.Sprintf("%s layer %d/%d", label, k, k+1), q{"shared face", lq.OtherGross, layers[k+1].Quantities.MeasuredGross})
		}
		meshAgrees(t, w.Element, lq.NetVolume, wallSurface(lq, thick[k]), snapSlack(lq, thick[k]),
			lq.Length, lq.Height, lq.MeasuredGross, lq.MeasuredNet, lq.OtherGross, lq.OtherNet, lq.RevealPerimeter, lq.GrossVolume, lq.NetVolume)
	}
	checkQ(t, label,
		q{"first MeasuredNet", layers[0].Quantities.MeasuredNet, wq.MeasuredNet},
		q{"last OtherNet", layers[len(layers)-1].Quantities.OtherNet, wq.OtherNet},
		q{"Σ NetVolume", vol, wq.NetVolume},
		q{"Σ NicheBack", backs, wq.NicheBack},
	)
}

func TestBuildWallLayers_Invariants(t *testing.T) {
	onBoundary := niche3
	onBoundary.Depth = 0.19
	for _, tc := range []struct {
		name string
		ops  []Opening
	}{{"window", []Opening{window3}}, {"niche", []Opening{niche3}}, {"niche on boundary", []Opening{onBoundary}}, {"solid", nil}} {
		whole, err := BuildWall(edge3, 0, wallHeight3, layers3Sum, SideLeft, tc.ops)
		if err != nil {
			t.Fatal(err)
		}
		layerInvariants(t, tc.name, buildLayers3(t, tc.ops...), whole, layers3)
	}
}

func TestBuildWallLayers_OneLayerIsBuildWall(t *testing.T) {
	for _, tc := range wallOracle() {
		want, err := BuildWall(tc.edge, 0, tc.height, tc.thickness, SideLeft, tc.ops)
		if err != nil {
			t.Fatal(err)
		}
		got, err := BuildWallLayers(tc.edge, 0, tc.height, []float64{tc.thickness}, SideLeft, tc.ops)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Errorf("%s: one layer differs from BuildWall", tc.name)
		}
	}
}

func TestBuildWallLayers_SameRefusalsAsWholeWall(t *testing.T) {
	ops := []Opening{{Offset: 2.5, Sill: 0, Width: 0.8, Height: 2.0}, window3, {Offset: 1.5, Sill: 1.0, Width: 0.5, Height: 0.5}}
	_, want := BuildWall(edge3, 0, wallHeight3, layers3Sum, SideLeft, ops)
	_, got := BuildWallLayers(edge3, 0, wallHeight3, layers3, SideLeft, ops)
	if want == nil || got == nil || got.Error() != want.Error() {
		t.Errorf("refusals differ:\n got %v\nwant %v", got, want)
	}
}

func TestBuildWallLayers_MeshCapCountsEveryLayer(t *testing.T) {
	// A staircase of 90 openings, each on its own u and v: about 182 × 182
	// grid cells per layer, over 1/16 of the cap but under the cap itself.
	var ops []Opening
	for i := range 90 {
		x := 0.5 + float64(i)
		ops = append(ops, Opening{Offset: x, Sill: x, Width: 0.5, Height: 0.5})
	}
	edge := [2][2]float64{{0, 0}, {100, 0}}
	span := wallSpan{length: 100, otherEnd: 100}
	if c := gridCells(span, 100, ops); c <= maxMeshCells/16 || c > maxMeshCells {
		t.Fatalf("fixture has %d cells, want in (%d, %d]", c, maxMeshCells/16, maxMeshCells)
	}
	if _, err := BuildWallLayers(edge, 0, 100, []float64{0.2}, SideLeft, ops); err != nil {
		t.Fatalf("one layer: %v", err)
	}
	sixteen := make([]float64, 16)
	for i := range sixteen {
		sixteen[i] = 0.01
	}
	if _, err := BuildWallLayers(edge, 0, 100, sixteen, SideLeft, ops); !errors.Is(err, errTooMuchMesh) {
		t.Errorf("16 layers: err = %v, want errTooMuchMesh", err)
	}
}
