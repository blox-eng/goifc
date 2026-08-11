package geometry

import (
	"math"

	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// maxWalkDepth bounds collectPoints' forward-reference recursion. A
// legitimate representation subgraph is only a handful of levels deep; this
// guards against a crafted/adversarial deeply-nested subgraph (e.g. an
// IfcBooleanResult chain nested millions deep, reached via the OBB fallback)
// recursing until it stack-overflows the bounded worker — the same
// untrusted-input threat maxMapDepth guards against for mapped items. The
// seen-set cycle guard alone doesn't bound depth on an acyclic but
// pathologically deep subgraph.
const maxWalkDepth = 4096

// maxApproxLadder bounds the collectPoints -> extrudedAreaApproxPoints ->
// collectPoints ladder. A valid IFC profile never references an extruded
// solid, so this ladder has depth 0 on well-formed files; the bound exists
// solely so a crafted IfcExtrudedAreaSolid whose SweptArea references back to
// an extruded solid (a self-reference or a solid<->profile cycle) cannot
// recurse across the per-call fresh-seen collectPoints boundary until the Go
// stack overflows — which recover() cannot catch, taking down the import
// worker. Kept small so the ladder can never multiply the per-call
// maxWalkDepth walk into an overflow of its own.
const maxApproxLadder = 8

// collectPoints returns every IfcCartesianPoint coordinate reachable from item
// (walking the forward-reference subgraph), in raw file units.
func collectPoints(item *step.Instance) []v3 {
	return collectPointsLadder(item, 0)
}

// collectPointsLadder is collectPoints with the extrude-approximation ladder
// depth threaded through, so the collectPoints <-> extrudedAreaApproxPoints
// recursion is bounded (see maxApproxLadder).
func collectPointsLadder(item *step.Instance, ladder int) []v3 {
	var pts []v3
	if ladder > maxApproxLadder {
		return pts
	}
	seen := map[int]bool{}
	var walk func(inst *step.Instance, depth int)
	walk = func(inst *step.Instance, depth int) {
		if inst == nil || seen[inst.ID()] || depth > maxWalkDepth {
			return
		}
		seen[inst.ID()] = true
		// collectPoints cannot apply an IfcMappedItem's MappingTarget transform,
		// so descending into its MappingSource would harvest raw
		// mapping-source-local points as if they were element-local, corrupting
		// the AABB — the same reason tessellateItemDepth returns empty (not an
		// OBB) for a failed mapped item. A boolean/OBB fallback that wraps a
		// mapped item reaches here; skip the subtree so only correctly-framed
		// points survive rather than mis-placed ones.
		if inst.IsA("IfcMappedItem") {
			return
		}
		// A real (mesh-accurate) clip is attempted earlier, in tessellateItemDepth
		// via clipMeshByDifference — this point-walk is the OBB-fallback path,
		// reached only when that mesh clip declined (e.g. a bounded polygon
		// footprint, which a plain point-classification can't model correctly:
		// see clip.go's doc comment). IfcHalfSpaceSolid (and its bounded variant
		// IfcPolygonalBoundedHalfSpace, used heavily by Revit for wall/slab
		// miter-join clips) carries its own CartesianPoints in the CLIP PLANE's
		// local 2D coordinate system — e.g. a polygon boundary at local
		// (-1.65, 2.795) — NOT the element's frame. A blind point-walk that
		// reached in here would add those raw numbers straight into the element
		// AABB as if they were element-local, badly corrupting it (seen as
		// multi-metre spurious offsets on Revit-authored walls). A clip can only
		// SHRINK the first operand, so skipping the half-space subtree entirely
		// and keeping the (uncut) first operand's own extent is a safe,
		// conservative superset of the true clipped extent.
		if inst.IsA("IfcHalfSpaceSolid") || inst.IsA("IfcPolygonalBoundedHalfSpace") || inst.IsA("IfcBoxedHalfSpace") {
			return
		}
		if inst.IsA("IfcCartesianPoint") {
			c := floatsOf(inst, attrCoordinates)
			if len(c) >= 3 {
				pts = append(pts, v3{c[0], c[1], c[2]})
			} else if len(c) == 2 {
				pts = append(pts, v3{c[0], c[1], 0})
			}
		}
		// IfcExtrudedAreaSolid stores its Z extent as a scalar Depth, not a
		// CartesianPoint — a plain point-walk never sees the extruded top ring.
		// This bites when the solid is buried under an unsupported boolean op
		// (e.g. IfcBooleanClippingResult for a gable-end wall/roof clip) and the
		// OBB fallback is all we have: without this, the box silently collapses
		// to the profile's local (z=0) extent, or to whatever unrelated point
		// happens to sit further up the tree. Compute the true extruded corners
		// (profile ring at z=0 and z=depth, in the solid's own placement) so the
		// fallback box at least spans the solid's real extent.
		if inst.IsA("IfcExtrudedAreaSolid") {
			if verts, _, ok := extrudeSolid(inst); ok {
				for i := 0; i+2 < len(verts); i += 3 {
					pts = append(pts, v3{float64(verts[i]), float64(verts[i+1]), float64(verts[i+2])})
				}
			} else {
				// profilePolygon couldn't build a full ordered polygon (e.g. a
				// composite-curve profile with an IfcTrimmedCurve/arc segment we
				// don't tessellate — common on I-beam/channel steel profiles with
				// filleted corners). Still extrude whatever raw profile points
				// ARE reachable (straight-segment endpoints) to z=0 and z=depth —
				// an approximate envelope beats a box that never saw the depth at
				// all and collapses to a sliver along the extrusion axis.
				pts = append(pts, extrudedAreaApproxPoints(inst, ladder)...)
			}
		}
		for _, a := range inst.Args() {
			a.Walk(func(vv step.Value) {
				if vv.Kind == step.KindRef && vv.Ref != nil {
					walk(vv.Ref, depth+1)
				}
			})
		}
	}
	walk(item, 0)
	return pts
}

const attrCoordinates = 0

// extrudedAreaApproxPoints extrudes every raw point reachable in the solid's
// profile subtree to z=0 and z=depth (under the solid's own Position and
// ExtrudedDirection) — a point-cloud approximation used only when extrudeSolid
// couldn't build the exact profile polygon. Point ORDER doesn't matter here,
// only that the returned cloud spans the solid's real min/max extent.
func extrudedAreaApproxPoints(solid *step.Instance, ladder int) []v3 {
	prof, ok := solid.Ref(attrSweptArea)
	if !ok {
		return nil
	}
	profPts := collectPointsLadder(prof, ladder+1)
	if len(profPts) == 0 {
		return nil
	}
	depth := scalarAt(solid, attrExtrudeDepth)
	dir := v3{0, 0, 1}
	if d, ok := solid.Ref(attrExtrudedDir); ok {
		if c := floatsOf(d, attrCoordinates); len(c) == 3 {
			dir = normv(v3{c[0], c[1], c[2]})
		}
	}
	place := identityMat()
	if pos, ok := solid.Ref(attrSolidPosition); ok {
		place = axisPlacement3D(pos)
	}
	out := make([]v3, 0, 2*len(profPts))
	for _, p := range profPts {
		out = append(out, applyMat(place, v3{p[0], p[1], 0}))
		out = append(out, applyMat(place, v3{p[0] + dir[0]*depth, p[1] + dir[1]*depth, dir[2] * depth}))
	}
	return out
}

// obbMesh builds an axis-aligned box (element-local meters) spanning pts.
func obbMesh(pts []v3, unitScale float64) (verts []float32, tris []uint32, lmin, lmax v3) {
	if len(pts) == 0 {
		return nil, nil, v3{}, v3{}
	}
	lmin = v3{math.Inf(1), math.Inf(1), math.Inf(1)}
	lmax = v3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, p := range pts {
		for k := 0; k < 3; k++ {
			s := p[k] * unitScale
			if s < lmin[k] {
				lmin[k] = s
			}
			if s > lmax[k] {
				lmax[k] = s
			}
		}
	}
	verts, tris = boxMesh(lmin, lmax)
	return verts, tris, lmin, lmax
}

// boxMesh returns 8 corner verts + 12 triangles for the AABB min..max.
func boxMesh(min, max v3) ([]float32, []uint32) {
	c := [8]v3{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]},
		{max[0], max[1], min[2]}, {min[0], max[1], min[2]},
		{min[0], min[1], max[2]}, {max[0], min[1], max[2]},
		{max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	verts := make([]float32, 0, 24)
	for _, p := range c {
		verts = append(verts, float32(p[0]), float32(p[1]), float32(p[2]))
	}
	tris := []uint32{
		0, 1, 2, 0, 2, 3, 4, 6, 5, 4, 7, 6, 0, 4, 5, 0, 5, 1,
		1, 5, 6, 1, 6, 2, 2, 6, 7, 2, 7, 3, 3, 7, 4, 3, 4, 0,
	}
	return verts, tris
}

// worldAABB transforms local verts by placement and returns the world min/max.
func worldAABB(verts []float32, placement model.Mat4) (min, max [3]float64) {
	min = [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	max = [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i := 0; i+2 < len(verts); i += 3 {
		w := applyMat(placement, v3{float64(verts[i]), float64(verts[i+1]), float64(verts[i+2])})
		for k := 0; k < 3; k++ {
			if w[k] < min[k] {
				min[k] = w[k]
			}
			if w[k] > max[k] {
				max[k] = w[k]
			}
		}
	}
	return min, max
}
