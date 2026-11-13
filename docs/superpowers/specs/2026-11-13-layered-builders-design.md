# Layered builders: walls, room walls and slabs in layers

Status: draft for approval, 2026-11-13.

## Intent

A drafted wall is rarely one material. It is plaster, brick, insulation and
render, in that order from the room. Each layer is priced on its own: plaster
by the m² of its face, brick by the m³, render by the m² of the outside face.
An app that drafts walls from numbers needs one mesh and one set of quantities
per layer, and the layers must add up to the wall.

v0.16.0 builds a wall, a room of walls and a slab as one solid. This design
adds a layered form of each builder. The single-solid builders stay as they
are.

## Goals

- `BuildWallLayers`, `BuildRoomWallLayers` and `BuildSlabLayers` return one
  `Wall` or `Slab` per layer, in the order given: from the measured face for
  walls, from the top for slabs.
- The element's thickness is the sum of its layers.
- Each layer's quantities are exact and come from the numbers, like v0.16.0.
- In a room, each layer mitres at its own corner, so each layer reports its own
  real length and area (maintainer, 2026-11-13, option A).
- Every through opening cuts every layer. A niche cuts the layers from the
  measured face to its depth.
- A one-layer call gives the same result as the single-solid builder.
- Hostile numbers never panic. The work is bounded.
- A semver minor release (v0.17.0) with a CHANGELOG entry.
- README and package docs state the unit: SI metres everywhere (maintainer,
  2026-11-13: "goifc can use something standard, and put it in the readme").

## Non-goals

- Layer names, materials or roles. The caller keeps them; the result is in the
  caller's order, so the index is the key.
- Layers of different heights, or a layer that stops short of a corner.
- T-joins (next design; the strip a partition covers is subtracted).
- Openings in slabs.
- A unit setting. goifc is SI only. Metric or imperial is the host's workspace
  setting (maintainer, 2026-11-13); the host converts at input and display.
- Checking the order of layers (structure inside finish, and so on). goifc
  builds what it is given.

## Anti-goals

- Layer quantities that do not add up to the wall. The sum of the layers'
  `NetVolume` must equal the single-solid wall's `NetVolume`.
- An opening that is refused in a layered wall but placed in the same
  single-solid wall, or the reverse.
- Any private or downstream data in the repository. Fixtures are synthetic.

## Approaches

**A. Each layer mitres at its own corner (chosen, 2026-11-13).** Layer
k lies between depths d_k and d_k+1 from the measured line (d_0 = 0). Its two
faces end where they meet the neighbouring wall's faces at the same depths. An
outer render layer is longer than the inner plaster at an outside corner. The
areas are the real areas, which matches the rule for T-joins: measure what is
there.

**B. Every layer reports the measured face's length (rejected).** Simpler, but
it under-counts outer layers at outside corners and over-counts them at inside
corners. A priced quantity that is silently wrong is an anti-goal.

