package geometry

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
)

// joinT is the through wall: its measured face lies on y = 0, its body in
// y ∈ [−0.2, 0].
var joinT = PlanWall{ID: "T", Edge: [2][2]float64{{0, 0}, {4, 0}}, Height: 2.5, Thickness: 0.2, Side: SideLeft}

// joinP is a 0.1 m partition from a to b whose end b meets T.
func joinP(a, b [2]float64, side Side) PlanWall {
	return PlanWall{ID: "P", Edge: [2][2]float64{a, b}, Height: 2.5, Thickness: 0.1, Side: side, Joins: []Join{{End: EndB, Wall: "T"}}}
}

// builtWall returns the wall built with this ID.
func builtWall(t *testing.T, b Built, id string) Wall {
	t.Helper()
	for _, w := range b.Walls {
		if w.ID == id {
			return w.Wall
		}
	}
	t.Fatalf("no wall %q built", id)
	return Wall{}
}

// joinErrors returns every *JoinError in err.
func joinErrors(err error) []*JoinError {
	var out []*JoinError
	for _, e := range unjoin(err) {
		var je *JoinError
		if errors.As(e, &je) {
			out = append(out, je)
		}
	}
	return out
}

func TestJoinSquare(t *testing.T) {
	p := joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	if err != nil {
		t.Fatal(err)
	}
	w := builtWall(t, got, "P")
	q := w.Quantities
	if !near(q.Length, 3, 1e-12) || !near(q.MeasuredGross, 7.5, 1e-12) || !near(q.GrossVolume, 0.75, 1e-12) {
		t.Errorf("Length %v MeasuredGross %v GrossVolume %v, want 3, 7.5, 0.75", q.Length, q.MeasuredGross, q.GrossVolume)
	}
	want, err := BuildWall(p.Edge, 0, 2.5, 0.1, SideLeft, nil)
	if err != nil {
		t.Fatal(err)
	}
	want.Element.GlobalID = "P"
	if !reflect.DeepEqual(w, want) {
		t.Error("a square join differs from BuildWall of the same edge")
	}
}

func TestJoinAngled(t *testing.T) {
	l := 3 * math.Sqrt2
	for _, c := range []struct {
		side  Side
		other float64
	}{
		{SideLeft, l + 0.1},
		{SideRight, l - 0.1},
	} {
		got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, joinP([2]float64{5, 3}, [2]float64{2, 0}, c.side)}})
		if err != nil {
			t.Fatalf("side %v: %v", c.side, err)
		}
		q := builtWall(t, got, "P").Quantities
		if !near(q.Length, l, 1e-9) {
			t.Errorf("side %v: Length %v, want %v", c.side, q.Length, l)
		}
		if !near(q.OtherGross, c.other*2.5, 1e-9) {
			t.Errorf("side %v: OtherGross %v, want %v", c.side, q.OtherGross, c.other*2.5)
		}
		if v := (l + c.other) / 2 * 0.1 * 2.5; !near(q.GrossVolume, v, 1e-9) {
			t.Errorf("side %v: GrossVolume %v, want %v", c.side, q.GrossVolume, v)
		}
	}
}

// End A trims the opposite face's start: the mirror of TestJoinAngled.
func TestJoinAngledEndA(t *testing.T) {
	l := 3 * math.Sqrt2
	p := PlanWall{ID: "P", Edge: [2][2]float64{{2, 0}, {5, 3}}, Height: 2.5, Thickness: 0.1, Side: SideRight, Joins: []Join{{End: EndA, Wall: "T"}}}
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	if err != nil {
		t.Fatal(err)
	}
	q := builtWall(t, got, "P").Quantities
	if !near(q.Length, l, 1e-9) || !near(q.OtherGross, (l+0.1)*2.5, 1e-9) {
		t.Errorf("Length %v OtherGross %v, want %v and %v", q.Length, q.OtherGross, l, (l+0.1)*2.5)
	}
}

