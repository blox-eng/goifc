package parity

import (
	"fmt"
	"sort"
	"testing"
)

// axisShortfall is how far outer falls short of containing inner on one bound
// of one axis (min-x, max-y, ...), in meters. Only positive shortfalls are
// ever recorded: a bound that already contains the other contributes nothing.
type axisShortfall struct {
	bound  string // e.g. "min-y", "max-z"
	amount float64
}

// shortfallsOf returns every bound on which outer fails to contain inner by
// more than tol — the same per-bound test Contains applies, but reporting
// which bound(s) and by how much instead of collapsing to a single bool. Call
// it only after Contains has already failed for outer/inner at Tolerance, so
// this is explaining a real violation, not float noise.
func shortfallsOf(outer, inner AABB) []axisShortfall {
	axes := [3]string{"x", "y", "z"}
	var out []axisShortfall
	for i := 0; i < 3; i++ {
		if d := outer.Min[i] - inner.Min[i]; d > Tolerance {
			out = append(out, axisShortfall{"min-" + axes[i], d})
		}
		if d := inner.Max[i] - outer.Max[i]; d > Tolerance {
			out = append(out, axisShortfall{"max-" + axes[i], d})
		}
	}
	return out
}

// worst returns the largest shortfall amount, or 0 if sf is empty.
func worstShortfall(sf []axisShortfall) float64 {
	var w float64
	for _, s := range sf {
		if s.amount > w {
			w = s.amount
		}
	}
	return w
}

func formatShortfalls(sf []axisShortfall) string {
	s := ""
	for i, f := range sf {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%s=%.6gm", f.bound, f.amount)
	}
	return s
}

// allowlistMargin absorbs run-to-run float noise on top of a recorded known
// shortfall (measured noise ceiling in this corpus is ~7.63e-7 m; this margin
// is over 100x that) without absorbing any real further regression, which
// would be orders of magnitude larger.
const allowlistMargin = 1e-4

// gate1Floor is how many elements Gate 1 must actually COMPARE per public
// model — the size of the intersection of the oracle and goifc's scene, as
// measured when this floor was recorded (oracle-only is 0 on all three models
// today, so the intersection is the whole oracle).
//
// Without it the gate can pass having compared nothing at all. If the scene
// came back empty, or its GlobalIDs stopped matching the oracle's, every
// oracle entry falls into onlyOracle, no violation can be found, and the
// stale-entry loop below skips every entry through its "not in this scene"
// guard — a collapse to zero comparisons reading as a clean pass.
//
// A floor rather than an equality, so the gate stays a gate and not a
// tripwire: goifc extracting more elements, or the oracle gaining an entry
// goifc also builds, both raise the count and neither is a regression. It is
// also deliberately not `onlyOracle == 0`, which would turn a selection
// difference — the two libraries disagreeing about which entity classes are
// elements — into a failure, and that is a diff, not a bug (see the log line
// below). A corpus or oracle change that legitimately lowers a count updates
// these numbers in the same commit that makes the change, the way
// `make parity-baseline` records a Gate 2 change.
var gate1Floor = map[string]int{
	"ifcopenhouse": 34,
	"duplex_a":     215,
	"fzk_haus":     82,
}

// knownViolation and knownViolations live in knownviolations.go, not here:
// Report() reads them too, to generate the published "Known Gate 1
// violations" section instead of hand-typing it.

