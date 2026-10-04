package geometry

import (
	"math"
	"sync"

	"github.com/blox-eng/goifc/step"
)

// mesh is one tessellated result, as tessellateItemDepth's callers see it.
// partial means an item inside it meshed to nothing and was left out, so the
// mesh can be smaller than the shape; overBudget means an item ran out of its
// budget and was boxed or refused. cost is the work a mapped representation
// took to mesh, charged to each element that maps it.
type mesh struct {
	verts      []float32
	tris       []uint32
	src        GeomSource
	ok         bool
	partial    bool
	overBudget bool
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
	boxes  sync.Map // boxKey -> *cachedBounds
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
	return onceBounds(&c.bounds, list.ID(), func() (v3, v3, bool) { return listBounds(list) })
}

// onceBounds returns m[key]'s bounds, computing them with walk the first time.
// Every worker asking for one key waits on the first to compute it.
func onceBounds(m *sync.Map, key any, walk func() (lo, hi v3, ok bool)) (lo, hi v3, ok bool) {
	e, found := m.Load(key)
	if !found {
		e, _ = m.LoadOrStore(key, &cachedBounds{})
	}
	cb := e.(*cachedBounds)
	cb.once.Do(func() { cb.lo, cb.hi, cb.ok = walk() })
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
		c, isPoint := numbersOf(pv)
		if !isPoint || len(c) != 3 {
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

// itemBox returns the OBB-fallback box of item, walked once per Build: many
// elements, or many references within one, can share an item whose walk is
// large even when its meshing declined at once.
func (c *meshCache) itemBox(item *step.Instance, scale float64, walk func() (lo, hi v3, ok bool)) (lo, hi v3, ok bool) {
	if c == nil {
		return walk()
	}
	return onceBounds(&c.boxes, boxKey{item.ID(), scale}, walk)
}
