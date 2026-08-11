package model

import "github.com/blox-eng/goifc/step"

// spatialClasses is the set of spatial-container classes emitted as import-tree
// nodes, matching import_emit.py's _SPATIAL. IfcProject is intentionally
// excluded (it is the file root, not a placed container in the object tree).
var spatialClasses = []string{"IfcSite", "IfcBuilding", "IfcBuildingStorey", "IfcSpace"}

// SpatialNodes returns the spatial-container elements (Site/Building/Storey/
// Space) as semantic Elements, in deterministic id order. model.Extract omits
// these — it is physical-only, to match the parity oracle
// (semantic_oracle.py's by_type("IfcElement")) — so SpatialNodes recovers
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

// StoreyElevations maps each IfcBuildingStorey GlobalID to its Elevation in
// meters (raw attr 9 × UnitScale). Storeys with no numeric Elevation are
// omitted. Used for ordering storey plans in the UI; NOT for cut-plane
// placement (that is min-world-Z + 1.2 m in the plan assembler).
func StoreyElevations(f *step.File) map[string]float64 {
	scale := UnitScale(f)
	const attrElevation = 9 // IfcBuildingStorey.Elevation
	out := map[string]float64{}
	for _, s := range f.ByType("IfcBuildingStorey") {
		gid := strVal(s, attrGlobalID)
		if gid == "" {
			continue
		}
		v, ok := s.Get(attrElevation)
		if !ok {
			continue
		}
		var raw float64
		switch v.Kind {
		case step.KindFloat:
			raw = v.F
		case step.KindInt:
			raw = float64(v.I)
		default:
			continue // unset ($) / non-numeric → omit
		}
		out[gid] = raw * scale
	}
	return out
}
