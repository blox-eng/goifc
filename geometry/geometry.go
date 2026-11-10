package geometry

import (
	"fmt"
	"runtime"
	"sync"
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

// buildBudgetPerInstance and buildBudgetFloor size the work one Build may do,
// across all its elements: buildBudgetPerInstance per instance in the file,
// plus buildBudgetFloor. elementBudget bounds each element, but a shape that
// thousands of elements share costs each of them up to that bound, so a few MB
// of file could cost CPU-hours (#106). The corpus spends at most 30 per
// instance (private_a), so the Build budget keeps the sixteenfold headroom the
// element budget keeps; the floor lets a small file spend one element budget.
const (
	buildBudgetPerInstance = 512
	buildBudgetFloor       = elementBudget
)

// Build assembles proxy geometry for every element in r, rendering ALL elements
// regardless of Emit. Elements are meshed in parallel; the result is identical
// to a serial build, in the same order.
//
// The elements draw on one Build budget. In element order, those that start
// once it is spent are boxed, and those that start once it is spent twice over
// are left out, with one warning for all of them. Which elements those are is
// settled in element order after the parallel pass, so it does not depend on
// which worker finished first.
func Build(f *step.File, r *model.Result) (*Scene, error) {
	s := &Scene{Elements: make([]Element, len(r.Elements))}
	c := &meshCache{}
	size := int64(buildBudgetPerInstance)*int64(f.Len()) + buildBudgetFloor
	stateFor := func(spent int64) buildState {
		switch {
		case spent < size:
			return stateMeshed
		case spent < 2*size:
			return stateBoxed
		}
		return stateLeftOut
	}
	var pool atomic.Int64
	pool.Store(size)
	meshes := make([]mesh, len(r.Elements))
	states := make([]buildState, len(r.Elements))
	owns := make([]int64, len(r.Elements))
	shares := make([][]sharedWork, len(r.Elements))
	var claimed sync.Map
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
				states[i] = stateFor(size - pool.Load())
				meshes[i], owns[i], shares[i] = meshAs(f, r.Elements[i].ExpressID, r.UnitScale, c, states[i])
				pool.Add(-poolCost(owns[i], shares[i], func(id int) bool {
					_, taken := claimed.LoadOrStore(id, true)
					return !taken
				}))
			}
		})
	}
	g.Wait()

	// The parallel pass charged shared work to whichever worker claimed it
	// first, so it may have meshed, boxed or left out an element that in
	// element order starts elsewhere: settle each element in order, and redo
	// those whose state differs. Shared work is charged once, to the first
	// element in order that does it.
	var spent int64
	boxed, leftOut := 0, 0
	charged := map[int]bool{}
	for i := range r.Elements {
		want := stateFor(spent)
		if want != states[i] {
			states[i] = want
			meshes[i], owns[i], shares[i] = meshAs(f, r.Elements[i].ExpressID, r.UnitScale, c, want)
		}
		spent += poolCost(owns[i], shares[i], func(id int) bool {
			first := !charged[id]
			charged[id] = true
			return first
		})
		switch want {
		case stateBoxed:
			boxed++
		case stateLeftOut:
			leftOut++
		}
		el := &r.Elements[i]
		m := meshes[i]
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
	}
	for i, e := range s.Elements {
		switch {
		case len(e.Verts) == 0:
			s.Warnings = append(s.Warnings, "no geometry for "+e.GlobalID)
		case e.partial:
			s.Warnings = append(s.Warnings, "partial geometry for "+e.GlobalID+": a representation item has none")
		}
		if meshes[i].overBudget && states[i] == stateMeshed {
			s.Warnings = append(s.Warnings, "tessellation budget exceeded for "+e.GlobalID+": items boxed or left out")
		}
	}
	if boxed+leftOut > 0 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("build budget exceeded: %d elements boxed and %d left out; the model costs more than %d per instance in the file", boxed, leftOut, buildBudgetPerInstance))
	}
	return s, nil
}

// buildState is how Build meshes one element, by how much of the Build budget
// the elements before it spent.
type buildState uint8

const (
	stateMeshed  buildState = iota // within the Build budget: meshed in full
	stateBoxed                     // past it: each item is boxed
	stateLeftOut                   // past it twice over: no geometry
)

// meshAs meshes one element in state st. It returns the work the element did
// on its own, and the shared work it read from the cache.
func meshAs(f *step.File, expressID int, unitScale float64, c *meshCache, st buildState) (mesh, int64, []sharedWork) {
	switch st {
	case stateLeftOut:
		return mesh{src: SourceOBB}, 0, nil
	case stateBoxed:
		m, walks := elementBox(representationItems(f, expressID), unitScale, c)
		return m, int64(len(m.verts)/3 + len(m.tris)/3), walks
	}
	b := newBudget()
	b.cache = c
	m := unionItems(representationItems(f, expressID), unitScale, 0, c, b)
	own := b.spent()
	for _, w := range b.shared {
		own -= w.cost
	}
	return m, own, b.shared
}

