// Package geometry builds proxy geometry (a view of the semantic model, no CAD
// kernel) from a parsed IFC step.File plus the model package's model.Result,
// and emits a single Y-up GLB whose node names are element GlobalIds.
//
// COORDINATE FRAMES — meshes are local, placements and bounding boxes are
// world. [Element.Verts] are element-local meters; [Element.Placement] maps
// them into world space; [Element.BBoxMin] and [Element.BBoxMax] are already
// world. Deriving a direction from Verts and a position from the BBox mixes
// the two frames and is wrong without erroring — use [Element.WorldVerts] for
// positions and [Element.WorldNormal] for directions.
package geometry
