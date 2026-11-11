package geometry

import (
	"bytes"
	"math"
	"testing"
)

func checkWallMesh(t *testing.T, name string, w Wall) {
	t.Helper()
	e := w.Element
	if len(e.Tris) == 0 {
		t.Fatalf("%s: no mesh", name)
	}
	if !isClosedManifold(e.Verts, e.Tris) {
		t.Fatalf("%s: wall mesh is not a closed shell", name)
	}
	v, ok := meshVolume(e.Verts, e.Tris)
	if !ok || !nearRel(v, w.Quantities.NetVolume, 1e-6) {
		t.Errorf("%s: mesh volume %v (ok %v), want NetVolume %v", name, v, ok, w.Quantities.NetVolume)
	}
}

func TestWallMeshIsClosedAndMatches(t *testing.T) {
	for _, tc := range wallOracle() {
		for _, side := range []Side{SideLeft, SideRight} {
			w, err := BuildWall(tc.edge, 0, tc.height, tc.thickness, side, tc.ops)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			checkWallMesh(t, tc.name, w)
		}
	}
}

func TestWallDerivedAreaMatches(t *testing.T) {
	w, err := BuildWall(doorEdge, 0, 2.60, 0.10, SideLeft, []Opening{door})
	if err != nil {
		t.Fatal(err)
	}
	w.Element.GlobalID = "w1"
	s := &Scene{Elements: []Element{w.Element}}
	q, ok := s.DerivedQuantities()["w1"]
	if !ok || q.Area == nil || q.Volume == nil {
		t.Fatalf("DerivedQuantities = %+v", q)
	}
	if !nearRel(*q.Area, 4.64, 1e-6) || !nearRel(*q.Volume, 0.464, 1e-6) {
		t.Errorf("derived area %v volume %v, want 4.64 and 0.464", *q.Area, *q.Volume)
	}
}

func TestWallFaceData(t *testing.T) {
	w, err := BuildWall(doorEdge, 3.0, 2.60, 0.10, SideLeft, []Opening{door})
	if err != nil {
		t.Fatal(err)
	}
	f := w.Measured
	if f.Origin != [3]float64{0, 0, 3.0} || f.U != [3]float64{1, 0, 0} || f.V != [3]float64{0, 0, 1} {
		t.Errorf("frame = %+v", f)
	}
	e := w.Element
	centre := [3]float64{(e.BBoxMin[0] + e.BBoxMax[0]) / 2, (e.BBoxMin[1] + e.BBoxMax[1]) / 2, 0}
	if d := f.Normal[0]*(centre[0]-f.Origin[0]) + f.Normal[1]*(centre[1]-f.Origin[1]); d >= 0 {
		t.Errorf("Normal %v points into the wall body", f.Normal)
	}
	want := [][2]float64{{0, 0}, {2.40, 0}, {2.40, 2.60}, {0, 2.60}}
	for i := range want {
		if f.Outline[i] != want[i] {
			t.Errorf("Outline = %v, want %v", f.Outline, want)
			break
		}
	}
	if len(f.Openings) != 1 || f.Openings[0] != door {
		t.Errorf("Openings = %v", f.Openings)
	}
	if !near(e.BBoxMin[2], 3.0, 1e-6) || !near(e.BBoxMax[2], 5.6, 1e-6) {
		t.Errorf("BBox z = [%v, %v], want [3.0, 5.6]", e.BBoxMin[2], e.BBoxMax[2])
	}
}

