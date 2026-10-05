package geometry

import (
	"runtime"
	"sync/atomic"

	"github.com/blox-eng/goifc/internal/par"
	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// GeomSource records which tessellation path produced an element's mesh:
// extrude/brep are real geometry, obb is the bounding-box fallback.
type GeomSource string

const (
	SourceExtrude GeomSource = "extrude"
	SourceBrep    GeomSource = "brep"
	SourceOBB     GeomSource = "obb"
)

// promoteSource applies the extrude > brep > obb fidelity precedence: a later
// representation item upgrades the element's source only toward higher-fidelity
// geometry, never back down to OBB once real geometry exists.
func promoteSource(cur, next GeomSource) GeomSource {
	if next == SourceExtrude || (next == SourceBrep && cur != SourceExtrude) {
		return next
	}
	return cur
}

// Element is one element's proxy mesh in ELEMENT-LOCAL meters, plus its world
// placement and world-space AABB. Verts is X,Y,Z triples; Tris indexes them.
type Element struct {
	GlobalID string
	// Verts are ELEMENT-LOCAL X,Y,Z triples in meters. They are NOT world
	// coordinates: apply Placement (or call [Element.WorldVerts]) to obtain
	// world positions. BBoxMin/BBoxMax below ARE world — mixing a
	// Verts-derived direction with a BBox-derived position silently yields
	// wrong results in the local frame. For directions use
	// [Element.WorldNormal], which drops the translation.
	Verts     []float32
	Tris      []uint32
	Placement model.Mat4 // local -> world, meters, IFC-native Z-up (-> GLB node.matrix)
	BBoxMin   [3]float64 // world-space AABB, meters
	BBoxMax   [3]float64
	Source    GeomSource
	// partial: a representation item meshed to nothing and was left out, so
	// the mesh (and its AABB) can be smaller than the element.
	partial bool
	// boxed: an item in the mesh is its box fallback, though Source names the
	// real solid beside it.
	boxed bool
}

// Scene is the assembled proxy geometry for a whole IFC model: one Element per
// source element, plus per-element warnings gathered during the build.
type Scene struct {
	Elements []Element
	Warnings []string
}

// Stats counts elements by the geometry path that produced them; Empty counts
// elements that yielded no mesh at all (Total == Extrude+Brep+OBB+Empty).
// Partial counts, across the other buckets, elements that shipped a mesh with
// a representation item left out: their bounds may be too small.
type Stats struct{ Total, Extrude, Brep, OBB, Empty, Partial int }

// Build assembles proxy geometry for every element in r, rendering ALL elements
// regardless of Emit. Elements are meshed in parallel; the result is identical
// to a serial build, in the same order.
func Build(f *step.File, r *model.Result) (*Scene, error) {
	s := &Scene{Elements: make([]Element, len(r.Elements))}
	c := &meshCache{}
	overBudget := make([]bool, len(r.Elements))
	workers := min(runtime.GOMAXPROCS(0), len(r.Elements))
	var next atomic.Int64
	var g par.Group
	for range workers {
		g.Go(func() {
			for {
				i := int(next.Add(1) - 1)
				if i >= len(r.Elements) {
					return
				}
				el := &r.Elements[i]
				m := elementMesh(f, el.ExpressID, r.UnitScale, c)
				ge := Element{
					GlobalID:  el.GlobalID,
					Verts:     m.verts,
					Tris:      m.tris,
					Placement: el.Placement,
					Source:    m.src,
					partial:   m.partial,
					boxed:     m.boxed,
				}
				if len(m.verts) > 0 {
					ge.BBoxMin, ge.BBoxMax = worldAABB(m.verts, el.Placement)
				}
				s.Elements[i] = ge
				overBudget[i] = m.overBudget
			}
		})
	}
	g.Wait()
	for i, e := range s.Elements {
		switch {
		case len(e.Verts) == 0:
			s.Warnings = append(s.Warnings, "no geometry for "+e.GlobalID)
		case e.partial:
			s.Warnings = append(s.Warnings, "partial geometry for "+e.GlobalID+": a representation item has none")
		}
		if overBudget[i] {
			s.Warnings = append(s.Warnings, "tessellation budget exceeded for "+e.GlobalID+": items boxed or left out")
		}
	}
	return s, nil
}

// maxMapDepth bounds IfcMappedItem recursion. Real IFC mapped items nest 0-2
// levels deep; a cyclic or deeply-nested chain in a malformed/adversarial file
// would otherwise recurse unbounded and stack-overflow the import.
const maxMapDepth = 8

// tessellateItemDepth meshes one representation item, element-local meters.
// Every case falls back to the item's box except a failed IfcMappedItem; b
// bounds the work, and an item that exhausts it is boxed.
func tessellateItemDepth(item *step.Instance, unitScale float64, depth int, c *meshCache, b *budget) mesh {
	switch {
	case item.IsA("IfcMappedItem"):
		m, ok := mappedItemMesh(item, unitScale, depth, c, b)
		if ok {
			return m
		}
		// Deliberately do NOT fall through to obbFromItem here like every other
		// case below. collectPoints would walk the MappingSource's item in ITS
		// OWN local coordinate system, ignoring MappingTarget's transform — the
		// resulting box would be built from untransformed points and placed at
		// the wrong location, silently corrupting the element's AABB rather
		// than just being conservatively empty. Returning nil/OBB-tagged-empty
		// is safer than a mis-placed box.
		return mesh{src: SourceOBB, partial: m.partial, overBudget: m.overBudget || b.exhausted()}
	case item.IsA("IfcExtrudedAreaSolid"):
		if v, t, ok := extrudeSolid(item, b); ok {
			return mesh{verts: scaleVerts(v, unitScale), tris: t, src: SourceExtrude, ok: true}
		}
	case item.IsA("IfcFacetedBrep"), item.IsA("IfcClosedShell"), item.IsA("IfcConnectedFaceSet"), item.IsA("IfcOpenShell"):
		if v, t, ok := brepMesh(item, b); ok {
			return mesh{verts: scaleVerts(v, unitScale), tris: t, src: SourceBrep, ok: true}
		}
	case item.IsA("IfcShellBasedSurfaceModel"):
		// SbsmBoundary is a SET of IfcShell (IfcClosedShell/IfcOpenShell) — union
		// their faces. Multi-shell family instances (doors/windows with a frame +
		// leaf + hardware, each its own shell) commonly use this representation
		// type alongside plain IfcFacetedBrep siblings in the SAME element; missing
		// this case silently OBB-boxed just those shells while the element's
		// overall reported Source stayed "brep" (since brep still won on the other
		// sibling items) — a few stray boxed sub-shells shift the whole element's
		// AABB by a few mm-cm without ever showing up as a Source mismatch.
		if v, t, ok := surfaceModelMesh(item, attrSbsmBoundary, b); ok {
			return mesh{verts: scaleVerts(v, unitScale), tris: t, src: SourceBrep, ok: true}
		}
	case item.IsA("IfcFaceBasedSurfaceModel"):
		// FbsmFaces is a SET of IfcConnectedFaceSet — union their faces, the
		// same traversal the shell-based case above does over IfcShell.
		// Without this case duplex_a's 235 nested face sets are unreachable,
		// and every element built from one becomes a box.
		if v, t, ok := surfaceModelMesh(item, attrFbsmFaces, b); ok {
			return mesh{verts: scaleVerts(v, unitScale), tris: t, src: SourceBrep, ok: true}
		}
	case item.IsA("IfcTriangulatedFaceSet"), item.IsA("IfcTriangulatedIrregularNetwork"), item.IsA("IfcPolygonalFaceSet"):
		// IFC4's native tessellated body. Its points live in an
		// IfcCartesianPointList3D, which the OBB fallback below also reads, so a
		// declined set still gets a box.
		// ok with no triangles is a network flagged all void: authored as
		// empty, not a failure.
		if v, t, ok := faceSetMesh(item, b); ok {
			return mesh{verts: scaleVerts(v, unitScale), tris: t, src: SourceBrep, ok: true}
		}
	case item.IsA("IfcBooleanClippingResult"), item.IsA("IfcBooleanResult"):
		if m, ok := clipMeshByDifference(item, unitScale, depth, c, b); ok {
			return m
		}
	}
	v, t := obbFromItem(item, unitScale, c)
	return mesh{verts: v, tris: t, src: SourceOBB, ok: len(t) > 0, overBudget: b.exhausted()}
}

// elementMesh returns the element-local mesh (meters) for expressID. Dispatches
// each representation item via tessellateItemDepth (extrude/brep/mapped/OBB
// fallback), all of them sharing one budget.
func elementMesh(f *step.File, expressID int, unitScale float64, c *meshCache) mesh {
	return unionItems(representationItems(f, expressID), unitScale, 0, c, newBudget())
}

// unionItems meshes items into one mesh. An item that comes back empty is left
// out, and marks the union partial unless it was authored empty.
func unionItems(items []*step.Instance, unitScale float64, depth int, c *meshCache, b *budget) mesh {
	u := mesh{src: SourceOBB}
	seen := make(map[int]bool, len(items))
	for _, item := range items {
		// The same item named twice is the same geometry twice: mesh it once.
		if seen[item.ID()] {
			continue
		}
		seen[item.ID()] = true
		m := tessellateItemDepth(item, unitScale, depth, c, b)
		u.partial = u.partial || m.partial || (len(m.verts) == 0 && !m.ok)
		u.overBudget = u.overBudget || m.overBudget
		if len(m.verts) == 0 {
			continue
		}
		u.boxed = u.boxed || m.boxed || m.src == SourceOBB
		if u.verts == nil {
			// tessellateItemDepth's slices are the caller's own, so the first
			// item's mesh becomes the union's without a copy.
			u.verts, u.tris = m.verts, m.tris
		} else {
			appendMesh(&u.verts, &u.tris, m.verts, m.tris)
		}
		u.src = promoteSource(u.src, m.src)
	}
	u.ok = len(u.tris) > 0
	if !u.ok {
		u.partial = false // nothing shipped, so nothing is partial: the element is empty
	}
	return u
}

func obbFromItem(item *step.Instance, unitScale float64, c *meshCache) ([]float32, []uint32) {
	lo, hi, ok := c.itemBox(item, unitScale, func() (v3, v3, bool) {
		b := pointsBox(collectPoints(item, c))
		return scaleV3(b.lo, unitScale), scaleV3(b.hi, unitScale), b.ok
	})
	if !ok {
		return nil, nil
	}
	return boxMesh(lo, hi)
}

func scaleV3(p v3, s float64) v3 { return v3{p[0] * s, p[1] * s, p[2] * s} }

func scaleVerts(v []float32, s float64) []float32 {
	out := make([]float32, len(v))
	for i := range v {
		out[i] = float32(float64(v[i]) * s)
	}
	return out
}

// appendMesh concatenates a (verts,tris) mesh onto the element accumulator,
// offsetting the appended indices by the current vertex count.
func appendMesh(verts *[]float32, tris *[]uint32, av []float32, at []uint32) {
	base := uint32(len(*verts) / 3)
	*verts = append(*verts, av...)
	for _, idx := range at {
		*tris = append(*tris, base+idx)
	}
}

// Stats tallies the scene's elements by their GeomSource (see Stats type).
func (s *Scene) Stats() Stats {
	st := Stats{Total: len(s.Elements)}
	for _, e := range s.Elements {
		if len(e.Tris) == 0 {
			// A zero-geometry element keeps the default SourceOBB (see
			// elementMesh), but never actually produced a box — count it as
			// Empty only, not also in the OBB bucket.
			st.Empty++
			continue
		}
		if e.partial {
			st.Partial++
		}
		switch e.Source {
		case SourceExtrude:
			st.Extrude++
		case SourceBrep:
			st.Brep++
		case SourceOBB:
			st.OBB++
		}
	}
	return st
}
