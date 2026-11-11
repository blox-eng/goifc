package geometry

import (
	"math"
	"testing"
)

// lOutline is an L-shaped ring: a 4 × 4 square with its 2 × 2 top-right
// quarter removed. Area 4·4 − 2·2 = 12, perimeter 16.
var lOutline = [][2]float64{{0, 0}, {4, 0}, {4, 2}, {2, 2}, {2, 4}, {0, 4}}

func regularPolygon(n int, r float64) [][2]float64 {
	p := make([][2]float64, n)
	for i := range p {
		a := 2 * math.Pi * float64(i) / float64(n)
		p[i] = [2]float64{r * math.Cos(a), r * math.Sin(a)}
	}
	return p
}

func TestPolygonArea2DIsUnsigned(t *testing.T) {
	ccw := [][2]float64{{0, 0}, {5, 0}, {5, 4}, {0, 4}}
	cw := [][2]float64{{0, 0}, {0, 4}, {5, 4}, {5, 0}}
	for name, tc := range map[string]struct {
		poly [][2]float64
		want float64
	}{
		"ccw rectangle 5·4": {ccw, 20},
		"cw rectangle 5·4":  {cw, 20},
		"L 4·4 − 2·2":       {lOutline, 12},
	} {
		if got := PolygonArea2D(tc.poly); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: PolygonArea2D = %v, want %v", name, got, tc.want)
		}
	}
}

func TestPointInPolygon2D(t *testing.T) {
	if !PointInPolygon2D([2]float64{1, 1}, lOutline) {
		t.Error("(1,1) is inside the L")
	}
	if PointInPolygon2D([2]float64{3, 3}, lOutline) {
		t.Error("(3,3) is in the removed quarter")
	}
}

func TestTriangulate2DCoversTheArea(t *testing.T) {
	tris, err := Triangulate2D(lOutline)
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for i := 0; i+2 < len(tris); i += 3 {
		sum += PolygonArea2D([][2]float64{lOutline[tris[i]], lOutline[tris[i+1]], lOutline[tris[i+2]]})
	}
	if math.Abs(sum-12) > 1e-9 {
		t.Errorf("triangles cover %v, want 12", sum)
	}
}

func TestTriangulate2DRefuses(t *testing.T) {
	for name, poly := range map[string][][2]float64{
		"two points":  {{0, 0}, {1, 0}},
		"bowtie":      {{0, 0}, {1, 1}, {1, 0}, {0, 1}},
		"2049 points": regularPolygon(maxOutlinePoints+1, 10),
	} {
		if _, err := Triangulate2D(poly); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := Triangulate2D(regularPolygon(maxOutlinePoints, 10)); err != nil {
		t.Errorf("a %d-point polygon is within the cap: %v", maxOutlinePoints, err)
	}
}