func TestJoinSnaps(t *testing.T) {
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, joinP([2]float64{2, 3}, [2]float64{2, 0.0009}, SideLeft)}})
	if err != nil {
		t.Fatal(err)
	}
	if l := builtWall(t, got, "P").Quantities.Length; !near(l, 3, 1e-9) {
		t.Errorf("Length %v, want 3", l)
	}

	a := PlanWall{ID: "P", Edge: [2][2]float64{{2, 0.0009}, {2, 3}}, Height: 2.5, Thickness: 0.1, Side: SideRight, Joins: []Join{{End: EndA, Wall: "T"}}}
	got, err = BuildPlan(Plan{Walls: []PlanWall{joinT, a}})
	if err != nil {
		t.Fatal(err)
	}
	w := builtWall(t, got, "P")
	if l := w.Quantities.Length; !near(l, 3, 1e-9) {
		t.Errorf("end A: Length %v, want 3", l)
	}
	if o := w.Measured.Origin; !near(o[0], 2, 1e-12) || !near(o[1], 0, 1e-12) {
		t.Errorf("end A: measured origin %v, want (2, 0)", o)
	}
}

func TestJoinLayeredPartition(t *testing.T) {
	p := joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)
	p.Thickness, p.Layers = 0, []float64{0.0125, 0.075, 0.0125}
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := BuildWallLayers(p.Edge, 0, 2.5, p.Layers, SideLeft, nil)
	if err != nil {
		t.Fatal(err)
	}
	for j, w := range want {
		id := "P/" + string(rune('0'+j))
		w.Element.GlobalID = id
		if !reflect.DeepEqual(builtWall(t, got, id), w) {
			t.Errorf("layer %d differs from BuildWallLayers", j)
		}
	}
}

func TestJoinTrimRefusesOpening(t *testing.T) {
	l := 3 * math.Sqrt2
	p := joinP([2]float64{5, 3}, [2]float64{2, 0}, SideRight)
	p.Openings = []Opening{{Offset: l - 0.55, Width: 0.5, Height: 2}}
	if _, err := BuildWall(p.Edge, 0, 2.5, 0.1, SideRight, p.Openings); err != nil {
		t.Fatalf("the untrimmed wall should take the opening: %v", err)
	}
	_, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	var ie *ItemError
	var pe *PlacementError
	if !errors.As(err, &ie) || ie.Item != "P" || !errors.As(ie, &pe) || pe.Reason != ReasonInCorner {
		t.Fatalf("got %v, want P refused %s", err, ReasonInCorner)
	}
}

func TestJoinRefusals(t *testing.T) {
	tan10 := math.Tan(10 * math.Pi / 180)
	for _, c := range []struct {
		name   string
		p      PlanWall
		reason JoinReason
	}{
		{"unknown wall", withJoin(joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft), "nope"), ReasonUnknownWall},
		{"itself", withJoin(joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft), "P"), ReasonUnknownWall},
		{"2 mm off the face", joinP([2]float64{2, 3}, [2]float64{2, 0.002}, SideLeft), ReasonNotOnFace},
		{"inside the wall", joinP([2]float64{2, 3}, [2]float64{2, -0.1}, SideLeft), ReasonNotOnFace},
		{"through the wall", joinP([2]float64{2, -3}, [2]float64{2, 0}, SideLeft), ReasonNotOnFace},
		{"10°", joinP([2]float64{2 + 3/tan10, 3}, [2]float64{2, 0}, SideLeft), ReasonTooShallow},
	} {
		_, err := BuildPlan(Plan{Walls: []PlanWall{joinT, c.p}})
		got := joinErrors(err)
		if len(got) != 1 || *got[0] != (JoinError{Item: "P", Join: 0, Reason: c.reason}) {
			t.Errorf("%s: got %v, want P join 0: %s", c.name, err, c.reason)
		}
	}
}

func withJoin(p PlanWall, wall string) PlanWall {
	p.Joins = []Join{{End: EndB, Wall: wall}}
	return p
}

