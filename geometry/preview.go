package geometry

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strconv"
)

// Detail is how exactly a preview draws its elements.
type Detail string

const (
	DetailExact Detail = "exact" // every flat face, with its openings
	DetailBoxes Detail = "boxes" // one prism per element, no openings
)

// Bounds on a preview: the work is checked before it starts, the output
// before it is written.
const (
	maxPreviewBytes     = 1 << 20
	maxPreviewElements  = 100_000
	maxPreviewTriangles = 4_000_000
	// maxPreviewPairs bounds the shape pairs paintOrder compares by plane.
	// Past it, the remaining pairs keep centroid order.
	maxPreviewPairs = 20_000_000
)

// The preview's fixed view: the viewer looks along previewDir, from the
// south-west and above. previewRight and previewUp span the screen.
var (
	previewDir   = v3{1 / math.Sqrt(3), 1 / math.Sqrt(3), -1 / math.Sqrt(3)}
	previewRight = v3{1 / math.Sqrt2, -1 / math.Sqrt2, 0}
	previewUp    = v3{1 / math.Sqrt(6), 1 / math.Sqrt(6), 2 / math.Sqrt(6)}
)

const (
	previewEdgeOn     = 1e-6 // |n·d| at or under this: the triangle is edge-on (float32 noise)
	previewNormalTol  = 1e-6 // unit normals closer than this share a plane
	previewCollinear  = 1e-6 // sine of the angle under which a loop vertex is dropped
	previewZeroArea   = 1e-12
	previewWeldPerM   = 1 / gridSnap // weld grid: the mesh grid snap
	previewTopNormalZ = 0.5          // n.z at or over this: within 60° of up
	previewHullTol    = 1e-6         // m²: a smaller turn on the plan hull is straight
)

// previewShape is one flat face of one element as it is drawn: one or more
// outline loops in world metres, on one plane.
type previewShape struct {
	class     string
	loops     [][]v3
	normal    v3
	offset    float64 // normal·p for a point p on the plane
	depth     float64 // the centroid·previewDir: larger is farther
	elem, seq int     // the element's index, and the shape's order in it
}

// previewPoint is where a world point draws: x right, y down.
func previewPoint(p v3) [2]float64 {
	return [2]float64{dotv(p, previewRight), -dotv(p, previewUp)}
}

// previewClass names a face by which way it faces.
func previewClass(n v3) string {
	switch {
	case n[2] >= previewTopNormalZ:
		return "top"
	case dotv(n, previewRight) < 0:
		return "left"
	}
	return "right"
}

// previewWorld takes an element's vertices to world in float64. WorldVerts
// rounds to float32, which loses millimetres at a large placement.
func previewWorld(e Element) ([]v3, error) {
	for _, m := range e.Placement {
		if !finite(m) {
			return nil, fmt.Errorf("the placement is not finite")
		}
	}
	out := make([]v3, len(e.Verts)/3)
	for i := range out {
		p := v3{float64(e.Verts[3*i]), float64(e.Verts[3*i+1]), float64(e.Verts[3*i+2])}
		if !finite(p[0]) || !finite(p[1]) || !finite(p[2]) {
			return nil, fmt.Errorf("vertex %d is not finite", i)
		}
		out[i] = applyMat(e.Placement, p)
	}
	return out, nil
}

// previewGroup gathers the kept triangles of one plane.
type previewGroup struct {
	normal v3
	offset float64
	tris   [][3]int // welded vertex ids, in the triangle's winding
}

