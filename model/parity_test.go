package model

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/blox-eng/common/ifc/step"
)

type oracleFile struct {
	Elements  []oracleElem `json:"elements"`
	ElapsedMs float64      `json:"elapsed_ms"`
	UnitScale float64      `json:"unit_scale"`
}

type oracleElem struct {
	GlobalID       string              `json:"global_id"`
	IFCClass       string              `json:"ifc_class"`
	Category       string              `json:"category"`
	Storey         string              `json:"storey"`
	Material       string              `json:"material"`
	PredefinedType string              `json:"predefined_type"`
	IsExternal     *bool               `json:"is_external"`
	QuantitySource string              `json:"quantity_source"`
	Quantities     map[string]*float64 `json:"quantities"`
}

// TestParity discovers every testdata/real/*.ifc that has a matching
// testdata/oracle/*.json and runs the same parity + perf comparison against
// each as a subtest. Files with no matching fixture pair are skipped
// individually (both are gitignored real-world corpus, absent in CI).
func TestParity(t *testing.T) {
	matches, err := filepath.Glob("testdata/real/*.ifc")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(matches)

	if len(matches) == 0 {
		t.Skip("no fixtures under testdata/real/ (gitignored real IFC); skipping")
	}

	for _, ifcPath := range matches {
		name := strings.TrimSuffix(filepath.Base(ifcPath), ".ifc")
		oraclePath := filepath.Join("testdata/oracle", name+".json")
		t.Run(name, func(t *testing.T) {
			skipIfMissing(t, ifcPath)
			skipIfMissing(t, oraclePath)
			runParity(t, ifcPath, oraclePath)
		})
	}
}

func runParity(t *testing.T, ifcPath, oraclePath string) {
	var oracle oracleFile
	b, err := os.ReadFile(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &oracle); err != nil {
		t.Fatal(err)
	}

	f, err := step.ParseFile(ifcPath)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	res, err := Extract(f)
	if err != nil {
		t.Fatal(err)
	}
	goMs := float64(time.Since(start).Microseconds()) / 1000

	byGUID := map[string]Element{}
	goSeen := map[string]bool{}
	for _, e := range res.Elements {
		byGUID[e.GlobalID] = e
		goSeen[e.GlobalID] = false
	}

	missingByClass := map[string]int{}
	var missingTotal int
	fieldMismatch := map[string]int{}
	fieldExamples := map[string][]string{}
	matched := 0
	anyMismatch := 0

	recordExample := func(field, guid, goVal, oracleVal string) {
		fieldMismatch[field]++
		if len(fieldExamples[field]) < 5 {
			fieldExamples[field] = append(fieldExamples[field], fmt.Sprintf("%s: go=%q oracle=%q", guid, goVal, oracleVal))
		}
	}

	for _, o := range oracle.Elements {
		g, ok := byGUID[o.GlobalID]
		if !ok {
			missingByClass[o.IFCClass]++
			missingTotal++
			continue
		}
		goSeen[o.GlobalID] = true
		matched++

		rowMismatch := false

		if !strings.EqualFold(g.IFCClass, o.IFCClass) {
			recordExample("ifc_class", o.GlobalID, g.IFCClass, o.IFCClass)
			rowMismatch = true
		}
		if !strings.EqualFold(g.Category, o.Category) {
			recordExample("category", o.GlobalID, g.Category, o.Category)
			rowMismatch = true
		}
		if g.Storey != o.Storey {
			recordExample("storey", o.GlobalID, g.Storey, o.Storey)
			rowMismatch = true
		}
		if g.Material != o.Material {
			recordExample("material", o.GlobalID, g.Material, o.Material)
			rowMismatch = true
		}
		if !boolEq(g.IsExternal, o.IsExternal) {
			recordExample("is_external", o.GlobalID, boolStr(g.IsExternal), boolStr(o.IsExternal))
			rowMismatch = true
		}
		if !predefinedTypeEq(g.PredefinedType, o.PredefinedType) {
			recordExample("predefined_type", o.GlobalID, g.PredefinedType, o.PredefinedType)
			rowMismatch = true
		}
		if o.QuantitySource == "qto" {
			if !floatEq(g.Qto.Area, o.Quantities["area"]) {
				recordExample("qto_area", o.GlobalID, floatStr(g.Qto.Area), floatStr(o.Quantities["area"]))
				rowMismatch = true
			}
			if !floatEq(g.Qto.Volume, o.Quantities["volume"]) {
				recordExample("qto_volume", o.GlobalID, floatStr(g.Qto.Volume), floatStr(o.Quantities["volume"]))
				rowMismatch = true
			}
		}

		if rowMismatch {
			anyMismatch++
		}
	}

	extraByClass := map[string]int{}
	var extraTotal int
	for guid, seen := range goSeen {
		if !seen {
			extraByClass[byGUID[guid].IFCClass]++
			extraTotal++
		}
	}

	// ---- report ----
	t.Logf("=== TOTALS === oracle=%d matched=%d missing=%d extra=%d field-mismatch-rows=%d",
		len(oracle.Elements), matched, missingTotal, extraTotal, anyMismatch)

	t.Logf("=== MISSING BY CLASS (%d classes, %d elements) ===", len(missingByClass), missingTotal)
	for _, kv := range sortedCounts(missingByClass) {
		t.Logf("  MISSING %-30s %d", kv.k, kv.v)
	}

	t.Logf("=== EXTRA BY CLASS (%d classes, %d elements) ===", len(extraByClass), extraTotal)
	for _, kv := range sortedCounts(extraByClass) {
		t.Logf("  EXTRA %-30s %d", kv.k, kv.v)
	}

	t.Logf("=== FIELD MISMATCH HISTOGRAM ===")
	for _, kv := range sortedCounts(fieldMismatch) {
		t.Logf("  FIELD %-16s %d mismatches", kv.k, kv.v)
		for _, ex := range fieldExamples[kv.k] {
			t.Logf("      e.g. %s", ex)
		}
	}

	bar := oracle.ElapsedMs / 5
	t.Logf("=== PERF === Go=%.1fms Python(no-geom)=%.1fms bar(python/5)=%.1fms pass=%v",
		goMs, oracle.ElapsedMs, bar, goMs <= bar)

	if missingTotal > 0 || extraTotal > 0 || anyMismatch > 0 {
		t.Errorf("%d missing, %d extra, %d rows with field mismatches vs oracle (see log for histograms)", missingTotal, extraTotal, anyMismatch)
	}
	if goMs > bar {
		t.Errorf("PERF: Go %.1fms > bar %.1fms (python/5)", goMs, bar)
	}
}

type kvCount struct {
	k string
	v int
}

func sortedCounts(m map[string]int) []kvCount {
	out := make([]kvCount, 0, len(m))
	for k, v := range m {
		out = append(out, kvCount{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].v != out[j].v {
			return out[i].v > out[j].v
		}
		return out[i].k < out[j].k
	})
	return out
}

func boolEq(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func boolStr(b *bool) string {
	if b == nil {
		return "nil"
	}
	if *b {
		return "true"
	}
	return "false"
}

func floatEq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Abs(*a-*b) <= 1e-6*math.Max(1, math.Abs(*b))
}

func floatStr(f *float64) string {
	if f == nil {
		return "nil"
	}
	return fmt.Sprintf("%v", *f)
}

// predefinedTypeEq treats "" and absent (nil/None -> "") as equal, else compares case-insensitively.
func predefinedTypeEq(g, o string) bool {
	if g == "" && o == "" {
		return true
	}
	return strings.EqualFold(g, o)
}