func TestJoinRefusalsAllAtOnce(t *testing.T) {
	off := joinP([2]float64{2, 3}, [2]float64{2, 0.002}, SideLeft)
	off.ID = "P1"
	shallow := joinP([2]float64{2 + 3/math.Tan(10*math.Pi/180), 3}, [2]float64{2, 0}, SideLeft)
	shallow.ID = "P2"
	_, err := BuildPlan(Plan{Walls: []PlanWall{joinT, off, shallow}})
	got := joinErrors(err)
	want := []JoinError{{Item: "P1", Reason: ReasonNotOnFace}, {Item: "P2", Reason: ReasonTooShallow}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	for i := range want {
		if *got[i] != want[i] {
			t.Errorf("refusal %d: %v, want %v", i, got[i], want[i])
		}
	}
}

func TestJoinSkipsFailedWalls(t *testing.T) {
	bad := joinT
	bad.Thickness = -1
	_, err := BuildPlan(Plan{Walls: []PlanWall{bad, joinP([2]float64{2, 3}, [2]float64{2, 0.002}, SideLeft)}})
	if err == nil || len(joinErrors(err)) != 0 {
		t.Errorf("got %v, want T's refusal and no join refusal", err)
	}
}

func TestJoinMalformed(t *testing.T) {
	p := joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)
	for _, c := range []struct {
		name  string
		joins []Join
	}{
		{"no end", []Join{{Wall: "T"}}},
		{"same end twice", []Join{{End: EndB, Wall: "T"}, {End: EndB, Wall: "T"}}},
		{"three joins", []Join{{End: EndA, Wall: "T"}, {End: EndB, Wall: "T"}, {End: EndB, Wall: "T"}}},
	} {
		p.Joins = c.joins
		_, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
		var ie *ItemError
		if !errors.As(err, &ie) || ie.Item != "P" {
			t.Errorf("%s: got %v, want P refused", c.name, err)
		}
	}
}

func TestEndText(t *testing.T) {
	for _, e := range []End{EndA, EndB} {
		b, err := e.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var back End
		if err := back.UnmarshalText(b); err != nil || back != e {
			t.Errorf("%q read back as %v, %v", b, back, err)
		}
	}
	if _, err := End(0).MarshalText(); err == nil {
		t.Error("End(0) marshalled")
	}
	var e End
	if err := e.UnmarshalText([]byte("c")); err == nil {
		t.Error(`"c" read as an end`)
	}
}

// joinFixture is T with a square 0.1 m partition P meeting its measured face
// at x = 2: the strip is x ∈ [1.9, 2], 2.5 m tall.
func joinFixture() Plan {
	return Plan{Walls: []PlanWall{joinT, joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)}}
}

// otherP is a square 0.1 m partition meeting T's opposite face (y = −0.2) in
// x ∈ [1.9, 2].
func otherP() PlanWall {
	return joinP([2]float64{2, -3}, [2]float64{2, -0.2}, SideRight)
}

// checkFaces checks a wall's join and net areas on both faces.
func checkFaces(t *testing.T, name string, q WallQuantities, mJoins, mNet, oJoins, oNet float64) {
	t.Helper()
	if !near(q.MeasuredJoins, mJoins, 1e-9) || !near(q.MeasuredNet, mNet, 1e-9) || !near(q.OtherJoins, oJoins, 1e-9) || !near(q.OtherNet, oNet, 1e-9) {
		t.Errorf("%s: MeasuredJoins %v MeasuredNet %v OtherJoins %v OtherNet %v, want %v %v %v %v",
			name, q.MeasuredJoins, q.MeasuredNet, q.OtherJoins, q.OtherNet, mJoins, mNet, oJoins, oNet)
	}
}

// plainT is T built alone.
func plainT(t *testing.T) Wall {
	t.Helper()
	w, err := BuildWall(joinT.Edge, 0, 2.5, 0.2, SideLeft, nil)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestStripMeasured(t *testing.T) {
	got, err := BuildPlan(joinFixture())
	if err != nil {
		t.Fatal(err)
	}
	w := builtWall(t, got, "T")
	checkFaces(t, "T", w.Quantities, 0.25, 9.75, 0, 10)
	want := plainT(t)
	if w.Quantities.GrossVolume != want.Quantities.GrossVolume || w.Quantities.NetVolume != want.Quantities.NetVolume ||
		w.Quantities.MeasuredGross != want.Quantities.MeasuredGross || !reflect.DeepEqual(w.Element, func() Element { e := want.Element; e.GlobalID = "T"; return e }()) {
		t.Error("the strip changed T's volumes, gross areas or mesh")
	}
}

func TestStripOther(t *testing.T) {
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, otherP()}})
	if err != nil {
		t.Fatal(err)
	}
	checkFaces(t, "T", builtWall(t, got, "T").Quantities, 0, 10, 0.25, 9.75)
}