// elementShapes turns one element's mesh into the shapes the viewer sees:
// triangles that face away are dropped, and the rest merge per plane.
func elementShapes(e Element, elem int) ([]previewShape, error) {
	world, err := previewWorld(e)
	if err != nil {
		return nil, err
	}
	weld := map[[3]int64]int{}
	var pos []v3
	id := func(p v3) int {
		k := [3]int64{int64(math.Round(p[0] * previewWeldPerM)), int64(math.Round(p[1] * previewWeldPerM)), int64(math.Round(p[2] * previewWeldPerM))}
		if i, ok := weld[k]; ok {
			return i
		}
		weld[k] = len(pos)
		pos = append(pos, p)
		return len(pos) - 1
	}
	var groups []*previewGroup
	// Groups are found through a coarse plane key, then checked exactly, so
	// a mesh of many planes is not scanned group by group.
	bucket := map[planeKey][]*previewGroup{}
	for t := 0; t+2 < len(e.Tris); t += 3 {
		var c [3]v3
		for k := range 3 {
			ix := e.Tris[t+k]
			if int(ix) >= len(world) {
				return nil, fmt.Errorf("triangle %d: index %d is out of range", t/3, ix)
			}
			c[k] = world[ix]
		}
		n := crossv(subv(c[1], c[0]), subv(c[2], c[0]))
		if math.Sqrt(dotv(n, n)) < previewZeroArea {
			continue
		}
		n = normv(n)
		if dotv(n, previewDir) >= -previewEdgeOn {
			continue
		}
		off := dotv(n, c[0])
		var g *previewGroup
		k := keyOfPlane(n, off)
		for _, h := range bucket[k] {
			if samePlane(n, off, previewShape{normal: h.normal, offset: h.offset}) {
				g = h
				break
			}
		}
		if g == nil {
			g = &previewGroup{normal: n, offset: off}
			groups = append(groups, g)
			bucket[k] = append(bucket[k], g)
		}
		tri := [3]int{id(c[0]), id(c[1]), id(c[2])}
		if tri[0] == tri[1] || tri[1] == tri[2] || tri[0] == tri[2] {
			continue
		}
		g.tris = append(g.tris, tri)
	}
	var shapes []previewShape
	for _, g := range groups {
		loops := previewLoops(g.tris, pos)
		if len(loops) == 0 {
			continue
		}
		var sum v3
		var n int
		for _, l := range loops {
			for _, p := range l {
				sum = addv(sum, p)
				n++
			}
		}
		shapes = append(shapes, previewShape{
			class: previewClass(g.normal), loops: loops, normal: g.normal, offset: g.offset,
			depth: dotv(scalev(sum, 1/float64(n)), previewDir), elem: elem, seq: len(shapes),
		})
	}
	return shapes, nil
}

// previewLoops chains a group's boundary edges (edges that one triangle of
// the group uses) into loops, drops collinear vertices, and starts each loop
// at its lowest vertex id. Loops come in order of that id.
func previewLoops(tris [][3]int, pos []v3) [][]v3 {
	count := map[[2]int]int{}
	for _, t := range tris {
		for k := range 3 {
			a, b := t[k], t[(k+1)%3]
			count[[2]int{min(a, b), max(a, b)}]++
		}
	}
	next := map[int][]int{} // boundary edges, directed as in their triangle
	var starts []int
	for _, t := range tris {
		for k := range 3 {
			a, b := t[k], t[(k+1)%3]
			if count[[2]int{min(a, b), max(a, b)}] == 1 {
				if len(next[a]) == 0 {
					starts = append(starts, a)
				}
				next[a] = append(next[a], b)
			}
		}
	}
	type loop struct {
		ids []int
		low int
	}
	var loops []loop
	for _, s := range starts {
		for len(next[s]) > 0 {
			ids := []int{s}
			cur := s
			for {
				nx := next[cur][0]
				next[cur] = next[cur][1:]
				if nx == s {
					break
				}
				ids = append(ids, nx)
				cur = nx
				if len(next[cur]) == 0 {
					ids = nil // an open chain: the group is not closed
					break
				}
			}
			ids = dropCollinear(ids, pos)
			if len(ids) < 3 {
				continue
			}
			low := 0
			for i, v := range ids {
				if v < ids[low] {
					low = i
				}
			}
			ids = append(ids[low:], ids[:low]...)
			loops = append(loops, loop{ids, ids[0]})
		}
	}
	for i := 1; i < len(loops); i++ {
		for j := i; j > 0 && loops[j].low < loops[j-1].low; j-- {
			loops[j], loops[j-1] = loops[j-1], loops[j]
		}
	}
	out := make([][]v3, len(loops))
	for i, l := range loops {
		out[i] = make([]v3, len(l.ids))
		for k, v := range l.ids {
			out[i][k] = pos[v]
		}
	}
	return out
}

