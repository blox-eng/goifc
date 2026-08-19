package ifc

import (
	"fmt"

	"github.com/blox-eng/goifc/geometry"
	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// ImportNode is one node of the import contract: spatial containers +
// physical elements in a single parents-first tree. Spatial nodes have no
// geometry, so their AABB stays zero.
type ImportNode struct {
	GlobalID       string
	ExpressID      int
	IFCClass       string
	Name           string
	ParentIndex    *int // index into the emitted slice; nil = import root
	Qto            model.Quantities
	QuantitySource string
	OriginMin      [3]float64 // world-AABB min (transform.origin); zero if no geometry
	BBoxMin        [3]float64
	BBoxMax        [3]float64
	Material       string   // model.Element.Material; "" when none
	IsExternal     *bool    // *Common.IsExternal tri-state; nil when unknown
	NetArea        *float64 // trusted net area (m²) from Scene.NetAreas; nil when absent/untrusted
	// OpeningPerimeter is the boundary length (m) of the opening union whose
	// area NetArea already nets out — the length facade trades bill reveals
	// along. Present exactly when NetArea is: both come from one trusted
	// reconciliation, so a consumer can never read a confident perimeter beside
	// an absent net.
	OpeningPerimeter *float64
	// OpeningDeduction is the area (m²) of that same opening union, and
	// ProjectedGross is the host's own silhouette on the same plane — the two
	// halves NetArea is the difference of. Both are measured on the host's
	// winning projection axis, which is what makes them a matched pair.
	//
	// They are published because NetArea alone cannot be aggregated: hosts with
	// no IfcRelVoidsElement are ABSENT from the reconciliation entirely, so
	// summing NetArea over a facade silently drops every solid wall. Netting a
	// total therefore means subtracting the DEDUCTION from whatever gross the
	// caller is totalling, not summing the nets.
	//
	// That matters most when the gross being netted is measured differently.
	// [geometry.Facing].FaceArea is the true on-face area and does not
	// foreshorten, while these two are a projection and do; the projection
	// foreshortens gross and deduction by the SAME factor, so a caller netting
	// an on-face gross should scale the deduction by ProjectedGross's ratio to
	// it rather than subtracting it raw. Without ProjectedGross that bias is
	// not merely uncorrected, it is invisible.
	//
	// Present exactly when NetArea is, for the reason OpeningPerimeter is: all
	// four come from one trusted reconciliation. A zero deduction here means a
	// host whose openings measured zero, never an untrusted one.
	OpeningDeduction *float64
	ProjectedGross   *float64

	// TypeGlobalID / TypeName / TypeClass identify the element's IfcTypeObject.
	// Empty when the element carries no IfcRelDefinesByType — most elements do.
	TypeGlobalID string
	TypeName     string
	TypeClass    string
}

// TypeLayerSet is one IfcTypeObject's assembly build-up, in declared EXPRESS
// LIST order. Direction/Sense are the raw IFC labels from
// IfcMaterialLayerSetUsage (AXIS1/AXIS2/AXIS3, POSITIVE/NEGATIVE); mapping them
// onto a product vocabulary belongs to the consumer.
type TypeLayerSet struct {
	Layers    []model.MaterialLayer
	Direction string
	Sense     string
}

// ImportModel is the assembled import contract: the parents-first node tree plus
// the proxy-geometry Scene the physical nodes' meshes are baked from (Scene.WriteGLB).
type ImportModel struct {
	Nodes       []ImportNode
	Scene       *geometry.Scene
	StoreyPlans []StoreyPlan
	// TypeLayers is the build-up per distinct IfcTypeObject, keyed by the type's
	// GlobalId — the same key ImportNode.TypeGlobalID carries. Keyed by TYPE, not
	// by occurrence: a real model has ~70 types and ~1,400 typed occurrences, and
	// this whole struct is typically serialized into a workflow payload with a
	// low-megabyte size cap.
	//
	// A type that resolved but carries no ordered build-up is PRESENT with an
	// empty Layers slice — that is a positive claim ("this type has no layers"),
	// distinct from absence ("no such type in this model"), and consumers
	// reconciling previously-imported layers need the difference: the first must
	// retire every stale row, the second must touch nothing.
	TypeLayers map[string]TypeLayerSet
}

// BuildImport turns a parsed STEP file into the import contract, assembling the
// model first. It is [BuildImportFrom] over a fresh [Assemble] — use that
// directly when you also need the Result or Scene the import was derived from,
// so the model is tessellated once rather than twice.
func BuildImport(f *step.File) (*ImportModel, error) {
	a, err := Assemble(f)
	if err != nil {
		return nil, err
	}
	return BuildImportFrom(f, a)
}

// BuildImportFrom turns a parsed STEP file plus its assembly into the import
// contract: spatial containers (SpatialNodes) + physical elements in ONE
// parents-first ordered tree.
//
// [Assemble] is the expensive stage, and its Result and Scene are worth more
// than the import contract alone: an elevation, a net-area reconciliation or a
// GLB all read them. Taking the assembly as an argument is what lets one caller
// have both without paying for the tessellation twice.
//
// f must be the same *step.File the assembly was built from. Below this point
// everything joins physical elements to spatial containers and geometry by
// bare ExpressID (forwardParentMap, f.ByID, Scene.NetAreas), and ExpressIDs
// are small sequential integers that restart per file — a mismatched pair
// would very plausibly collide rather than miss, handing an element another
// model's parent, material or opening data with no error at all. An Assembled
// stamped by [Assemble] is checked by pointer identity against f, on purpose:
// a re-parse of the same bytes produces an equal-but-distinct *step.File, and
// that is exactly the mismatch this guards against. An Assembled built some
// other way carries no stamp and is let through unchecked — that caller
// assembled Result and Scene itself and already owns the pairing.
//
//	parent map  = IfcRelAggregates ∪ IfcRelContainedInSpatialStructure, FORWARD
//	              (iterate each rel's Related* → RelatingObject/Structure). NEVER
//	              per-node model.Container — Container(storey) self-parents (a storey
//	              is the RelatingStructure of its own containment rels).
//	order       = topo BFS from roots; a parent always precedes its child, so
//	              a consumer's parentID[*ParentIndex] lookup never misses.
//	geometry    = joined to physical nodes by GlobalID (the emitted order is NOT
//	              index-aligned with Scene.Elements once spatial nodes are interleaved).
func BuildImportFrom(f *step.File, a *Assembled) (*ImportModel, error) {
	if f == nil {
		return nil, fmt.Errorf("ifc: nil step file")
	}
	if a == nil || a.Result == nil || a.Scene == nil {
		return nil, fmt.Errorf("ifc: nil assembly")
	}
	if a.file != nil && a.file != f {
		return nil, fmt.Errorf("ifc: assembly was built from a different step file")
	}

	// Combined node set: spatial (with authored Qto) + physical (Qto tier-back-filled).
	spatial := model.SpatialNodes(f)
	all := make([]model.Element, 0, len(spatial)+len(a.Result.Elements))
	all = append(all, spatial...)
	all = append(all, a.Result.Elements...)

	inSet := make(map[int]bool, len(all))
	byID := make(map[int]model.Element, len(all))
	for _, e := range all {
		inSet[e.ExpressID] = true
		byID[e.ExpressID] = e
	}

	parentOf := forwardParentMap(f)
	ordered := topoOrder(all, parentOf, inSet)

	pos := make(map[int]int, len(ordered))
	for i, id := range ordered {
		pos[id] = i
	}

	// world-AABB by GlobalID (C4 — do NOT assume Scene index-alignment).
	aabb := make(map[string]geometry.Element, len(a.Scene.Elements))
	for _, ge := range a.Scene.Elements {
		aabb[ge.GlobalID] = ge
	}

	// Net-area reconciliation, keyed by GlobalID. Call ONCE per Scene (it
	// appends an orphan-fill warning to s.Warnings). Hosts without voids are
	// absent from the map; untrusted hosts carry a nil Net.
	nets := a.Scene.NetAreas(f, a.Result)

	// One TYPE-level resolution per distinct IfcTypeObject. model.GetType and the
	// layer set hanging off the type are occurrence-independent — every
	// occurrence would read the same answer — so `typeProbed` caches a MISS as
	// safely as a hit.
	//
	// The occurrence-level fallback is NOT occurrence-independent: only some
	// occurrences of a type may carry an IfcMaterialLayerSetUsage. So it re-runs
	// per occurrence until one yields layers. That costs a lookup in step.File's
	// precomputed inverse map, not a walk — far too cheap to be worth caching a
	// miss, which would let one unlucky occurrence declare the type empty.
	//
	// A resolved type with NO layers still gets an entry, empty. "Read it, it has
	// no build-up" and "never saw this type" are different facts: the first is a
	// build-up that shrank to nothing and must still be reconciled downstream,
	// the second is nothing at all. Dropping the empty entry made the 1->0 shrink
	// invisible — the type disappeared from the map, so its stale rows were never
	// listed, never counted, never retracted.
	typeLayers := make(map[string]TypeLayerSet)
	typeProbed := make(map[string]bool)

	nodes := make([]ImportNode, len(ordered))
	for i, id := range ordered {
		e := byID[id]
		var pi *int
		if pid, ok := parentOf[id]; ok && inSet[pid] {
			p := pos[pid]
			pi = &p
		}
		n := ImportNode{
			GlobalID:       e.GlobalID,
			ExpressID:      id,
			IFCClass:       e.IFCClass,
			Name:           e.Name,
			ParentIndex:    pi,
			Qto:            e.Qto,
			QuantitySource: e.QuantitySource,
			Material:       e.Material,
			IsExternal:     e.IsExternal,
		}
		if inst, ok := f.ByID(id); ok {
			n.TypeGlobalID, n.TypeName, n.TypeClass = model.TypeIdentity(f, inst)
			if gid := n.TypeGlobalID; gid != "" {
				// The type usually carries the bare IfcMaterialLayerSet; the
				// occurrence carries the IfcMaterialLayerSetUsage that names the
				// axis. Read the type first for the layers, then the occurrence
				// for what the type cannot supply.
				if !typeProbed[gid] {
					typeProbed[gid] = true
					if typ := model.GetType(f, inst); typ != nil {
						set := model.MaterialLayers(f, typ, a.Result.UnitScale)
						typeLayers[gid] = TypeLayerSet{
							Layers:    set.Layers,
							Direction: set.Direction,
							Sense:     set.Sense,
						}
					}
				}
				if cur, ok := typeLayers[gid]; ok {
					if len(cur.Layers) == 0 {
						// The type names no build-up. It may hang off this occurrence's
						// usage instead — and off a later occurrence if not this one.
						if usage := model.MaterialLayers(f, inst, a.Result.UnitScale); len(usage.Layers) > 0 {
							typeLayers[gid] = TypeLayerSet{
								Layers:    usage.Layers,
								Direction: usage.Direction,
								Sense:     usage.Sense,
							}
						}
					} else if cur.Direction == "" {
						if usage := model.MaterialLayers(f, inst, a.Result.UnitScale); usage.Direction != "" {
							cur.Direction, cur.Sense = usage.Direction, usage.Sense
							typeLayers[gid] = cur
						}
					}
				}
			}
		}
		if ge, ok := aabb[e.GlobalID]; ok && len(ge.Verts) > 0 {
			n.OriginMin = ge.BBoxMin
			n.BBoxMin = ge.BBoxMin
			n.BBoxMax = ge.BBoxMax
		}
		if na, ok := nets[e.GlobalID]; ok {
			n.NetArea = na.Net // already nil when untrusted
			if na.Net != nil {
				// Gated on Net rather than on Trusted directly, so the fields
				// cannot disagree: an untrusted host publishes none of them.
				p := na.OpeningPerimeter
				n.OpeningPerimeter = &p
				d := na.OpeningDeduction
				n.OpeningDeduction = &d
				g := na.Gross
				n.ProjectedGross = &g
			}
		}
		nodes[i] = n
	}

	storeyPlans := buildStoreyPlans(nodes, aabb, model.StoreyElevations(f))
	return &ImportModel{Nodes: nodes, Scene: a.Scene, StoreyPlans: storeyPlans, TypeLayers: typeLayers}, nil
}

// forwardParentMap builds child-ExpressID → parent-ExpressID from the two
// containment relations. It iterates the rels FORWARD (RelatingObject/Structure
// is the parent, Related* the children) so a spatial container is never
// resolved as its own parent.
//
//	IfcRelAggregates(..., RelatingObject=4, RelatedObjects=5)
//	IfcRelContainedInSpatialStructure(..., RelatedElements=4, RelatingStructure=5)
func forwardParentMap(f *step.File) map[int]int {
	parent := make(map[int]int)
	addChildren := func(rel *step.Instance, childAttr, parentAttr int) {
		p, ok := rel.Ref(parentAttr)
		if !ok {
			return
		}
		v, ok := rel.Get(childAttr)
		if !ok {
			return
		}
		switch v.Kind {
		case step.KindList:
			for _, c := range v.List {
				if c.Kind == step.KindRef && c.Ref != nil {
					parent[c.Ref.ID()] = p.ID()
				}
			}
		case step.KindRef:
			if v.Ref != nil {
				parent[v.Ref.ID()] = p.ID()
			}
		}
	}
	for _, rel := range f.ByType("IfcRelAggregates") {
		addChildren(rel, 5, 4) // children=RelatedObjects(5), parent=RelatingObject(4)
	}
	for _, rel := range f.ByType("IfcRelContainedInSpatialStructure") {
		addChildren(rel, 4, 5) // children=RelatedElements(4), parent=RelatingStructure(5)
	}
	return parent
}

// topoOrder returns ExpressIDs parents-first: BFS from roots (a node whose parent
// is absent from the set). Nodes left unvisited by a reference cycle are
// appended in input order so none are dropped.
func topoOrder(all []model.Element, parentOf map[int]int, inSet map[int]bool) []int {
	children := make(map[int][]int)
	var roots []int
	for _, e := range all {
		if pid, ok := parentOf[e.ExpressID]; ok && inSet[pid] {
			children[pid] = append(children[pid], e.ExpressID)
		} else {
			roots = append(roots, e.ExpressID)
		}
	}
	ordered := make([]int, 0, len(all))
	seen := make(map[int]bool, len(all))
	queue := append([]int(nil), roots...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, id)
		queue = append(queue, children[id]...)
	}
	for _, e := range all {
		if !seen[e.ExpressID] {
			seen[e.ExpressID] = true
			ordered = append(ordered, e.ExpressID)
		}
	}
	return ordered
}
