package parity

// knownViolation is one Gate 1 containment violation that is a tracked, known
// goifc bug rather than a new regression: goifc's world AABB for this element
// under-reports the oracle's by roughly shortfall meters on axis, as measured
// when the entry was added.
//
// This lives in a non-test file, not parity_test.go, because Report() reads
// it too: the published "Known Gate 1 violations" section is generated from
// these entries rather than hand-typed, so it cannot drift from what the
// allowlist actually contains.
type knownViolation struct {
	globalID string
	// axis is the ONE bound this entry covers, e.g. "min-y". The gate compares
	// per bound: a violation on any other bound of the same element fails as a
	// new bug, and this bound ceasing to violate fails as a stale entry.
	axis      string
	shortfall float64
	note      string // per-entry context for the published report: what the element is
}

// knownViolations lists, per public model, every Gate 1 containment violation
// that is a tracked goifc bug rather than a new failure. This keeps CI green
// on known state without weakening Contains or Tolerance. Each entry is matched
// per (GlobalID, bound), so the gate still fails on: a violation on an element
// not listed here; a violation on a bound of a listed element other than the
// one its entry names; a listed bound that has grown past its recorded
// shortfall plus allowlistMargin; and a listed bound that no longer violates at
// all, which means the bug was fixed and the stale entry must be deleted rather
// than left to quietly rot into a lie.
//
// Empty: the only entries, duplex_a's two IfcStairFlight shortfalls (#53), were
// a dropped nosing arc and closed with it. An empty allowlist is the goal, not
// a gap to fill.
var knownViolations = map[string][]knownViolation{}