func TestStripAngled(t *testing.T) {
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, joinP([2]float64{5, 3}, [2]float64{2, 0}, SideLeft)}})
	if err != nil {
		t.Fatal(err)
	}
	a := 0.1 / math.Sin(math.Pi/4) * 2.5
	checkFaces(t, "T", builtWall(t, got, "T").Quantities, a, 10-a, 0, 10)
}

func TestStripVerticalOverlap(t *testing.T) {
	p := joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)
	p.Base, p.Height = 1, 3
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	if err != nil {
		t.Fatal(err)
	}
	checkFaces(t, "T", builtWall(t, got, "T").Quantities, 0.15, 9.85, 0, 10)
}

func TestStripLayeredThrough(t *testing.T) {
	tl := joinT
	tl.Thickness, tl.Layers = 0, []float64{0.015, 0.17, 0.015}
	alone, err := BuildWallLayers(tl.Edge, 0, 2.5, tl.Layers, SideLeft, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		p     PlanWall
		layer string
		other bool
	}{
		{"measured side", joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft), "T/0", false},
		{"other side", otherP(), "T/2", true},
	} {
		got, err := BuildPlan(Plan{Walls: []PlanWall{tl, c.p}})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		q := builtWall(t, got, c.layer).Quantities
		if c.other {
			checkFaces(t, c.name, q, 0, alone[2].Quantities.MeasuredNet, 0.25, 9.75)
		} else {
			checkFaces(t, c.name, q, 0.25, 9.75, 0, alone[0].Quantities.OtherNet)
		}
		if q := builtWall(t, got, "T/1").Quantities; q != alone[1].Quantities {
			t.Errorf("%s: layer 1 changed: %+v", c.name, q)
		}
	}
}

func TestStripLayeredPartition(t *testing.T) {
	p := joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)
	p.Thickness, p.Layers = 0, []float64{0.0125, 0.075, 0.0125}
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	if err != nil {
		t.Fatal(err)
	}
	checkFaces(t, "T", builtWall(t, got, "T").Quantities, 0.25, 9.75, 0, 10)
}

// joinR is a 4 × 3 room measured inside: R/0 runs along y = 0, R/2 along
// y = 3 from x = 4 to 0.
var joinR = PlanRoom{ID: "R", Outline: [][2]float64{{0, 0}, {4, 0}, {4, 3}, {0, 3}}, Height: 2.5, Thickness: 0.2, Side: SideLeft}

