package geometry

import (
	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
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
	GlobalID  string
	Verts     []float32
	Tris      []uint32
	Placement model.Mat4 // world, meters, IFC-native Z-up (-> GLB node.matrix)
	BBoxMin   [3]float64 // world-space AABB, meters
	BBoxMax   [3]float64
	Source    GeomSource
}

// Scene is the assembled proxy geometry for a whole IFC model: one Element per
// source element, plus per-element warnings gathered during the build.
type Scene struct {
	Elements []Element
	Warnings []string
}

// Stats counts elements by the geometry path that produced them; Empty counts
// elements that yielded no mesh at all (Total == Extrude+Brep+OBB+Empty).
type Stats struct{ Total, Extrude, Brep, OBB, Empty int }

// Build assembles proxy geometry for every element in r, rendering ALL elements
// regardless of Emit.
func Build(f *step.File, r *model.Result) (*Scene, error) {
	s := &Scene{Elements: make([]Element, 0, len(r.Elements))}
	for i := range r.Elements {
		el := &r.Elements[i]
		verts, tris, src := elementMesh(f, el.ExpressID, r.UnitScale)
		ge := Element{
			GlobalID:  el.GlobalID,
			Verts:     verts,
			Tris:      tris,
			Placement: el.Placement,
			Source:    src,
		}
		if len(verts) == 0 {
			s.Warnings = append(s.Warnings, "no geometry for "+el.GlobalID)
		} else {
			ge.BBoxMin, ge.BBoxMax = worldAABB(verts, el.Placement)
		}
		s.Elements = append(s.Elements, ge)
	}
	return s, nil
}

// tessellateItem tessellates ONE representation item into element-local meters.
func tessellateItem(item *step.Instance, unitScale float64) ([]float32, []uint32, GeomSource) {
	return tessellateItemDepth(item, unitScale, 0)
}

// maxMapDepth bounds IfcMappedItem recursion. Real IFC mapped items nest 0-2
// levels deep; a cyclic or deeply-nested chain in a malformed/adversarial file
// would otherwise recurse unbounded and stack-overflow the import.
const maxMapDepth = 8

func tessellateItemDepth(item *step.Instance, unitScale float64, depth int) ([]float32, []uint32, GeomSource) {
	switch {
	case item.IsA("IfcMappedItem"):
		v, t, s, ok := mappedItemMesh(item, unitScale, depth)
		if ok {
			return v, t, s
		}
		// Deliberately do NOT fall through to obbFromItem here like every other
		// case below. collectPoints would walk the MappingSource's item in ITS
		// OWN local coordinate system, ignoring MappingTarget's transform — the
		// resulting box would be built from untransformed points and placed at
		// the wrong location, silently corrupting the element's AABB rather
		// than just being conservatively empty. Returning nil/OBB-tagged-empty
		// is safer than a mis-placed box.
		return nil, nil, SourceOBB
	case item.IsA("IfcExtrudedAreaSolid"):
		if v, t, ok := extrudeSolid(item); ok {
			return scaleVerts(v, unitScale), t, SourceExtrude
		}
	case item.IsA("IfcFacetedBrep"), item.IsA("IfcClosedShell"), item.IsA("IfcConnectedFaceSet"), item.IsA("IfcOpenShell"):
		if v, t, ok := brepMesh(item); ok {
			return scaleVerts(v, unitScale), t, SourceBrep
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
		if v, t, ok := shellBasedSurfaceModelMesh(item); ok {
			return scaleVerts(v, unitScale), t, SourceBrep
		}
	case item.IsA("IfcBooleanClippingResult"), item.IsA("IfcBooleanResult"):
		if v, t, s, ok := clipMeshByDifference(item, unitScale, depth); ok {
			return v, t, s
		}
	}
	v, t := obbFromItem(item, unitScale)
	return v, t, SourceOBB
}

// elementMesh returns the element-local mesh (meters) for expressID. Dispatches
// each representation item via tessellateItem (extrude/brep/mapped/OBB fallback).
func elementMesh(f *step.File, expressID int, unitScale float64) ([]float32, []uint32, GeomSource) {
	items := representationItems(f, expressID)
	var verts []float32
	var tris []uint32
	src := SourceOBB
	for _, item := range items {
		v, t, s := tessellateItem(item, unitScale)
		if len(v) == 0 {
			continue
		}
		appendMesh(&verts, &tris, v, t)
		src = promoteSource(src, s)
	}
	if len(verts) == 0 {
		return nil, nil, src
	}
	return verts, tris, src
}

func obbFromItem(item *step.Instance, unitScale float64) ([]float32, []uint32) {
	v, t, _, _ := obbMesh(collectPoints(item), unitScale)
	return v, t
}

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
