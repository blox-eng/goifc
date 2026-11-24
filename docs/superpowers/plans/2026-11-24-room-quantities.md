# Room quantities: floor area, perimeter and volume (#175): plan

Spec: docs/superpowers/specs/2026-11-24-room-quantities-design.md
Spec approved: 2026-11-24
Test command: make test
Plan approved: 2026-11-24 by owner
Next: builder:build

## Files

- `geometry/plan.go` (change): `RoomQuantities`, `BuiltRoom`, `Built.Rooms`; `BuildPlan` fills them after `subtractStrips`.
- `geometry/build_room.go` (change): `roomMitres` gives the mitre ring; a `roomFloor` helper gives the room-side ring.
- `geometry/plan_room_test.go` (create): the tests of the room quantities.
- `internal/wasmapi/build.go` (change): the answer gains `rooms`, with `roomQuantities`.
- `internal/wasmapi/build_test.go`, `internal/wasmapi/testdata/build/*.answer.json` (change): the answer test and the rewritten goldens.
- `wasm/goifc-contract.schema.json` (change): `buildOK.rooms`, closed like `wallQuantities`.
- `docs/guides/builders.md`, `docs/guides/wasm.md`, `CHANGELOG.md` (change): the room quantities, documented.

## Task 1: a plan reports each room's floor area, perimeters and volume

- **Files:** `geometry/build_room.go`, `geometry/plan.go`, `geometry/plan_room_test.go`.
- **Test first:** in `plan_room_test.go`, all within 1e-9. A helper `builtRoom(t, b Built, id string) RoomQuantities` fails the test when the ID is missing.
  - `TestRoomQuantitiesGrowOut`: outline `{0,0},{4,0},{4,5},{0,5}` (counter-clockwise), `SideLeft`, `Thickness: 0.2`, `Height: 2.5`. Want `Area 20`, `Perimeter 18`, `NetPerimeter 18`, `Volume 50`.
  - `TestRoomQuantitiesGrowIn`: outline `{0,0},{4.4,0},{4.4,5.4},{0,5.4}`, `SideRight`, `Thickness: 0.2`, `Height: 2.5`. Want 20, 18, 18, 50. The same outline reversed (clockwise) with `SideLeft` gives the same numbers.
  - `TestRoomQuantitiesLayeredGrowIn`: the grow-in room with `Layers: []float64{0.05, 0.15}` and no `Thickness`. Want 20, 18, 18, 50.
  - `TestRoomQuantitiesL`: outline `{0,0},{6,0},{6,3},{3,3},{3,5},{0,5}`, `SideLeft`, 0.2 thick, 2.5 high. Want `Area 24`, `Perimeter 22`, `Volume 60`.
  - `TestRoomQuantitiesDoors`: the grow-out room with a door `{Offset: 1, Width: 0.9, Height: 2.1}` on wall 0 and a window `{Offset: 1, Sill: 0.9, Width: 1.2, Height: 1.2}` on wall 1. Want `Perimeter 18`, `NetPerimeter 17.1`. The grow-in room with the same door on wall 0 also gives 17.1.
  - `TestRoomQuantitiesPartition`: `joinR` and the partition `P` from `TestStripRoom` in `join_test.go`. Want `Area 12`, `Perimeter 14`, `NetPerimeter 13.8`.
  - `TestRoomQuantitiesRefusal`: a 4 × 0.1 m outline, `{0,0},{4,0},{4,0.1},{0,0.1}`, with 0.2 m walls growing in (`SideRight`): narrower than two walls. `BuildPlan`'s error, unwrapped from its `*ItemError`, has the same text as `BuildRoomWalls`' error, and `Built.Rooms` is empty.
  - `TestRoomQuantitiesOrder`: two rooms `A` and `B` after a wall item. `Built.Rooms` holds `A` then `B`.
- **Run:** `go test ./geometry -run 'RoomQuantities' -count=1`. Before: it fails to compile, because `Built.Rooms` is undefined. After: it passes.
- **Do:**
  - `roomMitres(ring, thickness, side) ([]wallSpan, [][2]float64, error)` holds `roomSpans`' corner loop and returns the `mitre` ring too. `roomSpans` calls it, then runs the overlap check as today.
  - `roomFloor(it PlanRoom, total float64) ([][2]float64, bool)` returns the room-side ring and whether the measured face is on the room side. The measured face is on the room side when `signedArea(outline) > 0` with `SideLeft`, or `< 0` with `SideRight`. Then the ring is `localRing(outline)`. Otherwise it is the mitre ring of `roomMitres(localRing(outline), total, it.Side)`. `total` is `Thickness`, or the sum of `Layers`. The room is built already, so `roomMitres` cannot fail here.
  - `type RoomQuantities struct { Area, Perimeter, NetPerimeter, Volume float64 }`. The comment gives the units: m², m, m, m³.
  - `type BuiltRoom struct { ID string; Quantities RoomQuantities }`, and `Built.Rooms []BuiltRoom`.
  - At the end of `BuildPlan`, after `subtractStrips`, append one `BuiltRoom` per room item, in plan order:
    - `Area` is `|signedArea(ring)|`.
    - `Perimeter` is the sum of the ring's edges.
    - `NetPerimeter` sums over wall `w` of the room. With `n = max(1, len(Layers))`, it takes `out.Walls[roomAt[i]+w*n].Wall.Quantities.MeasuredFloorLine` when the measured face is on the room side. Otherwise it takes `out.Walls[roomAt[i]+w*n+n-1]…OtherFloorLine`.
    - `Volume` is `Area * Height`.
  - Update the `Built` doc comment.
