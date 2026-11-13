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
//
// UNITS — goifc works in SI metres everywhere: builder input and output,
// quantities (m, m², m³) and GLB. IFC files in other units are converted to
// metres when they are read. A host that shows imperial units converts at its
// own input and display.
//
// BUILDERS — [BuildWall], [BuildRoomWalls] and [BuildSlab] make an Element
// from numbers instead of an IFC file. Their quantities are exact, from the
// numbers. Their mesh is float32 like every Element's and agrees with the
// quantities within that rounding, so [Scene.DerivedQuantities] on a built
// Element gives the same figures. The builders leave GlobalID empty, and
// DerivedQuantities skips such an element: set a unique GlobalID first. A
// wall read from IFC can differ: its derived volume is gross unless the
// file's own mesh cuts the openings. [BuildWallLayers], [BuildRoomWallLayers]
// and [BuildSlabLayers] build the same elements in layers, one Element per
// layer, with quantities per layer.
package geometry