// dropCollinear removes each loop vertex that lies on the line through its
// neighbours, in one pass.
func dropCollinear(ids []int, pos []v3) []int {
	straight := func(a, b, c int) bool {
		u, w := subv(pos[b], pos[a]), subv(pos[c], pos[b])
		lu, lw := math.Sqrt(dotv(u, u)), math.Sqrt(dotv(w, w))
		x := crossv(u, w)
		return lu == 0 || lw == 0 || math.Sqrt(dotv(x, x)) <= previewCollinear*lu*lw && dotv(u, w) > 0
	}
	out := make([]int, 0, len(ids))
	for _, v := range ids {
		out = append(out, v)
		for len(out) >= 3 && straight(out[len(out)-3], out[len(out)-2], out[len(out)-1]) {
			out[len(out)-2] = out[len(out)-1]
			out = out[:len(out)-1]
		}
	}
	for changed := true; changed && len(out) >= 3; {
		changed = false
		if straight(out[len(out)-2], out[len(out)-1], out[0]) {
			out, changed = out[:len(out)-1], true
		}
		if len(out) >= 3 && straight(out[len(out)-1], out[0], out[1]) {
			out, changed = out[1:], true
		}
	}
	return out
}

// planeKey is a coarse key for a plane: planes that pass samePlane almost
// always share it. A pair split across two keys draws as two shapes.
type planeKey [4]int64

func keyOfPlane(n v3, off float64) planeKey {
	return planeKey{int64(math.Round(n[0] * 1e5)), int64(math.Round(n[1] * 1e5)), int64(math.Round(n[2] * 1e5)), int64(math.Round(off * 1e3))}
}

// samePlane is the merge test: unit normals and plane offsets match.
func samePlane(n v3, off float64, s previewShape) bool {
	return math.Abs(s.normal[0]-n[0]) < previewNormalTol && math.Abs(s.normal[1]-n[1]) < previewNormalTol &&
		math.Abs(s.normal[2]-n[2]) < previewNormalTol && math.Abs(s.offset-off) < gridSnap
}

// wallOpenings draws each of a wall's openings on the face of that wall the
// viewer sees: the measured face when it faces the viewer, else, for a
// through-opening, the opposite face. A niche on a hidden face is not drawn.
// Each opening's seq is the seq of the shape it lies on, or -1.
func wallOpenings(w Wall, shapes []previewShape, elem int) []previewShape {
	f := w.Measured
	n := f.Normal
	shift := 0.0 // how far the rectangle moves along -n
	normal := n
	if dotv(n, previewDir) >= -previewEdgeOn {
		back := scalev(n, -1)
		best := math.Inf(1)
		for _, s := range shapes {
			if math.Abs(s.normal[0]-back[0]) < previewNormalTol && math.Abs(s.normal[1]-back[1]) < previewNormalTol &&
				math.Abs(s.normal[2]-back[2]) < previewNormalTol {
				if d := dotv(n, f.Origin) + s.offset; d > 0 && d < best {
					best = d
				}
			}
		}
		if math.IsInf(best, 1) {
			return nil
		}
		shift, normal = best, back
	}
	onPlane := map[planeKey][]previewShape{}
	for _, s := range shapes {
		if s.elem == elem {
			k := keyOfPlane(s.normal, s.offset)
			onPlane[k] = append(onPlane[k], s)
		}
	}
	var out []previewShape
	for _, o := range f.Openings {
		if shift > 0 && o.Depth != 0 {
			continue
		}
		var loop []v3
		for _, c := range [4][2]float64{{o.Offset, o.Sill}, {o.Offset + o.Width, o.Sill}, {o.Offset + o.Width, o.Sill + o.Height}, {o.Offset, o.Sill + o.Height}} {
			p := addv(addv(f.Origin, scalev(f.U, c[0])), scalev(f.V, c[1]))
			loop = append(loop, subv(p, scalev(n, shift)))
		}
		off := dotv(normal, loop[0])
		seq := -1
		for _, s := range onPlane[keyOfPlane(normal, off)] {
			if samePlane(normal, off, s) {
				seq = s.seq
				break
			}
		}
		c := scalev(addv(addv(loop[0], loop[1]), addv(loop[2], loop[3])), 0.25)
		out = append(out, previewShape{class: "opening", loops: [][]v3{loop}, normal: normal, offset: off,
			depth: dotv(c, previewDir), elem: elem, seq: seq})
	}
	return out
}

