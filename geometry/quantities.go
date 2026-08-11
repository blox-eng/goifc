package geometry

import (
	"math"

	"github.com/blox-eng/goifc/model"
)

// DerivedQuantities computes tier-2 (geometry-derived, GROSS) quantities per
// GlobalID from the tessellated proxy meshes, for elements the semantic Qto tier
// could not fill. Values are meters / m² / m³ in the world frame (Z-up):
//
//	Height = world-AABB vertical (Z) extent
//	Length = larger horizontal world-AABB extent
//	Width  = smaller horizontal world-AABB extent
//	Area   = horizontal world-AABB footprint (Length × Width)
//	Volume = closed-mesh gross volume (omitted when the mesh is not a closed shell)
//
// These are GROSS bounding estimates: openings are not subtracted (v1 walls are
// solid), and a rotated element's world AABB over-reports its plan dimensions.
// The model's quantity_source tag marks them "geometry" (see
// Result.ApplyDerivedQuantities) so downstream never mistakes a bounding
// estimate for an authored, net Qto value. Elements with no mesh are omitted
// (they stay quantity_source="none" — never a fabricated 0.0).
func (s *Scene) DerivedQuantities() map[string]model.Quantities {
	out := make(map[string]model.Quantities, len(s.Elements))
	for i := range s.Elements {
		e := &s.Elements[i]
		if len(e.Tris) == 0 || e.GlobalID == "" {
			continue
		}
		dx := e.BBoxMax[0] - e.BBoxMin[0]
		dy := e.BBoxMax[1] - e.BBoxMin[1]
		dz := e.BBoxMax[2] - e.BBoxMin[2]
		length, width := dx, dy
		if width > length {
			length, width = width, length
		}
		// Area + perimeter come from the MESH (ifcopenshell parity): area =
		// get_max_side_area (largest elevational face, e.g. a wall's face, NOT the
		// thin plan footprint); perimeter = get_footprint_perimeter. Length/Width/
		// Height stay world-AABB extents. For brep-passthrough elements the proxy
		// mesh IS the real mesh, so these match the Python baseline exactly; extrude/
		// OBB proxies approximate.
		mArea, _, mPerim := meshQuantities(e.Verts, e.Tris, e.Placement)
		q := model.Quantities{
			Height:    pos(dz),
			Length:    pos(length),
			Width:     pos(width),
			Area:      pos(mArea),
			Perimeter: pos(mPerim),
		}
		// Volume only from a true tessellation. An OBB fallback is the element's
		// bounding box, not a solid — emitting its "volume" over-reports hollow
		// elements by up to ~70x (benchmarked vs ifcopenshell). Kept manifold-gated
		// (stricter than ifcopenshell get_volume, which is unpredictable on
		// non-manifold meshes) to avoid emitting garbage volumes.
		if e.Source != SourceOBB {
			if v, ok := meshVolume(e.Verts, e.Tris); ok {
				q.Volume = pos(v)
			}
		}
		if q.IsEmpty() {
			continue // fully-degenerate mesh: no real extent -> stays source="none"
		}
		out[e.GlobalID] = q
	}
	return out
}

// pos returns &v for a positive extent, or nil when v <= 0. A zero-extent
// dimension is ABSENT, never a fabricated 0.0 — this is what upholds the
// model's nil-means-absent contract at the geometry tier.
func pos(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	return &v
}

// meshVolume returns the gross volume of a triangle mesh via the divergence
// theorem (signed tetrahedra from the origin), sign-folded so inward winding
// still yields a positive magnitude. ok is true only when isClosedManifold
// confirms the mesh is a genuine watertight, 2-manifold, consistently-wound
// closed shell — the Mirtich (1996) precondition the divergence-theorem sum
// requires to be a real volume. An open shell (common for IfcOpenShell breps)
// fails that gate and returns (0, false), since its signed sum is meaningless.
func meshVolume(verts []float32, tris []uint32) (float64, bool) {
	if len(tris) < 3 || len(verts) < 9 {
		return 0, false
	}
	nv := uint32(len(verts) / 3)
	at := func(i uint32) v3 {
		return v3{float64(verts[3*i]), float64(verts[3*i+1]), float64(verts[3*i+2])}
	}
	var vol float64
	for t := 0; t+2 < len(tris); t += 3 {
		i0, i1, i2 := tris[t], tris[t+1], tris[t+2]
		if i0 >= nv || i1 >= nv || i2 >= nv {
			return 0, false // malformed: index past the vertex array
		}
		a, b, c := at(i0), at(i1), at(i2)
		vol += dotv(a, crossv(b, c))
	}
	vol /= 6
	if vol < 0 {
		vol = -vol
	}
	// A closed, consistently-wound 2-manifold is the precondition under which the
	// divergence-theorem sum above is a real volume (Mirtich 1996). This replaces
	// the old `vol <= bbox` heuristic, which admitted 36% of topologically-broken
	// brep meshes as "valid" volumes.
	if vol <= 0 || !isClosedManifold(verts, tris) {
		return 0, false
	}
	return vol, true
}

// isClosedManifold reports whether tris form a closed 2-manifold with globally
// consistent winding: every DIRECTED edge appears exactly once and its reverse
// also appears exactly once. That single condition implies watertight (no
// boundary edge), 2-manifold (no edge shared by >2 triangles), and outward-
// consistent winding — the Mirtich (1996) precondition under which the
// divergence-theorem volume is a real volume.
//
// Vertices are welded by quantized position first: brepMesh emits a fresh vertex
// block per IfcFace (no cross-face index sharing), so raw-index adjacency is
// meaningless and every mesh would look "open".
func isClosedManifold(verts []float32, tris []uint32) bool {
	if len(tris) < 12 || len(tris)%3 != 0 { // < a tetrahedron's 4 faces
		return false
	}
	// weld coincident corners at 1e-5 m. Safe only because verts are ELEMENT-LOCAL
	// meters (element-sized, so float32 ulp sits well below the quantum) — feeding
	// WORLD coords with large georeferencing offsets (e.g. UTM eastings ~5e5, where
	// float32 ulp >> the quantum) would make genuine solids fail to weld. The
	// failure mode is honest rejection (0,false), never fabrication, but callers
	// must keep passing local verts.
	const q = 1e5
	weld := make(map[[3]int64]int)
	id := func(i uint32) int {
		if 3*i+2 >= uint32(len(verts)) {
			return -1
		}
		k := [3]int64{
			int64(math.Round(float64(verts[3*i]) * q)),
			int64(math.Round(float64(verts[3*i+1]) * q)),
			int64(math.Round(float64(verts[3*i+2]) * q)),
		}
		v, ok := weld[k]
		if !ok {
			v = len(weld)
			weld[k] = v
		}
		return v
	}
	directed := make(map[[2]int]int)
	for t := 0; t+2 < len(tris); t += 3 {
		a, b, c := id(tris[t]), id(tris[t+1]), id(tris[t+2])
		if a < 0 || b < 0 || c < 0 || a == b || b == c || a == c {
			return false // out-of-range index or degenerate triangle
		}
		directed[[2]int{a, b}]++
		directed[[2]int{b, c}]++
		directed[[2]int{c, a}]++
	}
	for e, n := range directed {
		if n != 1 || directed[[2]int{e[1], e[0]}] != 1 {
			return false // duplicated directed edge, boundary, non-manifold, or bad winding
		}
	}
	return true
}
