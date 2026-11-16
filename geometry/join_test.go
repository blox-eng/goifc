package geometry

import (
	"errors"
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
