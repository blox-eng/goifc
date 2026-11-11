package geometry

import (
	"errors"
	"fmt"
	"math"
)

// maxOpenings caps the openings on one wall. The overlap check compares every
// pair and the mesh grid grows with the square of the openings, so the cap
// bounds both.
const maxOpenings = 256

// Side names which side of the edge a→b the wall grows to. The measured face
// always lies on the edge line.
type Side int

const (
	// SideLeft measures the face on the left of a→b: the wall grows to the
	// right. A counter-clockwise room outline with SideLeft measures the
	// interior.
	SideLeft Side = iota + 1
	// SideRight measures the face on the right of a→b: the wall grows to
	// the left.
	SideRight
)

// MarshalText gives "left" or "right".
func (s Side) MarshalText() ([]byte, error) {
	switch s {
	case SideLeft:
		return []byte("left"), nil
	case SideRight:
		return []byte("right"), nil
	}
	return nil, fmt.Errorf("side %d is neither SideLeft nor SideRight", int(s))
}

// UnmarshalText reads "left" or "right" and refuses anything else.
func (s *Side) UnmarshalText(b []byte) error {
	switch string(b) {
	case "left":
		*s = SideLeft
	case "right":
		*s = SideRight
	default:
		return fmt.Errorf("side %q is neither \"left\" nor \"right\"", b)
	}
	return nil
}

func (s Side) valid() bool { return s == SideLeft || s == SideRight }

// PlacementReason is why an opening was refused.
type PlacementReason string

const (
	// ReasonTooWide: the opening reaches or passes an end of the measured face.
	ReasonTooWide PlacementReason = "too_wide"
	// ReasonOverlaps: the opening overlaps or touches an earlier opening.
	ReasonOverlaps PlacementReason = "overlaps"
	// ReasonAboveWall: the opening reaches or passes the top of the wall.
	ReasonAboveWall PlacementReason = "above_wall"
	// ReasonInCorner: the opening cuts into a corner join, where the face
	// opposite the measured one stops short.
	ReasonInCorner PlacementReason = "in_corner"
)

// PlacementError refuses one opening. The builders return every refusal at
// once, joined with errors.Join; errors.As finds the first.
type PlacementError struct {
	Wall    int // index of the wall (outline edge); 0 for BuildWall
	Opening int // index into that wall's openings
	Reason  PlacementReason
}

func (e *PlacementError) Error() string {
	return fmt.Sprintf("wall %d opening %d: %s", e.Wall, e.Opening, e.Reason)
}

// WallQuantities are a wall's exact quantities, from its numbers. Both faces
// lose the full area of every opening: every opening cuts through.
type WallQuantities struct {
	Length                     float64 // the measured face, along u
	Height                     float64
	MeasuredGross, MeasuredNet float64 // m², the face on the edge line
	OtherGross, OtherNet       float64 // m², the opposite face
	RevealPerimeter            float64 // m, Σ opening edges, minus the bottom edge when Sill == 0
	GrossVolume, NetVolume     float64 // m³
	NicheBack                  float64 // m², the back panels of the niches
}

// Wall is what BuildWall and BuildRoomWalls return.
type Wall struct {
	Element    Element
	Quantities WallQuantities
	// Measured: Origin = the edge start at the base; U along a→b; V up;
	// Normal away from the wall; Outline [0, Length] × [0, Height].
	Measured Face
}

// wallSpan is a wall in its own u: the measured face spans [0, length], the
// opposite face [otherStart, otherEnd]. BuildWall has square ends, so both
// faces span [0, length]; a mitred room corner moves the opposite face's end.
type wallSpan struct {
	length, otherStart, otherEnd float64
}

