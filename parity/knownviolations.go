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
		{
			globalID:  "1oKjKg9PD3fP1iIwXLh3lK",
			axis:      "min-y",
			shortfall: 0.009927508357675308,
			note:      `IfcStairFlight, Revit instance 151086 of "Stair:Residential - 200mm Max Riser 250mm Tread" (16 risers / 15 treads)`,
		},
		{
			globalID:  "3KMJUyUe9DfQ2FOCd5ZoiN",
			axis:      "max-y",
			shortfall: 0.009927222255395662,
			note:      `IfcStairFlight, Revit instance 198878 of "Stair:Residential - 200mm Max Riser 250mm Tread" (16 risers / 15 treads)`,
		},
	},
}
