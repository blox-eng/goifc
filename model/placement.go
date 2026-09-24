package model

import (
	"math"

	"github.com/blox-eng/goifc/step"
)

// LocalPlacement returns the world transform of inst's ObjectPlacement, composing
// the IfcLocalPlacement.PlacementRelTo chain (child placed relative to parent).
// Ports ifcopenshell.util.placement.get_local_placement. Units are the file's
// native length unit (Extract scales to meters). Missing/!IfcLocalPlacement -> identity.
func LocalPlacement(inst *step.Instance) Mat4 {
	place, ok := inst.Ref(attrObjectPlacement)
	if !ok {
		return Identity()
	}
	return localPlacementMatrix(place)
}

// maxPlacementDepth bounds the IfcLocalPlacement.PlacementRelTo chain. Cycle
// detection is what fixes the bug this file's guard exists for; this cap
// closes the second door — an ACYCLIC chain of distinct placements long
// enough to exhaust the stack on its own. Frames on this path run ~680 bytes,
// so the 1 GB goroutine limit falls near 1.5M of them, and a file that deep is
// something an uploader can construct.
//
// IFC bounds this depth nowhere, so the value is not a schema limit: it is far
// above any real building's spatial nesting and far below what a stack
// survives.
const maxPlacementDepth = 1024

// placementSeenInline is the chain depth that fits in placementSeen's inline
// array. Real placement chains are 2-4 deep, so every element of every
// well-formed file stays inside it and allocates nothing at all; only a chain
// deep enough for a linear scan's quadratic term to matter reaches for the
// map. The distinction is worth drawing because a file can point every one of
// its elements at a single shared deep chain, which multiplies whatever that
// chain costs by the element count.
const placementSeenInline = 16

// placementSeen is the set of express IDs already visited on the current
// chain. Membership is a scan of the inline array until the chain outgrows it,
// and O(1) in the map after that.
type placementSeen struct {
	n      int
	inline [placementSeenInline]int
	set    map[int]struct{}
}

func (s *placementSeen) has(id int) bool {
	if s.set != nil {
		_, ok := s.set[id]
		return ok
	}
	for _, v := range s.inline[:s.n] {
		if v == id {
			return true
		}
	}
	return false
}

func (s *placementSeen) add(id int) {
	if s.set == nil {
		if s.n < placementSeenInline {
			s.inline[s.n] = id
			s.n++
			return
		}
		s.set = make(map[int]struct{}, 2*placementSeenInline)
		for _, v := range s.inline {
			s.set[v] = struct{}{}
		}
	}
	s.set[id] = struct{}{}
	s.n++
}

func localPlacementMatrix(place *step.Instance) Mat4 {
	var seen placementSeen
	return localPlacementChain(place, &seen)
}

// localPlacementChain composes the PlacementRelTo chain, carrying the express
// IDs already visited so a cycle terminates instead of overflowing the stack.
func localPlacementChain(place *step.Instance, seen *placementSeen) Mat4 {
	if place == nil || !place.IsA("IfcLocalPlacement") {
		return Identity()
	}
	if seen.n >= maxPlacementDepth {
		return Identity()
	}
	if seen.has(place.ID()) {
		// A cycle. Return identity rather than a partial compose, so a
		// malformed placement is indistinguishable from a missing one —
		// the same answer the !IsA guard above gives.
		return Identity()
	}
	seen.add(place.ID())
	// world = parent(PlacementRelTo) * relative(RelativePlacement)
	parent := Identity()
	if p, ok := place.Ref(attrPlacementRelTo); ok {
		parent = localPlacementChain(p, seen)
	}
	rel := Identity()
	if a, ok := place.Ref(attrRelativePlacement); ok {
		rel = axis2placement(a)
	}
	return parent.Mul(rel)
}

