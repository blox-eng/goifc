package geometry

import (
	"cmp"
	"errors"
	"math"
	"slices"
)

var (
	errCapBudget  = errors.New("outline could not be triangulated whole within the work budget")
	errHoleBridge = errors.New("a hole could not be joined to the outline")
)

// triangulateWithHoles ear-clips a simple polygon with holes (any winding)
// into triangle indices into ring, the merged ring it clips. src[i] is ring[i]'s
// index among outer's points and then the holes' points, in order. The holes
// must lie inside outer and apart from it and from each other.
//
// Each hole is spliced into the ring by a bridge from its rightmost point to a
// ring vertex it can see, as earcut does (Eberly's method): holes with the
// rightmost points first, so a hole further right is already part of the ring.
// The bridge's two ends appear twice in ring. The error is errCapBudget when
// clipping stalls or b runs out, as for triangulatePolygon, and errHoleBridge
// when a hole has no vertex to bridge to.
func triangulateWithHoles(outer [][2]float64, holes [][][2]float64, b *budget) (ring [][2]float64, src, tris []uint32, err error) {
	ring = slices.Clone(outer)
	src = make([]uint32, len(outer))
	for i := range src {
		src[i] = uint32(i)
	}
	if polygonArea2D(ring) < 0 {
		slices.Reverse(ring)
		slices.Reverse(src)
	}
	start := make([]uint32, len(holes)) // each hole's first source index
	next := uint32(len(outer))
	rightX := make([]float64, len(holes))
	for k, h := range holes {
		start[k], next = next, next+uint32(len(h))
		rightX[k] = h[rightmost(h)][0]
	}
	order := make([]int, len(holes))
	for k := range order {
		order[k] = k
	}
	slices.SortStableFunc(order, func(i, j int) int { return cmp.Compare(rightX[j], rightX[i]) })
	for _, k := range order {
		if !b.spend(int64(len(ring))) {
			return nil, nil, nil, errCapBudget
		}
		// Wound against the ring, so the solid stays on the ring's left.
		h := slices.Clone(holes[k])
		hs := make([]uint32, len(h))
		for i := range hs {
			hs[i] = start[k] + uint32(i)
		}
		if polygonArea2D(h) > 0 {
			slices.Reverse(h)
			slices.Reverse(hs)
		}
		m := rightmost(h)
		p, found := bridgeVertex(ring, h[m])
		if !found {
			return nil, nil, nil, errHoleBridge
		}
		ring = splice(ring, p, h, m)
		src = splice(src, p, hs, m)
	}
	tris, ok := earClip(ring, b, true)
	if !ok || len(tris) == 0 {
		return nil, nil, nil, errCapBudget
	}
	return ring, src, tris, nil
}

// rightmost is the index of h's point with the largest x, the lowest of ties.
func rightmost(h [][2]float64) int {
	m := 0
	for i, p := range h {
		if p[0] > h[m][0] || (p[0] == h[m][0] && p[1] < h[m][1]) {
			m = i
		}
	}
	return m
}

// splice bridges hole h, from its point m, into ring at its vertex p: the
// ring to p, the hole round from m back to m, then p again and the rest.
func splice[T any](ring []T, p int, h []T, m int) []T {
	out := make([]T, 0, len(ring)+len(h)+2)
	out = append(out, ring[:p+1]...)
	out = append(out, h[m:]...)
	out = append(out, h[:m+1]...)
	return append(out, ring[p:]...)
}

// bridgeVertex is the index of a vertex of ring (counter-clockwise, holes
// bridged in) that m, a point inside it, can see. The ray from m towards +x
// first hits an edge at i; of the vertices in the triangle m, i and that edge's
// right end, the one at the smallest angle from the ray is visible. Of two
// copies of a bridge end, the one whose corner m lies in is taken.
func bridgeVertex(ring [][2]float64, m [2]float64) (int, bool) {
	n := len(ring)
	hit, edge := math.Inf(1), -1
	for i, a := range ring {
		c := ring[(i+1)%n]
		// Only upward edges: the ring's inside lies to their left, towards m.
		if a[1] <= m[1] && m[1] <= c[1] && a[1] < c[1] {
			x := a[0] + (m[1]-a[1])*(c[0]-a[0])/(c[1]-a[1])
			if x >= m[0] && x < hit {
				hit, edge = x, i
			}
		}
	}
	if edge < 0 {
		// No edge to the right of m: m is not inside the ring, so it cannot be bridged.
		return 0, false
	}
	a, c := ring[edge], ring[(edge+1)%n]
	right := a
	if c[0] > a[0] {
		right = c
	}
	in := [2]float64{hit, m[1]}
	best, bestTan, bestDist := -1, math.Inf(1), math.Inf(1)
	for j, r := range ring {
		if r[0] <= m[0] || r[0] > right[0] || !pointInTri(r, m, in, right) || !locallyInside(ring, j, m) {
			continue
		}
		tan, dist := math.Abs(r[1]-m[1])/(r[0]-m[0]), r[0]-m[0]
		if tan < bestTan || (tan == bestTan && dist < bestDist) {
			best, bestTan, bestDist = j, tan, dist
		}
	}
	return best, best >= 0
}

// locallyInside reports whether the direction from ring[j] to m lies inside
// the ring's corner at j (counter-clockwise: the inside is on the left).
func locallyInside(ring [][2]float64, j int, m [2]float64) bool {
	n := len(ring)
	a, p, c := ring[(j+n-1)%n], ring[j], ring[(j+1)%n]
	if cross2D(a, p, c) > 0 {
		return cross2D(a, p, m) >= 0 && cross2D(p, c, m) >= 0
	}
	return cross2D(a, p, m) > 0 || cross2D(p, c, m) > 0
}
