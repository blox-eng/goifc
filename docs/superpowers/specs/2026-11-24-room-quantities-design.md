# Room quantities: floor area, perimeter and volume (#175)

Status: approved 2026-11-24 (a small change: this short design).

## Intent

A cost estimate prices floors, ceilings and room finishes per room.
`BuildPlan` builds a room item's walls, but it reports nothing about the
room itself. The caller cannot get the floor inside the walls when the walls
grow inward, nor the perimeter net of doors and partitions.

## Goals and non-goals

- Goal: per room item of a plan, the floor area, the gross and net floor
  perimeter and the volume, from `BuildPlan` and the wasm build, giving the
  same numbers.
- Non-goals:
  - partition footprints: the area is gross, inside the room's walls;
  - a ceiling or slab height other than the room's `Height`;
  - room quantities from `BuildRoomWalls` alone (it does not know the joins);
  - finishes, skirting or pricing.
- Anti-goal: a net perimeter that disagrees with the walls' floor lines
  (#174), or a floor that lies on the wrong side of the walls.

## Approaches

1. **In `BuildPlan`, after the join strips (picked).** Area and gross
   perimeter come from the room-side ring. The net perimeter is the sum of
   the room walls' floor lines on the room side, after `subtractStrips`.
2. A standalone `RoomQuantities(outline, …)`: it cannot see joins, so its net
   perimeter would ignore partitions, giving two answers for one room.
3. Measure the built meshes: float32 rounding, more work, and less exact than
   the ring that `roomSpans` already computes.

Partitions (owner, 2026-11-23): option A, gross area. A partition splits a
room; the estimator draws two room items for that.

## Design

- **Room side.** The room is the inside of the closed outline. The measured
  face is on the room side when a counter-clockwise outline has `SideLeft`
  or a clockwise outline has `SideRight`. Otherwise the walls grow inward.
- **Ring.** The room-side ring is the outline when the measured face is on
  the room side. Otherwise it is the mitred opposite-face ring at the full
  wall depth: `Thickness`, or the sum of `Layers`.
- **`RoomQuantities`** (m, m², m³):
  - `Area`: the ring's area (shoelace, absolute).
  - `Perimeter`: the sum of the ring's edges.
  - `NetPerimeter`: Σ over the room's walls of the room-side floor line:
    `MeasuredFloorLine` of layer 0 when the measured face is on the room
    side, else `OtherFloorLine` of the last layer. Doors at sill 0 and
    partition strips at the base are already off it.
  - `Volume`: `Area × Height`.
- **`Built.Rooms []BuiltRoom{ID, Quantities}`**, one per room item, in plan
  order. The wasm answer gains `rooms: [{id, quantities: {area, perimeter,
  netPerimeter, volume}}]`; the schema closes it like `wallQuantities`.
- **Refusals.** A room that its walls refuse gives the same error, and no
  room entry: `BuildPlan` returns no elements on any refusal.

## Done when

- A 4 × 5 m room inside 0.2 m walls reports 20 m², 18 m, and 20 × height as
  its volume. Both cases pass: walls grow outward from a 4 × 5 outline, and
  walls grow inward from a 4.4 × 5.4 outline.
- A layered room that grows inward uses the ring at the full depth.
- An L-shaped room reports the area of the L.
- A 0.9 m door at sill 0 takes 0.9 m off `NetPerimeter` and nothing off
  `Perimeter`; a window takes nothing off.
- A partition T-joined to a room wall takes its strip width off
  `NetPerimeter` and nothing off `Area`.
- An outline that `BuildRoomWalls` refuses gives the same error.
- The wasm answer carries the room's numbers, equal to the Go numbers.

## Left to the builder

How to get the inner ring: from `roomSpans`' mitre points, or from the
outline and the spans. Names of private helpers.

## Rulings

- 2026-11-23: partitions do not come off the area (option A). Decided by: owner.
- 2026-11-24: spec approved. Decided by: owner.
