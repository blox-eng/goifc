package geometry

import (
	"github.com/blox-eng/goifc/model"
	"github.com/blox-eng/goifc/step"
)

// handledItemTypes lists every IFC representation-item type that
// tessellateItemDepth has a real path for. It MUST stay in lockstep with that
// switch (geometry/geometry.go); TestHandledItemTypesMatchesDispatch pins the
// two together by reading the source, so adding a case there without adding it
// here fails the build.
var handledItemTypes = []string{
	"IfcMappedItem",
	"IfcExtrudedAreaSolid",
	"IfcFacetedBrep",
	"IfcClosedShell",
	"IfcConnectedFaceSet",
	"IfcOpenShell",
	"IfcShellBasedSurfaceModel",
	"IfcFaceBasedSurfaceModel",
	"IfcTriangulatedFaceSet",
	"IfcTriangulatedIrregularNetwork",
	"IfcPolygonalFaceSet",
	"IfcBooleanClippingResult",
	"IfcBooleanResult",
}

// presentationItemTypes carry appearance, not shape. Some exporters place them
// directly in IfcShapeRepresentation.Items (duplex_a.ifc has 267 IfcStyledItem
// entities), so counting them as unhandled GEOMETRY would rank a colour
// assignment above real missing solids in the very list this diagnostic exists
// to produce.
//
// Without an EXPRESS schema there is no subtype test for
// IfcGeometricRepresentationItem (see the limitations page), so the exclusion is
// an explicit list. Add to it only for entities that genuinely carry no shape.
// IfcAnnotationFillArea is deliberately NOT here: it is an
// IfcGeometricRepresentationItem carrying OuterBoundary and InnerBoundaries
// curves, so an element that reaches one and has no other geometry falls back
// to a box, and this diagnostic must say so.
var presentationItemTypes = []string{
	"IfcStyledItem",
	"IfcPresentationLayerAssignment",
	"IfcPresentationStyleAssignment",
}

// handled reports whether any tessellation path claims this item.
func handled(item *step.Instance) bool {
	for _, t := range handledItemTypes {
		if item.IsA(t) {
			return true
		}
	}
	return false
}

// presentation reports whether this item carries appearance rather than shape,
// and so is not a geometry gap.
func presentation(item *step.Instance) bool {
	for _, t := range presentationItemTypes {
		if item.IsA(t) {
			return true
		}
	}
	return false
}

// UnhandledItemTypes counts, per IFC entity type, the representation items in f
// that no tessellation path handles and which therefore degrade to a bounding
// box (see [Element.Source] and [Scene.Stats]).
//
// It answers "why did this element come back as a box", and it ranks which
// geometry gaps are worth closing: a type with a high count costs accuracy on
// real files, whatever the IFC schema says about how common it ought to be.
//
// It does NOT see every cause of a fallback, and a caller ranking gaps from it
// needs to know what it is blind to. It reports types for which
// tessellateItemDepth has no dispatch case at all, looking THROUGH the two
// wrappers that would otherwise hide one: an IfcMappedItem is resolved to the
// items it maps, and a boolean is resolved to its operands (the wrapper is
// dispatched, so without that the whole operand subtree vanished from the
// ranking — see countUnhandledBoolean).
//
// What remains invisible is a dispatch case that IS entered and declines
// partway — a profile that cannot be built, a shell that cannot be closed, an
// extrusion with no direction — after which tessellateItemDepth falls through
// to obbFromItem just the same. Those elements become boxes while their type is
// absent from this map, so the counts here are a lower bound on the causes, and
// a type's absence is not evidence that it never falls back. Reporting declined
// attempts would mean instrumenting the dispatch itself; until that exists,
// read a model with fallback elements this map explains nothing about as
// exactly that — unattributed. The two figures are not commensurable in any
// case: this map counts representation items, [Scene.Stats] counts elements.
//
// Three things about the count that are easy to misread:
//
//  1. It counts OCCURRENCES, not distinct entities. An IfcMappedItem is
//     resolved through its MappingSource/MappedRepresentation (mirroring
//     mappedItemMesh in mapped.go) down to the real items it stands for, and
//     each resolution is counted again — once per element that reaches it.
//     Geometry shared by thirty elements (a Revit furniture/door/window
//     family placed thirty times via one IfcRepresentationMap) costs accuracy
//     thirty times over, so it must rank thirty times, not once. This means
//     a type's reported count can legitimately EXCEED the number of entities
//     of that type in the file — that is correct, not a bug. Recursion is
//     bounded by maxMapDepth, the same guard tessellateItemDepth uses, so a
//     cyclic or pathologically-nested mapping chain terminates instead of
//     hanging.
//  2. It only sees physical elements. UnhandledItemTypes walks r.Elements,
//     which model.Extract populates from emitOrRenderClasses
//     (model/extract.go) — deliberately excludes IfcSpace and other
//     non-IfcElement products. Geometry that hangs only off an IfcSpace is
//     out of scope for this diagnostic by the same design choice, not a gap
//     in it.
//  3. Map keys are upper-cased STEP keywords (e.g. "IFCFACEBASEDSURFACEMODEL"),
//     not the mixed-case spelling used in prose or in handledItemTypes/
//     presentationItemTypes. step.Instance.Type() — like STEP/SPF keyword
//     syntax itself — is always upper-cased; there is no mixed-case original
//     to recover from the file.
func UnhandledItemTypes(f *step.File, r *model.Result) map[string]int {
	out := make(map[string]int)
	for i := range r.Elements {
		for _, item := range representationItems(f, r.Elements[i].ExpressID) {
			countUnhandledItem(item, 0, out)
		}
	}
	return out
}

