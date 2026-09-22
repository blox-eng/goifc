package parity

import (
	"flag"
	"testing"
)

// updateBaseline rewrites testdata/baseline.json from the current measurements
// instead of asserting against it. Driven by `make parity-baseline`, which is
// how a closed geometry gap gets recorded.
var updateBaseline = flag.Bool("update-baseline", false,
	"rewrite testdata/baseline.json from the current measurements")

// An element that produced no mesh keeps the default SourceOBB but never built a
// box; Stats counts it as Empty only. The published OBB rate must exclude it, or
// an unparseable element masquerades as a coverage regression.
func TestOBBRateExcludesEmpty(t *testing.T) {
	c := Coverage{Total: 10, Extrude: 4, Brep: 3, OBB: 1, Empty: 2}
	// 1 OBB out of 8 elements that produced geometry.
	if got, want := OBBRate(c), 0.125; got != want {
		t.Errorf("OBBRate() = %v, want %v", got, want)
	}
}

func TestOBBRateNoGeometry(t *testing.T) {
	if got := OBBRate(Coverage{Total: 3, Empty: 3}); got != 0 {
		t.Errorf("OBBRate() with no geometry = %v, want 0", got)
	}
}

// Gate 2: the measured OBB rate must not exceed the committed baseline.
func TestGate2CoverageDoesNotRegress(t *testing.T) {
	if *updateBaseline {
		next := make(map[string]Coverage, len(Public))
		for _, name := range Public {
			c, err := MeasureCoverage(name)
			if err != nil {
				t.Fatalf("measure %s: %v", name, err)
			}
			next[name] = c
			t.Logf("%s: OBB rate %.4f, %+v", name, OBBRate(c), c)
		}
		if err := WriteBaseline(next); err != nil {
			t.Fatal(err)
		}
		t.Log("baseline rewritten; review the diff before committing")
		return
	}

	base, err := LoadBaseline()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Public {
		t.Run(name, func(t *testing.T) {
			want, ok := base[name]
			if !ok {
				t.Fatalf("no baseline for %q; run `make parity-baseline`", name)
			}
			got, err := MeasureCoverage(name)
			if err != nil {
				t.Fatal(err)
			}
			gotRate, wantRate := OBBRate(got), OBBRate(want)
			t.Logf("%s: OBB rate %.4f (baseline %.4f), %+v", name, gotRate, wantRate, got)
			if gotRate > wantRate+1e-9 {
				t.Errorf("%s: OBB rate rose to %.4f from %.4f — a geometry path regressed",
					name, gotRate, wantRate)
			}
		})
	}
}
