package geometry

import (
	"errors"
	"math"
	"testing"
)

var (
	roofS = math.Tan(30 * math.Pi / 180)
	roofC = math.Cos(30 * math.Pi / 180)
)

func roofSpec(gables ...int) RoofSpec { return RoofSpec{Pitch: 30, Gables: gables, Eaves: 3} }

// buildRoof builds the layers, one of 0.2 m by default, and checks the
// closed-form areas and volumes.
func buildRoof(t *testing.T, outline [][2]float64, spec RoofSpec, layers ...float64) []Roof {
	t.Helper()
	if len(layers) == 0 {
		layers = []float64{0.2}
	}
	roofs, err := BuildRoofLayers(outline, spec, layers)
	if err != nil {
		t.Fatalf("BuildRoofLayers: %v", err)
	}
	if len(roofs) != len(layers) {
		t.Fatalf("%d roofs for %d layers", len(roofs), len(layers))
	}
	for k, r := range roofs {
		q := r.Quantities
		c := math.Cos(spec.Pitch * math.Pi / 180)
		if !nearRel(q.SlopeArea, q.PlanArea/c, 1e-9) || !nearRel(q.Volume, q.PlanArea*layers[k]/c, 1e-9) {
			t.Errorf("layer %d: slope area %v, volume %v for plan area %v", k, q.SlopeArea, q.Volume, q.PlanArea)
		}
	}
	return roofs
}

// roofWant is a roof's hand values; NaN skips one.
type roofWant struct {
	planes                                      int
	plan, eaves, ridge, hip, valley, verge, top float64
}

func checkRoofQuantities(t *testing.T, r Roof, w roofWant) {
	t.Helper()
	if len(r.Planes) != w.planes {
		t.Errorf("%d planes, want %d", len(r.Planes), w.planes)
	}
	q := r.Quantities
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"PlanArea", q.PlanArea, w.plan}, {"Eaves", q.Eaves, w.eaves}, {"Ridge", q.Ridge, w.ridge},
		{"Hip", q.Hip, w.hip}, {"Valley", q.Valley, w.valley}, {"Verge", q.Verge, w.verge}, {"Top", q.Top, w.top},
	} {
		if !math.IsNaN(c.want) && !near(c.got, c.want, 1e-9*math.Max(1, c.want)) {
			t.Errorf("%s = %.12g, want %.12g", c.name, c.got, c.want)
		}
	}
}

func TestRoofRectangleHip(t *testing.T) {
	r := buildRoof(t, roofR, roofSpec())[0]
	checkRoofQuantities(t, r, roofWant{4, 80, 36, 2, 4 * math.Sqrt(32+16*roofS*roofS), 0, 0, 0})
}

func TestRoofRectangleGable(t *testing.T) {
	r := buildRoof(t, roofR, roofSpec(1, 3))[0]
	checkRoofQuantities(t, r, roofWant{2, 80, 20, 10, 0, 0, 4 * 4 / roofC, 0})
	p := r.Planes[0] // edge 0, the low edge y = 0
	sin := math.Sin(30 * math.Pi / 180)
	z := 3 + 0.2/roofC
	for _, v := range []struct {
		name      string
		got, want [3]float64
	}{
		{"Origin", p.Origin, [3]float64{0, 0, z}},
		{"U", p.U, [3]float64{1, 0, 0}},
		{"V", p.V, [3]float64{0, roofC, sin}},
		{"Normal", p.Normal, [3]float64{0, -sin, roofC}},
	} {
		if !closeVec(v.got, v.want, 1e-12) {
			t.Errorf("plane %s = %v, want %v", v.name, v.got, v.want)
		}
	}
	want := [][2]float64{{0, 0}, {10, 0}, {10, 4 / roofC}, {0, 4 / roofC}}
	if len(p.Outline) != len(want) {
		t.Fatalf("plane outline %v, want %v", p.Outline, want)
	}
	for i := range want {
		if !near(p.Outline[i][0], want[i][0], 1e-12) || !near(p.Outline[i][1], want[i][1], 1e-12) {
			t.Errorf("plane outline %v, want %v", p.Outline, want)
		}
	}
}

func TestRoofRectangleMono(t *testing.T) {
	r := buildRoof(t, roofR, roofSpec(1, 2, 3))[0]
	checkRoofQuantities(t, r, roofWant{1, 80, 10, 0, 0, 0, 2 * 8 / roofC, 10})
	p := r.Planes[0]
	var high float64
	for _, o := range p.Outline {
		high = math.Max(high, p.Origin[2]+o[1]*p.V[2])
	}
	if want := 3 + 0.2/roofC + 8*roofS; !nearRel(high, want, 1e-9) {
		t.Errorf("high edge at z %.12g, want %.12g", high, want)
	}
}