How to build A (the builder's call, with a recommendation): generalise the
existing span and mesh code so a span is computed at any depth, not only at 0
and the full thickness. Do not build layer k as `BuildRoomWalls` on an offset
outline: that would check openings again in a shifted u, and a layer could
refuse what the whole wall places.

## Design

### API

```go
// layers are thicknesses in metres, measured face first.
func BuildWallLayers(edge [2][2]float64, base, height float64, layers []float64,
    side Side, openings []Opening) ([]Wall, error)

// The result is indexed [wall][layer].
func BuildRoomWallLayers(outline [][2]float64, base, height float64, layers []float64,
    side Side, openings [][]Opening) ([][]Wall, error)

// layers are thicknesses in metres, top first.
func BuildSlabLayers(outline [][2]float64, z float64, layers []float64) ([]Slab, error)
```

Layers are plain thicknesses (`[]float64`), not a `Layer{Name, Thickness}`
struct. goifc would not read the name, and the index already keys the result.
This differs from the shape agreed with the consumer on 2026-11-12; it is
told before code lands.

### Input checks

- 1 to 16 layers (`maxLayers`). 0 layers is an input error, not "no wall".
- Each thickness passes `checkDim` (finite, 1 mm to 1,000 m). The error names
  the layer: `layer 2: thickness 0.0004 m is under the 1 mm minimum`.
- The sum passes `checkDim` as `thickness`.
- Everything else is checked as in v0.16.0, with the sum as the thickness.

### Openings are checked once, on the whole wall

Placement checks run one time, on the whole wall with thickness = the sum.
The refusals, their order and their indices (`Wall`, `Opening`) are exactly
those of `BuildWall` / `BuildRoomWalls` with that thickness. Layers add no
placement rule except the one below.

An opening keeps its place in the world in every layer. In layer k's faces the
offset is re-expressed from that face's own start.

### Niches across layers

A niche of depth D:

- cuts through every layer that ends at or before D (d_k+1 ≤ D);
- is a niche of depth D − d_k in the layer where D falls (d_k < D < d_k+1);
  that layer reports the `NicheBack`;
- leaves the layers after it whole.

A niche whose back lies on a boundary (D = d_k+1) cuts layer k through, and
layer k+1 reports the back as its `NicheBack` (its `MeasuredNet` loses that
area). So Σ `NicheBack` over the layers always equals the whole wall's
(builder check, my call: a niche finish priced from `NicheBack` must not drop
to zero when the depth is typed as a sum of layers).

Each layer's `Measured.Openings` lists an opening as it cuts that layer:
`Depth 0` where it cuts through, `Depth D − d_k` in the layer where the niche
ends, and not at all in the layers after (my call: a consumer draws each
layer's cut from its own face).

A niche whose back lies closer than 1 mm to a layer boundary, but not on it,
would leave a layer sliver under 1 mm. It is refused with a new
`PlacementReason`, `layer_sliver`. "On it" means within 1e-9 m, so a niche
depth typed as the sum of the first layers cuts those layers through.

### Wall quantities per layer

Each layer is a full `Wall`. Its `WallQuantities` mean the same as v0.16.0, for
that layer alone:

- `Length`: the layer's measured-side face, along u.
- `MeasuredGross`, `MeasuredNet`: the face at depth d_k (toward the measured
  side).
- `OtherGross`, `OtherNet`: the face at depth d_k+1.
- `GrossVolume`, `NetVolume`: the layer's own.
- `RevealPerimeter`: the edges of the openings that cut this layer. It repeats
  in each layer an opening cuts and is not additive across layers; the
  layer's reveal area is `RevealPerimeter × thickness` for through openings
  (my call: same meaning as v0.16.0, per layer).
- `NicheBack`: the backs of the niches that end in this layer.

`Wall.Measured` is the layer's face at depth d_k: Origin at its own start,
Normal toward the measured side, Outline `[0, Length] × [0, Height]`.

Invariants (each one a test):

- layer 0's `MeasuredNet` = the whole wall's `MeasuredNet`;
- the last layer's `OtherNet` = the whole wall's `OtherNet`;
- Σ `NetVolume` over the layers = the whole wall's `NetVolume`;
- layer k's `OtherGross` = layer k+1's `MeasuredGross` (a shared face, reported
  once by each layer);
- Σ `NicheBack` over the layers = the whole wall's `NicheBack`;
- for a straight `BuildWallLayers` wall every layer has the same `Length`.

### Slabs

Layer k's top lies at z − d_k. Each layer is a full `Slab`: the same `Area` and
`Perimeter`, its own `Volume`, its own `Top` face. Σ `Volume` = the whole
slab's.

### Bounds

The mesh cell cap (`maxMeshCells`) applies to the sum over all layers, so a
16-layer wall cannot build 16 times the mesh of the cap. The existing work
budget is unchanged.

### Units in the docs

The README and `geometry/doc.go` say: goifc works in SI metres everywhere,
including builder input and output, quantities (m, m², m³) and GLB. IFC files in
other units are converted to metres when they are read.

## Tests

- An oracle table of hand-computed layer quantities for a straight wall, a
  4 × 4 m room (interior and exterior side) and a slab.
- One layer gives the same `Wall`, `[]Wall` or `Slab` as the single-solid
  builder (the same quantities, verts and tris).
- The invariants above, on the oracle cases and in a fuzz target that extends
  the existing builder fuzz.
- A niche ending inside a layer, on a boundary and within 1 mm of one.
- Refusals are the same as the single-solid builder with the summed thickness.
- The mesh volume of each layer equals its `NetVolume` (float32 tolerance).
- The wasm CI build stays green.

## Left to the builder

- The internal shape of the span generalisation.
- Test file layout.
- The exact error text, within the "name the layer" rule.
- `layer_sliver` comes last in the refusal order; a slab layer's `Element`
  origin is at z − d_k; tolerances for the sum checks.

Builder check (2026-11-13): placement holds in every layer, because the mitre
point is linear in depth, so an opening the whole wall accepts lies inside
every layer's span. Per-layer mitres need no new placement rule.
