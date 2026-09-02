package geometry

// worldCache memoises each element's world-space vertices for the life of ONE
// BuildFacings call.
//
// Why this exists: BuildFacings builds one occupancy grid per distinct 10 cm
// mid-height band, and each grid transforms every element the slice crosses or
// sits beneath. An element therefore has its vertices transformed once per band
// below it, not once. On a real ~1,900-element building that is 152 bands and
// 88x redundancy — measured on kb645.ifc: 164,670 worldPoints calls for 1,878
// elements, transforming 239 million triangles' worth of vertices to cover
// 3.3 million.
//
// The transform is a pure function of (Verts, Placement), and neither changes
// during a run, so the repeat work is pure waste rather than a correctness
// requirement. Caching trades it for memory proportional to the model's
// transformed vertices — one copy, held for one call, released with the cache.
//
// Scope is deliberately one call. The single-element entry points (Facing on
// one Element) have nothing to amortise over and call worldPoints directly;
// they must not be given a cache, or a long-lived one would pin every model
// they ever touched.
type worldCache struct {
	elems []Element
	pts   [][]v3
}

func newWorldCache(elems []Element) *worldCache {
	return &worldCache{elems: elems, pts: make([][]v3, len(elems))}
}

// at returns element i's world points, transforming on first ask.
//
// worldPoints always returns a non-nil slice (make with a computed length), so
// a nil entry unambiguously means "not yet computed" — including for an element
// with no vertices, which caches as an empty non-nil slice and is not recomputed.
func (c *worldCache) at(i int) []v3 {
	if c.pts[i] == nil {
		c.pts[i] = worldPoints(c.elems[i].Verts, c.elems[i].Placement)
	}
	return c.pts[i]
}

// fill computes every entry up front, after which the cache is READ-ONLY and so
// safe to share across the concurrent band workers. Filling lazily inside those
// workers would be a data race on pts — and a benign-looking one, since the
// racing writers compute identical values, which is exactly the kind the race
// detector catches and production does not.
func (c *worldCache) fill() {
	for i := range c.pts {
		if c.pts[i] == nil {
			c.pts[i] = worldPoints(c.elems[i].Verts, c.elems[i].Placement)
		}
	}
}
