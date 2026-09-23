package parity

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/blox-eng/goifc/geometry"
	"github.com/blox-eng/goifc/model"
)

// AABB is a world-space axis-aligned bounding box in meters, matching the
// oracle JSON written by IfcOpenShell.
type AABB struct {
	Min [3]float64 `json:"min"`
	Max [3]float64 `json:"max"`
}

// Tolerance is the containment slack in meters. It absorbs float noise between
// two independent tessellations; it must stay far below any real geometric
// discrepancy. Measured (Task 2 Step 8) across the public corpus: float noise
// tops out at ~7.63e-7 m (float32 vertex precision at ~10 m coordinates); the
// worst genuine discrepancy found is ~0.0099 m (see TestGate1BoundsContainOracle
// duplex_a violations). 1e-5 sits comfortably above the former and three orders
// of magnitude below the latter.
const Tolerance = 1e-5

// Contains reports whether outer encloses inner on every axis, allowing tol
// meters of slack. This is the asymmetric assertion Gate 1 rests on: goifc's box
// may be larger than the oracle's (an OBB fallback legitimately is), but never
// smaller, because a bound that under-reports is the one failure a consumer
// cannot defend against.
func Contains(outer, inner AABB, tol float64) bool {
	// A NaN bound compares false against every comparison below, so without
	// this guard the loop falls straight through and the function reports
	// containment for a box that is not a box. Gate 1 exists to catch an
	// under-reporting bound; a NaN bound reports nothing at all.
	if !Finite(outer) || !Finite(inner) {
		return false
	}
	for i := 0; i < 3; i++ {
		if outer.Min[i] > inner.Min[i]+tol {
			return false
		}
		if outer.Max[i] < inner.Max[i]-tol {
			return false
		}
	}
	return true
}

// Finite reports whether every bound is a real number. Gate 1 compares these
// bounds and the looseness ratio multiplies them; NaN silently satisfies the
// first and poisons the second, so a non-finite box is rejected explicitly
// rather than left to float comparison semantics.
func Finite(b AABB) bool {
	for i := 0; i < 3; i++ {
		if math.IsNaN(b.Min[i]) || math.IsInf(b.Min[i], 0) ||
			math.IsNaN(b.Max[i]) || math.IsInf(b.Max[i], 0) {
			return false
		}
	}
	return true
}

// Volume returns the box's volume in cubic meters, clamped at zero so a planar,
// inverted, or non-finite box yields 0 rather than a negative number or a NaN
// that would corrupt every percentile downstream.
func Volume(b AABB) float64 {
	if !Finite(b) {
		return 0
	}
	v := 1.0
	for i := 0; i < 3; i++ {
		d := b.Max[i] - b.Min[i]
		if d <= 0 {
			return 0
		}
		v *= d
	}
	return v
}

// LoadOracle reads the IfcOpenShell AABBs for a model, keyed by GlobalID.
func LoadOracle(name string) (map[string]AABB, error) {
	p := filepath.Join(packageDir, "testdata", "oracle", name+".json")
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("parity: read oracle %s: %w", name, err)
	}
	var out map[string]AABB
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parity: decode oracle %s: %w", name, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("parity: oracle %s is empty", name)
	}
	return out, nil
}

// SceneOf runs the full library pipeline for a model: parse, extract semantics,
// build proxy geometry.
func SceneOf(name string) (*geometry.Scene, error) {
	f, err := Load(name)
	if err != nil {
		return nil, err
	}
	r, err := model.Extract(f)
	if err != nil {
		return nil, fmt.Errorf("parity: extract %s: %w", name, err)
	}
	sc, err := geometry.Build(f, r)
	if err != nil {
		return nil, fmt.Errorf("parity: build %s: %w", name, err)
	}
	return sc, nil
}

// ElementBox returns an element's world AABB as an AABB.
func ElementBox(e geometry.Element) AABB {
	return AABB{Min: e.BBoxMin, Max: e.BBoxMax}
}
