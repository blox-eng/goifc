# Roof precision: drafting noise and the mesh weld (#169, #170)

Status: approved 2026-11-25 (a small change: this short design).

## Intent

A roof outline from a CAD import carries noise: a point a few µm off a
straight edge. Today such a roof builds, but its `Planes` do not cover the
outline (#169), and the mesh weld moves points off their sloped planes by up
to `gridSnap × tan(pitch)` (#170). A viewer that draws the planes or the mesh
shows a roof that disagrees with its own quantities.

## Goals and non-goals

- Goal: the planes, the mesh and the quantities describe one roof, for
  outlines with sub-0.1 mm noise too.
- Goal: a roof whose planes still fail to cover its outline is refused, never
  returned wrong.
- Non-goals: the area and volume formulas, the walls, the preview weld, the
  wasm contract.
- Anti-goal: a roof that builds with planes that miss or overlap its outline.

## Approaches

1. **Straighten the noise (picked, owner 2026-11-25).** A point under
   `gridSnap` (0.1 mm) from the line through its neighbours is drafting
   noise: `joinCollinear` drops it. The area shifts by at most 0.1 mm × the
   edge length.
2. Keep the outline as drawn, and refuse a roof whose planes do not cover
   it: a 10 µm import glitch then stops the roof.
3. Snap welded points onto their planes: a point on three planes cannot
   move onto all of them.

## Design

- **Join.** `joinCollinear` drops a ring point between two edges of one kind
  (two sloped edges, or two gables) when it lies under `gridSnap` from the
  line through its neighbours, an absolute distance. A gable next to a sloped
  edge never joins, as today.
- **Plan area.** `PlanArea` is the joined ring's area, so the closed forms
  and the planes use one outline.
- **Cover check.** After the skeleton, the faces' plan areas must add up to
  the ring's area within tolerance. Otherwise the roof is refused as
  `roof_too_complex`.
- **Weld (#170).** The mesh welds only points within a few float32 steps at
  the roof's extent, never more than `gridSnap`. Each mesh vertex then lies
  on its planes within that weld. `roofTrue` uses the weld, not `gridSnap`.

## Done when

- Both #169 seeds build, and their planes cover `SlopeArea`.
- The #170 hexagon `(0,0) (10,0) (12,4) (10,8.000316) (0,8) (-2,4)`, 75°, no
  gables, has every mesh vertex on its planes within the weld.
- A ring whose faces do not cover it is refused as `roof_too_complex`.
- The existing roof tests and the fuzz corpus pass with the tighter
  `roofTrue`.

## Left to the builder

The weld threshold's exact form, how the cover check gets a ring the
skeleton fails to tile, and private helper names.

## Rulings

- 2026-11-25: straighten sub-0.1 mm jogs (option 1). Decided by: owner.
- 2026-11-25: spec approved. Decided by: owner.
- 2026-11-25: plan review: two gables join like two sloped edges, and the weld is capped at `gridSnap`; both found by running the plan. Decided by: owner.