// TestGate1BoundsContainOracle is the harness's central assertion: for every
// element IfcOpenShell knows, goifc's AABB must CONTAIN the oracle's.
//
// Containment, not equality. An OBB fallback box is legitimately larger than the
// solid it stands for. A goifc box SMALLER than the oracle's is always a bug,
// because it means a bound that under-reports.
func TestGate1BoundsContainOracle(t *testing.T) {
	for _, name := range Public {
		t.Run(name, func(t *testing.T) {
			oracle, err := LoadOracle(name)
			if err != nil {
				t.Fatal(err)
			}
			sc, err := SceneOf(name)
			if err != nil {
				t.Fatal(err)
			}

			got := make(map[string]AABB, len(sc.Elements))
			for _, e := range sc.Elements {
				got[e.GlobalID] = ElementBox(e)
			}

			violations := make(map[string][]axisShortfall)
			var onlyOracle, onlyGoifc int
			for gid, want := range oracle {
				have, ok := got[gid]
				if !ok {
					onlyOracle++
					continue
				}
				if !Contains(have, want, Tolerance) {
					violations[gid] = shortfallsOf(have, want)
				}
			}
			for gid := range got {
				if _, ok := oracle[gid]; !ok {
					onlyGoifc++
				}
			}

			// Selection differences are a diff, not a failure: the two libraries
			// disagree about which entity classes count as elements (see
			// model/extract.go:14-22). Report and move on.
			compared := len(oracle) - onlyOracle
			t.Logf("%s: %d compared, %d oracle-only, %d goifc-only, %d violation(s)",
				name, compared, onlyOracle, onlyGoifc, len(violations))

			// Everything below reports on the elements that WERE compared, and
			// says nothing about how many that was. Assert the count first, or
			// a gate that compared nothing passes silently.
			floor, ok := gate1Floor[name]
			if !ok {
				t.Fatalf("%s: no compared-element floor in gate1Floor; record one measured against this model, or the gate can pass having compared nothing",
					name)
			}
			if compared < floor {
				t.Errorf("%s: the gate compared only %d element(s), below the recorded floor of %d — the oracle and the scene have stopped lining up (%d oracle-only, %d goifc-only), so this gate is measuring almost nothing rather than passing",
					name, compared, floor, onlyOracle, onlyGoifc)
			}

			if l, err := MeasureLooseness(name); err != nil {
				t.Logf("%s: looseness measurement failed: %v", name, err)
			} else {
				t.Logf("%s: looseness compared=%d degenerate=%d p50=%.4g p90=%.4g max=%.4g",
					name, l.Compared, l.Degenerate, l.P50, l.P90, l.Max)
			}

			known := make(map[string]knownViolation, len(knownViolations[name]))
			for _, kv := range knownViolations[name] {
				known[kv.globalID] = kv
			}

			var newBugs, widened, fixed []string
			for gid, sf := range violations {
				kv, isKnown := known[gid]
				if !isKnown {
					newBugs = append(newBugs, fmt.Sprintf("%s: NEW, not in known-violations allowlist (%s)",
						gid, formatShortfalls(sf)))
					continue
				}
				if w := worstShortfall(sf); w > kv.shortfall+allowlistMargin {
					widened = append(widened, fmt.Sprintf("%s: known violation WIDENED from %.6gm to %.6gm (%s)",
						gid, kv.shortfall, w, formatShortfalls(sf)))
				}
			}
			for _, kv := range knownViolations[name] {
				if _, stillVio := violations[kv.globalID]; stillVio {
					continue
				}
				if _, present := got[kv.globalID]; !present {
					continue // not in this scene at all; not this gate's business
				}
				fixed = append(fixed, fmt.Sprintf("%s: allowlisted violation no longer reproduces (was %s=%.6gm) -- DELETE this stale known-violations entry",
					kv.globalID, kv.axis, kv.shortfall))
			}

			capList := func(s []string) []string {
				sort.Strings(s)
				if len(s) > 10 {
					return s[:10]
				}
				return s
			}
			if len(newBugs) > 0 {
				show := capList(newBugs)
				t.Errorf("%s: %d new violation(s) not in the known-violations allowlist; first %d:\n%s",
					name, len(newBugs), len(show), joinLines(show))
			}
			if len(widened) > 0 {
				show := capList(widened)
				t.Errorf("%s: %d known violation(s) widened beyond their recorded shortfall; first %d:\n%s",
					name, len(widened), len(show), joinLines(show))
			}
			if len(fixed) > 0 {
				show := capList(fixed)
				t.Errorf("%s: %d known-violations entr(y/ies) no longer reproduce; first %d:\n%s",
					name, len(fixed), len(show), joinLines(show))
			}
		})
	}
}

func joinLines(lines []string) string {
	s := ""
	for _, l := range lines {
		s += "  " + l + "\n"
	}
	return s
}
