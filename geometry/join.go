package geometry

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// End names one end of a wall's edge.
type End int

const (
	// EndA is the edge's start, edge[0].
	EndA End = iota + 1
	// EndB is the edge's end, edge[1].
	EndB
)

// MarshalText gives "a" or "b".
func (e End) MarshalText() ([]byte, error) {
	switch e {
	case EndA:
		return []byte("a"), nil
	case EndB:
		return []byte("b"), nil
	}
	return nil, fmt.Errorf("end %d is neither EndA nor EndB", int(e))
}

// UnmarshalText reads "a" or "b" and refuses anything else.
func (e *End) UnmarshalText(b []byte) error {
	switch string(b) {
	case "a":
		*e = EndA
	case "b":
		*e = EndB
	default:
		return fmt.Errorf("end %q is neither \"a\" nor \"b\"", b)
	}
	return nil
}

// Join declares that one end of a wall item (the partition) stops against a
// face of another wall (the through wall).
type Join struct {
	End  End
	Wall string // a wall item's ID, or a room wall "r1/2" (room ID, "/", wall index)
}

// JoinReason is why a join was refused.
type JoinReason string

const (
	// ReasonUnknownWall: the join names no wall item or room wall of the
	// plan, or the partition itself.
	ReasonUnknownWall JoinReason = "unknown_wall"
	// ReasonNotOnFace: the end lies more than 1 mm from both face planes of
	// the through wall, or the partition runs into the wall, not away from it.
	ReasonNotOnFace JoinReason = "not_on_face"
	// ReasonTooShallow: the walls' axes meet at under 15° or over 165°.
	ReasonTooShallow JoinReason = "too_shallow"
	// ReasonOffWall: the strip the partition covers leaves the face.
	ReasonOffWall JoinReason = "off_wall"
	// ReasonJoinsOverlap: the strip overlaps or touches an earlier strip on
	// the same face.
	ReasonJoinsOverlap JoinReason = "joins_overlap"
)

// JoinError refuses one join of a partition: Join indexes its Joins.
type JoinError struct {
	Item   string
	Join   int
	Reason JoinReason
}

func (e *JoinError) Error() string { return fmt.Sprintf("%s join %d: %s", e.Item, e.Join, e.Reason) }

// minJoinAngle bounds the angle between a partition's axis and the through
// wall's: 15° to 165°, inclusive.
const minJoinAngle = 15 * math.Pi / 180

// checkJoins refuses a join list that is malformed whatever the geometry:
// more than two joins, an end that is neither EndA nor EndB, or an end joined
// twice.
func checkJoins(joins []Join) error {
	if len(joins) > 2 {
		return fmt.Errorf("%d joins, more than one per end", len(joins))
	}
	var seen [3]bool
	for k, j := range joins {
		if j.End != EndA && j.End != EndB {
			return fmt.Errorf("join %d: end %d is neither EndA nor EndB", k, int(j.End))
		}
		if seen[j.End] {
			return fmt.Errorf("join %d: that end is already joined", k)
		}
		seen[j.End] = true
	}
	return nil
}

// face is one face plane of a wall: the points origin + u·s + g·offset, s in
// [start, end], from base to top. The measured face has offset 0; the
// opposite face lies the wall's whole thickness to the growth side.
type face struct {
	wall                          string
	frame                         wallFrame
	origin                        [2]float64
	offset, start, end, base, top float64
	measured                      bool
}

// outward is the face's unit normal, pointing away from the wall's body.
func (f face) outward() [2]float64 {
	if f.measured {
		return [2]float64{-f.frame.g[0], -f.frame.g[1]}
	}
	return f.frame.g
}

// depth is how far x lies from the face plane, towards the growth side.
func (f face) depth(x [2]float64) float64 {
	return (x[0]-f.origin[0])*f.frame.g[0] + (x[1]-f.origin[1])*f.frame.g[1] - f.offset
}

// wallFaces returns a wall's measured and opposite faces.
func wallFaces(name string, a, b [2]float64, base, height, thickness float64, side Side, span wallSpan) [2]face {
	f := face{wall: name, frame: newWallFrame(a, b, side), origin: a, base: base, top: base + height}
	m, o := f, f
	m.start, m.end, m.measured = 0, span.length, true
	o.offset, o.start, o.end = thickness, span.otherStart, span.otherEnd
	return [2]face{m, o}
}

// totalThickness is an item's thickness, summed over its layers if it has
// them.
func totalThickness(thickness float64, layers []float64) float64 {
	if layers == nil {
		return thickness
	}
	t := 0.0
	for _, l := range layers {
		t += l
	}
	return t
}

