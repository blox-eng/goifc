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
// by that times the surface area. surface bounds the surface area.
func meshAgrees(t *testing.T, e Element, want, surface float64, qs ...float64) {
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
	tol := 1e-9*want + 4*surface*extent*math.Pow(2, -23)
	if !ok || math.Abs(got-want) > tol {
		t.Fatalf("mesh volume %v (ok %v), want %v ± %v", got, ok, want, tol)
	}
}

func wallSurface(q WallQuantities, thickness float64) float64 {
	return q.MeasuredGross + q.OtherGross + 2*(q.Length+thickness)*thickness + 2*q.Height*thickness + q.RevealPerimeter*thickness
}

func FuzzBuildWall(f *testing.F) {
	f.Add(0.0, 0.0, 2.40, 0.0, 0.0, 2.60, 0.10, uint8(1), 0.50, 0.0, 0.80, 2.00, 0.0, 0.0, 0.0, 0.0)
	f.Add(1e6, 1e6, 1e6+3, 1e6+1, 3.0, 2.70, 0.20, uint8(2), 1.00, 0.90, 1.20, 1.20, 2.4, 0.9, 0.5, 1.0)
	f.Add(0.0, 0.0, 1000.0, 1.0, 0.0, 3.0, 0.001, uint8(1), 500.0, 1.0, 1.0, 1.0, 0.0, 0.0, 0.0, 0.0)
	f.Fuzz(func(t *testing.T, ax, ay, bx, by, base, h, th float64, side uint8, o1, s1, w1, h1, o2, s2, w2, h2 float64) {
		var ops []Opening
		if w1 != 0 {
			ops = append(ops, Opening{o1, s1, w1, h1})
		}
		if w2 != 0 {
			ops = append(ops, Opening{o2, s2, w2, h2})
		}
		w, err := BuildWall([2][2]float64{{ax, ay}, {bx, by}}, base, h, th, Side(side), ops)
		if err != nil {
			return
		}
		q := w.Quantities
		meshAgrees(t, w.Element, q.NetVolume, wallSurface(q, th),
			q.Length, q.Height, q.MeasuredGross, q.MeasuredNet, q.OtherGross, q.OtherNet, q.RevealPerimeter, q.GrossVolume, q.NetVolume)
	})
}

func FuzzBuildRoomWalls(f *testing.F) {
	f.Add(0.0, 0.0, 2.40, 0.0, 2.40, 1.80, 0.0, 1.80, 2.60, 0.10, uint8(1), uint8(0), 0.50, 0.0, 0.80, 2.00)
	f.Add(0.0, 0.0, 4.0, 0.0, 2.0, 2.0, 0.0, 4.0, 2.50, 0.20, uint8(2), uint8(2), 1.0, 0.5, 0.5, 1.0)
	f.Fuzz(func(t *testing.T, x0, y0, x1, y1, x2, y2, x3, y3, h, th float64, side, wall uint8, o, s, w, oh float64) {
		outline := [][2]float64{{x0, y0}, {x1, y1}, {x2, y2}, {x3, y3}}
		ops := make([][]Opening, len(outline))
		ops[int(wall)%len(outline)] = []Opening{{o, s, w, oh}}
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
			meshAgrees(t, w.Element, q.NetVolume, wallSurface(q, th)+2*th*th*maxMitre,
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
		meshAgrees(t, s.Element, q.Volume, 2*q.Area+q.Perimeter*th, q.Area, q.Perimeter, q.Volume)
	})
}