// checkOpenings checks a wall's openings. A malformed opening (not finite,
// negative, under 1 mm) is an input error and stops at the first. Otherwise
// every opening is checked in index order, the first failing rule wins for
// that opening, and all refusals return joined.
//
// "Touching" is anything closer than minDim: a sliver of wall under 1 mm is
// not wall, and the reveal behind a touching edge would not exist.
func checkOpenings(wall int, span wallSpan, height, thickness float64, ops []Opening) error {
	if len(ops) > maxOpenings {
		return fmt.Errorf("wall %d has %d openings, more than %d", wall, len(ops), maxOpenings)
	}
	for i, o := range ops {
		for _, f := range [...]struct {
			name string
			v    float64
		}{{"offset", o.Offset}, {"sill", o.Sill}, {"width", o.Width}, {"height", o.Height}, {"depth", o.Depth}} {
			if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
				return fmt.Errorf("wall %d opening %d: %s is not a finite number", wall, i, f.name)
			}
			if f.v < 0 {
				return fmt.Errorf("wall %d opening %d: %s is negative", wall, i, f.name)
			}
		}
		if o.Width < minDim || o.Height < minDim {
			return fmt.Errorf("wall %d opening %d: width and height must be at least 1 mm", wall, i)
		}
		if o.Sill > 0 && o.Sill < minDim {
			return fmt.Errorf("wall %d opening %d: sill %.6g m is under 1 mm; use 0 for a door", wall, i, o.Sill)
		}
		if o.Depth != 0 {
			if o.Depth < minDim || o.Depth > thickness-minDim {
				return fmt.Errorf("wall %d opening %d: niche depth %.6g m must be at least 1 mm and at least 1 mm under the %.6g m thickness; use 0 to cut through", wall, i, o.Depth, thickness)
			}
			if o.Sill == 0 {
				return fmt.Errorf("wall %d opening %d: a niche needs a sill; floor-level niches are not supported", wall, i)
			}
		}
	}
	var errs []error
	for i, o := range ops {
		var reason PlacementReason
		switch {
		case o.Offset < minDim || o.Offset+o.Width > span.length-minDim:
			reason = ReasonTooWide
		case o.Sill+o.Height > height-minDim:
			reason = ReasonAboveWall
		case overlapsEarlier(ops[:i], o):
			reason = ReasonOverlaps
		case outsideAtDepth(span, thickness, o):
			reason = ReasonInCorner
		default:
			continue
		}
		errs = append(errs, &PlacementError{Wall: wall, Opening: i, Reason: reason})
	}
	return errors.Join(errs...)
}

// outsideAtDepth reports whether o leaves the span the wall has at the depth
// o cuts to: between the end lines, which run from the measured face's ends
// (u = 0 and u = length) to the opposite face's (otherStart, otherEnd). A
// through opening needs wall on both faces; a niche only down to its depth.
func outsideAtDepth(span wallSpan, thickness float64, o Opening) bool {
	f := o.cut(thickness) / thickness
	lo := math.Max(0, span.otherStart*f)
	hi := math.Min(span.length, span.length+(span.otherEnd-span.length)*f)
	return o.Offset < lo+minDim || o.Offset+o.Width > hi-minDim
}

// overlapsEarlier reports whether o overlaps or touches any of earlier: the
// gap between them is under minDim along both u and v.
func overlapsEarlier(earlier []Opening, o Opening) bool {
	for _, p := range earlier {
		du := math.Max(o.Offset, p.Offset) - math.Min(o.Offset+o.Width, p.Offset+p.Width)
		dv := math.Max(o.Sill, p.Sill) - math.Min(o.Sill+o.Height, p.Sill+p.Height)
		if du < minDim && dv < minDim {
			return true
		}
	}
	return false
}

