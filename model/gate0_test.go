package model

// GATE-0 vertical-slice test — throwaway alongside internal/boxglb.
// TODO(#2209): delete when child 3 (common/ifc/geometry) supersedes the box-GLB stub.

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"

	"github.com/blox-eng/common/ifc/model/internal/boxglb"
	"github.com/blox-eng/common/ifc/step"
)

func TestGate0OneWallToGLB(t *testing.T) {
	const path = "testdata/real/kb645.ifc"
	skipIfMissing(t, path)
	f, err := step.ParseFile(path)
	if err != nil {
		t.Fatalf("parse kb645: %v", err)
	}
	walls := f.ByType("IfcWallStandardCase")
	if len(walls) == 0 {
		walls = f.ByType("IfcWall")
	}
	if len(walls) == 0 {
		t.Fatal("no walls in kb645")
	}
	scale := UnitScale(f)
	w := walls[0]
	m := LocalPlacement(w)
	// scale translation to meters
	m[12], m[13], m[14] = m[12]*scale, m[13]*scale, m[14]*scale
	q, _ := QtoQuantities(f, w, scale)
	l, h, wdt := valOr(q.Length, 1), valOr(q.Height, 3), valOr(q.Width, 0.2)

	const glbPath = "testdata/render/wall.glb"
	if err := os.MkdirAll("testdata/render", 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(glbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	if err := boxglb.WriteBox(out, m, l, h, wdt); err != nil {
		t.Fatalf("write glb: %v", err)
	}
	t.Logf("wrote wall.glb: guid=%s L=%.2f H=%.2f W=%.2f origin=(%.2f,%.2f,%.2f)",
		strVal(w, attrGlobalID), l, h, wdt, m[12], m[13], m[14])

	t.Run("glb_structural_validity", func(t *testing.T) {
		validateGLB(t, glbPath)
	})
}

func valOr(p *float64, d float64) float64 {
	if p != nil {
		return *p
	}
	return d
}

// validateGLB re-opens a .glb file and asserts it is a structurally valid
// binary glTF: 12-byte header (magic "glTF", version 2, total length ==
// file size), a JSON chunk with parseable content, and a BIN chunk.
func validateGLB(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(b) < 12+8 {
		t.Fatalf("file too small for GLB header: %d bytes", len(b))
	}
	magic := string(b[0:4])
	if magic != "glTF" {
		t.Fatalf("bad magic: %q", magic)
	}
	version := binary.LittleEndian.Uint32(b[4:8])
	if version != 2 {
		t.Fatalf("bad version: %d", version)
	}
	total := binary.LittleEndian.Uint32(b[8:12])
	if int(total) != len(b) {
		t.Fatalf("header length %d != file size %d", total, len(b))
	}

	off := 12
	jsonLen := int(binary.LittleEndian.Uint32(b[off : off+4]))
	off += 4
	jsonType := string(b[off : off+4])
	off += 4
	if jsonType != "JSON" {
		t.Fatalf("first chunk type %q, want JSON", jsonType)
	}
	jsonBytes := b[off : off+jsonLen]
	off += jsonLen

	var doc map[string]any
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		t.Fatalf("JSON chunk not parseable: %v", err)
	}

	if off+8 > len(b) {
		t.Fatalf("missing BIN chunk header")
	}
	binLen := int(binary.LittleEndian.Uint32(b[off : off+4]))
	off += 4
	binType := string(b[off : off+4])
	off += 4
	if binType != "BIN\x00" {
		t.Fatalf("second chunk type %q, want BIN\\0", binType)
	}
	if off+binLen != len(b) {
		t.Fatalf("BIN chunk length %d + offset %d != file size %d", binLen, off, len(b))
	}

	accessors, _ := doc["accessors"].([]any)
	if len(accessors) != 2 {
		t.Fatalf("expected 2 accessors, got %d", len(accessors))
	}
	posCount := int(accessors[0].(map[string]any)["count"].(float64))
	idxCount := int(accessors[1].(map[string]any)["count"].(float64))
	if posCount != 8 {
		t.Errorf("POSITION count = %d, want 8", posCount)
	}
	if idxCount != 36 {
		t.Errorf("indices count = %d, want 36", idxCount)
	}
	t.Logf("glb valid: size=%d bytes POSITION.count=%d indices.count=%d", len(b), posCount, idxCount)
}
