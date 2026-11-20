package geometry

import (
	"errors"
	"math"
	"math/rand"
	"testing"
)

var (
	skelS = math.Tan(30 * math.Pi / 180)
	roofL = [][2]float64{{0, 0}, {18, 0}, {18, 6}, {12, 6}, {12, 10}, {0, 10}}
	roofE = [][2]float64{{0, 0}, {18, 0}, {18, 6}, {6, 6}, {6, 14}, {0, 14}}
)

// skelSpeeds is 1 for every edge but the gables.
func skelSpeeds(n int, gables ...int) []float64 {
	speed := make([]float64, n)
	for i := range speed {
		speed[i] = 1
	}
	for _, g := range gables {
		speed[g] = 0
	}
	return speed
}

// skelLines returns the arcs as plan segments with heights, and fails the
// test when an arc is not on the boundary of the face of each sloped edge it
// separates.
func skelLines(t *testing.T, faces []skelFace, arcs []skelArc) [][2][3]float64 {
	t.Helper()
	byEdge := map[int][][3]float64{}
	for _, f := range faces {
		byEdge[f.Edge] = f.Poly
	}
	lines := make([][2][3]float64, 0, len(arcs))
	for _, a := range arcs {
		for _, e := range [2]int{a.Left, a.Right} {
			if poly, ok := byEdge[e]; ok && (!onFaceBoundary(poly, a.A) || !onFaceBoundary(poly, a.B)) {
				t.Errorf("arc %v→%v is not on the boundary of face %d %v", a.A, a.B, e, poly)
			}
		}
		lines = append(lines, [2][3]float64{a.A, a.B})
	}
	return lines
}

func onFaceBoundary(poly [][3]float64, p [3]float64) bool {
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		if segmentDistance([2]float64{p[0], p[1]}, [2]float64{a[0], a[1]}, [2]float64{b[0], b[1]}) < 1e-9 {
			return true
		}
	}
	return false
}

func near3(a, b [3]float64) bool {
	return math.Abs(a[0]-b[0]) < 1e-9 && math.Abs(a[1]-b[1]) < 1e-9 && math.Abs(a[2]-b[2]) < 1e-9
}

// hasLine reports whether lines holds the segment a–b, either way round.
func hasLine(lines [][2][3]float64, a, b [3]float64) bool {
	for _, l := range lines {
		if (near3(l[0], a) && near3(l[1], b)) || (near3(l[0], b) && near3(l[1], a)) {
			return true
		}
	}
	return false
}

func wantLines(t *testing.T, lines [][2][3]float64, want [][2][3]float64) {
	t.Helper()
	for _, w := range want {
		if !hasLine(lines, w[0], w[1]) {
			t.Errorf("no arc %v→%v in %v", w[0], w[1], lines)
		}
	}
}

// buildSkeleton runs the skeleton and checks what every result must hold:
// each face point is as high as slope × its distance to the face's edge line,
// and the faces tile the ring.
func buildSkeleton(t *testing.T, ring [][2]float64, speed []float64, b *budget) ([]skelFace, []skelArc) {
	t.Helper()
	faces, arcs, err := weightedSkeleton(ring, speed, skelS, b)
	if err != nil {
		t.Fatalf("weightedSkeleton: %v", err)
	}
	checkSkeletonFaces(t, ring, speed, faces)
	return faces, arcs
}

func checkSkeletonFaces(t *testing.T, ring [][2]float64, speed []float64, faces []skelFace) {
	t.Helper()
	n := len(ring)
	var total float64
	for _, f := range faces {
		if speed[f.Edge] == 0 {
			t.Errorf("face for gable edge %d", f.Edge)
		}
		a, b := ring[f.Edge], ring[(f.Edge+1)%n]
		l := math.Hypot(b[0]-a[0], b[1]-a[1])
		plan := make([][2]float64, len(f.Poly))
		for i, p := range f.Poly {
			plan[i] = [2]float64{p[0], p[1]}
			d := ((b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0])) / l
			if math.Abs(p[2]-skelS*d) > 1e-9 {
				t.Errorf("face %d point %v: z %v, want %v", f.Edge, p, p[2], skelS*d)
			}
		}
		total += polygonArea2D(plan)
	}
	if want := polygonArea2D(ring); math.Abs(total-want) > 1e-9*want {
		t.Errorf("faces cover %v, ring is %v", total, want)
	}
}