// wallQuantities computes a wall's quantities from its numbers. The plan of a
// wall is a trapezoid (its two faces are parallel, thickness apart), so its
// area is the mean face length times the thickness.
func wallQuantities(span wallSpan, height, thickness float64, ops []Opening) WallQuantities {
	other := span.otherEnd - span.otherStart
	var holes, through, cutVol, niches, reveal float64
	for _, o := range ops {
		a := o.Width * o.Height
		holes += a
		cutVol += a * o.cut(thickness)
		if o.Depth == 0 {
			through += a
		} else {
			niches += a
		}
		reveal += 2*o.Width + 2*o.Height
		if o.Sill == 0 {
			reveal -= o.Width
		}
	}
	gv := (span.length + other) / 2 * thickness * height
	return WallQuantities{
		Length:          span.length,
		Height:          height,
		MeasuredGross:   span.length * height,
		MeasuredNet:     span.length*height - holes,
		OtherGross:      other * height,
		OtherNet:        other*height - through,
		RevealPerimeter: reveal,
		GrossVolume:     gv,
		NetVolume:       gv - cutVol,
		NicheBack:       niches,
	}
}

// wallFrame is a wall's plan direction: u along the edge, g the unit
// direction the wall grows to.
type wallFrame struct{ u, g [2]float64 }

func newWallFrame(a, b [2]float64, side Side) wallFrame {
	l := math.Hypot(b[0]-a[0], b[1]-a[1])
	u := [2]float64{(b[0] - a[0]) / l, (b[1] - a[1]) / l}
	right := [2]float64{u[1], -u[0]}
	if side == SideLeft {
		return wallFrame{u, right}
	}
	return wallFrame{u, [2]float64{-right[0], -right[1]}}
}

// measuredFace is the measured face as data.
func measuredFace(origin [3]float64, f wallFrame, length, height float64, ops []Opening) Face {
	return Face{
		Origin:   origin,
		U:        [3]float64{f.u[0], f.u[1], 0},
		V:        [3]float64{0, 0, 1},
		Normal:   [3]float64{-f.g[0], -f.g[1], 0},
		Outline:  [][2]float64{{0, 0}, {length, 0}, {length, height}, {0, height}},
		Openings: append([]Opening(nil), ops...),
	}
}

// checkWallDims refuses a base, height or thickness out of bounds.
func checkWallDims(base, height, thickness float64, side Side) error {
	if !side.valid() {
		return fmt.Errorf("side %d is neither SideLeft nor SideRight", int(side))
	}
	if err := checkOrigin("base", base); err != nil {
		return err
	}
	if err := checkDim("height", height); err != nil {
		return err
	}
	return checkDim("thickness", thickness)
}

// BuildWall builds one straight wall with square ends from its edge in plan,
// its base elevation, height, thickness, the side it is measured on, and its
// openings. Quantities come from the numbers; the mesh is a closed shell that
// agrees with them. A misplaced opening returns *PlacementError values joined
// with errors.Join; other invalid input returns a plain error.
func BuildWall(edge [2][2]float64, base, height, thickness float64, side Side, openings []Opening) (Wall, error) {
	if err := checkWallDims(base, height, thickness, side); err != nil {
		return Wall{}, err
	}
	if err := checkPoints(edge[:]); err != nil {
		return Wall{}, err
	}
	a, b := edge[0], edge[1]
	length := math.Hypot(b[0]-a[0], b[1]-a[1])
	if err := checkDim("edge length", length); err != nil {
		return Wall{}, err
	}
	span := wallSpan{length: length, otherStart: 0, otherEnd: length}
	if err := checkOpenings(0, span, height, thickness, openings); err != nil {
		return Wall{}, err
	}
	if gridCells(span, height, openings) > maxMeshCells {
		return Wall{}, errTooMuchMesh
	}
	f := newWallFrame(a, b, side)
	origin := [3]float64{a[0], a[1], base}
	pts, tris := meshWall(span, height, thickness, openings)
	verts, tris := placeWall(f, pts, tris)
	return Wall{
		Element:    newElement(verts, tris, origin),
		Quantities: wallQuantities(span, height, thickness, openings),
		Measured:   measuredFace(origin, f, length, height, openings),
	}, nil
}
