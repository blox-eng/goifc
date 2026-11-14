package wasmapi

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type testAnswer struct {
	GoifcVersion string  `json:"goifcVersion"`
	OK           bool    `json:"ok"`
	Errors       []Error `json:"errors"`
	Walls        []struct {
		ID string `json:"id"`
	} `json:"walls"`
	Slabs []struct {
		ID string `json:"id"`
	} `json:"slabs"`
	Preview *struct {
		Detail string `json:"detail"`
		Error  *Error `json:"error"`
	} `json:"preview"`
}

func decodeAnswer(t *testing.T, answer []byte) testAnswer {
	t.Helper()
	var a testAnswer
	if err := json.Unmarshal(answer, &a); err != nil {
		t.Fatalf("answer is not JSON: %v\n%s", err, answer)
	}
	return a
}

// glbNodeNames reads the node names from a GLB's JSON chunk.
func glbNodeNames(t *testing.T, glb []byte) []string {
	t.Helper()
	if len(glb) < 20 || string(glb[:4]) != "glTF" {
		t.Fatalf("not a GLB: %d bytes", len(glb))
	}
	n := binary.LittleEndian.Uint32(glb[12:16])
	var doc struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(glb[20:20+n], &doc); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, nd := range doc.Nodes {
		if nd.Name != "Z_UP_TO_Y_UP" {
			names = append(names, nd.Name)
		}
	}
	return names
}

func TestBuild_Goldens(t *testing.T) {
	reqs, err := filepath.Glob("testdata/build/*.request.json")
	if err != nil || len(reqs) == 0 {
		t.Fatalf("no golden requests: %v", err)
	}
	for _, p := range reqs {
		name := strings.TrimSuffix(p, ".request.json")
		t.Run(filepath.Base(name), func(t *testing.T) {
			req, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			answer, glb := Build(testVersion, req)
			golden(t, name+".answer.json", answer)
			if glb != nil {
				golden(t, name+".glb", glb)
			} else if _, err := os.Stat(name + ".glb"); err == nil {
				t.Errorf("no GLB, but %s.glb exists", name)
			}
		})
	}
}

func TestBuild_EmptyRequest(t *testing.T) {
	answer, glb := Build(testVersion, []byte(`{}`))
	if !strings.Contains(string(answer), `"walls":[]`) || !strings.Contains(string(answer), `"slabs":[]`) {
		t.Errorf("answer %s, want empty walls and slabs lists", answer)
	}
	if glb == nil || len(glbNodeNames(t, glb)) != 0 {
		t.Errorf("want a valid GLB with no element nodes")
	}
	a := decodeAnswer(t, answer)
	if !a.OK || a.Preview != nil {
		t.Errorf("ok %v preview %v, want ok and no preview", a.OK, a.Preview)
	}
	answer, _ = Build(testVersion, []byte(`{"preview": true}`))
	a = decodeAnswer(t, answer)
	if !a.OK || a.Preview == nil || a.Preview.Error == nil || a.Preview.Error.Code != "nothing_to_draw" {
		t.Errorf("answer %s, want ok with a nothing_to_draw preview error", answer)
	}
}

