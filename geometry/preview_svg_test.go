package geometry

import (
	"bytes"
	"encoding/xml"
	"flag"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var updatePreview = flag.Bool("update", false, "rewrite geometry/testdata/preview golden files")

type previewFixture struct {
	name  string
	walls []Wall
	slabs []Slab
}

func mustSlab(t *testing.T, outline [][2]float64, z, thickness float64) Slab {
	t.Helper()
	s, err := BuildSlab(outline, z, thickness)
	if err != nil {
		t.Fatalf("BuildSlab: %v", err)
	}
	return s
}

func previewRoom(t *testing.T) previewFixture {
	t.Helper()
	outline := [][2]float64{{0, 0}, {4, 0}, {4, 3}, {0, 3}}
	ws, err := BuildRoomWalls(outline, 0, 2.7, 0.2, SideLeft, [][]Opening{
		{{Offset: 0.5, Sill: 0, Width: 0.9, Height: 2.1}}, nil, nil,
		{{Offset: 1, Sill: 0.9, Width: 1.2, Height: 1.4}},
	})
	if err != nil {
		t.Fatalf("BuildRoomWalls: %v", err)
	}
	return previewFixture{"room", ws, []Slab{mustSlab(t, outline, 0, 0.2)}}
}

func previewLayeredWall(t *testing.T, edge [2][2]float64) previewFixture {
	t.Helper()
	ws, err := BuildWallLayers(edge, 0, 2.7, []float64{0.015, 0.20, 0.10}, SideLeft, []Opening{
		{Offset: 0.5, Sill: 0.9, Width: 1.2, Height: 1.4},
		{Offset: 2.5, Sill: 1.0, Width: 0.6, Height: 0.4, Depth: 0.01},
	})
	if err != nil {
		t.Fatalf("BuildWallLayers: %v", err)
	}
	return previewFixture{"layered-wall", ws, nil}
}

func previewLHouse(t *testing.T) previewFixture {
	t.Helper()
	outline := [][2]float64{{0, 0}, {8, 0}, {8, 4}, {4, 4}, {4, 7}, {0, 7}}
	win := []Opening{{Offset: 1.5, Sill: 0.9, Width: 1.2, Height: 1.4}}
	ws, err := BuildRoomWalls(outline, 0, 2.7, 0.25, SideLeft, [][]Opening{win, nil, nil, nil, nil, win})
	if err != nil {
		t.Fatalf("BuildRoomWalls: %v", err)
	}
	return previewFixture{"l-house", ws, []Slab{mustSlab(t, outline, 0, 0.2)}}
}

func previewFixtures(t *testing.T) []previewFixture {
	return []previewFixture{previewRoom(t), previewLayeredWall(t, [2][2]float64{{0, 0}, {4, 0}}), previewLHouse(t)}
}

func mustPreview(t *testing.T, walls []Wall, slabs []Slab) ([]byte, Detail) {
	t.Helper()
	var buf bytes.Buffer
	d, err := WritePreviewSVG(&buf, walls, slabs)
	if err != nil {
		t.Fatalf("WritePreviewSVG: %v", err)
	}
	return buf.Bytes(), d
}

func TestWritePreviewSVG_Golden(t *testing.T) {
	for _, f := range previewFixtures(t) {
		got, d := mustPreview(t, f.walls, f.slabs)
		if d != DetailExact {
			t.Errorf("%s: detail %q, want exact", f.name, d)
		}
		path := filepath.Join("testdata", "preview", f.name+".svg")
		if *updatePreview {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run with -update to create it)", f.name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: SVG differs from %s (run with -update after checking the picture)", f.name, path)
		}
	}
}

