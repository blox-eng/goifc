package geometry

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

func TestWriteGLB_NodeNamesAndValidity(t *testing.T) {
	f, err := step.ParseFile("testdata/synthetic/known_box.ifc")
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
	var buf bytes.Buffer
	if err := s.WriteGLB(&buf); err != nil {
		t.Fatalf("WriteGLB: %v", err)
	}
	b := buf.Bytes()
	if string(b[0:4]) != "glTF" {
		t.Fatalf("bad magic %q", b[0:4])
	}
	if binary.LittleEndian.Uint32(b[8:12]) != uint32(len(b)) {
		t.Fatal("header length != file size")
	}
	jsonLen := binary.LittleEndian.Uint32(b[12:16])
	var doc struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(b[20:20+jsonLen], &doc); err != nil {
		t.Fatalf("json chunk: %v", err)
	}
	found := false
	for _, n := range doc.Nodes {
		if n.Name == "0box" {
			found = true
		}
	}
	if !found {
		t.Errorf("no node named by GlobalId '0box'; nodes=%+v", doc.Nodes)
	}
}
