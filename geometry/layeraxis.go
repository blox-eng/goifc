package geometry

import (
	"math"

	"github.com/blox-eng/goifc/model"
)

// LayerAxis returns the world direction ls stacks along, from the FIRST
// declared layer toward the last, or ok=false when the set carries no
// resolvable direction (a bare IfcMaterialLayerSet with no usage, or an
// unrecognized LayerSetDirection).
//
// This closes the gap model.MaterialLayers documents but cannot fill: the
// declared sense says which way the stack runs from the reference line, which
// is not enough on its own to say which end is outside, because that needs the
// element's placement — which is here and not there.
//
// Compare the result against a Facing.Normal to learn whether the declared
// order already runs from the exposed face inward: a NEGATIVE dot product means
// the first declared layer is the outermost one. Consumers that describe a
// build-up outside-in need exactly this; taking the declared order on trust is
// a coin flip per element, and getting it backwards silently swaps the outer
// cladding with the inner finish.
//
// This library reports the direction and does not reorder Layers — mapping it
// onto a build-up convention is the consumer's job, as with LayerSet.Direction.
func LayerAxis(e Element, ls model.LayerSet) ([3]float64, bool) {
	var local v3
	switch ls.Direction {
	case "AXIS1":
		local = v3{1, 0, 0}
	case "AXIS2":
		local = v3{0, 1, 0}
	case "AXIS3":
		local = v3{0, 0, 1}
	default:
		return [3]float64{}, false
	}

	// DirectionSense NEGATIVE runs the stack against the axis. An absent sense
	// is POSITIVE, which is the IFC default — not an unknown to refuse, since
	// the axis alone already fixes the line.
	if ls.Sense == "NEGATIVE" {
		local = v3{-local[0], -local[1], -local[2]}
	}

	w := e.WorldNormal(local)
	if !finite3(w) {
		return [3]float64{}, false
	}

	// Scale by the largest component BEFORE squaring. Placements come from
	// untrusted files, and on a coordinate near 1e200 dotv overflows to +Inf;
	// dividing a finite w by an infinite length yields a ZERO vector reported
	// with ok=true, which a caller comparing against Facing.Normal reads as
	// "perpendicular" rather than "no direction". Scaling first also stops a
	// legitimately tiny direction from being rejected as degenerate: after the
	// division the length always lands in [1, sqrt 3].
	scale := math.Max(math.Abs(w[0]), math.Max(math.Abs(w[1]), math.Abs(w[2])))
	if !(scale > 0) {
		return [3]float64{}, false
	}
	s := v3{w[0] / scale, w[1] / scale, w[2] / scale}
	l := math.Sqrt(dotv(s, s))
	if !(l > 0) {
		return [3]float64{}, false
	}
	return v3{s[0] / l, s[1] / l, s[2] / l}, true
}
