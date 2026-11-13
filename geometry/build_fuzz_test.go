package geometry

import (
	"errors"
	"math"
	"testing"
)

// meshAgrees checks the fuzz property for one built element: every quantity
// is finite and not negative, the mesh is a closed shell, and its volume is
// the expected volume within float32 rounding. Vertices are float32 local to
// the first point, so each is off by up to extent·2⁻²³; the volume can be off
// by that times the surface area. surface bounds the surface area. slack adds
// any known mesh shift: a merged grid line moves an opening edge by up to
// gridSnap.
func meshAgrees(t *testing.T, e Element, want, surface, slack float64, qs ...float64) {
	t.Helper()
	for _, q := range qs {
		if math.IsNaN(q) || math.IsInf(q, 0) || q < 0 {
			t.Fatalf("quantity %v is not a finite non-negative number (all: %v)", q, qs)
		}
	}
	if !isClosedManifold(e.Verts, e.Tris) {
		t.Fatal("mesh is not a closed shell")
	}
	var extent float64
	for _, v := range e.Verts {
		extent = math.Max(extent, math.Abs(float64(v)))
	}
	got, ok := meshVolume(e.Verts, e.Tris)
	tol := 1e-9*want + 4*surface*extent*math.Pow(2, -23) + slack
	if !ok || math.Abs(got-want) > tol {
		t.Fatalf("mesh volume %v (ok %v), want %v ± %v", got, ok, want, tol)
	}
}

// snapSlack bounds the volume a merged grid line can move: every opening edge
// shifts by at most gridSnap through the full thickness. 2·RevealPerimeter
// covers every edge, a door's bottom included.
func snapSlack(q WallQuantities, thickness float64) float64 {
	return 2 * q.RevealPerimeter * gridSnap * thickness
}

func wallSurface(q WallQuantities, thickness float64) float64 {
	return q.MeasuredGross + q.OtherGross + 2*(q.Length+thickness)*thickness + 2*q.Height*thickness + q.RevealPerimeter*thickness
}

func FuzzBuildWall(f *testing.F) {
	z4 := [4]float64{}
	f.Add(0.0, 0.0, 2.40, 0.0, 0.0, 2.60, 0.10, uint8(1), 0.50, 0.0, 0.80, 2.00, 0.0, 0.0, 0.0, 0.0, z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3])
	f.Add(1e6, 1e6, 1e6+3, 1e6+1, 3.0, 2.70, 0.20, uint8(2), 1.00, 0.90, 1.20, 1.20, 2.4, 0.9, 0.5, 1.0, z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3])
	f.Add(0.0, 0.0, 1000.0, 1.0, 0.0, 3.0, 0.001, uint8(1), 500.0, 1.0, 1.0, 1.0, 0.0, 0.0, 0.0, 0.0, z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3])
	// Stacked openings with jambs closer than the grid snap (final review).
	f.Add(0.0, 0.0, 3.0, 0.0, 0.0, 3.0, 0.2, uint8(1), 0.5, 1.2, 0.3, 0.3, 0.50008, 0.0, 0.3, 0.3, 0.50015, 0.7, 0.3, 0.3, z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3])
	// A door beside a niche.
	f.Add(0.0, 0.0, 2.40, 0.0, 0.0, 2.60, 0.10, uint8(2), 0.20, 0.0, 0.70, 2.00, 1.00, 1.20, 0.40, 0.60, z4[0], z4[1], z4[2], z4[3], z4[0], z4[1], z4[2], z4[3], 0.0, 0.08, 0.0, 0.0)
	f.Fuzz(func(t *testing.T, ax, ay, bx, by, base, h, th float64, side uint8, o1, s1, w1, h1, o2, s2, w2, h2, o3, s3, w3, h3, o4, s4, w4, h4, d1, d2, d3, d4 float64) {
		var ops []Opening
		for _, o := range []Opening{{o1, s1, w1, h1, d1}, {o2, s2, w2, h2, d2}, {o3, s3, w3, h3, d3}, {o4, s4, w4, h4, d4}} {
			if o.Width != 0 {
				ops = append(ops, o)
			}
		}
		w, err := BuildWall([2][2]float64{{ax, ay}, {bx, by}}, base, h, th, Side(side), ops)
		if err != nil {
			return
		}
		q := w.Quantities
		meshAgrees(t, w.Element, q.NetVolume, wallSurface(q, th), snapSlack(q, th),
			q.Length, q.Height, q.MeasuredGross, q.MeasuredNet, q.OtherGross, q.OtherNet, q.RevealPerimeter, q.GrossVolume, q.NetVolume)
	})
}

