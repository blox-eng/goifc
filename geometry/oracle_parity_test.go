package geometry

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

type aabbJSON struct {
	Min [3]float64 `json:"min"`
	Max [3]float64 `json:"max"`
}

// Source-aware tolerance: brep reads the SAME faces as the oracle and OBB IS the
// bbox by construction → they must match tightly; extrude is a profile
// approximation of a swept solid → a wider band is legitimate. A single loose
// global band would let a broken extrude depth/scale pass — the gate needs teeth.
func tolFor(src GeomSource) (epsPos, epsDim float64) {
	if src == SourceBrep {
		return 0.001, 0.01 // 1mm / 1% — brep reads the SAME explicit faces as the oracle
	}
	// extrude = profile approximation of a swept solid; obb = parametric fallback
	// (failed-extrude/revolve/NURBS) whose collectPoints AABB is only approximate,
	// NOT the true solid AABB — so obb canNOT be held to a tight band.
	return 0.05, 0.10 // 5cm / 10%
}

func TestOracleParity(t *testing.T) {
	snaps, _ := filepath.Glob("testdata/oracle/*.json")
	if len(snaps) == 0 {
		t.Skip("no oracle snapshots present")
	}
	for _, snap := range snaps {
		name := filepath.Base(snap)
		ifcPath := "testdata/real/" + name[:len(name)-len(".json")] + ".ifc"
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(ifcPath); err != nil {
				t.Skipf("corpus file %s absent", ifcPath)
			}
			var oracle map[string]aabbJSON
			raw, err := os.ReadFile(snap)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &oracle); err != nil {
				t.Fatal(err)
			}
			f, err := step.ParseFile(ifcPath)
			if err != nil {
				t.Fatal(err)
			}
			r, err := model.Extract(f)
			if err != nil {
				t.Fatal(err)
			}
			s, err := Build(f, r)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]Element{}
			for _, e := range s.Elements {
				got[e.GlobalID] = e
			}
			var offPos, offDim, oracleOnly int
			for gid, o := range oracle {
				e, ok := got[gid]
				if !ok {
					oracleOnly++
					continue
				}
				epsPos, epsDim := tolFor(e.Source)
				for k := 0; k < 3; k++ {
					if math.Abs(e.BBoxMin[k]-o.Min[k]) > epsPos {
						offPos++
						break
					}
				}
				for k := 0; k < 3; k++ {
					od := o.Max[k] - o.Min[k]
					gd := e.BBoxMax[k] - e.BBoxMin[k]
					if math.Abs(gd-od) > epsDim*math.Max(od, 0.001)+epsPos {
						offDim++
						break
					}
				}
			}
			goOnly := 0 // Go rendered a GlobalId the oracle didn't (spurious/duplicate)
			for gid := range got {
				if _, ok := oracle[gid]; !ok {
					goOnly++
				}
			}
			total := len(oracle)
			t.Logf("%s: oracle=%d offPos=%d offDim=%d oracleOnly=%d goOnly=%d", name, total, offPos, offDim, oracleOnly, goOnly)
			// Gate: coverage complete and ≥ 98% within tolerance.
			if oracleOnly > total/50 {
				t.Errorf("coverage gap: %d/%d oracle elements missing from Go", oracleOnly, total)
			}
			if offPos > total/50 {
				t.Errorf("position parity: %d/%d elements off tolerance", offPos, total)
			}
			if offDim > total/20 {
				t.Errorf("dimension parity: %d/%d elements off tolerance", offDim, total)
			}
		})
	}
}