func TestStripRoom(t *testing.T) {
	p := PlanWall{ID: "P", Edge: [2][2]float64{{2, 3}, {2, 0}}, Height: 2.5, Thickness: 0.1, Side: SideLeft,
		Joins: []Join{{End: EndA, Wall: "R/2"}, {End: EndB, Wall: "R/0"}}}
	got, err := BuildPlan(Plan{Walls: []PlanWall{p}, Rooms: []PlanRoom{joinR}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"R/0", "R/2"} {
		if q := builtWall(t, got, id).Quantities; !near(q.MeasuredJoins, 0.25, 1e-9) || !near(q.MeasuredNet, 9.75, 1e-9) {
			t.Errorf("%s MeasuredJoins %v MeasuredNet %v, want 0.25 9.75", id, q.MeasuredJoins, q.MeasuredNet)
		}
	}
	if l := builtWall(t, got, "P").Quantities.Length; !near(l, 3, 1e-9) {
		t.Errorf("P Length %v, want 3", l)
	}

	p.Joins[0].Wall = "R/00"
	_, err = BuildPlan(Plan{Walls: []PlanWall{p}, Rooms: []PlanRoom{joinR}})
	if got := joinErrors(err); len(got) != 1 || *got[0] != (JoinError{Item: "P", Join: 0, Reason: ReasonUnknownWall}) {
		t.Errorf(`"R/00": got %v, want P join 0: %s`, err, ReasonUnknownWall)
	}
}

// A layered room after wall items: the measured strips land on layer 0 of
// R/0 and R/2, the opposite one on R/0's last layer, and no other layer
// changes.
func TestStripLayeredRoom(t *testing.T) {
	in := PlanWall{ID: "P", Edge: [2][2]float64{{2, 3}, {2, 0}}, Height: 2.5, Thickness: 0.1, Side: SideLeft,
		Joins: []Join{{End: EndA, Wall: "R/2"}, {End: EndB, Wall: "R/0"}}}
	out := PlanWall{ID: "Q", Edge: [2][2]float64{{2, -3}, {2, -0.2}}, Height: 2.5, Thickness: 0.1, Side: SideRight,
		Joins: []Join{{End: EndB, Wall: "R/0"}}}
	r := joinR
	r.Thickness, r.Layers = 0, []float64{0.015, 0.17, 0.015}
	alone, err := BuildPlan(Plan{Rooms: []PlanRoom{r}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := BuildPlan(Plan{Walls: []PlanWall{in, out}, Rooms: []PlanRoom{r}})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range alone.Walls {
		want := w.Wall.Quantities
		switch w.ID {
		case "R/0/0", "R/2/0":
			want.MeasuredJoins, want.MeasuredNet = 0.25, want.MeasuredNet-0.25
		case "R/0/2":
			want.OtherJoins, want.OtherNet = 0.25, want.OtherNet-0.25
		}
		checkFaces(t, w.ID, builtWall(t, got, w.ID).Quantities, want.MeasuredJoins, want.MeasuredNet, want.OtherJoins, want.OtherNet)
	}
}

// The room's outside corner mitres R/0's opposite face out to x = 4.2, so a
// strip at x ∈ [4.05, 4.15] lies on it; T's square end stops at x = 4.
func TestStripRoomMitredFace(t *testing.T) {
	p := PlanWall{ID: "P", Edge: [2][2]float64{{4.15, -3}, {4.15, -0.2}}, Height: 2.5, Thickness: 0.1, Side: SideRight,
		Joins: []Join{{End: EndB, Wall: "R/0"}}}
	got, err := BuildPlan(Plan{Walls: []PlanWall{p}, Rooms: []PlanRoom{joinR}})
	if err != nil {
		t.Fatal(err)
	}
	if j := builtWall(t, got, "R/0").Quantities.OtherJoins; !near(j, 0.25, 1e-9) {
		t.Errorf("R/0 OtherJoins %v, want 0.25", j)
	}

	p.Joins[0].Wall = "T"
	_, err = BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	if got := joinErrors(err); len(got) != 1 || *got[0] != (JoinError{Item: "P", Join: 0, Reason: ReasonOffWall}) {
		t.Errorf("on T: got %v, want P join 0: %s", err, ReasonOffWall)
	}
}

func TestStripRefusals(t *testing.T) {
	high := joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)
	high.Base = 3
	second := joinP([2]float64{2.05, 3}, [2]float64{2.05, 0}, SideLeft)
	second.ID = "P2"
	for _, c := range []struct {
		name  string
		walls []PlanWall
		want  JoinError
	}{
		{"past T's start", []PlanWall{joinT, joinP([2]float64{0.05, 3}, [2]float64{0.05, 0}, SideLeft)}, JoinError{Item: "P", Reason: ReasonOffWall}},
		{"flush with T's end", []PlanWall{joinT, joinP([2]float64{4, 3}, [2]float64{4, 0}, SideLeft)}, JoinError{Item: "P", Reason: ReasonOffWall}},
		{"above T", []PlanWall{joinT, high}, JoinError{Item: "P", Reason: ReasonOffWall}},
		{"overlapping strips", []PlanWall{joinT, joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft), second}, JoinError{Item: "P2", Reason: ReasonJoinsOverlap}},
	} {
		_, err := BuildPlan(Plan{Walls: c.walls})
		if got := joinErrors(err); len(got) != 1 || *got[0] != c.want {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

// chainP is a 45° partition on T, measured on its upper-left face: its
// opposite face is the line x − y = 2 + 0.1√2 and, trimmed at T, ends at
// y = 0, 0.1 m short of the untrimmed end (y = −0.0707).
var chainP = joinP([2]float64{5, 3}, [2]float64{2, 0}, SideRight)

// chainQ is a horizontal 0.1 m partition below y = yq whose end A meets
// chainP's opposite face. Its strip there ends where the line y = yq − 0.1
// crosses that face.
func chainQ(yq float64) PlanWall {
	return PlanWall{ID: "Q", Edge: [2][2]float64{{yq + 2 + 0.1*math.Sqrt2, yq}, {3.5, yq}}, Height: 2.5, Thickness: 0.1, Side: SideLeft,
		Joins: []Join{{End: EndA, Wall: "P"}}}
}

func TestStripChainAfterTrim(t *testing.T) {
	// yq = 0.07: the strip reaches y = −0.03, past the trimmed end (y = 0)
	// but short of the untrimmed one.
	_, err := BuildPlan(Plan{Walls: []PlanWall{joinT, chainP, chainQ(0.07)}})
	if got := joinErrors(err); len(got) != 1 || *got[0] != (JoinError{Item: "Q", Join: 0, Reason: ReasonOffWall}) {
		t.Errorf("past the trimmed end: got %v, want Q join 0: %s", err, ReasonOffWall)
	}

	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, chainP, chainQ(0.37)}})
	if err != nil {
		t.Fatal(err)
	}
	if j, want := builtWall(t, got, "P").Quantities.OtherJoins, 0.1*math.Sqrt2*2.5; !near(j, want, 1e-9) {
		t.Errorf("P OtherJoins %v, want %v", j, want)
	}
}

func TestStripInJoin(t *testing.T) {
	door := Opening{Offset: 1.8, Width: 0.3, Height: 2}
	tw := joinT
	tw.Openings = []Opening{door}
	for _, c := range []struct {
		name string
		p    PlanWall
	}{
		{"measured face", joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)},
		{"other face", otherP()},
	} {
		_, err := BuildPlan(Plan{Walls: []PlanWall{tw, c.p}})
		var ie *ItemError
		var pe *PlacementError
		if !errors.As(err, &ie) || ie.Item != "T" || !errors.As(ie, &pe) || *pe != (PlacementError{Wall: 0, Opening: 0, Reason: ReasonInJoin}) {
			t.Errorf("%s: got %v, want T wall 0 opening 0: %s", c.name, err, ReasonInJoin)
		}
	}

	// A niche cuts the measured face only.
	tw.Openings = []Opening{{Offset: 1.8, Width: 0.3, Sill: 0.1, Height: 2, Depth: 0.05}}
	if _, err := BuildPlan(Plan{Walls: []PlanWall{tw, otherP()}}); err != nil {
		t.Errorf("a niche on the other face: %v", err)
	}
}

