package parity

import (
	"sort"
	"testing"
)

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

			var violations []string
			var onlyOracle, onlyGoifc int
			for gid, want := range oracle {
				have, ok := got[gid]
				if !ok {
					onlyOracle++
					continue
				}
				if !Contains(have, want, Tolerance) {
					violations = append(violations, gid)
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
			t.Logf("%s: %d compared, %d oracle-only, %d goifc-only",
				name, len(oracle)-onlyOracle, onlyOracle, onlyGoifc)

			if len(violations) > 0 {
				sort.Strings(violations)
				show := violations
				if len(show) > 10 {
					show = show[:10]
				}
				t.Errorf("%s: %d element(s) under-report the oracle bound; first %d: %v",
					name, len(violations), len(show), show)
			}
		})
	}
}
