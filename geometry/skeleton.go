package geometry

import (
	"container/heap"
	"math"
)

// skelFace is the region swept by ring edge Edge: a counter-clockwise plan
// polygon that starts with the edge itself, each point at z = time × slope.
type skelFace struct {
	Edge int
	Poly [][3]float64
}

// skelArc is an inner arc between the faces of ring edges Left and Right,
// Left's face on its left going A to B.
type skelArc struct {
	A, B        [3]float64
	Left, Right int
}

// Skeleton tolerances, as fractions of the ring's size.
const (
	skelTimeTol  = 1e-9 // events this close in time are one event
	skelPointTol = 1e-9 // points this close are one point
	skelLineTol  = 1e-7 // a vertex this close to an edge it is due to reach is on it
	skelAreaTol  = 1e-9 // the faces must tile the ring this closely, gables aside
)

// skelFoldTol is the sine under which two edges whose fronts meet head on
// fold into a ridge. Short of a fold, the vertex between them runs at
// 1/sin(θ/2) and float64 loses its velocity as ε/θ²; the fold is off by θ
// along the ridge. The two errors balance near θ = ε^⅓ ≈ 6e-6, rounded up to
// 1e-5: a few µm across a wall of a metre or more.
const skelFoldTol = 1e-5

type skelNode struct {
	p [2]float64
	t float64
}

// skelVert is a wavefront vertex: at time t0 it is at p0 on node, between
// edge in (arriving) and edge out (leaving), and it moves at vel.
type skelVert struct {
	p0, vel    [2]float64
	t0         float64
	in, out    int
	prev, next int
	node       int
	dead       bool
}

// skelNodeArc is an arc between two nodes.
type skelNodeArc struct{ a, b, left, right int }

type skelEventKind uint8

const (
	skelEdgeEvent  skelEventKind = iota // segment v→w shrinks to a point
	skelSplitEvent                      // vertex v reaches edge w's line
)

type skelEvent struct {
	t    float64
	kind skelEventKind
	v, w int
}

type skelQueue []skelEvent

func (q skelQueue) Len() int { return len(q) }
func (q skelQueue) Less(i, j int) bool {
	if q[i].t != q[j].t {
		return q[i].t < q[j].t
	}
	return q[i].kind < q[j].kind
}
func (q skelQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *skelQueue) Push(x any)   { *q = append(*q, x.(skelEvent)) }
func (q *skelQueue) Pop() any {
	old := *q
	e := old[len(old)-1]
	*q = old[:len(old)-1]
	return e
}

type skeleton struct {
	ring        [][2]float64
	dir, normal [][2]float64 // each edge's unit direction and inward normal
	speed       []float64
	verts       []skelVert
	nodes       []skelNode
	arcs        []skelNodeArc
	queue       skelQueue
	b           *budget
	now         float64
	tolT, tolP  float64
	size        float64
	eventNodes  int   // the first node made at now
	fresh       []int // the vertices made at now
}

// errSkelTooComplex is a fresh error each time: a caller maps its edges.
func errSkelTooComplex() error {
	return &RoofError{Reason: ReasonRoofTooComplex, Edge: -1, Other: -1}
}

// weightedSkeleton builds the weighted straight skeleton of a
// counter-clockwise ring: edge i moves inward at speed[i], 1 for a sloped
// edge and 0 for a gable, and slope = tan(Pitch) turns time into height.
// It returns a face for each sloped edge and the inner arcs, all in ring edge
// indices. A gable that would grow along its line, leaving a vertical step
// inside the roof, is refused roof_step; work past b is roof_too_complex.
//
// It is Felkel–Obdržálek with weights: vertices move with their two edges,
// edge events (a segment shrinks to a point) and split events (a vertex
// reaches an edge) are taken in time order, and events at one time are taken
// together, so a rectangle's ridge and a square's apex come out whole.
func weightedSkeleton(ring [][2]float64, speed []float64, slope float64, b *budget) ([]skelFace, []skelArc, error) {
	s := newSkeleton(ring, speed, b)
	if err := s.run(); err != nil {
		return nil, nil, err
	}
	return s.result(slope)
}