// axis2placement builds a 4x4 from IfcAxis2Placement3D (Location + optional Z axis
// + optional X ref direction). 2D and defaulted axes fall back to world axes.
func axis2placement(a *step.Instance) Mat4 {
	m := Identity()
	if loc, ok := a.Ref(attrAxisLocation); ok {
		c := coords(loc)
		for i := 0; i < len(c) && i < 3; i++ {
			m[12+i] = c[i]
		}
	}
	z := []float64{0, 0, 1}
	if d, ok := a.Ref(attrAxisZ); ok {
		// A zero Axis would hit normalize's zero-norm passthrough and stay
		// zero, collapsing y = z X x too and leaving the placement SINGULAR —
		// the same defect orthogonalize closes one line below, reached by a
		// different door. IFC's own MagnitudeGreaterZero rule forbids it, but
		// this package parses files it did not author, so keep the default
		// world axis rather than trust the input.
		if v := coords(d); len(v) == 3 && dot(v, v) > degenerateSq {
			z = normalize(v)
		}
	}
	x := []float64{1, 0, 0}
	if d, ok := a.Ref(attrAxisX); ok {
		if v := coords(d); len(v) == 3 {
			x = v
		}
	}
	// Gram-Schmidt: make x orthonormal to z, y = z × x.
	x = normalize(orthogonalize(x, z))
	y := cross(z, x)
	// Columns 0,1,2 are the basis vectors x,y,z (column-major).
	m[0], m[1], m[2] = x[0], x[1], x[2]
	m[4], m[5], m[6] = y[0], y[1], y[2]
	m[8], m[9], m[10] = z[0], z[1], z[2]
	return m
}

// degenerateSq is the squared-magnitude floor below which a direction vector is
// treated as absent rather than meaningful. Squared, so callers compare against
// dot(v,v) without a square root.
const degenerateSq = 1e-20

// orthogonalize removes z's component from x. A RefDirection that is parallel
// to Axis (or zero) leaves nothing to project: normalize would hand back the
// zero vector, collapsing basis columns 0 and 1 and making the whole placement
// SINGULAR. Every direction rotated through such a matrix comes back
// zero-length, so a caller that normalizes a "world normal" gets NaN with
// nothing to notice — the silent-wrong-answer class this package works to
// avoid. Fall back to whichever world axis z leans on least, which is at most
// 45 degrees from the plane and so always projects to a usable vector.
func orthogonalize(x, z []float64) []float64 {
	// Scale-RELATIVE, not absolute: IfcDirection carries no unit-length
	// requirement, so a legal RefDirection of (1e-11,0,0) is perpendicular to
	// Axis and perfectly unambiguous. Against a fixed epsilon its projection
	// looks degenerate, the fallback fires, and the element's X axis silently
	// comes back rotated 90 degrees — a wrong answer produced by the guard
	// against wrong answers. Measure the projection against x's own magnitude.
	// The comparison is against x's OWN squared magnitude, with no floor: a
	// floor of 1 would re-absolutize the test for exactly the small vectors it
	// is meant to protect. Zero x gives a zero threshold, and 0 > 0 is false,
	// so the genuinely-degenerate case still falls through.
	if p := sub(x, scale(z, dot(x, z))); dot(p, p) > degenerateSq*dot(x, x) {
		return p
	}
	fallback := []float64{1, 0, 0}
	if math.Abs(z[0]) >= math.Abs(z[1]) {
		fallback = []float64{0, 1, 0}
	}
	return sub(fallback, scale(z, dot(fallback, z)))
}

// coords extracts the float list from an IfcCartesianPoint / IfcDirection.
func coords(inst *step.Instance) []float64 {
	v, ok := inst.Get(attrCoordinates)
	if !ok || v.Kind != step.KindList {
		return nil
	}
	out := make([]float64, 0, len(v.List))
	for _, e := range v.List {
		switch e.Kind {
		case step.KindFloat:
			out = append(out, e.F)
		case step.KindInt:
			out = append(out, float64(e.I))
		}
	}
	return out
}

func dot(a, b []float64) float64             { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func sub(a, b []float64) []float64           { return []float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func scale(a []float64, s float64) []float64 { return []float64{a[0] * s, a[1] * s, a[2] * s} }
func cross(a, b []float64) []float64 {
	return []float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func normalize(a []float64) []float64 {
	n := dot(a, a)
	if n == 0 {
		return a
	}
	s := 1 / math.Sqrt(n)
	return scale(a, s)
}