func faceEdges(faces []skelFace) map[int]bool {
	got := map[int]bool{}
	for _, f := range faces {
		got[f.Edge] = true
	}
	return got
}

func TestSkeletonRectangleHip(t *testing.T) {
	faces, arcs := buildSkeleton(t, roofR, skelSpeeds(4), nil)
	if len(faces) != 4 {
		t.Fatalf("%d faces, want 4", len(faces))
	}
	lines := skelLines(t, faces, arcs)
	if len(lines) != 5 {
		t.Errorf("%d arcs, want 5: %v", len(lines), lines)
	}
	z := 4 * skelS
	wantLines(t, lines, [][2][3]float64{
		{{4, 4, z}, {6, 4, z}},
		{{0, 0, 0}, {4, 4, z}},
		{{10, 0, 0}, {6, 4, z}},
		{{10, 8, 0}, {6, 4, z}},
		{{0, 8, 0}, {4, 4, z}},
	})
}

func TestSkeletonSquarePyramid(t *testing.T) {
	square := [][2]float64{{0, 0}, {8, 0}, {8, 8}, {0, 8}}
	faces, arcs := buildSkeleton(t, square, skelSpeeds(4), nil)
	if len(faces) != 4 {
		t.Fatalf("%d faces, want 4", len(faces))
	}
	lines := skelLines(t, faces, arcs)
	if len(lines) != 4 {
		t.Errorf("%d arcs, want 4: %v", len(lines), lines)
	}
	apex := [3]float64{4, 4, 4 * skelS}
	for _, l := range lines {
		if !near3(l[0], apex) && !near3(l[1], apex) {
			t.Errorf("arc %v→%v misses the apex", l[0], l[1])
		}
		if math.Abs(l[0][2]-l[1][2]) < 1e-9 {
			t.Errorf("horizontal arc %v→%v", l[0], l[1])
		}
	}
}

func TestSkeletonL(t *testing.T) {
	faces, arcs := buildSkeleton(t, roofL, skelSpeeds(6), nil)
	lines := skelLines(t, faces, arcs)
	z3, z5 := 3*skelS, 5*skelS
	want := [][2][3]float64{
		{{15, 3, z3}, {9, 3, z3}}, // ridges
		{{5, 5, z5}, {7, 5, z5}},
		{{12, 6, 0}, {9, 3, z3}},  // valley
		{{18, 0, 0}, {15, 3, z3}}, // hips
		{{18, 6, 0}, {15, 3, z3}},
		{{0, 0, 0}, {5, 5, z5}},
		{{0, 10, 0}, {5, 5, z5}},
		{{12, 10, 0}, {7, 5, z5}},
		{{9, 3, z3}, {7, 5, z5}},
	}
	wantLines(t, lines, want)
	if len(lines) != len(want) {
		t.Errorf("%d arcs, want %d: %v", len(lines), len(want), lines)
	}
}

func TestSkeletonEqualArmCrossGable(t *testing.T) {
	faces, arcs := buildSkeleton(t, roofE, skelSpeeds(6, 1, 4), nil)
	if got := faceEdges(faces); len(got) != 4 || !got[0] || !got[2] || !got[3] || !got[5] {
		t.Errorf("faces for edges %v, want 0, 2, 3, 5", got)
	}
	z := 3 * skelS
	wantLines(t, skelLines(t, faces, arcs), [][2][3]float64{
		{{18, 3, z}, {3, 3, z}},  // ridge y=3
		{{3, 14, z}, {3, 3, z}},  // ridge x=3
		{{6, 6, 0}, {3, 3, z}},   // valley
		{{0, 0, 0}, {3, 3, z}},   // hip
		{{18, 0, 0}, {18, 3, z}}, // verges
		{{18, 6, 0}, {18, 3, z}},
		{{6, 14, 0}, {3, 14, z}},
		{{0, 14, 0}, {3, 14, z}},
	})
}

