# Floor-line lengths of wall faces (#174)

Status: approved 2026-11-23 (a small change: this short design).

## Intent

A cost estimate prices skirting per metre, along the bottom of each wall
face. Today a wall reports its face areas and the length of its measured
face only. A caller cannot get the opposite face's length, which a mitred
room corner changes, and cannot get either length net of the doors and of
the partitions that stand on the face.

## Goals and non-goals

- Goal: per wall and per layer, the floor-line length of each face, net of
  what stands on that line, from Go, `BuildPlan` and the wasm build, giving
  the same numbers.
- Non-goals: which face is the room side; skirting height or profile; a
  floor finish above the wall's base. The caller picks the face and prices it.
- Anti-goal: a length that counts a door or a partition twice, or counts a
  window at all.

## Approaches

1. **From the numbers (picked).** The face's span less each opening with
   `Sill == 0` and each partition strip that reaches the wall's base, as the
   net areas are computed today.
2. Measure the mesh's bottom edges: float32 rounding, and disagrees with the
   areas.
3. Leave it to the caller: it cannot see the mitred opposite span or the
   partition strips.

## Design

- `WallQuantities` gains `MeasuredFloorLine` and `OtherFloorLine`, in m. The
  wasm answer gains `measuredFloorLine` and `otherFloorLine`.
- `MeasuredFloorLine` = `Length` − Σ `Width` of the openings with
  `Sill == 0` that cut the layer. `OtherFloorLine` = the opposite span
  (`otherEnd − otherStart`) − the same sum. A door always cuts through: a
  niche needs a sill.
- A partition strip whose bottom is at the face's base takes its width
  (`hi − lo`) off the floor line of the face it lies on: layer 0's measured
  face or the last layer's opposite face, as `subtractStrips` does for the
  areas. A strip that starts higher takes nothing off.
- Nothing comes off twice: openings may not overlap (`overlaps`), strips may
  not overlap (`joins_overlap`), and an opening may not touch a strip
  (`in_join`). These refusals exist today.
- Every layer reports its own pair.

## Done when

- A 4 m wall with a 0.9 m door reports 3.1 m on both faces.
- A window (`Sill > 0`) takes nothing off.
- A mitred room corner gives the opposite face its own span, less its doors.
- A partition joined at the wall's base takes its strip width off the face it
  meets, on that layer only; one standing higher takes nothing off.
- The wasm answer carries both lengths, equal to the Go numbers.

## Rulings

- 2026-11-23: spec approved. Decided by: owner.
