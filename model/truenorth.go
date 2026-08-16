package model

import (
	"math"

	"github.com/blox-eng/goifc/step"
)

// attrTrueNorth is IfcGeometricRepresentationContext.TrueNorth, attribute 5 in
// both IFC2X3 and IFC4.
const attrTrueNorth = 5

// TrueNorth is the model's north direction in world XY, unit length. It lives
// on IfcGeometricRepresentationContext attribute 5 as an IfcDirection, and is
// the piece almost every consumer guesses instead of reading.
//
// Absent, malformed and zero-length norths all yield (0,1) — the IFC default,
// model +Y is north. Sub-contexts are skipped: IfcGeometricRepresentationSub-
// Context inherits TrueNorth from its parent rather than restating it, so
// reading one would return the default and mask a real north. Ordinary simple
// IfcGeometricRepresentationSubContext instances are already excluded by
// step.File.ByType's exact-match semantics (it never returns a different type
// keyword), so the IsA guard below is only load-bearing for the STEP
// complex-instance encoding "#id=(IfcGeometricRepresentationContext(...)
// IfcGeometricRepresentationSubContext(...))", which ByType indexes under
// both part types. Keeping the guard costs nothing and covers that case too.
// When a file carries several top-level contexts the lowest express ID wins,
// so the answer does not depend on file order.
func TrueNorth(f *step.File) [2]float64 {
	def := [2]float64{0, 1}

	var ctx *step.Instance
	for _, c := range f.ByType("IfcGeometricRepresentationContext") {
		if c.IsA("IfcGeometricRepresentationSubContext") {
			continue
		}
		if ctx == nil || c.ID() < ctx.ID() {
			ctx = c
		}
	}
	if ctx == nil {
		return def
	}

	d, ok := ctx.Ref(attrTrueNorth)
	if !ok || !d.IsA("IfcDirection") {
		return def
	}
	r := coords(d)
	if len(r) < 2 {
		return def
	}
	x, y := r[0], r[1]
	l := math.Hypot(x, y)
	// A zero-length or non-finite direction carries no bearing. Returning the
	// default is the honest answer; normalizing would emit NaN and poison every
	// azimuth downstream without erroring. The NaN/Inf half of this guard is not
	// reachable through step.Parse today — the STEP real-number grammar (digits,
	// '.', 'e'/'E', sign only) cannot spell "nan", and an overflowing literal
	// (e.g. "1E400") fails strconv.ParseFloat's error check in argparse.go and
	// aborts the parse before a Value ever exists. It stays as cheap defense in
	// depth against a future looser parser or a non-parser caller constructing
	// coordinates directly.
	if l < 1e-12 || math.IsNaN(l) || math.IsInf(l, 0) {
		return def
	}
	return [2]float64{x / l, y / l}
}