func TestRoofLHip(t *testing.T) {
	r := buildRoof(t, roofL, roofSpec())[0]
	s2 := roofS * roofS
	hip := math.Sqrt(8+4*s2) + 3*math.Sqrt(50+25*s2) + 2*math.Sqrt(18+9*s2)
	checkRoofQuantities(t, r, roofWant{6, 156, 56, 8, hip, math.Sqrt(18 + 9*s2), 0, 0})
}

func TestRoofEqualArmCrossGable(t *testing.T) {
	r := buildRoof(t, roofE, roofSpec(1, 4))[0]
	l := math.Sqrt(18 + 9*roofS*roofS)
	checkRoofQuantities(t, r, roofWant{4, 156, 52, 26, l, l, 4 * 3 / roofC, 0})
}

func TestRoofLMono(t *testing.T) {
	r := buildRoof(t, roofL, roofSpec(1, 2, 3, 4, 5))[0]
	checkRoofQuantities(t, r, roofWant{1, 156, 18, 0, 0, 0, (6 + 4 + 10) / roofC, 6 + 12})
}

func TestRoofLStepRefused(t *testing.T) {
	roofs, err := BuildRoofLayers(roofL, roofSpec(2), []float64{0.2})
	var re *RoofError
	if !errors.As(err, &re) || re.Reason != ReasonRoofStep || re.Edge != 2 || roofs != nil {
		t.Fatalf("got %v, %v; want roof_step on edge 2 and nothing else", roofs, err)
	}
}

func TestRoofGableUnderValley(t *testing.T) {
	r := buildRoof(t, roofL, roofSpec(0))[0]
	checkRoofQuantities(t, r, roofWant{5, 156, 6 + 6 + 4 + 12 + 10, math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()})
}

func TestRoofMonoNotch(t *testing.T) {
	notch := [][2]float64{{0, 0}, {10, 0}, {10, 8}, {6, 8}, {6, 5}, {4, 5}, {4, 8}, {0, 8}}
	r := buildRoof(t, notch, roofSpec(1, 2, 3, 4, 5, 6, 7))[0]
	checkRoofQuantities(t, r, roofWant{1, 74, 10, 0, 0, 0, 22 / roofC, 10})
}

// A point inside an eaves edge splits it in two on one line: the arc between
// their faces is no ridge.
func TestRoofCollinearEavesPoint(t *testing.T) {
	split := [][2]float64{{0, 0}, {5, 0}, {10, 0}, {10, 8}, {0, 8}}
	r := buildRoof(t, split, roofSpec())[0]
	checkRoofQuantities(t, r, roofWant{5, 80, 36, 2, 4 * math.Sqrt(32+16*roofS*roofS), 0, 0, 0})
}

func TestRoofOverhang(t *testing.T) {
	spec := roofSpec()
	spec.Overhang = 0.5
	r := buildRoof(t, roofR, spec)[0]
	checkRoofQuantities(t, r, roofWant{4, 99, math.NaN(), math.NaN(), math.NaN(), 0, 0, 0})
	low, ok := meshLowAt(r.Element, [2]float64{0, 0})
	if !ok || !near(low, 3, 1e-5) {
		t.Errorf("underside at (0, 0): z %v (found %v), want 3", low, ok)
	}
}

// meshLowAt is the lowest z of the mesh over a plan point, world.
func meshLowAt(e Element, p [2]float64) (float64, bool) {
	o := [3]float64{e.Placement[12], e.Placement[13], e.Placement[14]}
	q := [2]float64{p[0] - o[0], p[1] - o[1]}
	low, found := math.Inf(1), false
	at := func(i uint32) [3]float64 {
		return [3]float64{float64(e.Verts[3*i]), float64(e.Verts[3*i+1]), float64(e.Verts[3*i+2])}
	}
	for k := 0; k+2 < len(e.Tris); k += 3 {
		a, b, c := at(e.Tris[k]), at(e.Tris[k+1]), at(e.Tris[k+2])
		d := (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
		if math.Abs(d) < 1e-12 {
			continue // a side face
		}
		u := ((b[0]-q[0])*(c[1]-q[1]) - (b[1]-q[1])*(c[0]-q[0])) / d
		v := ((c[0]-q[0])*(a[1]-q[1]) - (c[1]-q[1])*(a[0]-q[0])) / d
		w := 1 - u - v
		if u < -1e-9 || v < -1e-9 || w < -1e-9 {
			continue
		}
		low, found = math.Min(low, u*a[2]+v*b[2]+w*c[2]), true
	}
	return low + o[2], found
}

func TestRoofMeshAgrees(t *testing.T) {
	notch := [][2]float64{{0, 0}, {10, 0}, {10, 8}, {6, 8}, {6, 5}, {4, 5}, {4, 8}, {0, 8}}
	overhang := roofSpec()
	overhang.Overhang = 0.5
	cases := []struct {
		name    string
		outline [][2]float64
		spec    RoofSpec
		layers  []float64
	}{
		{"R hip", roofR, roofSpec(), nil},
		{"R gable", roofR, roofSpec(1, 3), nil},
		{"R mono", roofR, roofSpec(1, 2, 3), nil},
		{"L hip", roofL, roofSpec(), nil},
		{"L mono", roofL, roofSpec(1, 2, 3, 4, 5), nil},
		{"L gable under valley", roofL, roofSpec(0), nil},
		{"E cross-gable", roofE, roofSpec(1, 4), nil},
		{"mono notch", notch, roofSpec(1, 2, 3, 4, 5, 6, 7), nil},
		{"R overhang", roofR, overhang, nil},
		{"R two layers", roofR, roofSpec(), []float64{0.05, 0.15}},
		{"steep", [][2]float64{{0, 0}, {50, 0}, {50, 10}, {0, 10}}, RoofSpec{Pitch: 75, Eaves: 3}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range buildRoof(t, c.outline, c.spec, c.layers...) {
				q := r.Quantities
				meshAgrees(t, r.Element, q.Volume, 2*q.SlopeArea+2*q.PlanArea, 0,
					q.SlopeArea, q.PlanArea, q.Volume, q.Eaves, q.Ridge, q.Hip, q.Valley, q.Verge, q.Top)
			}
		})
	}
}