func TestStripCountsTowardOpenings(t *testing.T) {
	tw := joinT
	for i := range maxOpenings {
		tw.Openings = append(tw.Openings, Opening{Offset: 0.01 + 0.0055*float64(i), Width: 0.002, Sill: 1, Height: 0.5})
	}
	if _, err := BuildPlan(Plan{Walls: []PlanWall{tw}}); err != nil {
		t.Fatalf("T alone: %v", err)
	}
	_, err := BuildPlan(Plan{Walls: []PlanWall{tw, joinP([2]float64{2, 3}, [2]float64{2, 0}, SideLeft)}})
	var ie *ItemError
	if !errors.As(err, &ie) || ie.Item != "T" || !errors.Is(err, ErrTooLarge) {
		t.Errorf("got %v, want T refused as too large", err)
	}
}

func TestJoinBothEnds(t *testing.T) {
	t2 := PlanWall{ID: "T2", Edge: [2][2]float64{{0, 3}, {4, 3}}, Height: 2.5, Thickness: 0.2, Side: SideRight}
	p := PlanWall{ID: "P", Edge: [2][2]float64{{3.5, 3}, {0.5, 0}}, Height: 2.5, Thickness: 0.1, Side: SideLeft,
		Joins: []Join{{End: EndA, Wall: "T2"}, {End: EndB, Wall: "T"}}}
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, t2, p}})
	if err != nil {
		t.Fatal(err)
	}
	// Both faces are trimmed 0.1 m along P at each end, the same way: a
	// parallelogram.
	l := 3 * math.Sqrt2
	q := builtWall(t, got, "P").Quantities
	if !near(q.Length, l, 1e-9) || !near(q.OtherGross, l*2.5, 1e-9) || !near(q.GrossVolume, l*0.25, 1e-9) {
		t.Errorf("Length %v OtherGross %v GrossVolume %v, want %v %v %v", q.Length, q.OtherGross, q.GrossVolume, l, l*2.5, l*0.25)
	}
	a := 0.1 * math.Sqrt2 * 2.5
	checkFaces(t, "T", builtWall(t, got, "T").Quantities, a, 10-a, 0, 10)
	checkFaces(t, "T2", builtWall(t, got, "T2").Quantities, a, 10-a, 0, 10)
}

