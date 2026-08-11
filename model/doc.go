// Package model walks a parsed STEP/IFC entity graph (see the step package) into a
// canonical semantic []Element. It ports ifcopenshell's util.element,
// util.placement and util.unit into Go: schema-agnostic functions over
// *step.Instance that return native Go values, assembled into Element records.
// No geometry/tessellation lives here — that is the geometry package.
package model
