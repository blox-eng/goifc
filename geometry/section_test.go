package geometry

import (
	"math"
	"reflect"
	"testing"

	"github.com/blox-eng/goifc/model"
)

// elemBox builds an Element from an axis-aligned box with identity placement
// (verts == world).
func elemBox(min, max v3) Element {
	var verts []float32
	w, tris := boxMeshWorld(min, max)
	for _, p := range w {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	return Element{GlobalID: "g", Verts: verts, Tris: tris,
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   [3]float64{min[0], min[1], min[2]}, BBoxMax: [3]float64{max[0], max[1], max[2]}}
}

// hollowBox builds an Element whose mesh is a concentric outer + inner box.
func hollowBox(outMin, outMax, inMin, inMax v3) Element {
	w1, t1 := boxMeshWorld(outMin, outMax)
	w2, t2 := boxMeshWorld(inMin, inMax)
	var verts []float32
	for _, p := range append(append([]v3{}, w1...), w2...) {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	return Element{GlobalID: "h", Verts: verts, Tris: tris,
		Placement: model.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		BBoxMin:   [3]float64{outMin[0], outMin[1], outMin[2]}, BBoxMax: [3]float64{outMax[0], outMax[1], outMax[2]}}
}

func TestFootprintCutWhenPlaneCrosses(t *testing.T) {
	loops := Footprint(elemBox(v3{0, 0, 0}, v3{1, 1, 3}), 1.5)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d: %v", len(loops), loops)
	}
	if loops[0].Role != LoopCut {
		t.Fatalf("want LoopCut, got %q", loops[0].Role)
	}
}

func TestFootprintBelowWhenPlaneAbove(t *testing.T) {
	loops := Footprint(elemBox(v3{0, 0, 0}, v3{1, 1, 1}), 2.0)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d: %v", len(loops), loops)
	}
	if loops[0].Role != LoopBelow {
		t.Fatalf("want LoopBelow, got %q", loops[0].Role)
	}
}

func TestFootprintAabbFallback(t *testing.T) {
	e := Element{BBoxMin: [3]float64{0, 0, 0}, BBoxMax: [3]float64{2, 3, 0}}
	loops := Footprint(e, 1.0)
	if len(loops) != 1 {
		t.Fatalf("want 1 loop, got %d: %v", len(loops), loops)
	}
	if loops[0].Role != LoopBelow {
		t.Fatalf("want LoopBelow, got %q", loops[0].Role)
	}
	if a := ringArea(loops[0].Points); math.Abs(a-6.0) > 1e-6 {
		t.Fatalf("want area 6.0, got %v", a)
	}
}

func TestFootprintHoleNesting(t *testing.T) {
	e := hollowBox(v3{0, 0, 0}, v3{4, 4, 4}, v3{1, 1, 0}, v3{3, 3, 4})
	loops := Footprint(e, 2.0)
	if len(loops) != 2 {
		t.Fatalf("want 2 cut loops, got %d: %v", len(loops), loops)
	}
	var outer, inner int
	for _, l := range loops {
		if l.Role != LoopCut {
			t.Fatalf("want LoopCut, got %q", l.Role)
		}
		if polygonArea2D(l.Points) > 0 {
			outer++
			if a := ringArea(l.Points); math.Abs(a-16.0) > 1e-6 {
				t.Fatalf("outer: want area 16.0, got %v", a)
			}
		} else {
			inner++
			if a := ringArea(l.Points); math.Abs(a-4.0) > 1e-6 {
				t.Fatalf("inner hole: want |area| 4.0, got %v", a)
			}
		}
	}
	if outer != 1 || inner != 1 {
		t.Fatalf("want 1 CCW outer + 1 CW hole, got outer=%d inner=%d", outer, inner)
	}
}

func TestFootprintDeterministic(t *testing.T) {
	e := hollowBox(v3{0, 0, 0}, v3{4, 4, 4}, v3{1, 1, 0}, v3{3, 3, 4})
	if !reflect.DeepEqual(Footprint(e, 2.0), Footprint(e, 2.0)) {
		t.Fatal("Footprint not deterministic")
	}
}

// axis-aligned box mesh [min,max] as world v3 verts + 12 tris.
func boxMeshWorld(min, max v3) ([]v3, []uint32) {
	c := [8]v3{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]}, {max[0], max[1], min[2]}, {min[0], max[1], min[2]},
		{min[0], min[1], max[2]}, {max[0], min[1], max[2]}, {max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	w := c[:]
	tris := []uint32{
		0, 2, 1, 0, 3, 2, 4, 5, 6, 4, 6, 7, // bottom, top
		0, 1, 5, 0, 5, 4, 1, 2, 6, 1, 6, 5, // sides
		2, 3, 7, 2, 7, 6, 3, 0, 4, 3, 4, 7,
	}
	return w, tris
}

func ringArea(r [][2]float64) float64 { // shoelace, abs
	var a float64
	for i := range r {
		j := (i + 1) % len(r)
		a += r[i][0]*r[j][1] - r[j][0]*r[i][1]
	}
	return math.Abs(a) / 2
}