func newSkeleton(ring [][2]float64, speed []float64, b *budget) *skeleton {
	n := len(ring)
	s := &skeleton{ring: ring, speed: speed, b: b, dir: make([][2]float64, n), normal: make([][2]float64, n)}
	lo, hi := ring[0], ring[0]
	for i, p := range ring {
		q := ring[(i+1)%n]
		l := math.Hypot(q[0]-p[0], q[1]-p[1])
		s.dir[i] = [2]float64{(q[0] - p[0]) / l, (q[1] - p[1]) / l}
		s.normal[i] = [2]float64{-s.dir[i][1], s.dir[i][0]}
		lo = [2]float64{min(lo[0], p[0]), min(lo[1], p[1])}
		hi = [2]float64{max(hi[0], p[0]), max(hi[1], p[1])}
	}
	s.size = math.Hypot(hi[0]-lo[0], hi[1]-lo[1])
	s.tolT, s.tolP = skelTimeTol*s.size, skelPointTol*s.size
	for k, p := range ring {
		s.nodes = append(s.nodes, skelNode{p, 0})
		s.verts = append(s.verts, skelVert{p0: p, in: (k + n - 1) % n, out: k, prev: (k + n - 1) % n, next: (k + 1) % n, node: k})
		s.fresh = append(s.fresh, k)
	}
	return s
}

// run takes every event until the wavefront is gone.
func (s *skeleton) run() error {
	if err := s.launch(); err != nil {
		return err
	}
	n := len(s.ring)
	for groups := 0; s.queue.Len() > 0; groups++ {
		if groups > 4*n*n+64 {
			return errSkelTooComplex()
		}
		if err := s.step(); err != nil {
			return err
		}
	}
	for i := range s.verts {
		if !s.verts[i].dead {
			return errSkelTooComplex()
		}
	}
	return nil
}

func (s *skeleton) charge(n int) error {
	if !s.b.spend(int64(n)) {
		return errSkelTooComplex()
	}
	return nil
}

func (s *skeleton) pos(v int) [2]float64 { return s.posAt(v, s.now) }

func (s *skeleton) posAt(v int, t float64) [2]float64 {
	x := &s.verts[v]
	dt := t - x.t0
	return [2]float64{x.p0[0] + x.vel[0]*dt, x.p0[1] + x.vel[1]*dt}
}

func (s *skeleton) push(e skelEvent) error {
	heap.Push(&s.queue, e)
	return s.charge(1)
}

// step takes every event within the time tolerance of the next one.
func (s *skeleton) step() error {
	first := heap.Pop(&s.queue).(skelEvent)
	s.now = max(s.now, first.t)
	s.eventNodes = len(s.nodes)
	s.fresh = s.fresh[:0]
	group := []skelEvent{first}
	for s.queue.Len() > 0 && s.queue[0].t <= first.t+s.tolT {
		group = append(group, heap.Pop(&s.queue).(skelEvent))
	}
	if err := s.charge(len(group)); err != nil {
		return err
	}
	var work []int
	for _, e := range group {
		switch {
		case e.kind == skelEdgeEvent && !s.verts[e.v].dead && !s.verts[e.w].dead && s.verts[e.v].next == e.w:
			work = append(work, s.merge(e.v, e.w, e.t))
		case e.kind == skelSplitEvent && !s.verts[e.v].dead:
			work = append(work, e.v)
		}
	}
	if err := s.settle(work); err != nil {
		return err
	}
	return s.launch()
}

// settle repairs the wavefront at now: it merges vertices that meet, closes
// loops that have shrunk to a line, folds away fronts that meet head on, and
// splits a loop where a vertex reaches one of its edges.
func (s *skeleton) settle(work []int) error {
	for len(work) > 0 {
		v := work[len(work)-1]
		work = work[:len(work)-1]
		x := s.verts[v]
		if x.dead {
			continue
		}
		pv := s.pos(v)
		switch {
		case x.next == v:
			s.end(v, s.nodeAt(pv))
		case dist2(pv, s.pos(x.next)) <= s.tolP:
			work = append(work, s.merge(v, x.next, s.now))
		case dist2(pv, s.pos(x.prev)) <= s.tolP:
			work = append(work, s.merge(x.prev, v, s.now))
		case x.prev == x.next:
			q := x.next
			nv, nq := s.nodeAt(pv), s.nodeAt(s.pos(q))
			s.end(v, nv)
			s.end(q, nq)
			s.arcs = append(s.arcs, skelNodeArc{nv, nq, x.in, x.out})
		case s.antiparallel(x.in, x.out):
			work = append(work, s.fold(v))
		default:
			split, err := s.touch(v)
			if err != nil {
				return err
			}
			work = append(work, split...)
		}
	}
	return nil
}