func TestRoofLayersSum(t *testing.T) {
	one := buildRoof(t, roofR, roofSpec(), 0.2)[0]
	two := buildRoof(t, roofR, roofSpec(), 0.05, 0.15)
	if v := two[0].Quantities.Volume + two[1].Quantities.Volume; !nearRel(v, one.Quantities.Volume, 1e-9) {
		t.Errorf("layer volumes sum to %v, want %v", v, one.Quantities.Volume)
	}
	lines := func(q RoofQuantities) [6]float64 {
		return [6]float64{q.Eaves, q.Ridge, q.Hip, q.Valley, q.Verge, q.Top}
	}
	if lines(two[0].Quantities) != lines(two[1].Quantities) || lines(two[0].Quantities) != lines(one.Quantities) {
		t.Errorf("line lengths differ: %v, %v, %v", lines(two[0].Quantities), lines(two[1].Quantities), lines(one.Quantities))
	}
	if two[0].Planes[0].Origin != one.Planes[0].Origin {
		t.Errorf("top layer's top at %v, the one-layer roof's at %v", two[0].Planes[0].Origin, one.Planes[0].Origin)
	}
	// Layer 1's top vertices are layer 0's bottom vertices: same layout, the
	// top half first.
	v0, v1 := two[0].Element.Verts, two[1].Element.Verts
	half := len(v0) / 2
	if len(v0) != len(v1) || two[0].Element.Placement != two[1].Element.Placement {
		t.Fatal("layers differ in layout")
	}
	for i := range half {
		if v1[i] != v0[half+i] {
			t.Fatalf("layer 1 top coordinate %d is %v, layer 0 bottom's %v", i, v1[i], v0[half+i])
		}
	}
	if low, ok := meshLowAt(two[1].Element, [2]float64{0, 0}); !ok || !near(low, 3, 1e-5) {
		t.Errorf("lowest layer's underside at (0, 0): z %v, want 3", low)
	}
}

func TestRoofWindingAndGrid(t *testing.T) {
	reverse := func(o [][2]float64) [][2]float64 {
		r := make([][2]float64, len(o))
		for i, p := range o {
			r[len(o)-1-i] = p
		}
		return r
	}
	// Edge g of an outline is edge n-2-g of its reverse.
	flip := func(n int, gables ...int) []int {
		f := make([]int, len(gables))
		for i, g := range gables {
			f[i] = (2*n - 2 - g) % n
		}
		return f
	}
	cases := []struct {
		name    string
		outline [][2]float64
		gables  []int
	}{
		{"R hip", roofR, nil}, {"R gable", roofR, []int{1, 3}}, {"R mono", roofR, []int{1, 2, 3}},
		{"L hip", roofL, nil}, {"L mono", roofL, []int{1, 2, 3, 4, 5}}, {"E", roofE, []int{1, 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ccw := buildRoof(t, c.outline, roofSpec(c.gables...))[0]
			cw := buildRoof(t, reverse(c.outline), roofSpec(flip(len(c.outline), c.gables...)...))[0]
			if !nearQuantities(ccw.Quantities, cw.Quantities) || len(ccw.Planes) != len(cw.Planes) {
				t.Errorf("clockwise %+v (%d planes), counter-clockwise %+v (%d planes)",
					cw.Quantities, len(cw.Planes), ccw.Quantities, len(ccw.Planes))
			}
		})
	}
	n := len(roofL)
	_, err := BuildRoofLayers(reverse(roofL), roofSpec(flip(n, 3)...), []float64{0.2})
	var re *RoofError
	if !errors.As(err, &re) || re.Reason != ReasonRoofStep || re.Edge != flip(n, 3)[0] {
		t.Errorf("clockwise L, gable %d: %v, want roof_step on that edge", flip(n, 3)[0], err)
	}

	far := make([][2]float64, len(roofR))
	for i, p := range roofR {
		far[i] = [2]float64{p[0] + 500000, p[1] + 4600000}
	}
	near0 := buildRoof(t, roofR, roofSpec())[0]
	moved := buildRoof(t, far, roofSpec())[0]
	if near0.Quantities != moved.Quantities {
		t.Errorf("moved %+v, at origin %+v", moved.Quantities, near0.Quantities)
	}
	if len(near0.Element.Verts) != len(moved.Element.Verts) {
		t.Fatal("moved roof has other vertices")
	}
	for i, v := range near0.Element.Verts {
		if moved.Element.Verts[i] != v {
			t.Fatalf("local vertex coordinate %d: moved %v, at origin %v", i, moved.Element.Verts[i], v)
		}
	}
}