func TestJoinOtherFaceAngled(t *testing.T) {
	l := 3 * math.Sqrt2
	a := 0.1 * math.Sqrt2 * 2.5
	for _, c := range []struct {
		side  Side
		other float64
	}{
		{SideLeft, l - 0.1},
		{SideRight, l + 0.1},
	} {
		got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, joinP([2]float64{5, -3.2}, [2]float64{2, -0.2}, c.side)}})
		if err != nil {
			t.Fatalf("side %v: %v", c.side, err)
		}
		q := builtWall(t, got, "P").Quantities
		if !near(q.Length, l, 1e-9) || !near(q.OtherGross, c.other*2.5, 1e-9) {
			t.Errorf("side %v: Length %v OtherGross %v, want %v %v", c.side, q.Length, q.OtherGross, l, c.other*2.5)
		}
		checkFaces(t, "T", builtWall(t, got, "T").Quantities, 0, 10, a, 10-a)
	}
}

func TestJoinAngleBounds(t *testing.T) {
	for _, c := range []struct {
		deg float64
		ok  bool
	}{{15, true}, {165, true}, {14.99, false}, {165.01, false}} {
		r := c.deg * math.Pi / 180
		p := joinP([2]float64{2 + 3*math.Cos(r), 3 * math.Sin(r)}, [2]float64{2, 0}, SideLeft)
		_, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
		switch got := joinErrors(err); {
		case c.ok && err != nil:
			t.Errorf("%v°: %v", c.deg, err)
		case !c.ok && (len(got) != 1 || got[0].Reason != ReasonTooShallow):
			t.Errorf("%v°: got %v, want %s", c.deg, err, ReasonTooShallow)
		}
	}
}

func TestJoinLayeredAngledVolume(t *testing.T) {
	single := joinP([2]float64{5, 3}, [2]float64{2, 0}, SideLeft)
	layered := single
	layered.Thickness, layered.Layers = 0, []float64{0.0125, 0.075, 0.0125}
	one, err := BuildPlan(Plan{Walls: []PlanWall{joinT, single}})
	if err != nil {
		t.Fatal(err)
	}
	many, err := BuildPlan(Plan{Walls: []PlanWall{joinT, layered}})
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for j := range 3 {
		sum += builtWall(t, many, "P/"+string(rune('0'+j))).Quantities.GrossVolume
	}
	if v := builtWall(t, one, "P").Quantities.GrossVolume; !near(sum, v, 1e-9) {
		t.Errorf("layer volumes sum to %v, want %v", sum, v)
	}
}

// End A lies 0.9 mm off T at 15° and snaps 0.9 mm / sin 15° along P; the
// opening stays where it was in the world.
func TestJoinSnapKeepsOpenings(t *testing.T) {
	r := 15 * math.Pi / 180
	a := [2]float64{2, 0.0009}
	u := [2]float64{math.Cos(r), math.Sin(r)}
	p := PlanWall{ID: "P", Edge: [2][2]float64{a, {a[0] + 3*u[0], a[1] + 3*u[1]}}, Height: 2.5, Thickness: 0.1, Side: SideRight,
		Openings: []Opening{{Offset: 1, Width: 0.5, Height: 2}}, Joins: []Join{{End: EndA, Wall: "T"}}}
	got, err := BuildPlan(Plan{Walls: []PlanWall{joinT, p}})
	if err != nil {
		t.Fatal(err)
	}
	m := builtWall(t, got, "P").Measured
	at := [2]float64{m.Origin[0] + m.Openings[0].Offset*m.U[0], m.Origin[1] + m.Openings[0].Offset*m.U[1]}
	if want := [2]float64{a[0] + u[0], a[1] + u[1]}; !near(at[0], want[0], 1e-9) || !near(at[1], want[1], 1e-9) {
		t.Errorf("opening starts at %v, want %v", at, want)
	}
}

