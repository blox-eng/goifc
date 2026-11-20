# Pitched roofs in layers (#161)

Status: approved 2026-11-20.

## Intent

goifc builds walls and slabs from numbers, with no IFC file. It cannot
build a pitched roof, so an editor that sketches a house from numbers can
only put a flat slab on top, and that house does not read as a house. A
roofer prices from slope area and from the ridge, hip, valley, eaves and
verge lengths. Today nothing measures them.

Real outlines are not rectangles. The L outline
`(0,0) (18,0) (18,6) (12,6) (12,10) (0,10)` needs a valley, and its wing
may carry its own gable (a cross-gable).

## Goals, non-goals, anti-goals

- Goal: a layered pitched roof over any simple outline, with a closed
  mesh per layer, one `Face` per sloped plane and exact quantities, from
  Go, `BuildPlan` and the wasm build, giving the same numbers.
- Non-goals: dormers, skylights, gutters, a different pitch per edge, the
  gable-end wall infill under a gable (a wall with a sloped top is its
  own ask), holes in a roof, and IFC export of a roof.
- Anti-goal: a roof shape goifc chose. The caller states the pitch and
  every gable end. goifc never picks a ridge direction or a low edge.
- Anti-goal: a roof built silently wrong. An input the skeleton cannot
  build within the budget is refused, never approximated.

## Decisions (2026-11-20)

1. **Eaves cut: plumb.** Every layer ends in a vertical face on one plan
   line: the outline offset outward by `Overhang`. `PlanArea` is one
   number for every layer.
2. **Gables are stated per edge.** `Gables []int` lists the outline edges
   that are vertical gable ends; every other edge slopes at `Pitch`. There
   is no roof kind. Hip: no gables. Gable: the two end edges. Mono: every
   edge but the low one. A cross-gable L lists its arms' end edges. This
   replaces the issue's `Kind` + `Edge`, and it removes any "parallel to"
   test, so a 45° bay edge is just an edge the caller states.
3. **Approach: our own weighted straight skeleton** (below).
4. **No steps inside a roof.** A gable that would separate two different
   planes inside the roof (a vertical cliff, as edge (18,6)→(12,6) as a
   gable makes on the L) is refused, and so is a gable on one line with a
   sloped neighbour (the corner between them has no direction). A mono
   roof on an L still builds: one plane has no step.