func nearQuantities(a, b RoofQuantities) bool {
	x := [9]float64{a.SlopeArea, a.PlanArea, a.Volume, a.Eaves, a.Ridge, a.Hip, a.Valley, a.Verge, a.Top}
	y := [9]float64{b.SlopeArea, b.PlanArea, b.Volume, b.Eaves, b.Ridge, b.Hip, b.Valley, b.Verge, b.Top}
	for i := range x {
		if !near(x[i], y[i], 1e-9*math.Max(1, x[i])) {
			return false
		}
	}
	return true
}

func TestRoofSteepPitch(t *testing.T) {
	r := buildRoof(t, [][2]float64{{0, 0}, {50, 0}, {50, 10}, {0, 10}}, RoofSpec{Pitch: 75, Eaves: 3})[0]
	checkRoofQuantities(t, r, roofWant{4, 500, 120, 40, math.NaN(), 0, 0, 0})
}

// Walls a few µm off straight or off parallel are survey noise, not a roof
// too complex to build.
func TestRoofNearParallelNoise(t *testing.T) {
	cases := []struct {
		name    string
		outline [][2]float64
		gables  []int
	}{
		{"hip, fronts meet head on", [][2]float64{{-378, 210}, {-44, -83}, {10, 1}, {10.00001, 8}, {0, 90}, {0, 15.666666666666666}}, nil},
		{"hip, opposite walls", [][2]float64{{-63, 0}, {-69, -83}, {10, 0.25}, {10.00001, 8}, {0, 8}, {0, 2.6666666666666665}}, nil},
		{"gables off straight", [][2]float64{{0, 0}, {10, -65}, {10, 4}, {10.00001, 58}, {0, 8}}, []int{1, 2, 4}},
		{"split beyond a gable's corner", [][2]float64{{-63, 0}, {10, -83}, {10, 1}, {10.00001, 8}, {0, 8}, {0, 2.6666666666666665}}, []int{0, 1, 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range buildRoof(t, c.outline, roofSpec(c.gables...), 0.05, 0.15) {
				q := r.Quantities
				meshAgrees(t, r.Element, q.Volume, 2*q.SlopeArea+2*q.PlanArea, 0,
					q.SlopeArea, q.PlanArea, q.Volume, q.Eaves, q.Ridge, q.Hip, q.Valley, q.Verge, q.Top)
			}
		})
	}
}

// A sloped front that meets a gable a few µm off head on, as a mono roof's
// high gable, builds as it does when exactly parallel.
func TestRoofNearHeadOnGable(t *testing.T) {
	cases := []struct {
		name    string
		outline [][2]float64
		gables  []int
	}{
		{"mono, high gable tilted down", [][2]float64{{0, 0}, {10, 0}, {10, 8}, {0, 7.99999}}, []int{1, 2, 3}},
		{"mono, high gable tilted up", [][2]float64{{0, 0}, {10, 0}, {10, 8}, {0, 8.00001}}, []int{1, 2, 3}},
		{"front reaches a gable", [][2]float64{{0, 0}, {1, 0}, {1.0001, 100}, {0, 100}}, []int{1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range buildRoof(t, c.outline, roofSpec(c.gables...)) {
				q := r.Quantities
				meshAgrees(t, r.Element, q.Volume, 2*q.SlopeArea+2*q.PlanArea, 0,
					q.SlopeArea, q.PlanArea, q.Volume, q.Eaves, q.Ridge, q.Hip, q.Valley, q.Verge, q.Top)
			}
		})
	}
}
