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

// amountOn returns the shortfall recorded for one bound, and whether that bound
// violates at all. The allowlist is compared per bound, never against a
// collapsed worst-of-all-axes figure: an entry recorded on min-y must not
// absorb a fresh violation on max-z just because the old one was larger.
func amountOn(sf []axisShortfall, bound string) (float64, bool) {
	for _, s := range sf {
		if s.bound == bound {
			return s.amount, true
		}
	}
	return 0, false
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
				// An entry covers exactly one bound. Every OTHER bound that
				// violates on the same element is a new bug, and keying the
				// allowlist on GlobalID alone would have absorbed it silently
				// whenever it stayed smaller than the recorded shortfall.
				for _, s := range sf {
					if s.bound == kv.axis {
						continue
					}
					newBugs = append(newBugs, fmt.Sprintf("%s: NEW violation on %s=%.6gm; the allowlist covers only %s on this element",
						gid, s.bound, s.amount, kv.axis))
				}
				amt, stillOnAxis := amountOn(sf, kv.axis)
				if !stillOnAxis {
					fixed = append(fixed, fmt.Sprintf("%s: allowlisted %s no longer violates (was %.6gm) -- DELETE this stale known-violations entry",
						gid, kv.axis, kv.shortfall))
					continue
				}
				if amt > kv.shortfall+allowlistMargin {
					widened = append(widened, fmt.Sprintf("%s: known violation WIDENED on %s from %.6gm to %.6gm (%s)",
						gid, kv.axis, kv.shortfall, amt, formatShortfalls(sf)))
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

// The allowlist is matched per bound. The function this replaced collapsed all
// six bounds to a single max, so a fresh violation on another bound of an
// allowlisted element was absorbed whenever it stayed below the recorded
// shortfall — the exact case below.
func TestAmountOnIsPerBound(t *testing.T) {
	sf := []axisShortfall{
		{"min-y", 0.0099275},
		{"max-z", 0.006},
	}
	if amt, ok := amountOn(sf, "min-y"); !ok || amt != 0.0099275 {
		t.Errorf("amountOn(min-y) = %v, %v; want 0.0099275, true", amt, ok)
	}
	if amt, ok := amountOn(sf, "max-z"); !ok || amt != 0.006 {
		t.Errorf("amountOn(max-z) = %v, %v; want 0.006, true", amt, ok)
	}
	// The bound an allowlist entry names may stop violating while others start.
	if amt, ok := amountOn(sf, "max-x"); ok || amt != 0 {
		t.Errorf("amountOn(max-x) = %v, %v; want 0, false", amt, ok)
	}
	if _, ok := amountOn(nil, "min-y"); ok {
		t.Error("amountOn on no shortfalls reported a violating bound")
	}
}

// Every allowlist entry must name a bound shortfallsOf can actually produce,
// or it can never match and the entry silently allows nothing.
func TestKnownViolationAxesAreRealBounds(t *testing.T) {
	valid := map[string]bool{}
	for _, p := range []string{"min", "max"} {
		for _, a := range []string{"x", "y", "z"} {
			valid[p+"-"+a] = true
		}
	}
	for model, kvs := range knownViolations {
		for _, kv := range kvs {
			if !valid[kv.axis] {
				t.Errorf("%s: known violation %s has axis %q, which shortfallsOf never emits",
					model, kv.globalID, kv.axis)
			}
			if kv.shortfall <= 0 {
				t.Errorf("%s: known violation %s has shortfall %v; a non-positive allowance is not a violation",
					model, kv.globalID, kv.shortfall)
			}
		}
	}
}

func joinLines(lines []string) string {
	s := ""
	for _, l := range lines {
		s += "  " + l + "\n"
	}
	return s
}