// visibleSurface checks painter order: at sample points on a 2 cm grid, the
// last drawn shape that covers the point is the one nearest the viewer there.
func visibleSurface(t *testing.T, name string, shapes []previewShape) {
	t.Helper()
	type flat struct {
		loops  [][][2]float64
		lo, hi [2]float64
		n      v3
		off    float64
	}
	fs := make([]flat, len(shapes))
	lo, hi := [2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}
	for i, s := range shapes {
		f := flat{lo: [2]float64{math.Inf(1), math.Inf(1)}, hi: [2]float64{math.Inf(-1), math.Inf(-1)}, n: s.normal, off: s.offset}
		for _, l := range s.loops {
			var pl [][2]float64
			for _, p := range l {
				q := previewPoint(p)
				pl = append(pl, q)
				for k := range 2 {
					f.lo[k], f.hi[k] = math.Min(f.lo[k], q[k]), math.Max(f.hi[k], q[k])
				}
			}
			f.loops = append(f.loops, pl)
		}
		for k := range 2 {
			lo[k], hi[k] = math.Min(lo[k], f.lo[k]), math.Max(hi[k], f.hi[k])
		}
		fs[i] = f
	}
	const step, edge = 0.02, 0.01
	inside := func(f flat, q [2]float64) (in, near bool) {
		for _, l := range f.loops {
			for i := range l {
				a, b := l[i], l[(i+1)%len(l)]
				if segDist(q, a, b) < edge {
					near = true
				}
				if (a[1] > q[1]) != (b[1] > q[1]) && q[0] < a[0]+(q[1]-a[1])*(b[0]-a[0])/(b[1]-a[1]) {
					in = !in
				}
			}
		}
		return in, near
	}
	bad := 0
	for x := lo[0]; x <= hi[0]; x += step {
	sample:
		for y := lo[1]; y <= hi[1]; y += step {
			q := [2]float64{x, y}
			base := addv(scalev(previewRight, q[0]), scalev(previewUp, -q[1]))
			last, nearest := -1, -1
			nearT := math.Inf(1)
			for i, f := range fs {
				if q[0] < f.lo[0]-edge || q[0] > f.hi[0]+edge || q[1] < f.lo[1]-edge || q[1] > f.hi[1]+edge {
					continue
				}
				in, near := inside(f, q)
				if near {
					continue sample
				}
				if !in {
					continue
				}
				last = i
				tt := (f.off - dotv(f.n, base)) / dotv(f.n, previewDir)
				if tt < nearT-1e-6 || nearest >= 0 && math.Abs(tt-nearT) <= 1e-6 {
					nearT, nearest = math.Min(tt, nearT), i
				}
			}
			if last != nearest {
				if bad < 5 {
					t.Errorf("%s: at %.2f,%.2f shape %d (%s, elem %d) is drawn last, but shape %d (%s, elem %d) is nearer",
						name, x, y, last, shapes[last].class, shapes[last].elem, nearest, shapes[nearest].class, shapes[nearest].elem)
				}
				bad++
			}
		}
	}
	if bad > 0 {
		t.Errorf("%s: %d samples show a farther shape over a nearer one", name, bad)
	}
}

func segDist(q, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	l := dx*dx + dy*dy
	u := 0.0
	if l > 0 {
		u = math.Max(0, math.Min(1, ((q[0]-a[0])*dx+(q[1]-a[1])*dy)/l))
	}
	return math.Hypot(q[0]-a[0]-u*dx, q[1]-a[1]-u*dy)
}

func mustOrdered(t *testing.T, walls []Wall, slabs []Slab) []previewShape {
	t.Helper()
	s, err := previewShapesFor(walls, slabs)
	if err != nil {
		t.Fatalf("previewShapesFor: %v", err)
	}
	return s
}

func TestWritePreviewSVG_VisibleSurface(t *testing.T) {
	fs := append(previewFixtures(t), previewLayeredWall(t, [2][2]float64{{4, 0}, {0, 0}}))
	fs[len(fs)-1].name = "layered-wall-measured-south"
	for _, f := range fs {
		visibleSurface(t, f.name, mustOrdered(t, f.walls, f.slabs))
	}
}

func TestWritePreviewSVG_InputOrderStillVisible(t *testing.T) {
	f := previewLHouse(t)
	slices.Reverse(f.walls)
	visibleSurface(t, "l-house reversed", mustOrdered(t, f.walls, f.slabs))
}