// countUnhandledItem classifies one representation item, resolving through
// IfcMappedItem to the items its mapped representation actually contains
// (mirroring mappedItemMesh, geometry/mapped.go) instead of stopping at the
// wrapper. depth bounds that resolution the same way tessellateItemDepth
// bounds tessellation, via maxMapDepth: a cyclic MappingSource chain
// terminates rather than recursing forever.
func countUnhandledItem(item *step.Instance, depth int, out map[string]int) {
	if item == nil {
		return
	}
	if item.IsA("IfcMappedItem") {
		if depth >= maxMapDepth {
			return
		}
		repMap, ok := item.Ref(attrMapSource)
		if !ok {
			return
		}
		mappedRep, ok := repMap.Ref(attrMappedRep)
		if !ok {
			return
		}
		itemsV, ok := mappedRep.Get(attrRepresentationItems)
		if !ok || itemsV.Kind != step.KindList {
			return
		}
		for _, iv := range itemsV.List {
			if iv.Kind != step.KindRef || iv.Ref == nil {
				continue
			}
			countUnhandledItem(iv.Ref, depth+1, out)
		}
		return
	}
	if item.IsA("IfcBooleanClippingResult") || item.IsA("IfcBooleanResult") {
		countUnhandledBoolean(item, depth, out)
		return
	}
	if handled(item) || presentation(item) {
		return
	}
	out[item.Type()]++
}

// countUnhandledBoolean attributes a boolean the way clipMeshByDifference
// actually accepts or rejects one (geometry/clip.go). The wrapper is in
// handledItemTypes, so without this the whole operand subtree vanished from the
// ranking: a boolean over an operand with NO dispatch case at all -- the exact
// thing this map exists to name -- produced an OBB element and no count.
func countUnhandledBoolean(item *step.Instance, depth int, out map[string]int) {
	if depth >= maxMapDepth {
		// clipMeshByDifference declines here too, and the cause is the depth
		// budget rather than any nameable type. Counting one would misattribute.
		return
	}
	// Only DIFFERENCE is supported. UNION and INTERSECTION are unsupported
	// operations, not unsupported operands, so the boolean itself is the gap.
	opV, ok := item.Get(attrBoolOperator)
	if !ok || opV.Kind != step.KindEnum || opV.Str != "DIFFERENCE" {
		out[item.Type()]++
		return
	}
	first, ok := item.Ref(attrBoolFirstOperand)
	if !ok {
		out[item.Type()]++
		return
	}
	countUnhandledItem(first, depth+1, out)

	second, ok := item.Ref(attrBoolSecondOperand)
	if !ok {
		out[item.Type()]++
		return
	}
	// An IfcHalfSpaceSolid second operand is precisely what the supported path
	// consumes, via halfSpacePlane rather than tessellateItemDepth, so it is
	// not a gap and must not be counted. Anything else makes
	// clipMeshByDifference decline, and that operand IS the gap.
	if !isHalfSpaceSolid(second) {
		countUnhandledItem(second, depth+1, out)
	}
}
