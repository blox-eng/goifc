package wasmapi

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blox-eng/goifc/geometry"
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

type joinsAnswer struct {
	Walls []struct {
		ID         string `json:"id"`
		Quantities struct {
			MeasuredJoins float64 `json:"measuredJoins"`
			MeasuredNet   float64 `json:"measuredNet"`
			OtherJoins    float64 `json:"otherJoins"`

			MeasuredFloorLine float64 `json:"measuredFloorLine"`
			OtherFloorLine    float64 `json:"otherFloorLine"`
		} `json:"quantities"`
	} `json:"walls"`
}

func TestBuild_Joins(t *testing.T) {
	req, err := os.ReadFile("testdata/build/joins.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	if glb == nil {
		t.Fatalf("answer %s, want a GLB", answer)
	}
	var a joinsAnswer
	if err := json.Unmarshal(answer, &a); err != nil {
		t.Fatal(err)
	}
	for _, w := range a.Walls {
		if w.ID != "T" {
			continue
		}
		q := w.Quantities
		if math.Abs(q.MeasuredJoins-0.25) > 1e-9 || math.Abs(q.MeasuredNet-9.75) > 1e-9 || q.OtherJoins != 0 {
			t.Errorf("T quantities %+v, want measuredJoins 0.25, measuredNet 9.75, otherJoins 0", q)
		}
		// The same plan built in Go: the answer's floor lines are its numbers,
		// the measured face less the 0.1 m partition.
		b, err := geometry.BuildPlan(geometry.Plan{Walls: []geometry.PlanWall{
			{ID: "T", Edge: [2][2]float64{{0, 0}, {4, 0}}, Height: 2.5, Thickness: 0.2, Side: geometry.SideLeft},
			{ID: "P", Edge: [2][2]float64{{2, 3}, {2, 0}}, Height: 2.5, Thickness: 0.1, Side: geometry.SideLeft,
				Joins: []geometry.Join{{End: geometry.EndB, Wall: "T"}}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		g := b.Walls[0].Wall.Quantities
		if q.MeasuredFloorLine != g.MeasuredFloorLine || q.OtherFloorLine != g.OtherFloorLine || math.Abs(q.MeasuredFloorLine-3.9) > 1e-9 {
			t.Errorf("T floor lines %v, %v, want Go's %v, %v (3.9 and 4)", q.MeasuredFloorLine, q.OtherFloorLine, g.MeasuredFloorLine, g.OtherFloorLine)
		}
		return
	}
	t.Fatalf("answer %s has no wall T", answer)
}

func TestBuild_RefusedJoin(t *testing.T) {
	req, err := os.ReadFile("testdata/build/refused-join.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	a := decodeAnswer(t, answer)
	if a.OK || glb != nil || len(a.Errors) != 1 {
		t.Fatalf("answer %s, want one refusal and no GLB", answer)
	}
	e := a.Errors[0]
	if e.Code != "not_on_face" || e.Item != "P" || e.Join == nil || *e.Join != 0 || e.Wall != nil || e.Opening != nil {
		t.Errorf("error %+v, want not_on_face on P join 0", e)
	}
}

func TestBuild_RejectsBadRequests(t *testing.T) {
	wall := func(fields string) string {
		return `{"walls": [{` + fields + `}]}`
	}
	join := func(j string) string {
		return `{"walls": [{"id": "P", "edge": [[2, 3], [2, 0]], "base": 0, "height": 2.5, "side": "left", "thickness": 0.1, "joins": [` + j + `]}]}`
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
		{"null in an edge point", wall(`"id": "w1", "edge": [[null, 0], [4, 0]], "base": 0, "height": 2.7, "side": "left", "thickness": 0.2`), "w1", "edge point 0"},
		{"null in a room outline", `{"rooms": [{"id": "r1", "outline": [[0, 0], [4, 0], [4, null]], "base": 0, "height": 2.7, "side": "left", "thickness": 0.2}]}`, "r1", "outline point 2"},
		{"null in a slab outline", `{"slabs": [{"id": "s1", "outline": [[0, 0], [1, 0], [null, 1]], "z": 0, "thickness": 0.2}]}`, "s1", "outline point 2"},
		{"null in a hole point", `{"slabs": [{"id": "s1", "outline": [[0, 0], [10, 0], [10, 8]], "holes": [[[1, 1], [null, 2], [2, 2]]], "z": 0, "thickness": 0.2}]}`, "s1", "holes[0] point 1"},
		{"hole with two points", `{"slabs": [{"id": "s1", "outline": [[0, 0], [10, 0], [10, 8]], "holes": [[[1, 1], [2, 2]]], "z": 0, "thickness": 0.2}]}`, "s1", "hole 0 needs at least 3 points"},
		{"null layer", wall(`"id": "w1", ` + ok + `, "layers": [0.1, null]`), "w1", "layers[1]"},
		{"join end not a or b", join(`{"end": "c", "wall": "T"}`), "P", "end"},
		{"join without end", join(`{"wall": "T"}`), "P", "end"},
		{"join without wall", join(`{"end": "a"}`), "P", "wall"},
		{"join with an unknown field", join(`{"end": "a", "wall": "T", "face": "x"}`), "", "face"},
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

func TestBuild_ElementCap(t *testing.T) {
	var outline, layers strings.Builder
	for i := range 64 {
		if i > 0 {
			outline.WriteByte(',')
		}
		a := 2 * math.Pi * float64(i) / 64
		fmt.Fprintf(&outline, "[%.6f, %.6f]", 10*math.Cos(a), 10*math.Sin(a))
	}
	for i := range 16 {
		if i > 0 {
			layers.WriteByte(',')
		}
		layers.WriteString("0.01")
	}
	var sb strings.Builder
	sb.WriteString(`{"rooms": [`)
	for i := range 98 { // 98 rooms × 64 walls × 16 layers = 100,352 elements
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id": "r%d", "outline": [%s], "base": 0, "height": 2.7, "side": "left", "layers": [%s]}`, i, outline.String(), layers.String())
	}
	sb.WriteString(`]}`)
	answer, glb := Build(testVersion, []byte(sb.String()))
	a := decodeAnswer(t, answer)
	if a.OK || glb != nil || len(a.Errors) != 1 || a.Errors[0].Code != "too_large" ||
		!strings.Contains(a.Errors[0].Message, "100,352") || !strings.Contains(a.Errors[0].Message, "100,000") {
		t.Errorf("answer %.300s, want one too_large naming 100,352 elements and the 100,000 limit", answer)
	}
}

func TestBuild_SlabHoles(t *testing.T) {
	req, err := os.ReadFile("testdata/build/slab-holes.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	if glb == nil {
		t.Fatalf("answer %s, want a GLB", answer)
	}
	var a struct {
		Slabs []struct {
			Quantities struct{ Area, HolePerimeter float64 } `json:"quantities"`
			Top        struct{ Holes [][][2]float64 }        `json:"top"`
		} `json:"slabs"`
	}
	if err := json.Unmarshal(answer, &a); err != nil || len(a.Slabs) != 1 {
		t.Fatalf("answer %s: %v", answer, err)
	}
	s := a.Slabs[0]
	if math.Abs(s.Quantities.Area-77) > 1e-9 || math.Abs(s.Quantities.HolePerimeter-7.4) > 1e-9 || len(s.Top.Holes) != 1 {
		t.Errorf("slab %+v, want area 77, holePerimeter 7.4, one top hole", s)
	}
}

func TestBuild_RefusedHole(t *testing.T) {
	req, err := os.ReadFile("testdata/build/refused-hole.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	a := decodeAnswer(t, answer)
	if a.OK || glb != nil || len(a.Errors) != 1 {
		t.Fatalf("answer %s, want one refusal and no GLB", answer)
	}
	e := a.Errors[0]
	if e.Code != "hole_outside" || e.Item != "s1" || e.Hole == nil || *e.Hole != 1 || e.Wall != nil || e.Join != nil {
		t.Errorf("error %+v, want hole_outside on s1 hole 1", e)
	}
}

func TestBuild_Roof(t *testing.T) {
	req, err := os.ReadFile("testdata/build/roof.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	if glb == nil {
		t.Fatalf("answer %s, want a GLB", answer)
	}
	var a struct {
		Roofs []struct {
			ID         string
			Quantities struct{ SlopeArea, PlanArea, Ridge float64 }
			Planes     []json.RawMessage
		} `json:"roofs"`
	}
	if err := json.Unmarshal(answer, &a); err != nil || len(a.Roofs) != 1 {
		t.Fatalf("answer %s: %v", answer, err)
	}
	r := a.Roofs[0]
	if r.ID != "r1" || math.Abs(r.Quantities.Ridge-2) > 1e-9 || math.Abs(r.Quantities.PlanArea-80) > 1e-9 ||
		math.Abs(r.Quantities.SlopeArea-80/math.Cos(math.Pi/6)) > 1e-9 || len(r.Planes) != 4 {
		t.Errorf("roof %+v, want ridge 2, plan area 80, slope area 80/cos 30, four planes", r)
	}
	if got := glbNodeNames(t, glb); !slices.Equal(got, []string{"r1"}) {
		t.Errorf("GLB nodes %v, want [r1]", got)
	}
	if strings.Contains(string(answer), `"holes"`) {
		t.Errorf("answer %s, want roofs with no hole lists", answer)
	}
}

func TestBuild_RoofLayersAndPreview(t *testing.T) {
	req, err := os.ReadFile("testdata/build/roof-layers.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	a := decodeAnswer(t, answer)
	if !a.OK || a.Preview == nil || a.Preview.Error != nil {
		t.Fatalf("answer %.300s, want ok with a preview", answer)
	}
	if got := glbNodeNames(t, glb); !slices.Equal(got, []string{"r1/0", "r1/1"}) {
		t.Errorf("GLB nodes %v, want [r1/0 r1/1]", got)
	}
}

func TestBuild_RefusedRoof(t *testing.T) {
	req, err := os.ReadFile("testdata/build/refused-roof.request.json")
	if err != nil {
		t.Fatal(err)
	}
	answer, glb := Build(testVersion, req)
	a := decodeAnswer(t, answer)
	if a.OK || glb != nil || len(a.Errors) != 1 {
		t.Fatalf("answer %s, want one refusal and no GLB", answer)
	}
	e := a.Errors[0]
	if e.Code != "roof_step" || e.Item != "r1" || e.Edge == nil || *e.Edge != 2 || e.Other != nil || e.Wall != nil || e.Hole != nil {
		t.Errorf("error %+v, want roof_step on r1 edge 2 and no other", e)
	}
}

func TestBuild_RoofInvalidInput(t *testing.T) {
	for _, c := range []struct{ name, req, want string }{
		{"no pitch", `{"roofs": [{"id": "r1", "outline": [[0, 0], [5, 0], [5, 4]], "eaves": 3, "thickness": 0.2}]}`, "pitch"},
		{"no eaves", `{"roofs": [{"id": "r1", "outline": [[0, 0], [5, 0], [5, 4]], "pitch": 30, "thickness": 0.2}]}`, "eaves"},
		{"null gable", `{"roofs": [{"id": "r1", "outline": [[0, 0], [5, 0], [5, 4]], "pitch": 30, "eaves": 3, "gables": [null], "thickness": 0.2}]}`, "gables"},
		{"both", `{"roofs": [{"id": "r1", "outline": [[0, 0], [5, 0], [5, 4]], "pitch": 30, "eaves": 3, "thickness": 0.2, "layers": [0.2]}]}`, "not both"},
	} {
		a := decodeAnswer(t, func() []byte { b, _ := Build(testVersion, []byte(c.req)); return b }())
		if a.OK || len(a.Errors) != 1 || a.Errors[0].Code != "invalid_input" || a.Errors[0].Item != "r1" || !strings.Contains(a.Errors[0].Message, c.want) {
			t.Errorf("%s: answer %+v, want invalid_input on r1 naming %q", c.name, a, c.want)
		}
	}
}

func TestBuild_RoofsCountTowardCaps(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"roofs": [`)
	for i := range maxItems + 1 {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id": "r%d", "outline": [[0, 0], [1, 0], [1, 1]], "pitch": 30, "eaves": 0, "thickness": 0.2}`, i)
	}
	sb.WriteString(`]}`)
	answer, _ := Build(testVersion, []byte(sb.String()))
	a := decodeAnswer(t, answer)
	if a.OK || len(a.Errors) != 1 || a.Errors[0].Code != "too_large" {
		t.Errorf("answer %.200s, want too_large", answer)
	}
}