// antiparallel reports whether edges a and b run opposite ways along one
// line, as two fronts that meet head on do. Two sloped fronts fold within
// skelFoldTol; a sloped front and a gable only when exactly on one line, as
// a mono roof's high gable is. Short of that, velocity slides the vertex
// along the gable and refuses the step where it would lengthen it.
func (s *skeleton) antiparallel(a, b int) bool {
	da, db := s.dir[a], s.dir[b]
	tol := skelFoldTol
	if s.speed[a] != s.speed[b] {
		tol = roofCollinear
	}
	return da[0]*db[0]+da[1]*db[1] < 0 && math.Abs(da[0]*db[1]-da[1]*db[0]) <= tol
}

// nodeAt returns the node at p made at now, making it if there is none.
func (s *skeleton) nodeAt(p [2]float64) int {
	for i := s.eventNodes; i < len(s.nodes); i++ {
		if dist2(s.nodes[i].p, p) <= s.tolP {
			return i
		}
	}
	s.nodes = append(s.nodes, skelNode{p, s.now})
	return len(s.nodes) - 1
}

// end stops vertex v at node, recording the arc it swept.
func (s *skeleton) end(v, node int) {
	x := &s.verts[v]
	if node != x.node {
		s.arcs = append(s.arcs, skelNodeArc{x.node, node, x.in, x.out})
	}
	x.dead = true
}

// spawn starts a vertex at node between edges in and out, linked between
// prev and next.
func (s *skeleton) spawn(node, in, out, prev, next int) int {
	v := len(s.verts)
	s.verts = append(s.verts, skelVert{p0: s.nodes[node].p, t0: s.now, in: in, out: out, prev: prev, next: next, node: node})
	if prev < 0 {
		s.verts[v].prev, s.verts[v].next = v, v
	} else {
		s.verts[prev].next, s.verts[next].prev = v, v
	}
	s.fresh = append(s.fresh, v)
	return v
}

// merge replaces neighbours u→w that meet at time t by one vertex. A vertex
// between near-parallel edges runs fast enough to cross metres within the
// time tolerance, so they meet where they are at t, not at now.
func (s *skeleton) merge(u, w int, t float64) int {
	pu, pw := s.posAt(u, t), s.posAt(w, t)
	node := s.nodeAt([2]float64{(pu[0] + pw[0]) / 2, (pu[1] + pw[1]) / 2})
	xu, xw := s.verts[u], s.verts[w]
	s.end(u, node)
	s.end(w, node)
	if xu.prev == w {
		return s.spawn(node, xu.in, xw.out, -1, -1)
	}
	return s.spawn(node, xu.in, xw.out, xu.prev, xw.next)
}

// fold takes vertex v, whose two edges meet head on along one line, and the
// overlap of its two segments up to the nearer neighbour: the overlap is a
// ridge between the two edges, and the neighbour carries on along the other
// edge.
func (s *skeleton) fold(v int) int {
	x := s.verts[v]
	pv := s.pos(v)
	node := s.nodeAt(pv)
	s.end(v, node)
	pp, pn := s.pos(x.prev), s.pos(x.next)
	if dist2(pv, pn) <= dist2(pv, pp) {
		nb := s.verts[x.next]
		m := s.nodeAt(pn)
		s.arcs = append(s.arcs, skelNodeArc{node, m, x.in, x.out})
		s.end(x.next, m)
		return s.spawn(m, x.in, nb.out, x.prev, nb.next)
	}
	nb := s.verts[x.prev]
	m := s.nodeAt(pp)
	s.arcs = append(s.arcs, skelNodeArc{m, node, x.out, x.in})
	s.end(x.prev, m)
	return s.spawn(m, nb.in, x.out, nb.prev, x.next)
}