// joinTargets finds the walls a join may name: wall items by ID, room walls
// as "room/index".
type joinTargets struct {
	p      Plan
	walls  map[string]int
	rooms  map[string]int
	failed map[string]bool
	spans  map[int][]wallSpan
	trims  map[int]trimmedWall // by wall item; nil until partitions are trimmed
}

// target is a wall a join names: wall item item (room < 0), or wall w of
// room room (item < 0).
type target struct{ item, room, w int }

// lookup finds the wall name.
func (x *joinTargets) lookup(name string) (target, bool) {
	if i, ok := x.walls[name]; ok {
		return target{item: i, room: -1}, true
	}
	room, idx, ok := strings.Cut(name, "/")
	r, okRoom := x.rooms[room]
	if !ok || !okRoom {
		return target{}, false
	}
	w, err := strconv.Atoi(idx)
	if err != nil || strconv.Itoa(w) != idx || w < 0 || w >= len(x.p.Rooms[r].Outline) {
		return target{}, false
	}
	return target{item: -1, room: r, w: w}, true
}

func newJoinTargets(p Plan, failed map[string]bool) *joinTargets {
	x := &joinTargets{p: p, walls: map[string]int{}, rooms: map[string]int{}, failed: failed, spans: map[int][]wallSpan{}}
	for i, it := range p.Walls {
		x.walls[it.ID] = i
	}
	for i, it := range p.Rooms {
		x.rooms[it.ID] = i
	}
	return x
}

// faces returns the two faces of the wall name. known is false when the plan
// has no such wall; built is false when its item failed to build, or, once
// partitions are trimmed, when its own joins were refused, so the join is not
// checked. A trimmed partition's faces are its trimmed ones.
func (x *joinTargets) faces(name string) (fs [2]face, known, built bool) {
	tg, ok := x.lookup(name)
	if !ok {
		return fs, false, false
	}
	if tg.room < 0 {
		it := x.p.Walls[tg.item]
		if x.failed[it.ID] {
			return fs, true, false
		}
		l := math.Hypot(it.Edge[1][0]-it.Edge[0][0], it.Edge[1][1]-it.Edge[0][1])
		edge, span := it.Edge, wallSpan{l, 0, l}
		if x.trims != nil && len(it.Joins) > 0 {
			tw, ok := x.trims[tg.item]
			if !ok {
				return fs, true, false
			}
			edge, span = tw.edge, tw.span
		}
		t := totalThickness(it.Thickness, it.Layers)
		return wallFaces(name, edge[0], edge[1], it.Base, it.Height, t, it.Side, span), true, true
	}
	r, w := tg.room, tg.w
	it := x.p.Rooms[r]
	if x.failed[it.ID] {
		return fs, true, false
	}
	t := totalThickness(it.Thickness, it.Layers)
	spans, ok := x.spans[r]
	if !ok {
		// The room built, so its spans exist.
		spans, _, _ = roomSpans(localRing(it.Outline), t, it.Side)
		x.spans[r] = spans
	}
	a, b := it.Outline[w], it.Outline[(w+1)%len(it.Outline)]
	return wallFaces(name, a, b, it.Base, it.Height, t, it.Side, spans[w]), true, true
}

// joinTrim checks one join of p against the face it names (checks 2 to 4)
// and cuts p's end flush with that face, like a mitre: the measured line's
// hit is the snapped end, the opposite line's hit sets otherStart (end A) or
// otherEnd (end B). The span is in the snapped edge's own u. A refusal is a
// *JoinError whose Join the caller sets.
func joinTrim(p PlanWall, j Join, f face) (wallSpan, [2][2]float64, error) {
	refuse := func(r JoinReason) (wallSpan, [2][2]float64, error) {
		return wallSpan{}, p.Edge, &JoinError{Item: p.ID, Reason: r}
	}
	a, b := p.Edge[0], p.Edge[1]
	end, from := b, a
	if j.End == EndA {
		end, from = a, b
	}
	if math.Abs(f.depth(end)) > minDim {
		return refuse(ReasonNotOnFace)
	}
	l := math.Hypot(b[0]-a[0], b[1]-a[1])
	in := [2]float64{(from[0] - end[0]) / l, (from[1] - end[1]) / l} // from the end into the partition
	n := f.outward()
	if in[0]*n[0]+in[1]*n[1] <= 0 {
		return refuse(ReasonNotOnFace)
	}
	if math.Abs(in[0]*f.frame.u[0]+in[1]*f.frame.u[1]) > math.Cos(minJoinAngle)+1e-12 {
		return refuse(ReasonTooShallow)
	}

	pf := newWallFrame(a, b, p.Side)
	t := totalThickness(p.Thickness, p.Layers)
	// hit is where the line x0 + u·s crosses the face plane; the angle check
	// keeps u off parallel.
	ug := pf.u[0]*f.frame.g[0] + pf.u[1]*f.frame.g[1]
	hit := func(x0 [2]float64) float64 { return -f.depth(x0) / ug }
	sm := hit(a)
	so := hit([2]float64{a[0] + t*pf.g[0], a[1] + t*pf.g[1]})
	snapped := [2]float64{a[0] + sm*pf.u[0], a[1] + sm*pf.u[1]}

	var span wallSpan
	var edge [2][2]float64
	if j.End == EndA {
		span = wallSpan{length: l - sm, otherStart: so - sm, otherEnd: l - sm}
		edge = [2][2]float64{snapped, b}
	} else {
		span = wallSpan{length: sm, otherStart: 0, otherEnd: so}
		edge = [2][2]float64{a, snapped}
	}
	// As roomSpans does: the mesh grid merges closer values.
	if math.Abs(span.otherStart) < gridSnap {
		span.otherStart = 0
	}
	if math.Abs(span.otherEnd-span.length) < gridSnap {
		span.otherEnd = span.length
	}
	return span, edge, nil
}

