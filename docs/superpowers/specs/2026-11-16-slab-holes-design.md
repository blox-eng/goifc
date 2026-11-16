# Slabs with holes (#162)

Status: approved 2026-11-16 (a small change: this short design).

## Intent

A stair, a shaft or a chimney passes through a floor. Today a slab built
from numbers takes one outline and no holes, so its area and volume
over-report by every hole and a renderer draws a floor across the
stairwell. #162 asks for slabs with stated holes.

## Goals and non-goals

- Goal: a slab with holes, its exact quantities and a closed mesh, from Go,
  `BuildPlan` and the wasm build, giving the same numbers.
- Non-goals: a hole that cuts the outline's edge (a notch is an outline
  change); the 2D polygon difference (#99).
- Anti-goal: a hole accepted silently when it does not fit.

## Approaches

1. **Stated holes, hole-aware triangulation (picked).** Splice each hole
   into the outer ring with a bridge and ear-clip as today.
2. Boolean difference (#99): not needed when the holes are given, and far
   bigger.
3. Holes in the wasm adapter only: breaks "Go and the browser compute the
   same numbers".

## Design

- `BuildSlabPolygon(p Polygon2D, z, thickness float64) (Slab, error)` and
  `BuildSlabPolygonLayers(p Polygon2D, z float64, layers []float64) ([]Slab, error)`.
  A polygon with no holes builds exactly what `BuildSlab` /
  `BuildSlabLayers` build. Those keep their signatures and results.
- Mesh: a closed shell. Top and bottom caps are triangulated with the
  holes; each hole gets side faces facing into the hole.
- Quantities: `Area` = outer area − holes; `Volume` = `Area` × thickness;
  `Perimeter` stays the outer ring; new `SlabQuantities.HolePerimeter`
  sums the hole rings.
- `Face` gains `Holes [][][2]float64` (in the face's own u, v like
  `Outline`). A slab's `Top.Outline` stays the outer ring and `Top.Holes`
  lists the holes; walls leave it empty.
- Refusals, as a `*HoleError{Hole int; Reason HoleReason}` (joined with
  `errors.Join`, all at once): `outside` (a hole point outside the outer
  ring, or on it within 1 mm), `overlaps` (two holes overlap or touch,
  within 1 mm; on the later hole), `self_crossing` (a hole that crosses
  itself). A hole with under 3 points or a dimension under 1 mm is an
  input error. More than 64 holes, or more than 2,048 points in total
  (today's outline cap), is too large.
- `PlanSlab.Holes [][][2]float64`; wasm slab items take `"holes"` (a list of
  point lists); answers gain `holePerimeter` and `top.holes`; hole
  refusals map to codes `hole_outside`, `holes_overlap`,
  `hole_self_crossing` with `item` and `hole` (its index). Schema and the
  wasm guide document them.
- The SVG preview shows the holes (it draws from the mesh; a test pins it).
- The builders guide gets a "Slabs with holes" section; CHANGELOG headline
  `- **Build slabs with holes (a stair or shaft through a floor).**`.

## Tests (synthetic)

From #162: a 10 × 8 m slab with a 2.5 × 1.2 m hole: area 77 m², hole
perimeter 7.4 m, mesh volume = quantity within float32 rounding; a hole
outside → refused naming hole 0; two touching holes → refused; three
layers share the holes and their volumes sum; identical results when
translated to national-grid coordinates. Plus: no holes == `BuildSlab`;
a concave outline with two holes; the caps; `BuildPlan` and wasm goldens;
a fuzz target.

## Left to the builder

The bridging algorithm's details, message strings, test names.
