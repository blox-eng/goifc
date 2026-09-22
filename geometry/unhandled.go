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
var presentationItemTypes = []string{
	"IfcStyledItem",
	"IfcAnnotationFillArea",
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
func UnhandledItemTypes(f *step.File, r *model.Result) map[string]int {
	out := make(map[string]int)
	for i := range r.Elements {
		for _, item := range representationItems(f, r.Elements[i].ExpressID) {
			if item == nil || handled(item) || presentation(item) {
				continue
			}
			out[item.Type()]++
		}
	}
	return out
}