// trimmedWall is a partition cut to the faces its joins name.
type trimmedWall struct {
	item  int // index into Plan.Walls
	edge  [2][2]float64
	span  wallSpan
	faces []face    // the face each join stops on, in join order
	ops   []Opening // the openings, offset from the trimmed edge's start
}

// trimJoins runs checks 1 to 4 on every join whose walls both built, and
// trims each partition whose joins all pass. A trimmed partition whose
// openings no longer fit is refused as an *ItemError, like any opening.
func trimJoins(p Plan, failed map[string]bool) ([]trimmedWall, []error) {
	x := newJoinTargets(p, failed)
	var (
		out  []trimmedWall
		errs []error
	)
	for i, it := range p.Walls {
		if len(it.Joins) == 0 || failed[it.ID] {
			continue
		}
		faces := make([]face, len(it.Joins))
		ok, skipped := true, false
		for k, j := range it.Joins {
			fs, known, built := x.faces(j.Wall)
			switch {
			case !known || j.Wall == it.ID:
				errs = append(errs, &JoinError{Item: it.ID, Join: k, Reason: ReasonUnknownWall})
				ok = false
				continue
			case !built:
				skipped = true
				continue
			}
			f := fs[0]
			if math.Abs(fs[1].depth(endPoint(it.Edge, j.End))) < math.Abs(fs[0].depth(endPoint(it.Edge, j.End))) {
				f = fs[1]
			}
			if _, _, err := joinTrim(it, j, f); err != nil {
				var je *JoinError
				if errors.As(err, &je) {
					je.Join = k
				}
				errs = append(errs, err)
				ok = false
				continue
			}
			faces[k] = f
		}
		if !ok || skipped {
			continue
		}
		t, err := trimWall(it, faces)
		if err != nil {
			errs = append(errs, &ItemError{Item: it.ID, Err: err})
			continue
		}
		t.item = i
		out = append(out, t)
	}
	return out, errs
}

func endPoint(edge [2][2]float64, e End) [2]float64 {
	if e == EndA {
		return edge[0]
	}
	return edge[1]
}

// trimWall cuts both of a partition's joined ends, end A first so end B's
// trim measures from the snapped start, and checks its openings against the
// trimmed span. The openings keep their place in the world: when end A snaps
// along the partition, their offsets move back by as much.
func trimWall(it PlanWall, faces []face) (trimmedWall, error) {
	l := math.Hypot(it.Edge[1][0]-it.Edge[0][0], it.Edge[1][1]-it.Edge[0][1])
	tw := trimmedWall{edge: it.Edge, span: wallSpan{l, 0, l}, faces: faces}
	for _, e := range [...]End{EndA, EndB} {
		for k, j := range it.Joins {
			if j.End != e {
				continue
			}
			cur := it
			cur.Edge = tw.edge
			s, edge, err := joinTrim(cur, j, faces[k])
			if err != nil {
				return tw, err
			}
			tw.edge = edge
			if e == EndA {
				tw.span = s
			} else {
				tw.span.length, tw.span.otherEnd = s.length, s.otherEnd
			}
		}
	}
	if tw.span.length < minDim || tw.span.otherEnd-tw.span.otherStart < minDim {
		return tw, errors.New("its joins leave the wall no length")
	}
	u := newWallFrame(it.Edge[0], it.Edge[1], it.Side).u
	shift := (tw.edge[0][0]-it.Edge[0][0])*u[0] + (tw.edge[0][1]-it.Edge[0][1])*u[1]
	tw.ops = append([]Opening(nil), it.Openings...)
	for i := range tw.ops {
		tw.ops[i].Offset -= shift
	}
	t := totalThickness(it.Thickness, it.Layers)
	if err := checkOpenings(0, tw.span, it.Height, t, tw.ops); err != nil {
		return tw, err
	}
	if layerCells(splitLayers(tw.span, wallLayerList(it), tw.ops), it.Height) > maxMeshCells {
		return tw, errTooMuchMesh
	}
	return tw, nil
}

