package geometry

import (
	"sync"

	"github.com/blox-eng/goifc/step"
)

// mesh is one tessellated result, as tessellateItemDepth's callers see it.
type mesh struct {
	verts []float32
	tris  []uint32
	src   GeomSource
	ok    bool
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
	m sync.Map // mapKey -> *cachedMesh
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
	e, _ := c.m.LoadOrStore(mapKey{rep.ID(), scale, depth}, &cachedMesh{})
	cm := e.(*cachedMesh)
	cm.once.Do(func() { cm.mesh = build() })
	return cm.mesh
}
