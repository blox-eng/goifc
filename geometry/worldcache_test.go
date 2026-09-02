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

// TestBandGridCellsMatchesAllocation pins the invariant the aggregate memory
// budget rests on: bandGridCells must report exactly what buildOccupancyWith
// goes on to allocate.
//
// BuildFacings sizes its worker pool from bandGridCells BEFORE any grid exists,
// so if the two ever disagree the pool is sized against a fiction and
// facingBandCellBudget stops bounding anything — silently, with every test still
// green, because the classification itself is unaffected. They share
// bandGridDims today precisely so they cannot drift; this fails if anyone splits
// them apart again.
//
// Note that a nil return does NOT mean nothing was allocated: buildOccupancyWith
// allocates the grid and only then discards it when the slice turns out to cut
// no element (`!sliced`). The budget is about memory, so bandGridCells predicts
// the ALLOCATION, and that case is deliberately not treated as a mismatch.
func TestBandGridCellsMatchesAllocation(t *testing.T) {
	elems := benchModel(41)
	for _, z := range []float64{-5, 0, 0.35, 1.5, 3, 1e6} {
		predicted := bandGridCells(elems, z)
		g := buildOccupancyWith(elems, z, newWorldCache(elems))

		if predicted == 0 {
			// Sizing refused, so nothing may be built at all.
			if g != nil {
				t.Fatalf("z=%v: bandGridCells predicted no grid, but one was built (%dx%d)",
					z, g.nx, g.ny)
			}
			continue
		}
		if g == nil {
			continue // allocated, then discarded for cutting nothing
		}
		actual := g.nx * g.ny
		if predicted != actual {
			t.Fatalf("z=%v: bandGridCells predicted %d cells, grid allocated %d (%dx%d)",
				z, predicted, actual, g.nx, g.ny)
		}
		if len(g.solid) != actual || len(g.covered) != actual {
			t.Fatalf("z=%v: grid planes are %d/%d cells, nx*ny is %d",
				z, len(g.solid), len(g.covered), actual)
		}
	}
}