// wallLayerList is an item's layers, or its one thickness as a single layer.
func wallLayerList(it PlanWall) []float64 {
	if it.Layers == nil {
		return []float64{it.Thickness}
	}
	return it.Layers
}

// buildTrimmed builds a trimmed partition's walls, one per layer.
func buildTrimmed(it PlanWall, tw trimmedWall) []Wall {
	return wallLayers(newWallFrame(tw.edge[0], tw.edge[1], it.Side), wallOrigin(tw.edge[0], it.Base), tw.span, it.Height, wallLayerList(it), tw.ops)
}

// strip is the band of a through wall's face a partition's end covers:
// [lo, hi] along the face's u, [bottom, top] in z.
type strip struct {
	face                face
	lo, hi, bottom, top float64
}

func (s strip) area() float64 { return (s.hi - s.lo) * (s.top - s.bottom) }

// touches reports whether s and o, on the same face, overlap or touch: the
// gap between them is under minDim along both u and z.
func (s *strip) touches(o *strip) bool {
	du := math.Max(s.lo, o.lo) - math.Min(s.hi, o.hi)
	dv := math.Max(s.bottom, o.bottom) - math.Min(s.top, o.top)
	return du < minDim && dv < minDim
}

// joinStrip is the strip that partition it, trimmed to tw, covers on the
// face f of its join j: from its measured line's hit, the snapped end, to its
// opposite line's.
func joinStrip(it PlanWall, tw trimmedWall, j Join, f face) strip {
	pf := newWallFrame(tw.edge[0], tw.edge[1], it.Side)
	t := totalThickness(it.Thickness, it.Layers)
	m := endPoint(tw.edge, j.End)
	o := [2]float64{m[0] + t*pf.g[0], m[1] + t*pf.g[1]}
	s := -f.depth(o) / (pf.u[0]*f.frame.g[0] + pf.u[1]*f.frame.g[1])
	o = [2]float64{o[0] + s*pf.u[0], o[1] + s*pf.u[1]}
	along := func(x [2]float64) float64 {
		return (x[0]-f.origin[0])*f.frame.u[0] + (x[1]-f.origin[1])*f.frame.u[1]
	}
	lo, hi := along(m), along(o)
	if lo > hi {
		lo, hi = hi, lo
	}
	return strip{face: f, lo: lo, hi: hi, bottom: math.Max(it.Base, f.base), top: math.Min(it.Base+it.Height, f.top)}
}

// stripJoins computes the strip of every join of every trimmed partition and
// runs checks 5 and 6 on it against the trimmed faces, in plan and join
// order. It then checks each through wall's openings against the strips that
// passed. It returns those strips and every refusal. A through wall stops
// taking strips once its openings and strips pass maxOpenings, which bounds
// each overlap check by maxOpenings; checkStrips then refuses it.
func stripJoins(p Plan, failed map[string]bool, trims []trimmedWall) ([]strip, []error) {
	x := newJoinTargets(p, failed)
	x.trims = make(map[int]trimmedWall, len(trims))
	for _, tw := range trims {
		x.trims[tw.item] = tw
	}
	type faceKey struct {
		wall     string
		measured bool
	}
	var (
		out    []strip
		errs   []error
		onFace = map[faceKey][]int{} // indices into out
		used   = map[string]int{}    // openings and strips on each through wall
	)
	for _, tw := range trims {
		it := p.Walls[tw.item]
		for k, j := range it.Joins {
			fs, _, built := x.faces(j.Wall)
			if !built {
				continue
			}
			f := fs[1]
			if tw.faces[k].measured {
				f = fs[0]
			}
			s := joinStrip(it, tw, j, f)
			if s.lo < f.start+minDim || s.hi > f.end-minDim || s.top-s.bottom < minDim {
				errs = append(errs, &JoinError{Item: it.ID, Join: k, Reason: ReasonOffWall})
				continue
			}
			n, seen := used[f.wall]
			if !seen {
				_, _, ops := x.openings(f.wall)
				n = len(ops)
			}
			if n > maxOpenings {
				continue
			}
			key := faceKey{f.wall, f.measured}
			if slices.ContainsFunc(onFace[key], func(i int) bool { return out[i].touches(&s) }) {
				errs = append(errs, &JoinError{Item: it.ID, Join: k, Reason: ReasonJoinsOverlap})
				used[f.wall] = n
				continue
			}
			onFace[key] = append(onFace[key], len(out))
			out = append(out, s)
			used[f.wall] = n + 1
		}
	}
	return out, append(errs, x.checkStrips(out)...)
}

