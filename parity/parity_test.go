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

// knownViolation is one Gate 1 containment violation that is a tracked, known
// goifc bug rather than a new regression: goifc's world AABB for this element
// under-reports the oracle's by roughly shortfall meters on axis, as measured
// when the entry was added.
type knownViolation struct {
	globalID  string
	axis      string // the worst-shortfall bound, e.g. "min-y"
	shortfall float64
}

// allowlistMargin absorbs run-to-run float noise on top of a recorded known
// shortfall (measured noise ceiling in this corpus is ~7.63e-7 m; this margin
// is over 100x that) without absorbing any real further regression, which
// would be orders of magnitude larger.
const allowlistMargin = 1e-4

// knownViolations lists, per public model, every Gate 1 containment violation
// that is a tracked goifc bug rather than a new failure. This keeps CI green
// on known state without weakening Contains or Tolerance: a violation not
// listed here, or one that has grown past its recorded shortfall plus
// allowlistMargin, still fails the gate (a new or worsened bug). So does a
// listed entry that no longer violates at all — that means the bug was fixed
// and the stale entry must be deleted, never left to quietly rot into a lie.
//
// duplex_a: goifc under-reports the world AABB of IfcStairFlight elements by
// ~0.99 cm on the Y axis. Both known instances are the SAME Revit stair type
// ("Stair:Residential - 200mm Max Riser 250mm Tread", 16 risers / 15 treads,
// Revit instances 151086 and 198878) — the near-identical shortfall on both
// (0.009927508 m and 0.009927222 m) points at a systematic issue with how
// goifc bounds a stepped solid along its extrusion path, not per-element
// noise. This is a known goifc geometry gap found by this gate; no issue has
// been filed for it yet.
var knownViolations = map[string][]knownViolation{
	"duplex_a": {
		{globalID: "1oKjKg9PD3fP1iIwXLh3lK", axis: "min-y", shortfall: 0.009927508357675308},
		{globalID: "3KMJUyUe9DfQ2FOCd5ZoiN", axis: "max-y", shortfall: 0.009927222255395662},
	},
}

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
			t.Logf("%s: %d compared, %d oracle-only, %d goifc-only, %d violation(s)",
				name, len(oracle)-onlyOracle, onlyOracle, onlyGoifc, len(violations))

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