func TestSkeletonMono(t *testing.T) {
	faces, arcs := buildSkeleton(t, roofR, skelSpeeds(4, 1, 2, 3), nil)
	if len(faces) != 1 || faces[0].Edge != 0 {
		t.Fatalf("faces %v, want one for edge 0", faces)
	}
	z := 8 * skelS
	wantLines(t, skelLines(t, faces, arcs), [][2][3]float64{{{10, 8, z}, {0, 8, z}}})

	faces, _ = buildSkeleton(t, roofL, skelSpeeds(6, 1, 2, 3, 4, 5), nil)
	if len(faces) != 1 || faces[0].Edge != 0 {
		t.Fatalf("L faces %v, want one for edge 0", faces)
	}
	plan := make([][2]float64, len(faces[0].Poly))
	for i, p := range faces[0].Poly {
		plan[i] = [2]float64{p[0], p[1]}
	}
	if a := polygonArea2D(plan); math.Abs(a-156) > 1e-9 {
		t.Errorf("L mono face area %v, want 156", a)
	}
}

func wantRoofError(t *testing.T, err error, reason RoofReason, edge int) {
	t.Helper()
	var re *RoofError
	if !errors.As(err, &re) {
		t.Fatalf("err = %v, want a *RoofError", err)
	}
	if re.Reason != reason || re.Edge != edge || re.Other != -1 {
		t.Errorf("got %+v, want %s edge %d", *re, reason, edge)
	}
}

func TestSkeletonRefusesAStep(t *testing.T) {
	_, _, err := weightedSkeleton(roofL, skelSpeeds(6, 2), skelS, nil)
	wantRoofError(t, err, ReasonRoofStep, 2)
}

func TestSkeletonNearlyStraightCorner(t *testing.T) {
	ring := [][2]float64{{0, 0}, {5, 0.0001}, {10, 0}, {10, 8}, {0, 8}}
	speed := skelSpeeds(5)
	faces, _, err := weightedSkeleton(ring, speed, skelS, nil)
	if err != nil {
		wantRoofError(t, err, ReasonRoofTooComplex, -1)
		return
	}
	checkSkeletonFaces(t, ring, speed, faces)
}

// skelZigzag is a 256-point comb: a straight bottom edge under a top of 254
// teeth.
func skelZigzag() [][2]float64 {
	ring := [][2]float64{{0, 0}, {253, 0}}
	for k := 0; k <= 253; k++ {
		ring = append(ring, [2]float64{float64(253 - k), 10 + float64(k%2)})
	}
	return ring
}

func TestSkeletonBudget(t *testing.T) {
	ring := skelZigzag()
	if len(ring) != 256 {
		t.Fatalf("%d points", len(ring))
	}
	speed := skelSpeeds(len(ring))
	_, _, err := weightedSkeleton(ring, speed, skelS, sizedBudget(100))
	wantRoofError(t, err, ReasonRoofTooComplex, -1)
	buildSkeleton(t, ring, speed, newBudget())
}

