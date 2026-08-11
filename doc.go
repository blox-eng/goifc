// Package ifc is the top-level entry to the common/ifc Go-native engine. It
// wires the per-stage packages — step (STEP/EXPRESS parse) -> model (semantic
// extraction) -> geometry (proxy tessellation + derived quantities) — into a
// single [Assemble] call that turns a parsed STEP file into quantity-back-filled
// semantic elements plus their proxy geometry:
//
//	f, _ := step.ParseBytes(src)
//	a, _ := ifc.Assemble(f)
//	for i := range a.Result.Elements {
//		e := a.Result.Elements[i] // e.Qto, e.QuantitySource ("qto"|"geometry"|"none")
//	}
//	a.Scene.WriteGLB(w) // proxy geometry for a viewer
//
// KNOWN LIMITATION: the geometry-derived Volume tier is GROSS. For walls with
// openings the extrude path reports the SOLID (un-subtracted) volume, so those
// elements over-report versus ifcopenshell's NET figure. The
// quantity_source="geometry" tag already flags these as bounding estimates, so
// no consumer mistakes one for an authored net Qto — netting openings out of
// the extrude volume is deliberately out of scope here.
package ifc