5. **`Top` is its own quantity.** A level top edge where a sloped plane
   meets a gable end (a mono roof's high edge) is priced apart from a
   ridge (top-edge flashing, not a ridge cap).

## Approaches

1. **Own event-driven weighted straight skeleton, pure Go (picked).**
   Felkel–Obdržálek with weights: sloped edges move at speed 1, gable
   edges at speed 0. Edge events, split events at reflex corners, and
   simultaneous events (the rectangle's ridge is one). No dependency,
   no cgo; bounded by a point cap and the work budget (#70). Cost: the
   largest and riskiest part; weighted split events are where
   implementations go wrong, so a fuzz target and property checks guard it.
2. Port an existing skeleton: CGAL is GPL; the small ports lack weights
   and need a licence check. Same weighted work, plus a licence risk.
3. Lower envelope of the edge planes: exact on convex outlines only. On
   the L a far edge's plane cuts through the wing. Fails the acceptance.

## Design

### API (package geometry)

```go
type RoofSpec struct {
    Pitch    float64 // degrees from horizontal, every sloped edge; (0, 75]
    Gables   []int   // outline edges that are vertical gable ends; edge i runs outline[i] → outline[i+1]
    Overhang float64 // metres, horizontal, beyond every edge (gable ends too); [0, 3]
    Eaves    float64 // z of the underside of the lowest layer, above the outline line
}

type RoofQuantities struct {
    SlopeArea float64 // the layer's top surface, all sloped planes
    PlanArea  float64 // its projection: the outline offset by Overhang
    Volume    float64
    Eaves, Ridge, Hip, Valley, Verge, Top float64 // line lengths in metres, 3D
}

type Roof struct {
    Element    Element
    Quantities RoofQuantities
    Planes     []Face // one per sloped plane, on the layer's top surface
}

func BuildRoofLayers(outline [][2]float64, spec RoofSpec, layers []float64) ([]Roof, error)
```

There is no single-thickness `BuildRoof`. One layer is `[]float64{t}`.

### Geometry

- **Roof surface.** Build the weighted skeleton of the plumb line: the
  outline offset outward by `Overhang`, each edge moved along its own
  normal (mitred corners). Sloped edges have weight 1, gable edges 0. Each
  sloped edge's region becomes a plane at `Pitch` that rises inward from
  that edge. The surface passes through `Eaves + total/cos(Pitch)` above
  the outline line, where `total` is the sum of the layers. Gable regions
  have no area: the plane next to a gable runs straight to the gable line.
- **Layers, top first.** All sloped planes share one pitch, so a thickness
  `t` measured normal to the slope is a vertical drop of `t / cos(Pitch)`.
  Layer k's top is the roof surface dropped by the layers above it; its
  bottom is that top dropped by `t_k / cos(Pitch)`. Every layer ends on the
  same plumb line with vertical sides. The lowest layer's bottom is at
  `Eaves` above the outline line, as the spec says.
- **Mesh.** Each layer is a closed shell: the top planes triangulated, the
  same triangles dropped for the bottom, and vertical side faces along the
  plumb line (a straight top on an eaves edge, the roof profile on a gable
  edge). float32 vertices local to `outline[0]`, placed like a slab.
- **Faces.** One `Face` per sloped plane of the layer's top: `Origin` on
  its eaves edge, `U` along the eaves, `V` up the slope, `Normal` outward
  (up), `Outline` in (u, v).

### Quantities, per layer

- `PlanArea` = area of the plumb line polygon. `SlopeArea` =
  `PlanArea / cos(Pitch)`. `Volume` = `PlanArea × t / cos(Pitch)`. These are
  closed forms; the mesh volume must agree to float32 rounding.
- Line lengths are measured in 3D on the layer's top. They are the same
  on every layer, because a vertical drop keeps them.
  - `Eaves`: the sloped edges of the plumb line.
  - `Ridge`: horizontal skeleton arcs between two sloped planes.
  - `Hip`: sloping arcs where the roof folds outward (the two planes
    fall away from the line), from a corner or not: the plain L at 30°
    has a hip (9,3)→(7,5) that touches no corner.
  - `Valley`: sloping arcs where the roof folds inward (the planes meet
    in a trough).
  - `Verge`: the top's sloping edges along gable ends.
  - `Top`: the top's level edges along gable ends, where one sloped
    plane meets the gable (a mono roof's high edge).
  - An arc between two planes of collinear edges is not a line; it is not
    counted.

### Refusals (an error and nothing else)

- The outline as `checkOutline` refuses it, and over 256 points (a cap
  for the skeleton; slabs keep 2,048).
- `Pitch` not finite, ≤ 0° or > 75° ("a flat roof is a slab").
- A `Gables` index outside the outline, or repeated; every edge a gable
  ("no sloped edge").
- `Overhang` not finite, below 0 or over 3 m; a plumb line that is not a
  simple polygon (an overhang that closes a notch, or a mitred corner that
  crosses another edge), naming the edges.
- `Eaves` as `checkOrigin` refuses it; layers as `checkLayers` refuses
  them; a total thickness whose drop, `total / cos(Pitch)`, is over the
  maximum dimension.
- A gable that would leave a vertical step inside the roof: two
  different planes meet across it (decision 4), naming the gable.
- A gable on one line with a sloped neighbour, naming both edges.
- A skeleton that does not finish within the work budget (#70).

### BuildPlan and wasm

- `PlanRoof{ID, Outline, Pitch, Gables, Overhang, Eaves, Thickness,
  Layers}`; `Plan.Roofs`; `Built.Roofs []BuiltRoof`, named like slabs
  (`r1`, `r1/j`). Roofs count toward the item and element caps.
- wasm request: `"roofs"` items with the same fields in camelCase. The
  answer carries the quantities and the planes. Refusal codes:
  `pitch_out_of_range`, `gable_out_of_range`, `gable_repeated`,
  `no_sloped_edge`, `roof_step`, `gable_collinear`,
  `overhang_out_of_range`, `overhang_closes_outline`, `roof_too_complex`, each with `item` (and `gable` / `edge` where it
  applies).
- The plan SVG preview draws a roof's plumb line and its ridge, hip and
  valley lines.

## Acceptance

- Overhang 0 unless a check says otherwise.
- 10 × 8 m rectangle, no gables, 30°: four planes, a 2 m
  ridge, slope area = 80 ÷ cos 30°.
- The L outline, no gables, 30°: one valley; slope area = plan area ÷
  cos 30°.
- The rectangle with its two 8 m edges as gables: two planes, verge = 4 ×
  the rafter length.
- The rectangle with all edges but one 10 m edge as gables (mono): one
  plane, its high edge at `Eaves + total/cos 30° + 8 × tan 30°` on the top
  surface; `Top` = 10 m, `Ridge` = 0.
- The L with edge 2, (18,6)→(12,6), as a gable: refused, `roof_step`.
- An L mono roof (edge 0 low, every other edge a gable): one plane, no
  refusal.
- The plain L, no gables, 30° (worked by hand): ridges (15,3)→(9,3)
  and (5,5)→(7,5), valley (12,6)→(9,3), hip (9,3)→(7,5).
- An L with equal 6 m arms, `(0,0) (18,0) (18,6) (6,6) (6,14) (0,14)`,
  edges 1 and 4 as gables (a cross-gable): ridges y=3 and x=3 at one
  height meet at (3,3); one valley (6,6)→(3,3) and one hip (0,0)→(3,3). (On the issue's L the
  arms are 6 and 12 m wide, so its ridges sit at different heights and a
  hip joins them; they never meet.)
- Two layers: volumes sum to the one-layer roof of the total thickness;
  each layer's mesh volume matches its quantity within float32 rounding;
  each mesh is a closed shell.
- An overhang of 0.5 m: plan area = the offset outline's area.
- Identical results when the outline is moved to national-grid
  coordinates.
- Go, `BuildPlan` and wasm give the same numbers (goldens).
- A fuzz target over outlines, gables and pitch: no panic, each answer a
  closed shell whose slope area equals plan area ÷ cos(Pitch), or a
  refusal.

## Left to the builder

- The skeleton's internal event queue and its tolerances, as long as the
  properties above hold and the work is charged to the budget.
- Triangulation of each plane (the existing ear clipper is fine).
- Test names, and the exact wording of each refusal, following the
  slab and wall refusals.
