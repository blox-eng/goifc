package geometry

import (
	"fmt"
	"math"
)

// Detail is how exactly a preview draws its elements.
type Detail string

const (
	DetailExact Detail = "exact" // every flat face, with its openings
	DetailBoxes Detail = "boxes" // one prism per element, no openings
)

// The preview's fixed view: the viewer looks along previewDir, from the
// south-west and above. previewRight and previewUp span the screen.
var (
	previewDir   = v3{1 / math.Sqrt(3), 1 / math.Sqrt(3), -1 / math.Sqrt(3)}
	previewRight = v3{1 / math.Sqrt2, -1 / math.Sqrt2, 0}
	previewUp    = v3{1 / math.Sqrt(6), 1 / math.Sqrt(6), 2 / math.Sqrt(6)}
)

const (
	previewEdgeOn     = 1e-9 // |n·d| at or under this: the triangle is edge-on
	previewNormalTol  = 1e-6 // unit normals closer than this share a plane
	previewCollinear  = 1e-6 // sine of the angle under which a loop vertex is dropped
	previewZeroArea   = 1e-12
	previewWeldPerM   = 1 / gridSnap // weld grid: the mesh grid snap
	previewTopNormalZ = 0.5          // n.z at or over this: within 60° of up
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
		for _, h := range groups {
			if math.Abs(h.normal[0]-n[0]) < previewNormalTol && math.Abs(h.normal[1]-n[1]) < previewNormalTol &&
				math.Abs(h.normal[2]-n[2]) < previewNormalTol && math.Abs(h.offset-off) < gridSnap {
				g = h
				break
			}
		}
		if g == nil {
			g = &previewGroup{normal: n, offset: off}
			groups = append(groups, g)
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
// neighbours.
func dropCollinear(ids []int, pos []v3) []int {
	for changed := true; changed && len(ids) >= 3; {
		changed = false
		for i := 0; i < len(ids) && len(ids) >= 3; i++ {
			a, b, c := pos[ids[(i+len(ids)-1)%len(ids)]], pos[ids[i]], pos[ids[(i+1)%len(ids)]]
			u, w := subv(b, a), subv(c, b)
			lu, lw := math.Sqrt(dotv(u, u)), math.Sqrt(dotv(w, w))
			x := crossv(u, w)
			if lu == 0 || lw == 0 || math.Sqrt(dotv(x, x)) <= previewCollinear*lu*lw && dotv(u, w) > 0 {
				ids = append(ids[:i], ids[i+1:]...)
				changed = true
				i--
			}
		}
	}
	return ids
}