// TestSkeletonTilesOutlines runs shapes full of simultaneous events, and
// random stars, through the checks every result must pass.
func TestSkeletonTilesOutlines(t *testing.T) {
	plus := [][2]float64{{4, 0}, {8, 0}, {8, 4}, {12, 4}, {12, 8}, {8, 8}, {8, 12}, {4, 12}, {4, 8}, {0, 8}, {0, 4}, {4, 4}}
	u := [][2]float64{{0, 0}, {12, 0}, {12, 10}, {8, 10}, {8, 4}, {4, 4}, {4, 10}, {0, 10}}
	star := make([][2]float64, 20)
	for i := range star {
		r := 10 - 6*float64(i%2)
		a := float64(i) * math.Pi / 10
		star[i] = [2]float64{r * math.Cos(a), r * math.Sin(a)}
	}
	tests := []struct {
		name  string
		ring  [][2]float64
		speed []float64
	}{
		{"plus", plus, skelSpeeds(12)},
		{"plus, four gables", plus, skelSpeeds(12, 0, 3, 6, 9)},
		{"U, gables on the arm ends", u, skelSpeeds(8, 2, 6)},
		{"star", star, skelSpeeds(20)},
		{"256-gon", regularPolygon(256, 10), skelSpeeds(256)},
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		n := 5 + rng.Intn(40)
		ring := make([][2]float64, n)
		for i := range ring {
			a, r := 2*math.Pi*float64(i)/float64(n), 5+5*rng.Float64()
			ring[i] = [2]float64{r * math.Cos(a), r * math.Sin(a)}
		}
		tests = append(tests, struct {
			name  string
			ring  [][2]float64
			speed []float64
		}{"random star", ring, skelSpeeds(n)})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			faces, arcs := buildSkeleton(t, tt.ring, tt.speed, newBudget())
			skelLines(t, faces, arcs)
		})
	}
}

// The tiling check is the one guard against a face gone missing: dropping
// any face of a slender hip strip must fail it.
func TestSkelTilingCatchesMissingFace(t *testing.T) {
	ring := [][2]float64{{0, 0}, {20, 0}, {20, 0.001}, {0, 0.001}}
	faces, _, err := weightedSkeleton(ring, skelSpeeds(len(ring)), skelS, newBudget())
	if err != nil {
		t.Fatal(err)
	}
	s := newSkeleton(ring, skelSpeeds(len(ring)), nil)
	area := func(f skelFace) float64 {
		plan := make([][2]float64, len(f.Poly))
		for i, p := range f.Poly {
			plan[i] = [2]float64{p[0], p[1]}
		}
		return polygonArea2D(plan)
	}
	var total float64
	for _, f := range faces {
		total += area(f)
	}
	if !s.tiles(total) {
		t.Fatalf("the whole face set (area %v) does not tile the ring", total)
	}
	for _, f := range faces {
		if s.tiles(total - area(f)) {
			t.Errorf("the faces tile the ring without edge %d's face (area %v)", f.Edge, area(f))
		}
	}
}

// The result refuses a skeleton whose faces are not the roof: a node off
// the planes of the faces it bounds, or an arc no face walk takes.
func TestSkelResultRefusesAWrongRoof(t *testing.T) {
	cases := []struct {
		name  string
		spoil func(s *skeleton)
	}{
		{"ridge ends 1 cm off", func(s *skeleton) {
			for i := range s.nodes {
				if s.nodes[i].t > 0 {
					s.nodes[i].p[0] += 0.01
				}
			}
		}},
		{"ridge ends 1 cm low", func(s *skeleton) {
			for i := range s.nodes {
				if s.nodes[i].t > 0 {
					s.nodes[i].t -= 0.01
				}
			}
		}},
		{"an arc no walk takes", func(s *skeleton) { s.arcs = append(s.arcs, s.arcs[len(s.arcs)-1]) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSkeleton(roofR, skelSpeeds(4), nil)
			if err := s.run(); err != nil {
				t.Fatal(err)
			}
			c.spoil(s)
			_, _, err := s.result(skelS)
			wantRoofError(t, err, ReasonRoofTooComplex, -1)
		})
	}
}

func ringSize(ring [][2]float64) float64 {
	lo, hi := ring[0], ring[0]
	for _, p := range ring {
		lo = [2]float64{min(lo[0], p[0]), min(lo[1], p[1])}
		hi = [2]float64{max(hi[0], p[0]), max(hi[1], p[1])}
	}
	return math.Hypot(hi[0]-lo[0], hi[1]-lo[1])
}
