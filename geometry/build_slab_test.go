package geometry

import (
	"math"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func nearRel(a, b, rel float64) bool { return math.Abs(a-b) <= rel*math.Max(math.Abs(a), math.Abs(b)) }

var rect54 = [][2]float64{{0, 0}, {5, 0}, {5, 4}, {0, 4}}

func TestBuildSlabOracle(t *testing.T) {
	for _, tc := range []struct {
		name                string
		outline             [][2]float64
		thickness           float64
		area, perim, volume float64
	}{
		// area 5·4; perimeter 2·(5+4); volume 20·0.2
		{"rect 5×4", rect54, 0.20, 20, 18, 4},
		// area 4·4 − 2·2; perimeter 4+2+2+2+2+4; volume 12·0.25
		{"L", lOutline, 0.25, 12, 16, 3},
	} {
		s, err := BuildSlab(tc.outline, 0, tc.thickness)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		q := s.Quantities
		if !near(q.Area, tc.area, 1e-9) || !near(q.Perimeter, tc.perim, 1e-9) || !near(q.Volume, tc.volume, 1e-9) {
			t.Errorf("%s: got %+v, want area %v perimeter %v volume %v", tc.name, q, tc.area, tc.perim, tc.volume)
		}
	}
}

func TestBuildSlabMeshMatches(t *testing.T) {
	for _, o := range [][][2]float64{rect54, lOutline} {
		s, err := BuildSlab(o, 0, 0.2)
		if err != nil {
			t.Fatal(err)
		}
		if !isClosedManifold(s.Element.Verts, s.Element.Tris) {
			t.Fatal("slab mesh is not a closed shell")
		}
		v, ok := meshVolume(s.Element.Verts, s.Element.Tris)
		if !ok || !nearRel(v, s.Quantities.Volume, 1e-6) {
			t.Errorf("mesh volume %v (ok %v), want %v", v, ok, s.Quantities.Volume)
		}
	}
}

func TestBuildSlabPlacement(t *testing.T) {
	o := make([][2]float64, len(rect54))
	for i, p := range rect54 {
		o[i] = [2]float64{p[0] + 1e6, p[1] + 2e6}
	}
	s, err := BuildSlab(o, 6.0, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Element
	if !near(e.BBoxMax[2], 6.0, 1e-6) || !near(e.BBoxMin[2], 5.8, 1e-6) {
		t.Errorf("BBox z = [%v, %v], want [5.8, 6.0]", e.BBoxMin[2], e.BBoxMax[2])
	}
	if !near(e.BBoxMin[0], 1e6, 1e-6) || !near(e.BBoxMax[1], 2e6+4, 1e-6) {
		t.Errorf("BBox = %v .. %v", e.BBoxMin, e.BBoxMax)
	}
	f := s.Top
	if f.Origin != [3]float64{1e6, 2e6, 6.0} || f.U != [3]float64{1, 0, 0} || f.V != [3]float64{0, 1, 0} || f.Normal != [3]float64{0, 0, 1} {
		t.Errorf("Top frame = %+v", f)
	}
	for i, p := range f.Outline {
		if !near(p[0], rect54[i][0], 1e-9) || !near(p[1], rect54[i][1], 1e-9) {
			t.Errorf("Top.Outline[%d] = %v, want %v", i, p, rect54[i])
		}
	}
	if e.GlobalID != "" || e.Source != SourceExtrude {
		t.Errorf("GlobalID %q Source %q", e.GlobalID, e.Source)
	}
}

func TestBuildSlabRefuses(t *testing.T) {
	far := append([][2]float64{}, rect54...)
	far[2] = [2]float64{1001, 4}
	origin := make([][2]float64, len(rect54))
	for i, p := range rect54 {
		origin[i] = [2]float64{p[0] + 1.1e7, p[1]}
	}
	for _, tc := range []struct {
		name    string
		outline [][2]float64
		z, t    float64
	}{
		{"NaN point", [][2]float64{{0, 0}, {math.NaN(), 0}, {1, 1}}, 0, 0.2},
		{"Inf z", rect54, math.Inf(1), 0.2},
		{"zero thickness", rect54, 0, 0},
		{"thickness under 1 mm", rect54, 0, 0.0005},
		{"NaN thickness", rect54, 0, math.NaN()},
		{"two points", [][2]float64{{0, 0}, {1, 0}}, 0, 0.2},
		{"repeated point", [][2]float64{{0, 0}, {5, 0}, {5, 0}, {5, 4}, {0, 4}}, 0, 0.2},
		{"bowtie", [][2]float64{{0, 0}, {1, 1}, {1, 0}, {0, 1}}, 0, 0.2},
		{"collinear", [][2]float64{{0, 0}, {1, 0}, {2, 0}}, 0, 0.2},
		{"point 1,001 m away", far, 0, 0.2},
		{"first point 1.1e7 m out", origin, 0, 0.2},
		{"2049 points", regularPolygon(maxOutlinePoints+1, 10), 0, 0.2},
	} {
		if _, err := BuildSlab(tc.outline, tc.z, tc.t); err == nil {
			t.Errorf("%s: want an error", tc.name)
		}
	}
}
