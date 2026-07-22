// Package geometry builds proxy geometry (a view of the semantic model, no CAD
// kernel) from a parsed IFC step.File plus child-2's model.Result, and emits a
// single Y-up GLB whose node names are element GlobalIds. See
// docs/superpowers/specs/2026-07-22-ifc-proxy-geometry-design.md.
package geometry
