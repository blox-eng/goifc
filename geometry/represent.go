package geometry

import "github.com/blox-eng/goifc/step"

// Positional attribute indices (0-based) along the representation chain:
// IfcProduct.Representation, IfcProductDefinitionShape.Representations,
// IfcShapeRepresentation.RepresentationIdentifier and .Items. Stable across
// IFC2X3 and IFC4 — model/ifcattr.go applies the same convention to the
// semantic entities.
const (
	attrProductRepresentation = 6
	attrShapeRepresentations  = 2
	attrRepresentationIdent   = 1
	attrRepresentationItems   = 3
)

// representationItems returns every representation item from the product's
// 'Body' IfcShapeRepresentation(s) — the solid/surface geometry. It deliberately
// excludes sibling representations like 'Axis' (a 2D centerline curve) and 'Box'
// (a coarse IfcBoundingBox placeholder): those live in the SAME element-local
// frame as Body and, if tessellated in, silently stretch the element's mesh/AABB
// to include a line or placeholder box that isn't the real geometry (e.g. an
// Axis curve running past the wall's actual clipped end). Falls back to ALL
// representations only when no 'Body' rep exists, so unusual exports still
// render something. Mapped items are returned as-is (IfcMappedItem instances)
// and resolved by the caller.
func representationItems(f *step.File, expressID int) []*step.Instance {
	prod, ok := f.ByID(expressID)
	if !ok {
		return nil
	}
	shape, ok := prod.Ref(attrProductRepresentation)
	if !ok {
		return nil
	}
	repsV, ok := shape.Get(attrShapeRepresentations)
	if !ok || repsV.Kind != step.KindList {
		return nil
	}
	var body, all []*step.Instance
	for _, rv := range repsV.List {
		if rv.Kind != step.KindRef || rv.Ref == nil || !rv.Ref.IsA("IfcShapeRepresentation") {
			continue
		}
		itemsV, ok := rv.Ref.Get(attrRepresentationItems)
		if !ok || itemsV.Kind != step.KindList {
			continue
		}
		var items []*step.Instance
		for _, iv := range itemsV.List {
			if iv.Kind == step.KindRef && iv.Ref != nil {
				items = append(items, iv.Ref)
			}
		}
		all = append(all, items...)
		if identV, ok := rv.Ref.Get(attrRepresentationIdent); ok && identV.Kind == step.KindString && identV.Str == "Body" {
			body = append(body, items...)
		}
	}
	if len(body) > 0 {
		return body
	}
	return all
}