// WritePreviewSVG draws walls and slabs, with the walls' openings, as one
// SVG document in a fixed isometric view from the south-west. Pass the
// builders' results as they are: a layered wall is one Wall per layer.
//
// Every shape has one class, top, left, right or opening, and no colour:
// style them under .goifc-preview. The root's data-detail names the Detail
// returned. On an error w gets nothing.
func WritePreviewSVG(w io.Writer, walls []Wall, slabs []Slab) (Detail, error) {
	var buf bytes.Buffer
	for _, d := range []Detail{DetailExact, DetailBoxes} {
		shapes, err := previewShapesFor(walls, slabs, d)
		if err != nil {
			return "", err
		}
		buf.Reset()
		writePreview(&buf, shapes, d)
		if buf.Len() <= maxPreviewBytes {
			if _, err := w.Write(buf.Bytes()); err != nil {
				return "", err
			}
			return d, nil
		}
	}
	return "", fmt.Errorf("preview is %s bytes as boxes, over the %s-byte cap", thousands(buf.Len()), thousands(maxPreviewBytes))
}

// previewShapesFor checks the input and returns every drawn shape in paint
// order, at the given detail. Elements are indexed walls first, then slabs.
func previewShapesFor(walls []Wall, slabs []Slab, detail Detail) ([]previewShape, error) {
	if err := checkPreview(walls, slabs); err != nil {
		return nil, err
	}
	shapesOf := elementShapes
	if detail == DetailBoxes {
		shapesOf = boxShapes
	}
	var shapes []previewShape
	for i, w := range walls {
		s, err := shapesOf(w.Element, i)
		if err != nil {
			return nil, fmt.Errorf("wall %d: %w", i, err)
		}
		shapes = append(shapes, s...)
		if detail == DetailExact {
			shapes = append(shapes, wallOpenings(w, s, i)...)
		}
	}
	for i, sl := range slabs {
		s, err := shapesOf(sl.Element, len(walls)+i)
		if err != nil {
			return nil, fmt.Errorf("slab %d: %w", i, err)
		}
		shapes = append(shapes, s...)
	}
	if len(shapes) == 0 {
		return nil, fmt.Errorf("nothing to draw")
	}
	paintOrder(shapes)
	return shapes, nil
}

// checkPreview refuses input that is too big or not finite, before any work.
func checkPreview(walls []Wall, slabs []Slab) error {
	n := len(walls) + len(slabs)
	if n == 0 {
		return fmt.Errorf("nothing to draw")
	}
	if n > maxPreviewElements {
		return fmt.Errorf("%s elements, over the %s limit", thousands(n), thousands(maxPreviewElements))
	}
	tris := 0
	for _, w := range walls {
		tris += len(w.Element.Tris)/3 + 2*len(w.Measured.Openings) // an opening draws as 2 triangles' worth
	}
	for _, s := range slabs {
		tris += len(s.Element.Tris) / 3
	}
	if tris > maxPreviewTriangles {
		return fmt.Errorf("%s triangles, over the %s limit", thousands(tris), thousands(maxPreviewTriangles))
	}
	for i, w := range walls {
		f := w.Measured
		for _, v := range [][3]float64{f.Origin, f.U, f.V, f.Normal} {
			if !finite(v[0]) || !finite(v[1]) || !finite(v[2]) {
				return fmt.Errorf("wall %d: the measured face is not finite", i)
			}
		}
		for k, o := range f.Openings {
			for _, x := range []float64{o.Offset, o.Sill, o.Width, o.Height, o.Depth} {
				if !finite(x) {
					return fmt.Errorf("wall %d: opening %d is not finite", i, k)
				}
			}
		}
	}
	return nil
}