// Two long walls with more partitions between them than maxOpenings: each
// wall is refused as too large, and once it is over the cap no more strips
// are taken onto it, so no overlap scan runs over more than maxOpenings strips.
func TestStripCapStopsEarly(t *testing.T) {
	const n = 3 * maxOpenings
	l := 0.3 * float64(n+1)
	p := Plan{Walls: []PlanWall{
		{ID: "T1", Edge: [2][2]float64{{0, 0}, {l, 0}}, Height: 2.5, Thickness: 0.2, Side: SideLeft},
		{ID: "T2", Edge: [2][2]float64{{0, 3}, {l, 3}}, Height: 2.5, Thickness: 0.2, Side: SideRight},
	}}
	for i := range n {
		x := 0.3 * float64(i+1)
		p.Walls = append(p.Walls, PlanWall{ID: fmt.Sprintf("P%d", i), Edge: [2][2]float64{{x, 0}, {x, 3}}, Height: 2.5, Thickness: 0.1, Side: SideLeft,
			Joins: []Join{{End: EndA, Wall: "T1"}, {End: EndB, Wall: "T2"}}})
	}

	_, err := BuildPlan(p)
	refused := map[string]bool{}
	for _, e := range unjoin(err) {
		var ie *ItemError
		if !errors.As(e, &ie) || !errors.Is(e, ErrTooLarge) {
			t.Fatalf("got %v, want only too-large item refusals", e)
		}
		refused[ie.Item] = true
	}
	if !refused["T1"] || !refused["T2"] || len(refused) != 2 {
		t.Errorf("refused %v, want T1 and T2", refused)
	}

	trims, errs := trimJoins(p, map[string]bool{})
	if errs != nil {
		t.Fatal(errs)
	}
	strips, _ := stripJoins(p, map[string]bool{}, trims)
	on := map[string]int{}
	for _, s := range strips {
		on[s.face.wall]++
	}
	for _, w := range []string{"T1", "T2"} {
		if on[w] > maxOpenings+1 {
			t.Errorf("%s holds %d strips, want at most %d", w, on[w], maxOpenings+1)
		}
	}
}

// A partition standing on a face takes its strip's width off that face's
// floor line, on the layer the strip lands on. One standing higher covers
// area but not the floor line.
func TestJoinFloorLine(t *testing.T) {
	floor := func(t *testing.T, p Plan, id string) (float64, float64) {
		t.Helper()
		b, err := BuildPlan(p)
		if err != nil {
			t.Fatal(err)
		}
		q := builtWall(t, b, id).Quantities
		return q.MeasuredFloorLine, q.OtherFloorLine
	}
	check := func(name string, m, o, wantM, wantO float64) {
		if !near(m, wantM, 1e-9) || !near(o, wantO, 1e-9) {
			t.Errorf("%s: floor lines %v, %v, want %v, %v", name, m, o, wantM, wantO)
		}
	}
	m, o := floor(t, joinFixture(), "T")
	check("measured face", m, o, 3.9, 4)
	m, o = floor(t, Plan{Walls: []PlanWall{joinT, otherP()}}, "T")
	check("opposite face", m, o, 4, 3.9)

	layered := joinT
	layered.Thickness, layered.Layers = 0, []float64{0.1, 0.1}
	for _, p := range []struct {
		name        string
		partition   PlanWall
		first, last [2]float64
	}{
		{"layered, measured face", joinFixture().Walls[1], [2]float64{3.9, 4}, [2]float64{4, 4}},
		{"layered, opposite face", otherP(), [2]float64{4, 4}, [2]float64{4, 3.9}},
	} {
		plan := Plan{Walls: []PlanWall{layered, p.partition}}
		m, o = floor(t, plan, "T/0")
		check(p.name+" layer 0", m, o, p.first[0], p.first[1])
		m, o = floor(t, plan, "T/1")
		check(p.name+" layer 1", m, o, p.last[0], p.last[1])
	}

	high := joinFixture()
	high.Walls[1].Base, high.Walls[1].Height = 1, 1.5
	b, err := BuildPlan(high)
	if err != nil {
		t.Fatal(err)
	}
	q := builtWall(t, b, "T").Quantities
	if !near(q.MeasuredJoins, 0.15, 1e-9) {
		t.Errorf("raised partition: MeasuredJoins %v, want 0.15", q.MeasuredJoins)
	}
	check("raised partition", q.MeasuredFloorLine, q.OtherFloorLine, 4, 4)
}