func TestSectionRingsCubeMidCut(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 1})
	rings := sectionRings(w, tris, 0.5)
	if len(rings) != 1 {
		t.Fatalf("want 1 ring, got %d: %v", len(rings), rings)
	}
	if a := ringArea(rings[0]); math.Abs(a-1.0) > 1e-6 {
		t.Fatalf("want area 1.0, got %v", a)
	}
}

func TestSectionRingsPlaneMisses(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 1})
	rings := sectionRings(w, tris, 5.0)
	if len(rings) != 0 {
		t.Fatalf("want 0 rings, got %d: %v", len(rings), rings)
	}
}

func TestSectionRingsTwoDisjointBoxes(t *testing.T) {
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{3, 0, 0}, v3{4, 1, 2})
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	rings := sectionRings(w, tris, 1.0)
	if len(rings) != 2 {
		t.Fatalf("want 2 rings, got %d: %v", len(rings), rings)
	}
	for i, r := range rings {
		if a := ringArea(r); math.Abs(a-1.0) > 1e-6 {
			t.Fatalf("ring %d: want area 1.0, got %v", i, a)
		}
	}
}

func TestSectionRingsTJunction(t *testing.T) {
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{1, 0, 0}, v3{2, 1, 2}) // shares x=1 face
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	rings := sectionRings(w, tris, 1.0)
	if len(rings) == 0 {
		t.Fatalf("want >=1 ring, got 0")
	}
	var total float64
	for _, r := range rings {
		if len(r) < 3 {
			t.Fatalf("degenerate ring: %v", r)
		}
		total += ringArea(r)
	}
	if math.Abs(total-2.0) > 1e-6 {
		t.Fatalf("want total area 2.0, got %v (rings=%v)", total, rings)
	}
}

func TestSectionRingsOnPlaneFace(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 1})
	rings := sectionRings(w, tris, 1.0) // top face coplanar with cut
	// Nothing spans the plane; the box merely touches it -> 0 section rings.
	if len(rings) != 0 {
		t.Fatalf("want 0 rings (coplanar top face skipped), got %d: %v", len(rings), rings)
	}
}

func TestSectionRingsCornerTouch(t *testing.T) {
	// Two boxes touching only along the x=1,y=1 vertical edge. The cut welds a
	// single point (1,1) shared by BOTH boxes -> degree-4 vertex that survives
	// parity cancellation (no full face is shared), forcing nextEdge's
	// smallest-turning-angle choice. A tangled figure-8 walk would fail here.
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{1, 1, 0}, v3{2, 2, 2})
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	rings := sectionRings(w, tris, 1.0)
	if len(rings) != 2 {
		t.Fatalf("want 2 rings (corner-touch resolved cleanly), got %d: %v", len(rings), rings)
	}
	var total float64
	for _, r := range rings {
		if len(r) < 3 {
			t.Fatalf("degenerate ring: %v", r)
		}
		if a := ringArea(r); math.Abs(a-1.0) > 1e-6 {
			t.Fatalf("want each ring area 1.0, got %v", a)
		}
		total += ringArea(r)
	}
	if math.Abs(total-2.0) > 1e-6 {
		t.Fatalf("want total area 2.0, got %v", total)
	}
}

func TestBelowRingsCube(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{2, 3, 1})
	rings := belowRings(w, tris)
	if len(rings) != 1 {
		t.Fatalf("want 1 silhouette ring, got %d", len(rings))
	}
	if got := ringArea(rings[0]); math.Abs(got-6.0) > 1e-6 { // 2*3
		t.Fatalf("want footprint area 6.0, got %v", got)
	}
}

func TestBelowRingsDeterministic(t *testing.T) {
	w, tris := boxMeshWorld(v3{0, 0, 0}, v3{2, 3, 1})
	if !reflect.DeepEqual(belowRings(w, tris), belowRings(w, tris)) {
		t.Fatal("belowRings not deterministic")
	}
}

func TestAabbRing(t *testing.T) {
	r := aabbRing([3]float64{1, 1, 0}, [3]float64{4, 3, 2})
	if got := ringArea(r); math.Abs(got-6.0) > 1e-6 {
		t.Fatalf("want 6.0, got %v", got)
	}
}

func TestRingSelfIntersects(t *testing.T) {
	// bowtie: edges (0,0)-(1,1) and (1,0)-(0,1) cross
	bowtie := [][2]float64{{0, 0}, {1, 1}, {1, 0}, {0, 1}}
	if !ringSelfIntersects(bowtie) {
		t.Fatalf("bowtie not detected")
	}
	square := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	if ringSelfIntersects(square) {
		t.Fatalf("simple square flagged as self-intersecting")
	}
}

func TestSectionRingsDeterministic(t *testing.T) {
	w1, t1 := boxMeshWorld(v3{0, 0, 0}, v3{1, 1, 2})
	w2, t2 := boxMeshWorld(v3{1, 0, 0}, v3{2, 1, 2})
	w := append(append([]v3{}, w1...), w2...)
	off := uint32(len(w1))
	tris := append([]uint32{}, t1...)
	for _, i := range t2 {
		tris = append(tris, i+off)
	}
	r1 := sectionRings(w, tris, 1.0)
	r2 := sectionRings(w, tris, 1.0)
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("non-deterministic output:\n r1=%v\n r2=%v", r1, r2)
	}
}
