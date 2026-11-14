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
	"time"

	"github.com/blox-eng/goifc/model"
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
	s, err := previewShapesFor(walls, slabs, DetailExact)
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
		{"vertices", []Wall{{Element: Element{Verts: make([]float32, 3*(maxPreviewVertices+1))}}}, nil, []string{"4,000,001 vertices", "4,000,000"}},
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

func TestBoxShapes_ObliqueWallIsItsFootprint(t *testing.T) {
	w := mustWall(t, [2][2]float64{{0, 0}, {3, 3}}, 0, 2.5, 0.2, SideRight, previewWindow)
	got, err := boxShapes(w.Element, 0)
	if err != nil {
		t.Fatal(err)
	}
	var top *previewShape
	for i := range got {
		if got[i].class == "top" {
			top = &got[i]
		}
	}
	if top == nil || len(top.loops) != 1 || len(top.loops[0]) != 4 {
		t.Fatalf("shapes %v, want a top of 4 points", got)
	}
	f := w.Measured
	L, T := w.Quantities.Length, 0.2
	for _, c := range []v3{
		f.Origin,
		addv(f.Origin, scalev(f.U, L)),
		subv(addv(f.Origin, scalev(f.U, L)), scalev(f.Normal, T)),
		subv(f.Origin, scalev(f.Normal, T)),
	} {
		c[2] = 2.5
		found := false
		for _, p := range top.loops[0] {
			if math.Abs(p[0]-c[0]) < 1e-3 && math.Abs(p[1]-c[1]) < 1e-3 && math.Abs(p[2]-c[2]) < 1e-3 {
				found = true
			}
		}
		if !found {
			t.Errorf("footprint corner %v is not on the top %v", c, top.loops[0])
		}
	}
}

// previewRoomsModel is the spec's measured model, rooms of three-layer walls
// with a door, two windows and a niche, plus a slab each, 13 elements a room.
func previewRoomsModel(t *testing.T, rooms int) ([]Wall, []Slab) {
	t.Helper()
	var walls []Wall
	var slabs []Slab
	ops := [][]Opening{
		{{Offset: 0.5, Sill: 0, Width: 0.9, Height: 2.1}},
		{{Offset: 1.2, Sill: 0.9, Width: 1.2, Height: 1.4}},
		{{Offset: 1.0, Sill: 0.9, Width: 1.5, Height: 1.4}},
		{{Offset: 1.0, Sill: 1.0, Width: 0.6, Height: 0.4, Depth: 0.01}},
	}
	for room := range rooms {
		x0, y0 := float64(room%12)*5, float64(room/12)*4.5
		out := [][2]float64{{x0, y0}, {x0 + 4.5, y0}, {x0 + 4.5, y0 + 4}, {x0, y0 + 4}}
		ws, err := BuildRoomWallLayers(out, 0, 2.7, []float64{0.015, 0.20, 0.10}, SideLeft, ops)
		if err != nil {
			t.Fatal(err)
		}
		for _, layers := range ws {
			walls = append(walls, layers...)
		}
		slabs = append(slabs, mustSlab(t, out, 0, 0.2))
	}
	return walls, slabs
}

func TestWritePreviewSVG_BigModelDrawsBoxes(t *testing.T) {
	walls, slabs := previewRoomsModel(t, 240)
	if n := len(walls) + len(slabs); n != 3120 {
		t.Fatalf("%d elements, want 3,120", n)
	}
	exact, err := previewShapesFor(walls, slabs, DetailExact)
	if err != nil {
		t.Fatal(err)
	}
	var eb bytes.Buffer
	writePreview(&eb, exact, DetailExact)
	t.Logf("exact size %d bytes", eb.Len())
	out, d := mustPreview(t, walls, slabs)
	t.Logf("boxes size %d bytes", len(out))
	if d != DetailBoxes || !bytes.Contains(out, []byte(`data-detail="boxes"`)) {
		t.Errorf("detail %q, want boxes in the result and the SVG", d)
	}
	if bytes.Contains(out, []byte(`class="opening"`)) {
		t.Error("the boxes drawing has an opening")
	}
	if len(out) > maxPreviewBytes {
		t.Errorf("%d bytes, over the %d cap", len(out), maxPreviewBytes)
	}
}

func TestWritePreviewSVG_SpecModelStaysExact(t *testing.T) {
	walls, slabs := previewRoomsModel(t, 154)
	if n := len(walls) + len(slabs); n != 2002 {
		t.Fatalf("%d elements, want 2,002", n)
	}
	out, d := mustPreview(t, walls, slabs)
	t.Logf("exact size %d bytes", len(out))
	if d != DetailExact {
		t.Errorf("detail %q, want exact", d)
	}
}

