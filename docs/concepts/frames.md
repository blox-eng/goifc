# Local and world frames

A `geometry.Element` carries data in **two different coordinate frames**, and
nothing in the type system stops you from mixing them. This page exists because
mixing them is wrong *without erroring* — the code runs, the numbers look
plausible, and the answer is silently rotated or offset.

| Field / method            | Frame | Units |
|---------------------------|-------|-------|
| `Element.Verts`           | element-local | metres |
| `Element.Placement`       | local → world transform | — |
| `Element.BBoxMin` / `BBoxMax` | **world** | metres |
| `Element.WorldVerts()`    | **world** | metres |
| `Element.WorldNormal(v)`  | **world** | direction |

Meshes are local. Placements and bounding boxes are world.

## The trap

```go
// WRONG — mixes frames.
dir := sub(e.Verts[3:6], e.Verts[0:3]) // local direction
pos := midpoint(e.BBoxMin, e.BBoxMax)  // world position
```

Deriving a direction from `Verts` and a position from the `BBox` mixes the two
frames. There is no error, no panic, and no NaN — just a direction expressed in
the element's own rotated frame paired with a position in the building's.

## The rule

Use `WorldVerts` for positions and `WorldNormal` for directions.

```go
world := e.WorldVerts()                       // []float32, world X,Y,Z triples
up := e.WorldNormal([3]float64{0, 0, 1})      // local +Z as a world direction
```

## Why directions need their own call

`WorldNormal` applies only the 3×3 rotation part of the placement. A direction
must not pick up the placement's translation — a face normal does not move when
the wall it belongs to moves.

Magnitude is preserved, not normalised: a placement composed from
`IfcAxis2Placement3D` is orthonormal and right-handed, so a unit local direction
comes back unit-length and a scaled one comes back scaled by the same factor.
That orthonormality is also why rotating the direction is the correct transform
and no inverse-transpose is needed.

Do **not** derive a direction by differencing two `WorldVerts`. It gives the
right answer for a rigid placement, but it rounds through `float32` twice and
loses precision that `WorldNormal` keeps in `float64`.

## Precision

`WorldVerts` runs the transform in `float64` and rounds to `float32` on the way
out, so its result agrees with `BBoxMin`/`BBoxMax` to `float32` precision — not
exactly. That gap grows with distance from the origin, which matters on a
georeferenced model whose coordinates are in the hundreds of thousands of metres.
Code needing exact world positions there should transform through `Placement` in
`float64` itself.

`WorldVerts` also allocates a fresh slice on every call. Hoist it out of a hot
loop.

## Edge cases

`Verts` holding fewer than 3 floats returns `nil`. A trailing partial triple is
dropped rather than emitted half-transformed.

!!! note "GLB output is a third frame"
    `Scene.WriteGLB` writes **Y-up** GLB, because that is what glTF viewers
    expect. Everything described on this page is IFC-native **Z-up**. The
    conversion happens inside `WriteGLB` and does not affect any field above.