func FuzzBuildRoomWalls(f *testing.F) {
	f.Add(0.0, 0.0, 2.40, 0.0, 2.40, 1.80, 0.0, 1.80, 2.60, 0.10, uint8(1), uint8(0), 0.50, 0.0, 0.80, 2.00)
	f.Add(0.0, 0.0, 4.0, 0.0, 2.0, 2.0, 0.0, 4.0, 2.50, 0.20, uint8(2), uint8(2), 1.0, 0.5, 0.5, 1.0)
	f.Fuzz(func(t *testing.T, x0, y0, x1, y1, x2, y2, x3, y3, h, th float64, side, wall uint8, o, s, w, oh float64) {
		outline := [][2]float64{{x0, y0}, {x1, y1}, {x2, y2}, {x3, y3}}
		ops := make([][]Opening, len(outline))
		ops[int(wall)%len(outline)] = []Opening{{o, s, w, oh, 0}}
		walls, err := BuildRoomWalls(outline, 0, h, th, Side(side), ops)
		if err != nil {
			var pe *PlacementError
			if errors.As(err, &pe) && pe.Wall != int(wall)%len(outline) {
				t.Fatalf("refusal names wall %d, the opening is on wall %d", pe.Wall, int(wall)%len(outline))
			}
			return
		}
		for _, w := range walls {
			q := w.Quantities
			meshAgrees(t, w.Element, q.NetVolume, wallSurface(q, th)+2*th*th*maxMitre, snapSlack(q, th),
				q.Length, q.Height, q.MeasuredGross, q.MeasuredNet, q.OtherGross, q.OtherNet, q.RevealPerimeter, q.GrossVolume, q.NetVolume)
		}
	})
}

func FuzzBuildSlab(f *testing.F) {
	f.Add(0.0, 0.0, 5.0, 0.0, 5.0, 4.0, 0.0, 4.0, 2.0, 2.0, 0.0, 0.20)
	f.Add(1e6, 2e6, 1e6+4, 2e6, 1e6+4, 2e6+2, 1e6+2, 2e6+2, 1e6+2, 2e6+4, 6.0, 0.25)
	f.Fuzz(func(t *testing.T, x0, y0, x1, y1, x2, y2, x3, y3, x4, y4, z, th float64) {
		s, err := BuildSlab([][2]float64{{x0, y0}, {x1, y1}, {x2, y2}, {x3, y3}, {x4, y4}}, z, th)
		if err != nil {
			return
		}
		q := s.Quantities
		meshAgrees(t, s.Element, q.Volume, 2*q.Area+q.Perimeter*th, 0, q.Area, q.Perimeter, q.Volume)
	})
}

func FuzzBuildSlabLayers(f *testing.F) {
	f.Add(0.0, 0.0, 5.0, 0.0, 5.0, 4.0, 0.0, 4.0, 2.0, 2.0, 0.0, 0.05, 0.20, 0.0)
	f.Add(1e6, 2e6, 1e6+4, 2e6, 1e6+4, 2e6+2, 1e6+2, 2e6+2, 1e6+2, 2e6+4, 6.0, 0.02, 0.25, 0.01)
	f.Fuzz(func(t *testing.T, x0, y0, x1, y1, x2, y2, x3, y3, x4, y4, z, t1, t2, t3 float64) {
		var layers []float64
		for _, th := range []float64{t1, t2, t3} {
			if th != 0 {
				layers = append(layers, th)
			}
		}
		slabs, err := BuildSlabLayers([][2]float64{{x0, y0}, {x1, y1}, {x2, y2}, {x3, y3}, {x4, y4}}, z, layers)
		if err != nil {
			return
		}
		var vol, total float64
		for k, s := range slabs {
			q := s.Quantities
			meshAgrees(t, s.Element, q.Volume, 2*q.Area+q.Perimeter*layers[k], 0, q.Area, q.Perimeter, q.Volume)
			vol += q.Volume
			total += layers[k]
		}
		if a := slabs[0].Quantities.Area; !nearRel(vol, a*total, 1e-9) {
			t.Fatalf("layer volumes sum to %v, want area·thickness %v", vol, a*total)
		}
	})
}