func TestWritePreviewSVG_TooBigEvenAsBoxes(t *testing.T) {
	var walls []Wall
	for i := range 10_000 {
		x, y := float64(i%100)*4, float64(i/100)*4
		walls = append(walls, mustWall(t, [2][2]float64{{x, y}, {x + 3, y}}, 0, 2.5, 0.2, SideRight, nil))
	}
	var buf bytes.Buffer
	d, err := WritePreviewSVG(&buf, walls, nil)
	if err == nil || !strings.Contains(err.Error(), "as boxes") || !strings.Contains(err.Error(), "1,048,576") {
		t.Fatalf("err %v, want one naming the boxes size and the 1,048,576-byte cap", err)
	}
	if d != "" || buf.Len() != 0 {
		t.Errorf("detail %q and %d bytes written, want none", d, buf.Len())
	}
}

func TestWritePreviewSVG_OpeningsCountAsTriangles(t *testing.T) {
	w := Wall{Element: Element{Tris: make([]uint32, 3*(maxPreviewTriangles-1))}}
	w.Measured.Openings = []Opening{{}}
	var buf bytes.Buffer
	if _, err := WritePreviewSVG(&buf, []Wall{w}, nil); err == nil || !strings.Contains(err.Error(), "4,000,001") {
		t.Errorf("err %v, want the opening counted as 2 triangles over the limit", err)
	}
}

func TestElementShapes_ManyPlanesStayFast(t *testing.T) {
	// 200,000 small triangles, each on its own plane: grouping must not scan
	// every group for every triangle.
	const n = 200_000
	e := Element{Placement: model.Identity()}
	for i := range n {
		x := float32(i%1000) * 0.01
		y := float32(i/1000) * 0.01
		z := float32(i) * 1e-3
		k := uint32(len(e.Verts) / 3)
		e.Verts = append(e.Verts, x, y, z, x+0.005, y, z+float32(i%7)*1e-3, x, y+0.005, z+float32(i%11)*1e-3)
		e.Tris = append(e.Tris, k, k+1, k+2)
	}
	start := time.Now()
	if _, err := elementShapes(e, 0); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("elementShapes took %v on %d planes", d, n)
	}
}

func TestRefinePaintOrder_PairsAreBounded(t *testing.T) {
	// 8,000 squares stacked on one spot would need 32 million comparisons.
	shapes := make([]previewShape, 8000)
	for i := range shapes {
		z := float64(i) * 0.001
		shapes[i] = previewShape{class: "top", normal: v3{0, 0, 1}, offset: z, seq: i,
			loops: [][]v3{{{0, 0, z}, {1, 0, z}, {1, 1, z}, {0, 1, z}}}}
	}
	order, pairs := refinePaintOrder(shapes)
	if pairs > maxPreviewPairs {
		t.Errorf("%d pair comparisons, over the %d budget", pairs, maxPreviewPairs)
	}
	if len(order) != len(shapes) {
		t.Errorf("%d shapes ordered, want %d", len(order), len(shapes))
	}
}

func TestElementShapes_LongStraightOutlineStaysFast(t *testing.T) {
	// One flat strip of 300,000 quads: its outline has 600,000 collinear points.
	const n = 300_000
	e := Element{Placement: model.Identity()}
	for i := 0; i <= n; i++ {
		x := float32(i) * 0.001
		e.Verts = append(e.Verts, x, 0, 0, x, 1, 0)
	}
	for i := range uint32(n) {
		a, b, c, d := 2*i, 2*i+2, 2*i+3, 2*i+1
		e.Tris = append(e.Tris, a, b, c, a, c, d)
	}
	start := time.Now()
	got, err := elementShapes(e, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("elementShapes took %v", d)
	}
	if len(got) != 1 || len(got[0].loops) != 1 || len(got[0].loops[0]) != 4 {
		t.Fatalf("got %d shapes, want one loop of 4 points", len(got))
	}
}

func TestWallOpenings_ManyOpeningsStayFast(t *testing.T) {
	// 400,000 openings on a wall of 200,000 faces.
	e := Element{Placement: model.Identity()}
	for i := range 200_000 {
		x, y, z := float32(i%1000)*0.01, float32(i/1000)*0.01, float32(i)*1e-3
		k := uint32(len(e.Verts) / 3)
		e.Verts = append(e.Verts, x, y, z, x+0.005, y, z+float32(i%7)*1e-3, x, y+0.005, z+float32(i%11)*1e-3)
		e.Tris = append(e.Tris, k, k+1, k+2)
	}
	shapes, err := elementShapes(e, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) < 100_000 {
		t.Fatalf("%d shapes, want the wall to have over 100,000", len(shapes))
	}
	w := Wall{Element: e, Measured: Face{Origin: v3{0, 0, -5}, U: v3{1, 0, 0}, V: v3{0, 1, 0}, Normal: v3{0, 0, 1}}} // on no face of the mesh
	w.Measured.Openings = make([]Opening, 400_000)
	for i := range w.Measured.Openings {
		w.Measured.Openings[i] = Opening{Offset: float64(i), Width: 0.5, Height: 0.5}
	}
	start := time.Now()
	if got := wallOpenings(w, shapes, 0); len(got) != 400_000 {
		t.Fatalf("%d openings drawn, want 400,000", len(got))
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("wallOpenings took %v", d)
	}
}
