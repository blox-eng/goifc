package geometry

import (
	"errors"
	"fmt"
	"math"
)

// maxMitre caps how far a mitre point may lie from its outline corner, in
// wall thicknesses. The distance is thickness / sin(angle / 2), so 10 refuses
// interior angles under about 11.5°: past that the corner is a spike, not a
// wall join.
const maxMitre = 10

// roomSpans gives each wall of a closed outline its opposite-face span. The
// opposite face of edge i is its line offset by thickness to the growth side;
// it ends where it crosses the neighbouring walls' opposite lines (a mitre),
// or square where the neighbour runs straight on.
func roomSpans(ring [][2]float64, thickness float64, side Side) ([]wallSpan, error) {
	n := len(ring)
	frames := make([]wallFrame, n)
	spans := make([]wallSpan, n)
	for i := range ring {
		a, b := ring[i], ring[(i+1)%n]
		frames[i] = newWallFrame(a, b, side)
		spans[i].length = math.Hypot(b[0]-a[0], b[1]-a[1])
	}
	mitre := make([][2]float64, n) // the opposite-face corner at each outline point
	for i := range ring {
		prev, cur := frames[(i-1+n)%n], frames[i]
		lam, err := mitreAt(prev, cur, thickness)
		if err != nil {
			return nil, fmt.Errorf("corner %d: %w", i, err)
		}
		if math.Abs(lam) < gridSnap {
			lam = 0
		}
		if math.Hypot(thickness, lam) > maxMitre*thickness {
			return nil, fmt.Errorf("corner %d is too sharp: its mitre reaches more than %d wall thicknesses out", i, maxMitre)
		}
		p := (i - 1 + n) % n
		spans[i].otherStart = lam
		// The same point, measured along the previous wall from its start.
		spans[p].otherEnd = spans[p].length + mitreBack(prev, cur, thickness, lam)
		if math.Abs(spans[p].otherEnd-spans[p].length) < gridSnap {
			spans[p].otherEnd = spans[p].length
		}
		mitre[i] = [2]float64{
			ring[i][0] + thickness*cur.g[0] + lam*cur.u[0],
			ring[i][1] + thickness*cur.g[1] + lam*cur.u[1],
		}
	}
	for i, s := range spans {
		if s.otherEnd-s.otherStart < minDim {
			return nil, fmt.Errorf("wall %d: its opposite face has no length; the room is narrower than two walls there", i)
		}
	}
	if edgesCrossOrOverlap([][][2]float64{mitre}) {
		return nil, errors.New("walls overlap: a part of the outline is narrower than two wall thicknesses")
	}
	return spans, nil
}

// mitreAt returns where the opposite lines of prev and cur cross, as a u on
// cur measured from their shared outline point. Parallel walls running
// straight on meet square (0); walls that fold back on themselves have no
// mitre.
func mitreAt(prev, cur wallFrame, t float64) (float64, error) {
	// Solve lam·cur.u − mu·prev.u = t·(prev.g − cur.g).
	det := cross2(cur.u, prev.u)
	if math.Abs(det) < 1e-12 {
		if cur.u[0]*prev.u[0]+cur.u[1]*prev.u[1] > 0 {
			return 0, nil
		}
		return 0, errors.New("the outline folds back on itself")
	}
	r := [2]float64{t * (prev.g[0] - cur.g[0]), t * (prev.g[1] - cur.g[1])}
	return cross2(r, prev.u) / det, nil
}

// mitreBack returns the same mitre point as a u on prev, measured from the
// shared outline point (so prev's opposite face ends at length + this).
func mitreBack(prev, cur wallFrame, t, lam float64) float64 {
	m := [2]float64{t*cur.g[0] + lam*cur.u[0] - t*prev.g[0], t*cur.g[1] + lam*cur.u[1] - t*prev.g[1]}
	return m[0]*prev.u[0] + m[1]*prev.u[1]
}

func cross2(a, b [2]float64) float64 { return a[0]*b[1] - a[1]*b[0] }

// BuildRoomWalls builds the walls of one closed outline (meters, either
// winding), wall i running from outline[i] to outline[i+1]. Adjacent walls
// join on a mitre, so the core at a corner counts once and each measured face
// stops exactly at the outline's corner. openings[i] are wall i's openings;
// nil means none. A misplaced opening anywhere returns every *PlacementError
// joined with errors.Join; other invalid input returns a plain error.
func BuildRoomWalls(outline [][2]float64, base, height, thickness float64, side Side, openings [][]Opening) ([]Wall, error) {
	if err := checkWallDims(base, height, thickness, side); err != nil {
		return nil, err
	}
	if err := checkOutline(outline); err != nil {
		return nil, err
	}
	if openings != nil && len(openings) != len(outline) {
		return nil, fmt.Errorf("got openings for %d walls, the outline has %d", len(openings), len(outline))
	}
	ring := localRing(outline)
	spans, err := roomSpans(ring, thickness, side)
	if err != nil {
		return nil, err
	}
	opsOf := func(i int) []Opening {
		if openings == nil {
			return nil
		}
		return openings[i]
	}
	// One flat errors.Join of *PlacementError across all walls, in wall then
	// opening order: a caller that unwraps one level sees every refusal.
	var refusals []error
	cells := 0
	for i, s := range spans {
		err := checkOpenings(i, s, height, thickness, opsOf(i))
		var pe *PlacementError
		switch {
		case err == nil:
		case !errors.As(err, &pe):
			return nil, err
		default:
			refusals = append(refusals, err.(interface{ Unwrap() []error }).Unwrap()...)
		}
		cells += gridCells(s, height, opsOf(i))
	}
	if len(refusals) > 0 {
		return nil, errors.Join(refusals...)
	}
	if cells > maxMeshCells {
		return nil, errTooMuchMesh
	}
	walls := make([]Wall, len(spans))
	for i, s := range spans {
		a, b := outline[i], outline[(i+1)%len(outline)]
		f := newWallFrame(a, b, side)
		origin := [3]float64{a[0], a[1], base}
		pts, tris := meshWall(s, height, thickness, opsOf(i))
		verts, tris := placeWall(f, pts, tris)
		walls[i] = Wall{
			Element:    newElement(verts, tris, origin),
			Quantities: wallQuantities(s, height, thickness, opsOf(i)),
			Measured:   measuredFace(origin, f, s.length, height, opsOf(i)),
		}
	}
	return walls, nil
}