- **Done when:** `go test ./geometry -run 'RoomQuantities|Strip|Room' -count=1` passes, and `make test` passes.
- **Commit:** `feat(geometry): report each room's floor area, perimeter and volume (#175)`

## Task 2: the wasm answer carries the room quantities

- **Files:** `internal/wasmapi/build.go`, `internal/wasmapi/build_test.go`, `internal/wasmapi/testdata/build/*.answer.json`, `wasm/goifc-contract.schema.json`.
- **Test first:** `TestBuild_Rooms` in `build_test.go`. Request: the grow-out 4 × 5 room `r1` of Task 1, with the door on wall 0. The answer's `rooms` has one entry, `id: "r1"`. Its `quantities` equal `geometry.BuildPlan`'s for the same plan: `area 20`, `perimeter 18`, `netPerimeter 17.1`, `volume 50`. A request with no rooms has no `rooms` key.
- **Run:** `go test ./internal/wasmapi -run 'TestBuild_Rooms|TestSchema' -count=1`. Before: `TestBuild_Rooms` fails with no `rooms` key. After: it passes. Then `go test ./internal/wasmapi -update` rewrites the room goldens. A diff of each golden with the `rooms` key stripped shows no other change.
- **Do:**
  - `roomOut{ID string "id"; Quantities roomQuantities "quantities"}`, and `roomQuantities{Area "area", Perimeter "perimeter", NetPerimeter "netPerimeter", Volume "volume"}`.
  - `built.Rooms []roomOut` with the JSON tag `rooms,omitempty`, the same as `roofs`. It is filled from `b.Rooms`.
  - In the schema, add `buildOK.properties.rooms`: an array of `{id, quantities}`, where `quantities` has the four numbers, all required, `additionalProperties: false`. `rooms` is not in `buildOK.required`, the same as `roofs`.
- **Done when:** `go test ./internal/wasmapi -count=1` passes, `TestSchema_Goldens` included.
- **Commit:** `feat(wasmapi): carry room quantities in the build answer (#175)`

## Task 3: the guides and the changelog name the room quantities

- **Files:** `docs/guides/builders.md`, `docs/guides/wasm.md`, `CHANGELOG.md`.
- **Check (no test sees prose):** run `rg -n 'NetPerimeter|netPerimeter' docs/guides CHANGELOG.md` before (0 matches) and after (a match in each file).
- **Do:**
  - `builders.md`, in the plan section: `Built.Rooms`. Give the four numbers and say which ring the area and perimeter come from. State that a partition does not come off the area, and give the `joinR` example (12 m², 14 m, 13.8 m).
  - `wasm.md`: the `rooms` array.
  - `CHANGELOG.md`, under Unreleased/Added: "A room in a plan reports its floor area, its floor perimeter gross and net of doors and partitions, and its volume."
- **Done when:** the check finds a match in each file.
- **Commit:** `docs: document room quantities (#175)`

## Task 4: review the whole change

Load `builder:review` on `feat/175-room-quantities` against `origin/main`. Give it the spec `docs/superpowers/specs/2026-11-24-room-quantities-design.md`, this plan and the test command `make test`.

## Lens findings

- Engineering: `NetPerimeter` reads the floor lines after `subtractStrips`. A future change that moves the strip subtraction later would silently drop partitions. `TestRoomQuantitiesPartition` pins the order. No other change.
- Engineering: the room is built already when `roomFloor` asks for its ring, so the error cannot occur. This finding first said a second `roomSpans` costs O(n). The review measured it: the overlap check inside is O(n²). See Changes during build.
- Architecture (wasm contract): the change is additive. `rooms` is optional, like `roofs`, so a request with no room gets an answer that is byte-identical to today's. The downstream viewer is told already (2026-11-24). No change.
- Design: no screen. Every number carries its unit in the Go comment and the guide. No change.

## Spec check

| Spec line | Task |
|---|---|
| 4 × 5 room inside 0.2 m walls: 20 m², 18 m, 20 × height; outward and inward | Task 1: `GrowOut`, `GrowIn` |
| A layered room that grows inward uses the ring at full depth | Task 1: `LayeredGrowIn` |
| An L-shaped room reports the area of the L | Task 1: `L` |
| A door at sill 0 takes 0.9 off `NetPerimeter`, not `Perimeter`; a window nothing | Task 1: `Doors` |
| A T-joined partition takes its strip off `NetPerimeter`, nothing off `Area` | Task 1: `Partition` |
| A refused outline gives the same error | Task 1: `Refusal` |
| The wasm answer equals the Go numbers | Task 2: `TestBuild_Rooms` |
| Anti-goal: the net perimeter disagrees with the floor lines | Task 1: computed from the floor lines; `Doors`, `Partition` |
| Anti-goal: the floor on the wrong side of the walls | Task 1: `GrowIn` (both windings), `LayeredGrowIn` |

## Cold read

Read as a builder with only this plan and the repository. Each name it uses exists: `roomSpans`, `checkRoom`, `localRing`, `signedArea` in `preview.go`, `subtractStrips`, `roomAt`, `joinR`, `TestStripRoom`, `MeasuredFloorLine`, `OtherFloorLine`, and `roofs,omitempty` in `build.go`. One gap was closed: the plan now says the too-narrow refusal input, and that `Rooms` is empty on refusal.

## Changes during build

- 2026-11-24, review: `roomFloor` called `roomSpans`, which repeats the O(n²) overlap check the build already ran. At the plan cap (48 inward rooms of 2,048 points) that added 3.1 s to a 10.7 s build. `roomSpans`' corner loop moved to `roomMitres`, which `roomFloor` calls; `roomSpans` keeps its old signature.
