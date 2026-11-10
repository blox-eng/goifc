package geometry

import (
	"math"
	"sync"

	"github.com/blox-eng/goifc/step"
)

// mesh is one tessellated result, as tessellateItemDepth's callers see it.
// partial means an item inside it meshed to nothing and was left out, so the
// mesh can be smaller than the shape; overBudget means an item ran out of its
// budget and was boxed or refused; boxed means an item that shipped is its box
// fallback, which src does not show once a real solid joins it. cost is the
// work a mapped representation took to mesh, charged to each element that
// maps it.
type mesh struct {
	verts      []float32
	tris       []uint32
	src        GeomSource
	ok         bool
	partial    bool
	overBudget bool
	boxed      bool
	cost       int64
}

// meshCache memoizes mapped representations for one Build. IFC maps a shared
// shape (a pipe fitting, a window type) into every occurrence; without the
// cache each occurrence re-tessellates it. The cached mesh is shared and must
// not be mutated: mappedItemMesh transforms it into a fresh slice per
// occurrence, and the triangle slice is only ever read or copied by appendMesh.
//
// A nil *meshCache disables caching, for callers meshing one element in
// isolation. It is safe for concurrent use.
type meshCache struct {
	m      sync.Map // mapKey -> *cachedMesh
	bounds sync.Map // point list instance id -> *cachedBounds
	boxes  sync.Map // boxKey -> *cachedBox
	tris   sync.Map // loop instance id -> *cachedTris
}

// cachedTris holds a loop's triangulation once a run completes. A run that
// ran out of its caller's budget is not kept, so mu, not a sync.Once, guards it.
type cachedTris struct {
	mu   sync.Mutex
	done bool
	tris []uint32
	ok   bool
	cost int64
}

// run triangulates the loop within what b has left, unless a run already
// completed, and keeps the result once one does. It returns the work of a run
// that ran out, or 0. The lock is released by defer: Build's workers recover a
// panic, and a loop left locked would stall every element that names it.
func (ct *cachedTris) run(b *budget, triangulate func(*budget) ([]uint32, bool)) (ranOut int64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if ct.done {
		return 0
	}
	run := sizedBudget(max(b.left, 0))
	tris, ok := triangulate(run)
	if run.exhausted() {
		return run.spent()
	}
	ct.tris, ct.ok, ct.cost, ct.done = tris, ok, run.spent(), true
	return 0
}

// triangulated returns the triangulation of the loop instance id: a face or a
// profile. triangulate computes it within the budget it is given, and its
// result must depend on the loop alone. With b's cache a run is bounded by what
// b has left and kept once it completes, so a loop is triangulated in full once
// per Build. b is charged what the triangulation cost: the same for every
// element that names the loop, whoever ran it first. A loop that costs more
// than b has left is refused, as if it had run out; a run that ran out is
// charged to b, as it would be with no cache.
//
// The triangles are shared and must not be changed.
func (b *budget) triangulated(id int, triangulate func(*budget) ([]uint32, bool)) ([]uint32, bool) {
	if b == nil || b.cache == nil {
		return triangulate(b)
	}
	e, found := b.cache.tris.Load(id)
	if !found {
		e, _ = b.cache.tris.LoadOrStore(id, &cachedTris{})
	}
	ct := e.(*cachedTris)
	if ranOut := ct.run(b, triangulate); ranOut > 0 {
		b.spend(ranOut)
		return nil, false
	}
	if !b.reserve(ct.cost) {
		return nil, false
	}
	b.share(sharedKey{kind: sharedTriangulation, id: id}, ct.cost)
	return ct.tris, ct.ok
}

// boxKey includes scale because a box is built in scaled units.
type boxKey struct {
	item  int
	scale float64
}

type cachedMesh struct {
	once sync.Once
	mesh mesh
}

// mapKey includes scale and depth because both change the result: the clip
// path meshes in raw units (scale 1), and maxMapDepth truncates a mapped chain
// differently depending on where it is entered.
type mapKey struct {
	rep   int
	scale float64
	depth int
}

func (c *meshCache) mapped(rep *step.Instance, scale float64, depth int, build func() mesh) mesh {
	if c == nil {
		return build()
	}
	// Every worker asking for one shape waits on the first to build it: a
	// popular fitting requested by all workers at once is still meshed once.
	key := mapKey{rep.ID(), scale, depth}
	e, ok := c.m.Load(key)
	if !ok {
		e, _ = c.m.LoadOrStore(key, &cachedMesh{})
	}
	cm := e.(*cachedMesh)
	cm.once.Do(func() { cm.mesh = build() })
	return cm.mesh
}

type cachedBounds struct {
	once   sync.Once
	lo, hi v3
	ok     bool
}

// pointListBounds returns the box around every well-formed point of an
// IfcCartesianPointList3D. A box is all the OBB fallback needs from a list, and
// many elements may share one large list: each is read once per Build.
func (c *meshCache) pointListBounds(list *step.Instance) (lo, hi v3, ok bool) {
	if c == nil {
		return listBounds(list)
	}
	// Every worker asking for one list waits on the first to read it.
	e, found := c.bounds.Load(list.ID())
	if !found {
		e, _ = c.bounds.LoadOrStore(list.ID(), &cachedBounds{})
	}
	cb := e.(*cachedBounds)
	cb.once.Do(func() { cb.lo, cb.hi, cb.ok = listBounds(list) })
	return cb.lo, cb.hi, cb.ok
}

func listBounds(list *step.Instance) (lo, hi v3, ok bool) {
	listV, has := list.Get(attrPointListCoords)
	if !has || listV.Kind() != step.KindList {
		return lo, hi, false
	}
	lo = v3{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi = v3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, pv := range listV.List() {
		// The walk charges one unit an entry, so an entry is read only when it
		// can be a point: a long one would be work the charge never saw.
		if pv.Kind() != step.KindList || len(pv.List()) != 3 {
			continue
		}
		c, isPoint := numbersOf(pv)
		if !isPoint {
			continue
		}
		ok = true
		// Comparisons, not min/max: a NaN coordinate is skipped, as pointsBox skips it.
		for k := range 3 {
			if c[k] < lo[k] {
				lo[k] = c[k]
			}
			if c[k] > hi[k] {
				hi[k] = c[k]
			}
		}
	}
	// An axis no point bounded (every value NaN) would box to infinity.
	for k := range 3 {
		if !(lo[k] <= hi[k]) {
			return lo, hi, false
		}
	}
	return lo, hi, ok
}

// itemBox returns the OBB-fallback box of item and what its walk cost, walked
// once per Build: many elements, or many references within one, can share an
// item whose walk is large even when its meshing declined at once. Every caller
// is charged the cost, so an element's budget does not depend on who walked
// the item first.
func (c *meshCache) itemBox(item *step.Instance, scale float64, walk func() (lo, hi v3, ok bool, cost int64)) (lo, hi v3, ok bool, cost int64) {
	if c == nil {
		return walk()
	}
	e, found := c.boxes.Load(boxKey{item.ID(), scale})
	if !found {
		e, _ = c.boxes.LoadOrStore(boxKey{item.ID(), scale}, &cachedBox{})
	}
	cb := e.(*cachedBox)
	cb.once.Do(func() { cb.lo, cb.hi, cb.ok, cb.cost = walk() })
	return cb.lo, cb.hi, cb.ok, cb.cost
}

type cachedBox struct {
	once   sync.Once
	lo, hi v3
	ok     bool
	cost   int64
}