// poolCost is what one element costs the Build budget: its own work, and each
// piece of shared work in full where first reports that this element is the
// first to be charged it, and 1 where it is not.
func poolCost(own int64, shared []sharedWork, first func(id int) bool) int64 {
	cost := own
	for _, w := range shared {
		if first(w.id) {
			cost += w.cost
		} else {
			cost++
		}
	}
	return cost
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
	v, t := obbFromItem(item, unitScale, c, b)
	return mesh{verts: v, tris: t, src: SourceOBB, ok: len(t) > 0, overBudget: b.exhausted()}
}

// elementMesh returns the element-local mesh (meters) for expressID. Dispatches
// each representation item via tessellateItemDepth (extrude/brep/mapped/OBB
// fallback), all of them sharing one budget.
func elementMesh(f *step.File, expressID int, unitScale float64, c *meshCache) mesh {
	b := newBudget()
	b.cache = c
	return unionItems(representationItems(f, expressID), unitScale, 0, c, b)
}

// unionItems meshes items into one mesh, charging each item's output to b. An
// item that comes back empty is left out, and marks the union partial unless it
// was authored empty; once b is overdrawn the remaining items are left out.
func unionItems(items []*step.Instance, unitScale float64, depth int, c *meshCache, b *budget) mesh {
	u := mesh{src: SourceOBB}
	seen := make(map[int]bool, len(items))
	for _, item := range items {
		// The same item named twice is the same geometry twice: mesh it once.
		if seen[item.ID()] {
			continue
		}
		seen[item.ID()] = true
		if !b.canOverdraw() {
			u.partial, u.overBudget = true, true
			break
		}
		m := tessellateItemDepth(item, unitScale, depth, c, b)
		u.partial = u.partial || m.partial || (len(m.verts) == 0 && !m.ok)
		u.overBudget = u.overBudget || m.overBudget
		if len(m.verts) == 0 {
			continue
		}
		// Output is charged too: a shape cheap to mesh may still be large, and a
		// mapped clone is as large as its shape at every placement. An item whose
		// output overspends shipped whole, so this alone flags nothing.
		b.spend(int64(len(m.verts)/3 + len(m.tris)/3))
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

// elementBox returns one box around items, and the walk of each item it read,
// as recorded the first time any element walked it: the same whoever walked
// it first. The walks stop at one element budget. A mapped item is left out:
// its box is in its own frame (see tessellateItemDepth).
func elementBox(items []*step.Instance, unitScale float64, c *meshCache) (mesh, []sharedWork) {
	m := mesh{src: SourceOBB}
	var lo, hi v3
	have := false
	var cost int64
	var walks []sharedWork
	seen := make(map[int]bool, len(items))
	for _, item := range items {
		if seen[item.ID()] {
			continue
		}
		seen[item.ID()] = true
		if item.IsA("IfcMappedItem") || cost > elementBudget {
			m.partial = true
			continue
		}
		l, h, ok, walked := itemBoxOf(item, unitScale, c)
		cost += walked
		walks = append(walks, sharedWork{item.ID(), walked})
		if !ok || walked > maxBoxWalk {
			m.partial = true
			continue
		}
		if !have {
			lo, hi, have = l, h, true
			continue
		}
		for k := range 3 {
			lo[k], hi[k] = min(lo[k], l[k]), max(hi[k], h[k])
		}
	}
	if !have {
		m.partial = false // nothing shipped, so the element is empty, not partial
		return m, walks
	}
	m.verts, m.tris = boxMesh(lo, hi)
	m.ok, m.boxed = true, true
	return m, walks
}

// itemBoxOf is item's box in scaled units, and what walking it cost, read once
// per Build through c.
func itemBoxOf(item *step.Instance, unitScale float64, c *meshCache) (lo, hi v3, ok bool, cost int64) {
	return c.itemBox(item, unitScale, func() (v3, v3, bool, int64) {
		pts, cost := collectPoints(item, c)
		bx := pointsBox(pts)
		return scaleV3(bx.lo, unitScale), scaleV3(bx.hi, unitScale), bx.ok, cost
	})
}

// obbFromItem returns item's box, charging its walk to b. An item whose walk
// ran past maxBoxWalk, or that arrives once b is overdrawn, gets no box.
func obbFromItem(item *step.Instance, unitScale float64, c *meshCache, b *budget) ([]float32, []uint32) {
	if !b.canOverdraw() {
		return nil, nil
	}
	lo, hi, ok, cost := itemBoxOf(item, unitScale, c)
	b.spend(cost)
	b.share(item.ID(), cost)
	if cost > maxBoxWalk || !ok {
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
