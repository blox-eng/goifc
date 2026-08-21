package geometry

import "testing"

// square returns the four directed boundary segments of a unit square, CCW.
func square() [][2][2]float64 {
	return [][2][2]float64{
		{{0, 0}, {1, 0}},
		{{1, 0}, {1, 1}},
		{{1, 1}, {0, 1}},
		{{0, 1}, {0, 0}},
	}
}

// TestBridgeOpenBoundary_ClosedIsUntouched: a boundary that already closes must
// come back byte-identical and unbridged. The repair is a fallback, not a pass.
func TestBridgeOpenBoundary_ClosedIsUntouched(t *testing.T) {
	in := square()
	out, bridged, ok := bridgeOpenBoundary(in)
	if !ok || bridged {
		t.Fatalf("ok=%v bridged=%v, want true/false", ok, bridged)
	}
	if len(out) != len(in) {
		t.Fatalf("%d segments, want %d — a closed outline must not gain one", len(out), len(in))
	}
}

// TestBridgeOpenBoundary_OneShortGapIsClosed is the case that costs 45 elements
// on real data: a single seam that failed to weld.
func TestBridgeOpenBoundary_OneShortGapIsClosed(t *testing.T) {
	in := square()
	in[3] = [2][2]float64{{0, 1}, {0, 0.004}} // 4 mm short of home
	out, bridged, ok := bridgeOpenBoundary(in)
	if !ok || !bridged {
		t.Fatalf("ok=%v bridged=%v, want true/true", ok, bridged)
	}
	if len(out) != len(in)+1 {
		t.Fatalf("%d segments, want %d", len(out), len(in)+1)
	}
	if !boundaryCloses(out) {
		t.Error("bridged boundary still does not close")
	}
}

// TestBridgeOpenBoundary_RefusesWhatItCannotJustify. Each of these is a torn
// outline rather than a seam, and inventing geometry across it would put a wall
// on the sheet that the mesh does not describe.
func TestBridgeOpenBoundary_RefusesWhatItCannotJustify(t *testing.T) {
	t.Run("a gap longer than the bound", func(t *testing.T) {
		in := square()
		in[3] = [2][2]float64{{0, 1}, {0, 0.5}} // 500 mm
		if _, _, ok := bridgeOpenBoundary(in); ok {
			t.Error("bridged a 0.5 m gap; the bound is 10 mm")
		}
	})
	t.Run("two separate open chains", func(t *testing.T) {
		in := square()
		in[1] = [2][2]float64{{1, 0}, {1, 0.996}}
		in[3] = [2][2]float64{{0, 1}, {0, 0.004}}
		if _, _, ok := bridgeOpenBoundary(in); ok {
			t.Error("bridged two chains; only ONE is a failed weld, more is a tear")
		}
	})
	t.Run("a vertex unbalanced by more than one", func(t *testing.T) {
		in := append(square(), [2][2]float64{{0, 0}, {2, 2}}, [2][2]float64{{0, 0}, {3, 3}})
		if _, _, ok := bridgeOpenBoundary(in); ok {
			t.Error("bridged a vertex short by two; that is not one seam")
		}
	})
	t.Run("nothing at all", func(t *testing.T) {
		if _, _, ok := bridgeOpenBoundary(nil); !ok {
			t.Error("empty input should close vacuously rather than error")
		}
	})
}

// TestBridgeOpenBoundary_IsDeterministic guards the thing map iteration would
// break: the same open outline must bridge identically on every run, or two
// renders of one building disagree.
func TestBridgeOpenBoundary_IsDeterministic(t *testing.T) {
	var first [][2][2]float64
	for i := 0; i < 50; i++ {
		in := square()
		in[3] = [2][2]float64{{0, 1}, {0, 0.004}}
		out, bridged, ok := bridgeOpenBoundary(in)
		if !ok || !bridged {
			t.Fatalf("run %d: ok=%v bridged=%v", i, ok, bridged)
		}
		if first == nil {
			first = out
			continue
		}
		for j := range out {
			if out[j] != first[j] {
				t.Fatalf("run %d segment %d = %v, first run had %v", i, j, out[j], first[j])
			}
		}
	}
}