// layersExplain reports whether a layered build may fail where the whole one
// built: a layer input error, a layer sliver, or the summed mesh cap.
func layersExplain(err error, layers []float64) bool {
	if _, lerr := checkLayers(layers); lerr != nil {
		return true
	}
	var pe *PlacementError
	return errors.Is(err, errTooMuchMesh) || (errors.As(err, &pe) && pe.Reason == ReasonLayerSliver)
}

func FuzzBuildWallLayers(f *testing.F) {
	f.Add(0.0, 0.0, 3.0, 0.0, 0.0, 2.50, 0.015, 0.175, 0.10, uint8(1), 1.00, 0.90, 1.20, 1.20, 0.0, 1.0, 1.0, 0.5, 0.5, 0.10)
	f.Add(1e6, 1e6, 1e6+3, 1e6+1, 3.0, 2.70, 0.02, 0.2, 0.0, uint8(2), 0.40, 0.0, 0.80, 2.0, 0.0, 1.8, 1.0, 0.5, 0.5, 0.02)
	f.Add(0.0, 0.0, 3.0, 0.0, 0.0, 2.50, 0.015, 0.175, 0.10, uint8(1), 0.20, 1.0, 0.5, 0.5, 0.19, 1.0, 1.0, 0.5, 0.5, 0.0155)
	f.Fuzz(func(t *testing.T, ax, ay, bx, by, base, h, t1, t2, t3 float64, side uint8, o1, s1, w1, h1, d1, o2, s2, w2, h2, d2 float64) {
		var layers []float64
		for _, th := range []float64{t1, t2, t3} {
			if th != 0 {
				layers = append(layers, th)
			}
		}
		var ops []Opening
		for _, o := range []Opening{{o1, s1, w1, h1, d1}, {o2, s2, w2, h2, d2}} {
			if o.Width != 0 {
				ops = append(ops, o)
			}
		}
		edge := [2][2]float64{{ax, ay}, {bx, by}}
		var total float64
		for _, th := range layers {
			total += th
		}
		whole, werr := BuildWall(edge, base, h, total, Side(side), ops)
		ws, err := BuildWallLayers(edge, base, h, layers, Side(side), ops)
		switch {
		case err != nil && werr == nil:
			if !layersExplain(err, layers) {
				t.Fatalf("layered build refused what the whole wall built: %v", err)
			}
			return
		case err == nil && werr != nil:
			t.Fatalf("layered build placed what the whole wall refused: %v", werr)
		case err != nil:
			return
		}
		var vol, backs float64
		for k, w := range ws {
			q := w.Quantities
			meshAgrees(t, w.Element, q.NetVolume, wallSurface(q, layers[k]), snapSlack(q, layers[k]),
				q.Length, q.Height, q.MeasuredGross, q.MeasuredNet, q.OtherGross, q.OtherNet, q.RevealPerimeter, q.GrossVolume, q.NetVolume)
			vol += q.NetVolume
			backs += q.NicheBack
		}
		wq := whole.Quantities
		if math.Abs(vol-wq.NetVolume) > 1e-9*wq.NetVolume+1e-12 || math.Abs(backs-wq.NicheBack) > 1e-9*wq.NicheBack+1e-12 {
			t.Fatalf("layers sum to volume %v, niche back %v; whole wall %v, %v", vol, backs, wq.NetVolume, wq.NicheBack)
		}
	})
}
