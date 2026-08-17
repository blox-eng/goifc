package geometry

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// loadProjectedTris reads a whitespace-separated dump of projected triangles,
// six numbers per line (u0 v0 u1 v1 u2 v2).
func loadProjectedTris(t *testing.T, path string) [][3][2]float64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out [][3][2]float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) == 0 {
			continue
		}
		if len(fs) != 6 {
			t.Fatalf("want 6 numbers per line, got %d: %q", len(fs), sc.Text())
		}
		var n [6]float64
		for i, s := range fs {
			if n[i], err = strconv.ParseFloat(s, 64); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, [3][2]float64{{n[0], n[1]}, {n[2], n[3]}, {n[4], n[5]}})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// rasterCoveredArea is an INDEPENDENT reference for the area a triangle set
// covers: sample an n x n grid over the bounding box and count the cells whose
// centre lies in some triangle. It shares no code with the boundary walk, so
// the two agreeing is real evidence rather than a tautology. Accurate to
// roughly the perimeter times the cell size, so a few percent here.
func rasterCoveredArea(tris [][3][2]float64, n int) float64 {
	if len(tris) == 0 {
		return 0
	}
	minU, minV := math.Inf(1), math.Inf(1)
	maxU, maxV := math.Inf(-1), math.Inf(-1)
	for _, tr := range tris {
		u0, v0, u1, v1 := triBounds(tr)
		minU, minV = math.Min(minU, u0), math.Min(minV, v0)
		maxU, maxV = math.Max(maxU, u1), math.Max(maxV, v1)
	}
	w, h := maxU-minU, maxV-minV
	if !(w > 0) || !(h > 0) {
		return 0
	}
	g := newGrid(len(tris), func(i int) (a, b, c, d float64) { return triBounds(tris[i]) })
	var covered int
	for iy := 0; iy < n; iy++ {
		for ix := 0; ix < n; ix++ {
			p := [2]float64{
				minU + (float64(ix)+0.5)*w/float64(n),
				minV + (float64(iy)+0.5)*h/float64(n),
			}
			hit := false
			g.query(p[0], p[1], p[0], p[1], func(i int) {
				if !hit && strictlyInsideCCW(p, tris[i]) {
					hit = true
				}
			})
			if hit {
				covered++
			}
		}
	}
	return float64(covered) * (w / float64(n)) * (h / float64(n))
}

// TestUnionAreaOnRealFacadePanel: the projected outward faces of one facade
// panel from a 29 MB ArchiCAD IFC2X3 export, at the world coordinates the file
// actually carries. Thirty triangles, tessellation slivers and near-coincident
// corners included — the input shape that synthetic small-integer rectangles
// cannot express.
//
// Checked against a rasterization, which shares no code with the boundary walk.
// This is the test that catches an outline silently opening: the reported area
// is then not slightly wrong but unbounded, because the area integral is taken
// about the world origin and scales with the model's distance from it.
func TestUnionAreaOnRealFacadePanel(t *testing.T) {
	tris := loadProjectedTris(t, "testdata/silhouette_panel_tris.txt")
	if len(tris) != 30 {
		t.Fatalf("fixture changed: %d triangles", len(tris))
	}

	var sum float64
	for _, tr := range tris {
		sum += math.Abs(signedArea2(tr)) / 2
	}
	want := rasterCoveredArea(tris, 1200)

	// The contract: a correct area, or an honest refusal. Never a wrong number.
	// This panel is one the boundary walk cannot currently close, so today it
	// must refuse; when the engine is re-founded on exact arithmetic it will
	// start answering, and this test keeps holding without being touched.
	got, ok := unionArea2D(tris)
	t.Logf("union %.6f ok=%v  raster %.6f  Σ %.6f", got, ok, want, sum)
	if !ok {
		return
	}
	if got > sum {
		t.Errorf("union %v exceeds the Σ of its triangles %v", got, sum)
	}
	if math.Abs(got-want) > 0.02*want {
		t.Errorf("union %v disagrees with the rasterized area %v by more than 2%%", got, want)
	}
}

// TestUnionAreaRefusesAnOpenBoundary pins the refusal itself, so that "ok" can
// never quietly become an unconditional true. The area an unclosed boundary
// would otherwise report is not merely wrong, it is unbounded: Green's theorem
// is integrated about the world origin, so the residual scales with how far the
// model sits from it. This panel is 47 m out and reads 30.69 m² against a true
// 0.58 m².
func TestUnionAreaRefusesAnOpenBoundary(t *testing.T) {
	tris := loadProjectedTris(t, "testdata/silhouette_panel_tris.txt")
	if _, ok := unionArea2D(tris); ok {
		t.Skip("engine now closes this boundary — replace this test with the value assertion")
	}
	if segs, ok := unionBoundary(tris); ok || len(segs) != 0 {
		t.Errorf("unionBoundary = (%d segs, ok=%v), want (0, false)", len(segs), ok)
	}
}