// touch splits v's loop where v lies on a segment of it that v is not on
// already, and returns the vertices it made.
func (s *skeleton) touch(v int) ([]int, error) {
	x := s.verts[v]
	pv := s.pos(v)
	best, bestD := -1, s.size*skelLineTol
	steps := 0
	for u := x.next; u != x.prev; u = s.verts[u].next {
		steps++
		w := s.verts[u].next
		if u == x.next || w == x.prev {
			continue
		}
		if d := segmentDistance(pv, s.pos(u), s.pos(w)); d <= bestD {
			best, bestD = u, d
		}
	}
	if err := s.charge(steps); err != nil {
		return nil, err
	}
	if best < 0 {
		return nil, nil
	}
	u := best
	w := s.verts[u].next
	// v is on that segment: the split is there, not up to skelLineTol off
	// it, where the faces beyond would miss the ring.
	pu, pw := s.pos(u), s.pos(w)
	d := [2]float64{pw[0] - pu[0], pw[1] - pu[1]}
	if l2 := d[0]*d[0] + d[1]*d[1]; l2 > 0 {
		k := max(0, min(1, ((pv[0]-pu[0])*d[0]+(pv[1]-pu[1])*d[1])/l2))
		pv = [2]float64{pu[0] + k*d[0], pu[1] + k*d[1]}
	}
	node := s.nodeAt(pv)
	for _, c := range [2]int{u, w} {
		if dist2(pv, s.pos(c)) > s.size*skelLineTol {
			continue
		}
		// v meets vertex c head on: each carries on along the other's edge.
		xc := s.verts[c]
		s.end(v, node)
		s.end(c, node)
		a := s.spawn(node, x.in, xc.out, x.prev, xc.next)
		b := s.spawn(node, xc.in, x.out, xc.prev, x.next)
		return []int{a, b}, nil
	}
	e := s.verts[u].out
	s.end(v, node)
	a := s.spawn(node, x.in, e, x.prev, w)
	b := s.spawn(node, e, x.out, u, x.next)
	return []int{a, b}, nil
}

// launch gives each fresh vertex its velocity and schedules its events.
func (s *skeleton) launch() error {
	for _, v := range s.fresh {
		if s.verts[v].dead {
			continue
		}
		if err := s.velocity(v); err != nil {
			return err
		}
	}
	for _, v := range s.fresh {
		x := s.verts[v]
		if x.dead {
			continue
		}
		if err := s.scheduleEdge(x.prev); err != nil {
			return err
		}
		if err := s.scheduleEdge(v); err != nil {
			return err
		}
		if err := s.scheduleSplits(v); err != nil {
			return err
		}
	}
	return nil
}

// velocity moves v so that it stays on both its edges' fronts. A gable that
// v would lengthen leaves a step inside the roof.
func (s *skeleton) velocity(v int) error {
	x := &s.verts[v]
	a, b := x.in, x.out
	na, nb := s.normal[a], s.normal[b]
	sa, sb := s.speed[a], s.speed[b]
	dot := na[0]*nb[0] + na[1]*nb[1]
	det := na[0]*nb[1] - na[1]*nb[0]
	switch {
	case sa == sb:
		if 1+dot < 1e-12 {
			return errSkelTooComplex()
		}
		f := sa / (1 + dot)
		x.vel = [2]float64{(na[0] + nb[0]) * f, (na[1] + nb[1]) * f}
	case math.Abs(det) <= roofCollinear:
		// one line, one edge moving and the other not: a cliff between them
		if sa == 0 {
			return &RoofError{Reason: ReasonRoofStep, Edge: a, Other: -1}
		}
		return &RoofError{Reason: ReasonRoofStep, Edge: b, Other: -1}
	default:
		x.vel = [2]float64{(sa*nb[1] - sb*na[1]) / det, (na[0]*sb - nb[0]*sa) / det}
	}
	const grow = 1e-9
	switch {
	case sa == 0 && sb != 0 && x.vel[0]*s.dir[a][0]+x.vel[1]*s.dir[a][1] > grow:
		return &RoofError{Reason: ReasonRoofStep, Edge: a, Other: -1}
	case sb == 0 && sa != 0 && -(x.vel[0]*s.dir[b][0]+x.vel[1]*s.dir[b][1]) > grow:
		return &RoofError{Reason: ReasonRoofStep, Edge: b, Other: -1}
	}
	return nil
}

// scheduleEdge queues the event of segment u→next shrinking to a point.
func (s *skeleton) scheduleEdge(u int) error {
	x := s.verts[u]
	w := x.next
	if w == u {
		return nil
	}
	d := s.dir[x.out]
	pu, pw := s.pos(u), s.pos(w)
	vw := s.verts[w].vel
	rate := (vw[0]-x.vel[0])*d[0] + (vw[1]-x.vel[1])*d[1]
	if rate >= 0 {
		return nil
	}
	length := (pw[0]-pu[0])*d[0] + (pw[1]-pu[1])*d[1]
	return s.push(skelEvent{s.now + max(length, 0)/-rate, skelEdgeEvent, u, w})
}

