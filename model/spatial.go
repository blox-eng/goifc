package model

import "github.com/blox-eng/common/ifc/step"

// spatialClasses is the set of spatial-container classes emitted as import-tree
// nodes, matching import_emit.py's _SPATIAL. IfcProject is intentionally
// excluded (it is the file root, not a placed container in the object tree).
var spatialClasses = []string{"IfcSite", "IfcBuilding", "IfcBuildingStorey", "IfcSpace"}

// SpatialNodes returns the spatial-container elements (Site/Building/Storey/
// Space) as semantic Elements, in deterministic id order. model.Extract omits
// these — it is physical-only, to match the #2211 parity oracle
// (semantic_oracle.py's by_type("IfcElement")) — so the #2213 cutover recovers
// them here to reproduce the Python import contract's site→storey→element tree.
//
// These carry identity, name, and AUTHORED (tier-1) Qto — import_emit.py runs
// _qto_quantities for spatial nodes too, so an IfcSpace/IfcBuildingStorey with
// Qto_SpaceBaseQuantities.GrossFloorArea emits real dimensions (contract parity).
// Spatial containers have no proxy geometry (their AABB is zero), so no geometry
// tier is applied; ParentIndex is assigned by the import-assembly layer.
func SpatialNodes(f *step.File) []Element {
	scale := UnitScale(f)
	var insts []*step.Instance
	seen := map[int]bool{}
	for _, cls := range spatialClasses {
		for _, e := range f.ByType(cls) {
			if seen[e.ID()] {
				continue
			}
			seen[e.ID()] = true
			insts = append(insts, e)
		}
	}
	sortByID(insts)

	nodes := make([]Element, 0, len(insts))
	for _, e := range insts {
		q, hasQto := QtoQuantities(f, e, scale)
		src := QuantitySourceQto
		if !hasQto {
			q = Quantities{}
			src = QuantitySourceNone
		}
		nodes = append(nodes, Element{
			GlobalID:       strVal(e, attrGlobalID),
			ExpressID:      e.ID(),
			IFCClass:       e.Type(),
			Name:           strVal(e, attrName),
			Qto:            q,
			QuantitySource: src,
			Placement:      Identity(),
		})
	}
	return nodes
}
