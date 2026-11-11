package geometry

import (
	"errors"
	"fmt"
	"math"

	"github.com/blox-eng/goifc/model"
)

// Bounds on builder input. Vertices are float32 and local to the first point,
// so coordinates must stay near it: at 1,000 m a float32 step is 6e-5 m, still
// sub-millimetre. The first point goes into the float64 placement, and 1e7 m
// keeps every sum of areas and volumes finite.
const (
	minDim         = 1e-3 // 1 mm: below this a dimension is a typo or a rounding error
	maxLocalExtent = 1000.0
	maxOrigin      = 1e7
)

// Opening is a rectangle on a wall's measured face, in the wall's (u, v):
// u runs along the edge from its start, v up from the wall's base.
//
// Depth 0 cuts through the wall. Any other Depth makes a niche: it is cut from
// the measured face to that depth and leaves the opposite face whole. A niche
// needs a sill, and its depth must stay at least 1 mm inside the wall.
type Opening struct {
	Offset, Sill, Width, Height float64
	Depth                       float64
}

// cut is how deep the opening cuts: the full thickness unless it is a niche.
func (o Opening) cut(thickness float64) float64 {
	if o.Depth == 0 {
		return thickness
	}
	return o.Depth
}

// Face is one planar face as data, for drawing and dimensioning without
// re-deriving the frame. Origin, U, V and Normal are world; U and V are unit;
// Outline and Openings are in (u, v) meters on that plane.
type Face struct {
	Origin, U, V, Normal [3]float64
	Outline              [][2]float64
	Openings             []Opening
}

// SlabQuantities are a slab's exact quantities: Area is one face, in plan.
type SlabQuantities struct {
	Area, Perimeter, Volume float64
}

// Slab is what BuildSlab returns.
type Slab struct {
	Element    Element
	Quantities SlabQuantities
	// Top: Origin = outline[0] at z; U = +X; V = +Y; Normal = +Z; Outline =
	// the outline relative to outline[0].
	Top Face
}

// checkDim refuses a dimension that is not finite, under 1 mm or over 1,000 m.
func checkDim(name string, v float64) error {
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0):
		return fmt.Errorf("%s is not a finite number", name)
	case v < minDim:
		return fmt.Errorf("%s %.6g m is under the 1 mm minimum", name, v)
	case v > maxLocalExtent:
		return fmt.Errorf("%s %.6g m is over the %g m maximum", name, v, maxLocalExtent)
	}
	return nil
}

// checkOrigin refuses a placement coordinate that is not finite or is more
// than 1e7 m from the origin.
func checkOrigin(name string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s is not a finite number", name)
	}
	if math.Abs(v) > maxOrigin {
		return fmt.Errorf("%s %.6g m is more than %g m from the origin", name, v, maxOrigin)
	}
	return nil
}

// checkPoints refuses a point list whose first point is not finite or is far
// from the origin, or whose other points are not finite or lie more than
// 1,000 m from the first.
func checkPoints(pts [][2]float64) error {
	if err := checkOrigin("first point x", pts[0][0]); err != nil {
		return err
	}
	if err := checkOrigin("first point y", pts[0][1]); err != nil {
		return err
	}
	for i, p := range pts {
		if math.IsNaN(p[0]) || math.IsNaN(p[1]) || math.IsInf(p[0], 0) || math.IsInf(p[1], 0) {
			return fmt.Errorf("point %d is not finite", i)
		}
		if math.Hypot(p[0]-pts[0][0], p[1]-pts[0][1]) > maxLocalExtent {
			return fmt.Errorf("point %d is more than %g m from the first point", i, maxLocalExtent)
		}
	}
	return nil
}

// localRing returns the outline relative to its first point.
func localRing(outline [][2]float64) [][2]float64 {
	r := make([][2]float64, len(outline))
	for i, p := range outline {
		r[i] = [2]float64{p[0] - outline[0][0], p[1] - outline[0][1]}
	}
	return r
}

// checkOutline refuses an outline that is not a simple polygon of 3 to 2,048
// points within the bounds: a repeated point, an edge under 1 mm, crossing or
// overlapping edges, or zero area.
func checkOutline(outline [][2]float64) error {
	if len(outline) < 3 {
		return errors.New("an outline needs at least 3 points")
	}
	if len(outline) > maxOutlinePoints {
		return errTooManyPoints
	}
	if err := checkPoints(outline); err != nil {
		return err
	}
	ring := localRing(outline)
	for i := range ring {
		a, b := ring[i], ring[(i+1)%len(ring)]
		if math.Hypot(b[0]-a[0], b[1]-a[1]) < minDim {
			return fmt.Errorf("edge %d is under 1 mm: repeated point?", i)
		}
	}
	if math.Abs(polygonArea2D(ring)) < minDim*minDim {
		return errors.New("outline has zero area")
	}
	if edgesCrossOrOverlap([][][2]float64{ring}) {
		return errors.New("outline edges cross or overlap")
	}
	// edgesCrossOrOverlap lets edges meet at a point; a builder outline must
	// not pinch, so every point keeps 1 mm from the edges it is not on.
	n := len(ring)
	for i, p := range ring {
		for j := range ring {
			if j == i || (j+1)%n == i {
				continue
			}
			if segmentDistance(p, ring[j], ring[(j+1)%n]) < minDim {
				return fmt.Errorf("point %d touches edge %d: the outline pinches", i, j)
			}
		}
	}
	return nil
}

// segmentDistance is the distance from p to the segment a–b.
func segmentDistance(p, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	k := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / (dx*dx + dy*dy)
	k = math.Max(0, math.Min(1, k))
	return math.Hypot(p[0]-a[0]-k*dx, p[1]-a[1]-k*dy)
}

// newElement wraps a mesh local to origin: Placement translates to origin,
// the BBox is world, and the GlobalID is left for the caller.
func newElement(verts []float32, tris []uint32, origin [3]float64) Element {
	p := model.Identity()
	p[12], p[13], p[14] = origin[0], origin[1], origin[2]
	e := Element{Verts: verts, Tris: tris, Placement: p, Source: SourceExtrude}
	e.BBoxMin, e.BBoxMax = worldAABB(verts, p)
	return e
}