func TestBuild_ElementIDs(t *testing.T) {
	req := `{"rooms": [{"id": "r1", "outline": [[0, 0], [5, 0], [5, 4], [0, 4]], "base": 0, "height": 2.7, "layers": [0.02, 0.2], "side": "left"}]}`
	answer, glb := Build(testVersion, []byte(req))
	a := decodeAnswer(t, answer)
	var want, got []string
	for i := range 4 {
		for j := range 2 {
			want = append(want, fmt.Sprintf("r1/%d/%d", i, j))
		}
	}
	for _, w := range a.Walls {
		got = append(got, w.ID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("wall ids %v, want %v", got, want)
	}
	if names := glbNodeNames(t, glb); !slices.Equal(names, want) {
		t.Errorf("GLB node names %v, want %v", names, want)
	}
}

func TestBuild_RefusalInLayeredRoom(t *testing.T) {
	req, err := os.ReadFile("testdata/build/refused-layered-room.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	a := decodeAnswer(t, answer)
	if a.OK || glb != nil || len(a.Errors) != 1 {
		t.Fatalf("answer %s, want one refusal and no GLB", answer)
	}
	e := a.Errors[0]
	if e.Code != "too_wide" || e.Item != "r1" || e.Wall == nil || *e.Wall != 2 || e.Opening == nil || *e.Opening != 0 {
		t.Errorf("error %+v, want too_wide on r1 wall 2 opening 0", e)
	}
}

func TestBuild_RejectsBadRequests(t *testing.T) {
	wall := func(fields string) string {
		return `{"walls": [{` + fields + `}]}`
	}
	const ok = `"edge": [[0, 0], [4, 0]], "base": 0, "height": 2.7, "side": "left"`
	for _, c := range []struct {
		name, req, item, msg string
	}{
		{"malformed JSON", `{"walls": [`, "", ""},
		{"not an object", `[]`, "", "object"},
		{"null", `null`, "", "object"},
		{"trailing object", `{}{}`, "", "after the request"},
		{"trailing text", `{} x`, "", "after the request"},
		{"unknown field", wall(`"id": "w1", ` + ok + `, "thikness": 0.2`), "", "thikness"},
		{"key in another case", wall(`"id": "w1", ` + ok + `, "Thickness": 0.2`), "", "Thickness"},
		{"list key in another case", `{"SLABS": [{"id": "s1", "outline": [[0, 0], [1, 0], [1, 1]], "z": 0, "thickness": 0.2}]}`, "", "SLABS"},
		{"case variant overriding a key", wall(`"id": "a", "ID": "b", ` + ok + `, "thickness": 0.2`), "", "ID"},
		{"key given twice", wall(`"id": "a", "id": "b", ` + ok + `, "thickness": 0.2`), "", "twice"},
		{"opening key in another case", wall(`"id": "w1", ` + ok + `, "thickness": 0.2, "openings": [{"offset": 1, "sill": 0, "Width": 1, "height": 2}]`), "", "Width"},
		{"id with a slash", wall(`"id": "a/b", ` + ok + `, "thickness": 0.2`), "a/b", "id"},
		{"id too long", wall(`"id": "` + strings.Repeat("a", 65) + `", ` + ok + `, "thickness": 0.2`), strings.Repeat("a", 65), "id"},
		{"empty id", wall(`"id": "", ` + ok + `, "thickness": 0.2`), "", "id"},
		{"missing id", wall(ok + `, "thickness": 0.2`), "", "id"},
		{"same id on two kinds", `{"walls": [{"id": "a", ` + ok + `, "thickness": 0.2}], "slabs": [{"id": "a", "outline": [[0, 0], [5, 0], [5, 4]], "z": 0, "thickness": 0.2}]}`, "a", "more than once"},
		{"thickness and layers", wall(`"id": "w1", ` + ok + `, "thickness": 0.2, "layers": [0.2]`), "w1", "thickness or layers"},
		{"neither thickness nor layers", wall(`"id": "w1", ` + ok), "w1", "thickness or layers"},
		{"bad side", wall(`"id": "w1", "edge": [[0, 0], [4, 0]], "base": 0, "height": 2.7, "side": "up", "thickness": 0.2`), "w1", "side"},
		{"missing side", wall(`"id": "w1", "edge": [[0, 0], [4, 0]], "base": 0, "height": 2.7, "thickness": 0.2`), "w1", "side"},
		{"missing height", wall(`"id": "w1", "edge": [[0, 0], [4, 0]], "base": 0, "side": "left", "thickness": 0.2`), "w1", "height"},
		{"edge with three points", wall(`"id": "w1", "edge": [[0, 0], [4, 0], [5, 0]], "base": 0, "height": 2.7, "side": "left", "thickness": 0.2`), "w1", "edge"},
		{"point with three numbers", wall(`"id": "w1", "edge": [[0, 0, 1], [4, 0]], "base": 0, "height": 2.7, "side": "left", "thickness": 0.2`), "w1", "edge"},
		{"opening without width", wall(`"id": "w1", ` + ok + `, "thickness": 0.2, "openings": [{"offset": 1, "sill": 0, "height": 2}]`), "w1", "width"},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer, glb := Build(testVersion, []byte(c.req))
			a := decodeAnswer(t, answer)
			if a.OK || glb != nil || len(a.Errors) != 1 {
				t.Fatalf("answer %s, want one error and no GLB", answer)
			}
			e := a.Errors[0]
			if e.Code != "invalid_input" || e.Item != c.item || !strings.Contains(e.Message, c.msg) {
				t.Errorf("error %+v, want invalid_input on item %q naming %q", e, c.item, c.msg)
			}
		})
	}
}

func TestBuild_Caps(t *testing.T) {
	big := make([]byte, maxRequestBytes+1)
	for i := range big {
		big[i] = ' '
	}
	big[0], big[1] = '{', '}'
	answer, _ := Build(testVersion, big)
	a := decodeAnswer(t, answer)
	if a.OK || len(a.Errors) != 1 || a.Errors[0].Code != "too_large" || a.Errors[0].Item != "" || !strings.Contains(a.Errors[0].Message, "16,777,216") {
		t.Errorf("answer %s, want one too_large naming the 16,777,216-byte cap", answer)
	}

	var sb strings.Builder
	sb.WriteString(`{"slabs": [`)
	for i := range maxItems + 1 {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id": "s%d", "outline": [[0, 0], [1, 0], [1, 1]], "z": 0, "thickness": 0.2}`, i)
	}
	sb.WriteString(`]}`)
	answer, _ = Build(testVersion, []byte(sb.String()))
	a = decodeAnswer(t, answer)
	if a.OK || len(a.Errors) != 1 || a.Errors[0].Code != "too_large" || !strings.Contains(a.Errors[0].Message, "10,000") {
		t.Errorf("answer %.300s, want one too_large naming the 10,000-item limit", answer)
	}
}

func TestBuild_OKMatchesErrors(t *testing.T) {
	reqs, _ := filepath.Glob("testdata/build/*.request.json")
	for _, p := range reqs {
		req, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		answer, glb := Build(testVersion, req)
		a := decodeAnswer(t, answer)
		if a.OK != (len(a.Errors) == 0) || (glb == nil) == a.OK || a.GoifcVersion != testVersion {
			t.Errorf("%s: ok %v, %d errors, GLB %v, version %q", p, a.OK, len(a.Errors), glb != nil, a.GoifcVersion)
		}
	}
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"w1": true, "A-z_0.9:x": true, strings.Repeat("a", 64): true,
		"": false, strings.Repeat("a", 65): false, "a/b": false, "a b": false, "é": false, "a\x00": false,
	} {
		if got := validID(id); got != want {
			t.Errorf("validID(%q) = %v, want %v", id, got, want)
		}
	}
}
