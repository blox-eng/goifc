// Package boxglb is a THROWAWAY Gate-0 box-GLB writer for #2208's vertical slice.
// It exists only to prove the tokenize→semantic→GLB→render pipeline shape and is
// superseded by common/ifc/geometry (child 3, #2209). Do not build on it.
//
// TODO(#2209): superseded by common/ifc/geometry — delete this package once
// the real geometry-tier renderer lands.
package boxglb

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
)

// WriteBox writes a single-mesh binary glTF box of size l×wdt×h (X,Z,Y meters)
// centered at the origin of m and placed by m's translation.
func WriteBox(w io.Writer, m [16]float64, l, h, wdt float64) error {
	hx, hy, hz := l/2, h/2, wdt/2
	tx, ty, tz := m[12], m[13], m[14]
	// 8 corners
	c := [][3]float32{
		{f(-hx + tx), f(-hy + ty), f(-hz + tz)}, {f(hx + tx), f(-hy + ty), f(-hz + tz)},
		{f(hx + tx), f(hy + ty), f(-hz + tz)}, {f(-hx + tx), f(hy + ty), f(-hz + tz)},
		{f(-hx + tx), f(-hy + ty), f(hz + tz)}, {f(hx + tx), f(-hy + ty), f(hz + tz)},
		{f(hx + tx), f(hy + ty), f(hz + tz)}, {f(-hx + tx), f(hy + ty), f(hz + tz)},
	}
	idx := []uint16{
		0, 1, 2, 0, 2, 3, 4, 6, 5, 4, 7, 6, 0, 4, 5, 0, 5, 1,
		1, 5, 6, 1, 6, 2, 2, 6, 7, 2, 7, 3, 3, 7, 4, 3, 4, 0,
	}
	// binary buffer: positions then indices
	var bin []byte
	minv := [3]float32{c[0][0], c[0][1], c[0][2]}
	maxv := minv
	for _, p := range c {
		for k := 0; k < 3; k++ {
			if p[k] < minv[k] {
				minv[k] = p[k]
			}
			if p[k] > maxv[k] {
				maxv[k] = p[k]
			}
		}
		bin = appendF32(bin, p[0], p[1], p[2])
	}
	idxOffset := len(bin)
	for _, v := range idx {
		bin = binary.LittleEndian.AppendUint16(bin, v)
	}
	for len(bin)%4 != 0 {
		bin = append(bin, 0)
	}

	gltf := map[string]any{
		"asset":  map[string]any{"version": "2.0", "generator": "boxglb (throwaway #2208)"},
		"scenes": []any{map[string]any{"nodes": []int{0}}},
		"scene":  0,
		"nodes":  []any{map[string]any{"mesh": 0}},
		"meshes": []any{map[string]any{"primitives": []any{map[string]any{
			"attributes": map[string]int{"POSITION": 0}, "indices": 1, "mode": 4}}}},
		"buffers": []any{map[string]any{"byteLength": len(bin)}},
		"bufferViews": []any{
			map[string]any{"buffer": 0, "byteOffset": 0, "byteLength": idxOffset, "target": 34962},
			map[string]any{"buffer": 0, "byteOffset": idxOffset, "byteLength": len(bin) - idxOffset, "target": 34963},
		},
		"accessors": []any{
			map[string]any{"bufferView": 0, "componentType": 5126, "count": 8, "type": "VEC3",
				"min": []float32{minv[0], minv[1], minv[2]}, "max": []float32{maxv[0], maxv[1], maxv[2]}},
			map[string]any{"bufferView": 1, "componentType": 5123, "count": len(idx), "type": "SCALAR"},
		},
	}
	jsonChunk, err := json.Marshal(gltf)
	if err != nil {
		return err
	}
	for len(jsonChunk)%4 != 0 {
		jsonChunk = append(jsonChunk, ' ')
	}

	total := 12 + 8 + len(jsonChunk) + 8 + len(bin)
	hdr := make([]byte, 0, total)
	hdr = append(hdr, []byte("glTF")...)
	hdr = binary.LittleEndian.AppendUint32(hdr, 2)
	hdr = binary.LittleEndian.AppendUint32(hdr, uint32(total))
	hdr = binary.LittleEndian.AppendUint32(hdr, uint32(len(jsonChunk)))
	hdr = append(hdr, []byte("JSON")...)
	hdr = append(hdr, jsonChunk...)
	hdr = binary.LittleEndian.AppendUint32(hdr, uint32(len(bin)))
	hdr = append(hdr, []byte("BIN\x00")...)
	hdr = append(hdr, bin...)
	_, err = w.Write(hdr)
	return err
}

func f(v float64) float32 { return float32(v) }
func appendF32(b []byte, xs ...float32) []byte {
	for _, x := range xs {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(x))
	}
	return b
}