// paintOrder sorts shapes from the farthest to the nearest. It starts from
// centroid depth (ties by element, then order in the element; an opening
// right after the shape it lies on). Then, for each pair whose screen boxes
// overlap, a shape that lies wholly behind the other's plane, or has the
// other wholly in front of its own plane, is drawn first. A cycle of such
// rules breaks by centroid order.
func paintOrder(shapes []previewShape) {
	type key struct {
		depth     float64
		elem, seq int
		opening   int
	}
	depthOf := map[[2]int]float64{}
	for _, s := range shapes {
		if s.class != "opening" {
			depthOf[[2]int{s.elem, s.seq}] = s.depth
		}
	}
	keyOf := func(s previewShape) key {
		if s.class != "opening" {
			return key{s.depth, s.elem, s.seq, 0}
		}
		if d, ok := depthOf[[2]int{s.elem, s.seq}]; ok && s.seq >= 0 {
			return key{d, s.elem, s.seq, 1}
		}
		return key{s.depth, s.elem, math.MaxInt, 1}
	}
	sort.SliceStable(shapes, func(i, j int) bool {
		a, b := keyOf(shapes[i]), keyOf(shapes[j])
		switch {
		case a.depth != b.depth:
			return a.depth > b.depth
		case a.elem != b.elem:
			return a.elem < b.elem
		case a.seq != b.seq:
			return a.seq < b.seq
		}
		return a.opening < b.opening
	})
	order, _ := refinePaintOrder(shapes)
	sorted := make([]previewShape, len(shapes))
	for i, k := range order {
		sorted[i] = shapes[k]
	}
	copy(shapes, sorted)
}

// previewPlaneTol is how far a vertex may stray off the near side of a plane
// and still count as behind it: float32 mesh vertices are local to the
// element, so a few micrometres of noise.
const previewPlaneTol = gridSnap / 10

// behind reports whether every vertex of a is behind (or on) b's plane, as
// the viewer sees it. b faces the viewer, so its near side is where
// normal·p > offset.
func behind(a, b previewShape) bool {
	for _, l := range a.loops {
		for _, p := range l {
			if dotv(b.normal, p) > b.offset+previewPlaneTol {
				return false
			}
		}
	}
	return true
}

// drawnBefore reports whether a must be drawn before b, by their planes.
func drawnBefore(a, b previewShape) bool {
	return behind(a, b) || inFront(b, a)
}

func inFront(a, b previewShape) bool {
	for _, l := range a.loops {
		for _, p := range l {
			if dotv(b.normal, p) < b.offset-previewPlaneTol {
				return false
			}
		}
	}
	return true
}