// scheduleSplits queues the times a reflex vertex v reaches each edge's
// front; whether a segment of that edge is there is checked at the time.
func (s *skeleton) scheduleSplits(v int) error {
	x := s.verts[v]
	da, db := s.dir[x.in], s.dir[x.out]
	if da[0]*db[1]-da[1]*db[0] >= -roofCollinear {
		return nil
	}
	if err := s.charge(len(s.ring)); err != nil {
		return err
	}
	pv := s.pos(v)
	for e, ne := range s.normal {
		if e == x.in || e == x.out {
			continue
		}
		rate := ne[0]*x.vel[0] + ne[1]*x.vel[1] - s.speed[e]
		gap := ne[0]*(pv[0]-s.ring[e][0]) + ne[1]*(pv[1]-s.ring[e][1]) - s.speed[e]*s.now
		if rate >= 0 || gap < -s.tolP {
			continue
		}
		if err := s.push(skelEvent{s.now + max(gap, 0)/-rate, skelSplitEvent, v, e}); err != nil {
			return err
		}
	}
	return nil
}

// result walks each sloped edge's face from the arcs and checks that the
// faces are the roof: each walk takes every arc of its edge, every point is
// on its edge's plane, and the faces tile the ring.
func (s *skeleton) result(slope float64) ([]skelFace, []skelArc, error) {
	n := len(s.ring)
	at := func(i int) [3]float64 {
		nd := s.nodes[i]
		return [3]float64{nd.p[0], nd.p[1], nd.t * slope}
	}
	var faces []skelFace
	var total float64
	for e := range n {
		if s.speed[e] == 0 {
			continue
		}
		if err := s.charge(len(s.arcs)); err != nil {
			return nil, nil, err
		}
		next := map[int][]int{}
		for _, a := range s.arcs {
			switch e {
			case a.left:
				next[a.a] = append(next[a.a], a.b)
			case a.right:
				next[a.b] = append(next[a.b], a.a)
			}
		}
		poly := [][3]float64{at(e), at((e + 1) % n)}
		plan := [][2]float64{s.ring[e], s.ring[(e+1)%n]}
		for cur := (e + 1) % n; ; {
			outs := next[cur]
			if len(outs) == 0 || len(poly) > len(s.arcs)+2 {
				return nil, nil, errSkelTooComplex()
			}
			to := outs[0]
			next[cur] = outs[1:]
			if to == e {
				break
			}
			if !s.onPlane(e, to) {
				return nil, nil, errSkelTooComplex()
			}
			poly = append(poly, at(to))
			plan = append(plan, s.nodes[to].p)
			cur = to
		}
		for _, outs := range next {
			if len(outs) > 0 {
				return nil, nil, errSkelTooComplex()
			}
		}
		total += polygonArea2D(plan)
		faces = append(faces, skelFace{e, poly})
	}
	if !s.tiles(total) {
		return nil, nil, errSkelTooComplex()
	}
	arcs := make([]skelArc, 0, len(s.arcs))
	for _, a := range s.arcs {
		if dist2(s.nodes[a.a].p, s.nodes[a.b].p) > s.tolP {
			arcs = append(arcs, skelArc{at(a.a), at(a.b), a.left, a.right})
		}
	}
	return faces, arcs, nil
}

// onPlane reports whether node i is where edge e's front passes at the
// node's time, within skelLineTol of the ring's size. A fold between two
// near-parallel fronts can land further off, up to θ along its ridge (see
// skelFoldTol), which at a steep pitch is millimetres off the plane: that
// roof is refused, not built a little wrong.
func (s *skeleton) onPlane(e, i int) bool {
	nd, a, ne := s.nodes[i], s.ring[e], s.normal[e]
	d := ne[0]*(nd.p[0]-a[0]) + ne[1]*(nd.p[1]-a[1])
	return math.Abs(nd.t-d) <= skelLineTol*s.size
}

// tiles reports whether faces of this total plan area tile the ring. A
// face starts on its own eaves edge's two points, so only along a gable,
// where no ring point pins it, can a face's side stray: by up to tolP, the
// distance within which nodes merge.
func (s *skeleton) tiles(total float64) bool {
	var gables float64
	for e := range s.ring {
		if s.speed[e] == 0 {
			gables += dist2(s.ring[e], s.ring[(e+1)%len(s.ring)])
		}
	}
	want := polygonArea2D(s.ring)
	return math.Abs(total-want) <= skelAreaTol*want+s.tolP*gables
}

func dist2(a, b [2]float64) float64 { return math.Hypot(a[0]-b[0], a[1]-b[1]) }