func TestWritePreviewSVG_OnlyAllowedMarkup(t *testing.T) {
	tags := map[string]bool{"svg": true, "g": true, "polygon": true, "path": true}
	attrs := map[string]bool{"xmlns": true, "viewBox": true, "data-detail": true, "class": true, "points": true, "d": true, "fill-rule": true}
	classes := map[string]bool{"top": true, "left": true, "right": true, "opening": true}
	for _, f := range previewFixtures(t) {
		out, d := mustPreview(t, f.walls, f.slabs)
		dec := xml.NewDecoder(bytes.NewReader(out))
		shapes := 0
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: not XML: %v", f.name, err)
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				if cd, ok := tok.(xml.CharData); ok && strings.TrimSpace(string(cd)) != "" {
					t.Errorf("%s: text %q in the SVG", f.name, cd)
				}
				continue
			}
			if !tags[se.Name.Local] {
				t.Errorf("%s: tag %q", f.name, se.Name.Local)
			}
			for _, a := range se.Attr {
				if !attrs[a.Name.Local] {
					t.Errorf("%s: attribute %q on %s", f.name, a.Name.Local, se.Name.Local)
				}
				switch {
				case se.Name.Local == "svg" && a.Name.Local == "data-detail" && a.Value != string(d):
					t.Errorf("%s: data-detail %q, returned %q", f.name, a.Value, d)
				case se.Name.Local == "g" && a.Name.Local == "class" && a.Value != "goifc-preview":
					t.Errorf("%s: g class %q", f.name, a.Value)
				case (se.Name.Local == "polygon" || se.Name.Local == "path") && a.Name.Local == "class":
					shapes++
					if !classes[a.Value] {
						t.Errorf("%s: class %q", f.name, a.Value)
					}
				}
			}
		}
		if want := len(mustOrdered(t, f.walls, f.slabs)); shapes != want {
			t.Errorf("%s: %d shapes in the SVG, %d drawn", f.name, shapes, want)
		}
	}
}

func TestWritePreviewSVG_SameBytesTwice(t *testing.T) {
	f := previewLHouse(t)
	a, _ := mustPreview(t, f.walls, f.slabs)
	b, _ := mustPreview(t, f.walls, f.slabs)
	if !bytes.Equal(a, b) {
		t.Error("two calls gave different bytes")
	}
}

func TestWritePreviewSVG_NoFaceAwayFromViewer(t *testing.T) {
	for _, f := range previewFixtures(t) {
		for _, s := range mustOrdered(t, f.walls, f.slabs) {
			if dotv(s.normal, previewDir) >= -1e-6 {
				t.Errorf("%s: %s shape of element %d faces away or is edge-on (%v)", f.name, s.class, s.elem, s.normal)
			}
		}
	}
}

func TestWritePreviewSVG_Refusals(t *testing.T) {
	room := previewRoom(t)
	nanVert := slices.Clone(room.walls)
	nanVert[0].Element.Verts = slices.Clone(nanVert[0].Element.Verts)
	nanVert[0].Element.Verts[0] = float32(math.NaN())
	nanNormal := slices.Clone(room.walls)
	nanNormal[0].Measured.Normal[0] = math.NaN()
	nanWidth := slices.Clone(room.walls)
	nanWidth[0].Measured.Openings = []Opening{{Offset: 0.5, Width: math.NaN(), Height: 2}}
	manyTris := []Wall{{Element: Element{Tris: make([]uint32, 3*(maxPreviewTriangles+1))}}}
	cases := []struct {
		name  string
		walls []Wall
		slabs []Slab
		want  []string
	}{
		{"empty", nil, nil, []string{"nothing to draw"}},
		{"elements", make([]Wall, maxPreviewElements+1), nil, []string{"100,001", "100,000"}},
		{"triangles", manyTris, nil, []string{"4,000,001", "4,000,000"}},
		{"NaN vertex", nanVert, room.slabs, []string{"wall 0", "vertex"}},
		{"NaN normal", nanNormal, room.slabs, []string{"wall 0", "measured face"}},
		{"NaN width", nanWidth, room.slabs, []string{"wall 0", "opening 0"}},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		d, err := WritePreviewSVG(&buf, c.walls, c.slabs)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: error %q does not name %q", c.name, err, w)
			}
		}
		if d != "" || buf.Len() != 0 {
			t.Errorf("%s: detail %q and %d bytes written, want none", c.name, d, buf.Len())
		}
	}
}

func TestWritePreviewSVG_EmptyElementIsSkipped(t *testing.T) {
	room := previewRoom(t)
	empty := Slab{}
	got, _ := mustPreview(t, room.walls, append(slices.Clone(room.slabs), empty))
	want, _ := mustPreview(t, room.walls, room.slabs)
	if !bytes.Equal(got, want) {
		t.Error("an empty slab changed the drawing")
	}
	var buf bytes.Buffer
	if _, err := WritePreviewSVG(&buf, nil, []Slab{empty}); err == nil || !strings.Contains(err.Error(), "nothing to draw") {
		t.Errorf("only empty elements: err %v, want nothing to draw", err)
	}
}