// refinePaintOrder returns the indices of shapes (in centroid order) in an
// order that honours every decided pair, by a topological sort that always
// takes the earliest ready shape in centroid order. It also returns how many
// pairs it compared.
func refinePaintOrder(shapes []previewShape) ([]int, int) {
	n := len(shapes)
	type box struct{ lo, hi [2]float64 }
	boxes := make([]box, n)
	for i, s := range shapes {
		b := box{[2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}}
		for _, l := range s.loops {
			for _, p := range l {
				q := previewPoint(p)
				for k := range 2 {
					b.lo[k], b.hi[k] = math.Min(b.lo[k], q[k]), math.Max(b.hi[k], q[k])
				}
			}
		}
		boxes[i] = b
	}
	byX := make([]int, n)
	for i := range byX {
		byX[i] = i
	}
	sort.SliceStable(byX, func(i, j int) bool { return boxes[byX[i]].lo[0] < boxes[byX[j]].lo[0] })
	after := make([][]int, n) // after[a]: shapes drawn after a
	need := make([]int, n)    // how many shapes must come first
	edge := func(a, b int) {
		after[a] = append(after[a], b)
		need[b]++
	}
	pairs := 0
pairing:
	for x, i := range byX {
		for _, j := range byX[x+1:] {
			if boxes[j].lo[0] >= boxes[i].hi[0] {
				break
			}
			if pairs == maxPreviewPairs {
				break pairing
			}
			pairs++
			if boxes[j].lo[1] >= boxes[i].hi[1] || boxes[i].lo[1] >= boxes[j].hi[1] {
				continue
			}
			a, b := min(i, j), max(i, j)
			sa, sb := shapes[a], shapes[b]
			if sb.class == "opening" && sb.elem == sa.elem && sb.seq == sa.seq && sa.class != "opening" {
				edge(a, b) // an opening stays on its own face
				continue
			}
			ab, ba := drawnBefore(sa, sb), drawnBefore(sb, sa)
			switch {
			case ab && !ba:
				edge(a, b)
			case ba && !ab:
				edge(b, a)
			}
		}
	}
	// Ready shapes in a min-heap of their centroid-order index.
	ready := &intHeap{}
	for i := range n {
		if need[i] == 0 {
			ready.push(i)
		}
	}
	done := make([]bool, n)
	out := make([]int, 0, n)
	next := 0 // the earliest shape not yet placed, to break a cycle
	for len(out) < n {
		var i int
		if ready.len() > 0 {
			i = ready.pop()
		} else {
			for done[next] {
				next++
			}
			i = next
		}
		if done[i] {
			continue
		}
		done[i] = true
		out = append(out, i)
		for _, j := range after[i] {
			need[j]--
			if need[j] == 0 && !done[j] {
				ready.push(j)
			}
		}
	}
	return out, pairs
}

// intHeap is a min-heap of ints.
type intHeap struct{ a []int }

func (h *intHeap) len() int { return len(h.a) }

func (h *intHeap) push(x int) {
	h.a = append(h.a, x)
	for i := len(h.a) - 1; i > 0; {
		p := (i - 1) / 2
		if h.a[p] <= h.a[i] {
			break
		}
		h.a[p], h.a[i] = h.a[i], h.a[p]
		i = p
	}
}

func (h *intHeap) pop() int {
	top := h.a[0]
	last := len(h.a) - 1
	h.a[0] = h.a[last]
	h.a = h.a[:last]
	for i := 0; ; {
		l, r, m := 2*i+1, 2*i+2, i
		if l < last && h.a[l] < h.a[m] {
			m = l
		}
		if r < last && h.a[r] < h.a[m] {
			m = r
		}
		if m == i {
			break
		}
		h.a[m], h.a[i] = h.a[i], h.a[m]
		i = m
	}
	return top
}

// writePreview writes the document for shapes, already in paint order.
func writePreview(buf *bytes.Buffer, shapes []previewShape, detail Detail) {
	lo, hi := [2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}
	for _, s := range shapes {
		for _, l := range s.loops {
			for _, p := range l {
				q := previewPoint(p)
				for k := range 2 {
					lo[k], hi[k] = math.Min(lo[k], q[k]), math.Max(hi[k], q[k])
				}
			}
		}
	}
	m := 0.02 * math.Max(hi[0]-lo[0], hi[1]-lo[1])
	fmt.Fprintf(buf, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s" data-detail="%s">`+"\n",
		mm(lo[0]-m), mm(lo[1]-m), mm(hi[0]-lo[0]+2*m), mm(hi[1]-lo[1]+2*m), detail)
	buf.WriteString(`<g class="goifc-preview">` + "\n")
	for _, s := range shapes {
		if len(s.loops) == 1 {
			buf.WriteString(`<polygon class="` + s.class + `" points="`)
			for i, p := range s.loops[0] {
				if i > 0 {
					buf.WriteByte(' ')
				}
				writePoint(buf, p)
			}
		} else {
			buf.WriteString(`<path class="` + s.class + `" fill-rule="evenodd" d="`)
			for i, l := range s.loops {
				if i > 0 {
					buf.WriteByte(' ')
				}
				for k, p := range l {
					if k == 0 {
						buf.WriteByte('M')
					} else {
						buf.WriteByte('L')
					}
					writePoint(buf, p)
				}
				buf.WriteByte('Z')
			}
		}
		buf.WriteString("\"/>\n")
	}
	buf.WriteString("</g>\n</svg>\n")
}