func TestWallGLB(t *testing.T) {
	w, err := BuildWall(doorEdge, 0, 2.60, 0.10, SideLeft, []Opening{door})
	if err != nil {
		t.Fatal(err)
	}
	s, err := BuildSlab(rect54, 0, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	w.Element.GlobalID, s.Element.GlobalID = "w1", "s1"
	var buf bytes.Buffer
	if err := (&Scene{Elements: []Element{w.Element, s.Element}}).WriteGLB(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("glTF")) {
		t.Errorf("output does not start with glTF")
	}
}

func TestWallRotated(t *testing.T) {
	a := 30 * math.Pi / 180
	edge := [2][2]float64{{1, 2}, {1 + 2.40*math.Cos(a), 2 + 2.40*math.Sin(a)}}
	w, err := BuildWall(edge, 0, 2.60, 0.10, SideRight, []Opening{door})
	if err != nil {
		t.Fatal(err)
	}
	if !near(w.Quantities.MeasuredNet, 4.64, 1e-9) || !near(w.Quantities.NetVolume, 0.464, 1e-9) {
		t.Errorf("rotated quantities %+v", w.Quantities)
	}
	checkWallMesh(t, "rotated", w)
}

func TestWallFarFromOrigin(t *testing.T) {
	edge := [2][2]float64{{1e6, 1e6}, {1e6 + 2.40, 1e6}}
	w, err := BuildWall(edge, 3.0, 2.60, 0.10, SideLeft, []Opening{door})
	if err != nil {
		t.Fatal(err)
	}
	checkWallMesh(t, "far", w)
	if !near(w.Element.BBoxMin[2], 3.0, 1e-6) || !near(w.Element.BBoxMin[0], 1e6, 1e-6) {
		t.Errorf("BBoxMin = %v", w.Element.BBoxMin)
	}
}

func TestWallOpeningOrder(t *testing.T) {
	edge := [2][2]float64{{0, 0}, {4, 0}}
	window := Opening{Offset: 2.5, Sill: 0.9, Width: 1.0, Height: 1.2}
	sorted, err := BuildWall(edge, 0, 2.6, 0.1, SideLeft, []Opening{door, window})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := BuildWall(edge, 0, 2.6, 0.1, SideLeft, []Opening{window, door})
	if err != nil {
		t.Fatal(err)
	}
	if sorted.Quantities != reversed.Quantities {
		t.Errorf("order changes quantities: %+v vs %+v", sorted.Quantities, reversed.Quantities)
	}
	checkWallMesh(t, "reversed", reversed)
	_, err = BuildWall(edge, 0, 2.6, 0.1, SideLeft, []Opening{window, {Offset: 3.9, Sill: 0, Width: 0.5, Height: 2}})
	wantRefusals(t, err, PlacementError{Wall: 0, Opening: 1, Reason: ReasonTooWide})
}

func TestWallManyOpenings(t *testing.T) {
	ops := make([]Opening, maxOpenings)
	for i := range ops {
		ops[i] = Opening{Offset: 0.1 + float64(i)*0.1, Sill: 0.9, Width: 0.05, Height: 1.0}
	}
	w, err := BuildWall([2][2]float64{{0, 0}, {30, 0}}, 0, 2.6, 0.1, SideLeft, ops)
	if err != nil {
		t.Fatal(err)
	}
	checkWallMesh(t, "256 openings", w)
}

// Three stacked openings whose jambs sit 0.08 and 0.07 mm apart: the grid
// merges those lines, and every hole must still be decided by grid index.
func TestWallStackedOpeningsNearlyAligned(t *testing.T) {
	ops := []Opening{{0.5, 1.2, 0.3, 0.3, 0}, {0.50008, 0, 0.3, 0.3, 0}, {0.50015, 0.7, 0.3, 0.3, 0}}
	w, err := BuildWall([2][2]float64{{0, 0}, {3, 0}}, 0, 3, 0.2, SideLeft, ops)
	if err != nil {
		t.Fatal(err)
	}
	if !isClosedManifold(w.Element.Verts, w.Element.Tris) {
		t.Fatal("mesh is not a closed shell")
	}
}

// staircase is 256 openings on a 30 m wall, each at its own u and v, so the
// mesh grid is about 516 × 514 cells: the worst case the opening cap allows.
func staircase() []Opening {
	ops := make([]Opening, maxOpenings)
	for i := range ops {
		ops[i] = Opening{Offset: 0.1 + 0.1*float64(i), Sill: 0.01 + 0.009*float64(i), Width: 0.05, Height: 0.5}
	}
	return ops
}

func TestWallStaircaseIsBounded(t *testing.T) {
	w, err := BuildWall([2][2]float64{{0, 0}, {30, 0}}, 0, 3, 0.2, SideLeft, staircase())
	if err != nil {
		t.Fatal(err)
	}
	checkWallMesh(t, "staircase", w)
}
