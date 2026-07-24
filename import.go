package ifc

import (
	"github.com/blox-eng/common/ifc/geometry"
	"github.com/blox-eng/common/ifc/model"
	"github.com/blox-eng/common/ifc/step"
)

// ImportNode is one node of the flow import contract (import_emit.py): spatial
// containers + physical elements in a single parents-first tree. It adapts to
// flow's types.IFCElement downstream — GlobalID/IFCClass/Name/ParentIndex map
// directly; Qto → dimensions; OriginMin → transform.origin; BBox{Min,Max} →
// bounding_box. Spatial nodes have no geometry, so their AABB stays zero.
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
}

// ImportModel is the assembled import contract: the parents-first node tree plus
// the proxy-geometry Scene the physical nodes' meshes are baked from (Scene.WriteGLB).
type ImportModel struct {
	Nodes []ImportNode
	Scene *geometry.Scene
}

// BuildImport turns a parsed STEP file into the flow import contract, reproducing
// import_emit.py: spatial containers (SpatialNodes) + physical elements (Assemble)
// in ONE parents-first ordered tree.
//
//	parent map  = IfcRelAggregates ∪ IfcRelContainedInSpatialStructure, FORWARD
//	              (iterate each rel's Related* → RelatingObject/Structure). NEVER
//	              per-node model.Container — Container(storey) self-parents (a storey
//	              is the RelatingStructure of its own containment rels).
//	order       = topo BFS from roots; a parent always precedes its child, so the
//	              flow workflow's createdIDs[*ParentIndex] lookup never misses.
//	geometry    = joined to physical nodes by GlobalID (the emitted order is NOT
//	              index-aligned with Scene.Elements once spatial nodes are interleaved).
func BuildImport(f *step.File) (*ImportModel, error) {
	a, err := Assemble(f)
	if err != nil {
		return nil, err
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
		}
		if ge, ok := aabb[e.GlobalID]; ok && len(ge.Verts) > 0 {
			n.OriginMin = ge.BBoxMin
			n.BBoxMin = ge.BBoxMin
			n.BBoxMax = ge.BBoxMax
		}
		nodes[i] = n
	}

	return &ImportModel{Nodes: nodes, Scene: a.Scene}, nil
}

// forwardParentMap builds child-ExpressID → parent-ExpressID from the two
// containment relations, matching import_emit._parent_map. It iterates the rels
// FORWARD (RelatingObject/Structure is the parent, Related* the children) so a
// spatial container is never resolved as its own parent.
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
// is absent from the set), matching import_emit._topo_order. Nodes left unvisited
// by a reference cycle are appended in input order so none are dropped.
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
