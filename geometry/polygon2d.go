package geometry

import (
	"errors"
	"fmt"
	"math"
)

// maxOutlinePoints caps every outline the builders and Triangulate2D accept.
// Ear clipping a convex n-gon costs about n²/2 ear tests: the element work
// budget (1<<23) clips a 3,000-point convex outline and refuses a 4,096-point
// one, so 2,048 leaves headroom for concave input.
const maxOutlinePoints = 2048

var errTooManyPoints = fmt.Errorf("outline has more than %d points", maxOutlinePoints)

// PolygonArea2D returns the unsigned area of a simple polygon, in either
// winding. It works relative to the first point, so a polygon far from the
// origin keeps its area.
func PolygonArea2D(poly [][2]float64) float64 { return math.Abs(polygonArea2D(localRing(poly))) }

// PointInPolygon2D reports whether p is inside poly (even-odd rule). A point
// exactly on an edge may fall either way.
func PointInPolygon2D(p [2]float64, poly [][2]float64) bool { return pointInPolygon(p, poly) }

// Triangulate2D ear-clips a simple polygon (CCW or CW, concave allowed, no
// holes) into triangle indices into poly. It refuses fewer than 3 points, more
// than 2,048, and a loop it cannot clip whole within the work budget: one that
// touches or crosses itself.
func Triangulate2D(poly [][2]float64) ([]uint32, error) {
	if len(poly) < 3 {
		return nil, errors.New("a polygon needs at least 3 points")
	}
	if len(poly) > maxOutlinePoints {
		return nil, errTooManyPoints
	}
	if edgesCrossOrOverlap([][][2]float64{localRing(poly)}) {
		return nil, errors.New("polygon touches or crosses itself")
	}
	tris, ok := triangulatePolygon(poly, newBudget())
	if !ok || len(tris) == 0 {
		return nil, errors.New("polygon could not be triangulated whole within the work budget: it touches or crosses itself, or is too complex")
	}
	return tris, nil
}