func writePoint(buf *bytes.Buffer, p v3) {
	q := previewPoint(p)
	buf.WriteString(mm(q[0]))
	buf.WriteByte(' ')
	buf.WriteString(mm(q[1]))
}

// mm writes a length in metres rounded to 1 mm, with no trailing zeros.
func mm(v float64) string {
	r := math.Round(v*1000) / 1000
	if r == 0 {
		return "0"
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// thousands writes n with comma separators: 1,048,576.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// boxShapes draws an element as a prism: the convex hull of its vertices in
// plan, from its lowest to its highest point. A wall's prism is its exact
// footprint; a slab's fills in any concave corner.
func boxShapes(e Element, elem int) ([]previewShape, error) {
	world, err := previewWorld(e)
	if err != nil || len(world) == 0 {
		return nil, err
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	plan := make([][2]float64, len(world))
	for i, p := range world {
		plan[i] = [2]float64{p[0], p[1]}
		lo, hi = math.Min(lo, p[2]), math.Max(hi, p[2])
	}
	h := planHull(plan)
	if len(h) < 3 || hi-lo < previewZeroArea {
		return nil, nil
	}
	shape := func(class string, n v3, loop []v3) previewShape {
		var c v3
		for _, p := range loop {
			c = addv(c, p)
		}
		c = scalev(c, 1/float64(len(loop)))
		return previewShape{class: class, loops: [][]v3{loop}, normal: n, offset: dotv(n, loop[0]),
			depth: dotv(c, previewDir), elem: elem}
	}
	top := make([]v3, len(h))
	for i, p := range h {
		top[i] = v3{p[0], p[1], hi}
	}
	shapes := []previewShape{shape("top", v3{0, 0, 1}, top)}
	for i, a := range h {
		b := h[(i+1)%len(h)]
		n := normv(v3{b[1] - a[1], a[0] - b[0], 0}) // outward: the hull runs counter-clockwise
		if dotv(n, previewDir) >= -previewEdgeOn {
			continue
		}
		shapes = append(shapes, shape(previewClass(n), n, []v3{{a[0], a[1], lo}, {b[0], b[1], lo}, {b[0], b[1], hi}, {a[0], a[1], hi}}))
	}
	for i := range shapes {
		shapes[i].seq = i
	}
	return shapes, nil
}

// planHull is the convex hull of pts, counter-clockwise, by the monotone
// chain. Points on a hull edge are left out.
func planHull(pts [][2]float64) [][2]float64 {
	ps := slices.Clone(pts)
	slices.SortFunc(ps, func(a, b [2]float64) int {
		if c := cmp.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return cmp.Compare(a[1], b[1])
	})
	ps = slices.Compact(ps)
	if len(ps) < 3 {
		return ps
	}
	// A turn under previewHullTol (m²) is straight: float32 vertices leave a
	// point on a hull edge a few micrometres off the line.
	turn := func(o, a, b [2]float64) float64 { return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0]) }
	var h [][2]float64
	for _, p := range ps {
		for len(h) >= 2 && turn(h[len(h)-2], h[len(h)-1], p) <= previewHullTol {
			h = h[:len(h)-1]
		}
		h = append(h, p)
	}
	for i, k := len(ps)-2, len(h)+1; i >= 0; i-- {
		for len(h) >= k && turn(h[len(h)-2], h[len(h)-1], ps[i]) <= previewHullTol {
			h = h[:len(h)-1]
		}
		h = append(h, ps[i])
	}
	return h[:len(h)-1]
}
