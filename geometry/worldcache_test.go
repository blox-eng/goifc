package geometry

import (
	"testing"

	"github.com/blox-eng/goifc/model"
)

// TestWorldPointsNeverNil pins the invariant the concurrent band workers depend
// on for their safety.
//
// After fill(), every cache entry must be non-nil, because at() only writes when
// it finds a nil — and at() IS called from inside the workers. If worldPoints
// could ever return nil, fill() would leave that entry nil, at() would write to
// it from several goroutines at once, and the result would be a data race whose
// racing writers all compute the same value. That is the flavour that passes
// every functional test, shows up only under -race, and is invisible in
// production.
//
// The empty-vertex case is the one that could plausibly regress: `make([]v3, 0)`
// is non-nil today, but a future rewrite of worldPoints that returns a bare `nil`
// for empty input would silently arm the race.
func TestWorldPointsNeverNil(t *testing.T) {
	for _, tc := range []struct {
		name  string
		verts []float32
	}{
		{"empty", nil},
		{"empty non-nil", []float32{}},
		{"one point", []float32{1, 2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := worldPoints(tc.verts, model.Identity())
			if got == nil {
				t.Fatal("worldPoints returned nil; worldCache.at would then write from several band workers at once")
			}
		})
	}
}

// TestWorldCacheFillLeavesNoNil is the invariant stated end to end: after fill,
// at() takes its read-only branch for every element, so sharing the cache across
// workers writes nothing.
func TestWorldCacheFillLeavesNoNil(t *testing.T) {
	elems := []Element{
		namedWall("a", v3{0, 0, 0}, v3{1, 1, 3}),
		{GlobalID: "no-geometry"}, // no Verts, no Tris — the interesting case
		namedWall("c", v3{4, 0, 0}, v3{5, 1, 3}),
	}
	wc := newWorldCache(elems)
	wc.fill()
	for i := range elems {
		if wc.pts[i] == nil {
			t.Fatalf("element %d still nil after fill; at() would write to it concurrently", i)
		}
	}
}

// TestBuildFacingsDeterministic guards the parallel merge.
//
// Bands are classified on a worker pool and merged afterwards, so completion
// order varies run to run. Nothing about the OUTPUT may. This runs the same
// model repeatedly and requires byte-identical classification each time —
// the in-repo counterpart to the cross-model digest check done by hand.
func TestBuildFacingsDeterministic(t *testing.T) {
	elems := benchModel(305)
	first := BuildFacings(elems)

	for run := 0; run < 8; run++ {
		got := BuildFacings(elems)
		if len(got) != len(first) {
			t.Fatalf("run %d classified %d elements, first run classified %d", run, len(got), len(first))
		}
		for id, want := range first {
			have, ok := got[id]
			if !ok {
				t.Fatalf("run %d dropped %s", run, id)
			}
			if have != want {
				t.Fatalf("run %d disagrees on %s:\n  first = %+v\n  run   = %+v", run, id, want, have)
			}
		}
	}
}
