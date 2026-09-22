package parity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Coverage mirrors geometry.Stats for serialization: how many of a model's
// elements each geometry path produced.
type Coverage struct {
	Total   int `json:"total"`
	Extrude int `json:"extrude"`
	Brep    int `json:"brep"`
	OBB     int `json:"obb"`
	Empty   int `json:"empty"`
}

// OBBRate is the share of elements THAT PRODUCED GEOMETRY which fell back to a
// bounding box. Empty elements are excluded from both numerator and denominator:
// they kept the default SourceOBB but never built a box, so counting them would
// let an unparseable element look like a coverage regression.
func OBBRate(c Coverage) float64 {
	withGeom := c.Total - c.Empty
	if withGeom <= 0 {
		return 0
	}
	return float64(c.OBB) / float64(withGeom)
}

// MeasureCoverage builds a model and tallies its geometry paths.
func MeasureCoverage(name string) (Coverage, error) {
	sc, err := SceneOf(name)
	if err != nil {
		return Coverage{}, err
	}
	st := sc.Stats()
	return Coverage{
		Total:   st.Total,
		Extrude: st.Extrude,
		Brep:    st.Brep,
		OBB:     st.OBB,
		Empty:   st.Empty,
	}, nil
}

func baselinePath() string {
	return filepath.Join(packageDir, "testdata", "baseline.json")
}

// LoadBaseline reads the committed coverage numbers.
func LoadBaseline() (map[string]Coverage, error) {
	raw, err := os.ReadFile(baselinePath())
	if err != nil {
		return nil, fmt.Errorf("parity: read baseline: %w", err)
	}
	var out map[string]Coverage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parity: decode baseline: %w", err)
	}
	return out, nil
}

// WriteBaseline rewrites the committed coverage numbers. Called by
// `make parity-baseline` when a geometry gap closes.
func WriteBaseline(m map[string]Coverage) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(baselinePath(), append(raw, '\n'), 0o644)
}
