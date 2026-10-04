package geometry

import (
	"math"
	"sync"

	"github.com/blox-eng/goifc/step"
)

// mesh is one tessellated result, as tessellateItemDepth's callers see it.
// partial means an item inside it meshed to nothing and was left out, so the
// mesh can be smaller than the shape; overBudget means an item ran out of its
// budget and was boxed.
type mesh struct {
	verts      []float32
	tris       []uint32
	src        GeomSource
	ok         bool
	partial    bool
	overBudget bool
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
	e, _ := c.bounds.LoadOrStore(list.ID(), &cachedBounds{})
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
		c, isPoint := numbersOf(pv)
		if !isPoint || len(c) != 3 {
			continue
		}
		ok = true
		// Comparisons, not min/max: a NaN coordinate is skipped, as obbMesh skips it.
		for k := range 3 {
			if c[k] < lo[k] {
				lo[k] = c[k]
			}
			if c[k] > hi[k] {
				hi[k] = c[k]
			}
		}
	}
	return lo, hi, ok
}