// checkStrips refuses, on each through wall, more openings and strips than
// maxOpenings, and every opening that overlaps or touches a strip on a face
// it cuts (ReasonInJoin): a through opening cuts both faces, a niche the
// measured one. Each refusal is an *ItemError of the through wall's item.
func (x *joinTargets) checkStrips(strips []strip) []error {
	var walls []string
	on := map[string][]strip{}
	for _, s := range strips {
		if on[s.face.wall] == nil {
			walls = append(walls, s.face.wall)
		}
		on[s.face.wall] = append(on[s.face.wall], s)
	}
	var items []string
	refused := map[string][]error{}
	for _, name := range walls {
		item, wall, ops := x.openings(name)
		ss := on[name]
		var errs []error
		if len(ops)+len(ss) > maxOpenings {
			errs = append(errs, tooLarge("wall %d has more than %d openings and joins", wall, maxOpenings))
		} else {
			for i, o := range ops {
				if slices.ContainsFunc(ss, func(s strip) bool { return openingInStrip(o, s) }) {
					errs = append(errs, &PlacementError{Wall: wall, Opening: i, Reason: ReasonInJoin})
				}
			}
		}
		if errs == nil {
			continue
		}
		if refused[item] == nil {
			items = append(items, item)
		}
		refused[item] = append(refused[item], errs...)
	}
	var out []error
	for _, item := range items {
		out = append(out, &ItemError{Item: item, Err: errors.Join(refused[item]...)})
	}
	return out
}

// openingInStrip reports whether o cuts the face s lies on and overlaps or
// touches s.
func openingInStrip(o Opening, s strip) bool {
	if !s.face.measured && o.Depth != 0 {
		return false
	}
	z := s.face.base + o.Sill
	du := math.Max(o.Offset, s.lo) - math.Min(o.Offset+o.Width, s.hi)
	dv := math.Max(z, s.bottom) - math.Min(z+o.Height, s.top)
	return du < minDim && dv < minDim
}

// openings returns the item that owns the wall name, the wall's index in it,
// and its openings as they are built: a trimmed partition's offset from its
// trimmed start.
func (x *joinTargets) openings(name string) (item string, wall int, ops []Opening) {
	tg, _ := x.lookup(name)
	if tg.room >= 0 {
		it := x.p.Rooms[tg.room]
		return it.ID, tg.w, roomOpenings(it.Openings, tg.w)
	}
	it := x.p.Walls[tg.item]
	if tw, ok := x.trims[tg.item]; ok {
		return it.ID, 0, tw.ops
	}
	return it.ID, 0, it.Openings
}

// subtractStrips takes each strip off the net area of the built face it lies
// on, and off its floor line when the strip reaches the face's base: layer
// 0's measured face or the last layer's opposite face. wallAt and
// roomAt are where each wall item's and room's walls start in walls.
func subtractStrips(p Plan, walls []BuiltWall, wallAt, roomAt []int, strips []strip) {
	x := newJoinTargets(p, nil)
	for _, s := range strips {
		tg, _ := x.lookup(s.face.wall)
		var first, n int
		if tg.room >= 0 {
			n = max(1, len(p.Rooms[tg.room].Layers))
			first = roomAt[tg.room] + tg.w*n
		} else {
			n = len(wallLayerList(p.Walls[tg.item]))
			first = wallAt[tg.item]
		}
		a := s.area()
		var floor float64
		if s.bottom-s.face.base < minDim { // touching, as checkOpenings defines it
			floor = s.hi - s.lo
		}
		if s.face.measured {
			q := &walls[first].Wall.Quantities
			q.MeasuredJoins += a
			q.MeasuredNet -= a
			q.MeasuredFloorLine -= floor
		} else {
			q := &walls[first+n-1].Wall.Quantities
			q.OtherJoins += a
			q.OtherNet -= a
			q.OtherFloorLine -= floor
		}
	}
}
