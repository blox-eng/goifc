package wasmapi

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// readFixtures are synthetic IFC files already in the repository: storeys,
// property sets on a type, a wall with a window, and a type with layers.
var readFixtures = []string{
	"../../testdata/two_storey_spanning.ifc",
	"../../model/testdata/synthetic/type_inheritance.ifc",
	"../../geometry/testdata/synthetic/netarea_rect_window.ifc",
	"../../model/testdata/synthetic/wall_layerset_attrs.ifc",
}

var allParts = []string{"nodes", "glb", "psets", "storeyPlans", "typeLayers"}

func readFixture(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRead_Goldens(t *testing.T) {
	for _, p := range readFixtures {
		src := readFixture(t, p)
		base := filepath.Join("testdata/read", strings.TrimSuffix(filepath.Base(p), ".ifc"))
		for _, c := range []struct {
			suffix  string
			include []string
		}{{"default", nil}, {"all", allParts}} {
			t.Run(filepath.Base(base)+"/"+c.suffix, func(t *testing.T) {
				answer, glb := Read(testVersion, src, c.include)
				if !json.Valid(answer) || !bytes.Contains(answer, []byte(`"ok":true`)) {
					t.Fatalf("answer %.400s, want ok", answer)
				}
				golden(t, base+"."+c.suffix+".answer.json", answer)
				golden(t, base+"."+c.suffix+".glb", glb)
			})
		}
	}
}

func TestRead_DefaultEqualsExplicit(t *testing.T) {
	src := readFixture(t, readFixtures[0])
	a1, g1 := Read(testVersion, src, nil)
	a2, g2 := Read(testVersion, src, []string{"glb", "nodes"})
	if !bytes.Equal(a1, a2) || !bytes.Equal(g1, g2) {
		t.Error("the default answer differs from asking for nodes and glb")
	}
}

func TestRead_PartsAreAbsent(t *testing.T) {
	answer, glb := Read(testVersion, readFixture(t, readFixtures[1]), []string{"nodes"})
	var a map[string]json.RawMessage
	if err := json.Unmarshal(answer, &a); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"psets", "storeyPlans", "typeLayers"} {
		if _, ok := a[k]; ok {
			t.Errorf("answer has %q, want it absent", k)
		}
	}
	if glb != nil {
		t.Error("a GLB came back, want none")
	}
	if string(a["parts"]) != `["nodes"]` {
		t.Errorf("parts %s, want [\"nodes\"]", a["parts"])
	}
}

func TestRead_RejectsInclude(t *testing.T) {
	src := readFixture(t, readFixtures[0])
	for _, c := range []struct {
		name    string
		include []string
		msg     string
	}{
		{"empty", []string{}, "no parts"},
		{"twice", []string{"nodes", "nodes"}, `"nodes"`},
		{"unknown", []string{"meshes"}, `"meshes"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer, glb := Read(testVersion, src, c.include)
			a := decodeAnswer(t, answer)
			if a.OK || glb != nil || len(a.Errors) != 1 || a.Errors[0].Code != "invalid_input" || !strings.Contains(a.Errors[0].Message, c.msg) {
				t.Errorf("answer %s, want one invalid_input naming %s", answer, c.msg)
			}
		})
	}
}

func TestRead_TooLarge(t *testing.T) {
	answer, glb := Read(testVersion, make([]byte, maxReadBytes+1), nil)
	a := decodeAnswer(t, answer)
	if a.OK || glb != nil || len(a.Errors) != 1 || a.Errors[0].Code != "too_large" || !strings.Contains(a.Errors[0].Message, "50,331,648") {
		t.Errorf("answer %s, want one too_large naming the 50,331,648-byte cap", answer)
	}
}

func TestRead_InvalidIFC(t *testing.T) {
	answer, glb := Read(testVersion, []byte("not an ifc"), nil)
	a := decodeAnswer(t, answer)
	if a.OK || glb != nil || len(a.Errors) != 1 || a.Errors[0].Code != "invalid_ifc" || a.Errors[0].Message == "" {
		t.Errorf("answer %s, want one invalid_ifc with the Go error text", answer)
	}
}

func TestRead_NonFinitePset(t *testing.T) {
	var warnings []string
	got := psetsOut("0GUID", map[string]map[string]any{"Pset_X": {"Big": math.Inf(1), "Ok": 1.5, "Name": "a"}}, &warnings)
	if got["Pset_X"]["Big"] != nil || got["Pset_X"]["Ok"] != 1.5 || got["Pset_X"]["Name"] != "a" {
		t.Errorf("psets %v, want Big null and the rest kept", got)
	}
	want := []string{"0GUID Pset_X.Big: not a finite number, written as null"}
	if !slices.Equal(warnings, want) {
		t.Errorf("warnings %q, want %q", warnings, want)
	}
}

func TestRead_ParentIndex(t *testing.T) {
	answer, _ := Read(testVersion, readFixture(t, readFixtures[0]), []string{"nodes"})
	var a struct {
		Nodes []struct {
			ParentIndex *int `json:"parentIndex"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(answer, &a); err != nil || len(a.Nodes) == 0 {
		t.Fatalf("nodes: %v, %d", err, len(a.Nodes))
	}
	if !bytes.Contains(answer, []byte(`"parentIndex":null`)) || a.Nodes[0].ParentIndex != nil {
		t.Error("the root's parentIndex is not null")
	}
	for i, n := range a.Nodes[1:] {
		if n.ParentIndex != nil && *n.ParentIndex > i {
			t.Errorf("node %d's parent %d does not come before it", i+1, *n.ParentIndex)
		}
	}
}
