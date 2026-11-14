package wasmapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var goCodes = []string{"too_wide", "overlaps", "above_wall", "in_corner", "layer_sliver",
	"invalid_input", "invalid_ifc", "too_large", "nothing_to_draw"}

// checkEnvelope holds for every answer: valid JSON, ok exactly when there
// are no errors, and only contract codes.
func checkEnvelope(t *testing.T, answer []byte) bool {
	t.Helper()
	if !json.Valid(answer) {
		t.Fatalf("answer is not JSON: %q", answer)
	}
	var a struct {
		OK     bool    `json:"ok"`
		Errors []Error `json:"errors"`
	}
	if err := json.Unmarshal(answer, &a); err != nil {
		t.Fatal(err)
	}
	if a.OK != (len(a.Errors) == 0) {
		t.Fatalf("ok %v with %d errors: %s", a.OK, len(a.Errors), answer)
	}
	for _, e := range a.Errors {
		if !slices.Contains(goCodes, e.Code) || e.Message == "" {
			t.Fatalf("error %+v: unknown code or empty message", e)
		}
	}
	return a.OK
}

func FuzzBuildRequest(f *testing.F) {
	reqs, _ := filepath.Glob("testdata/build/*.request.json")
	for _, p := range reqs {
		b, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, req []byte) {
		answer, glb := Build(testVersion, req)
		if ok := checkEnvelope(t, answer); (glb != nil) != ok {
			t.Fatalf("ok %v but GLB %v", ok, glb != nil)
		}
	})
}

func FuzzReadInclude(f *testing.F) {
	for _, p := range readFixtures {
		b, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b, "")
		f.Add(b, "nodes,glb,psets,storeyPlans,typeLayers")
		f.Add(b, "psets")
	}
	f.Fuzz(func(t *testing.T, src []byte, parts string) {
		if len(src) > 1<<20 {
			t.Skip("over 1 MiB")
		}
		var include []string
		if parts != "" {
			include = strings.Split(parts, ",")
		}
		answer, glb := Read(testVersion, src, include)
		ok := checkEnvelope(t, answer)
		wantGLB := ok && (include == nil || slices.Contains(include, "glb"))
		if wantGLB != (glb != nil) {
			t.Fatalf("ok %v, include %q, but GLB %v", ok, include, glb != nil)
		}
	})
}
