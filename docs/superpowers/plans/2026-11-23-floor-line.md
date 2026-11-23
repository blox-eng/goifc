# Floor-line lengths of wall faces (#174): plan

Spec: docs/superpowers/specs/2026-11-23-floor-line-design.md
Spec approved: 2026-11-23
Test command: make test
Plan approved: 2026-11-23 by owner
Next: builder:build

## Files

- `geometry/build_wall.go` (change): `WallQuantities` gains `MeasuredFloorLine` and `OtherFloorLine`, and `wallQuantities` computes them.
- `geometry/join.go` (change): `subtractStrips` also takes a strip's width off the floor line when the strip reaches the face's base.
- `geometry/build_wall_test.go`, `geometry/build_room_test.go`, `geometry/build_layers_test.go`, `geometry/join_test.go` (change): the tests of the two lengths.
- `geometry/build_fuzz_test.go` (change): the fuzz invariant 0 ≤ floor line ≤ face span.
- `internal/wasmapi/build.go` (change): `wallQuantities` gains `measuredFloorLine` and `otherFloorLine`.
- `internal/wasmapi/build_test.go`, `internal/wasmapi/testdata/build/*.answer.json` (change): the answer test and the rewritten goldens.
- `docs/guides/builders.md`, `docs/guides/wasm.md`, `CHANGELOG.md` (change): the two lengths, documented.

## Task 1: a wall reports each face's floor line, net of its doors

- **Files:** `geometry/build_wall.go`, `geometry/build_wall_test.go`, `geometry/build_room_test.go`, `geometry/build_layers_test.go`, `geometry/build_fuzz_test.go`.
- **Test first:**
  - `TestWallFloorLine` in `build_wall_test.go`. A 4 m wall, 2.7 m high, 0.2 m thick, with a door `{Offset: 0.5, Width: 0.9, Height: 2.1}` and a window `{Offset: 2, Sill: 0.9, Width: 1.2, Height: 1.2}`. The test asserts `MeasuredFloorLine == 3.1` and `OtherFloorLine == 3.1`, within 1e-12. The same wall with no openings reports 4 and 4.
  - `TestRoomWallFloorLine` in `build_room_test.go`. A 4 × 5 m room from `BuildRoomWalls`, 0.2 m thick, `SideLeft`, with a 0.9 m door on wall 0. For each wall, `MeasuredFloorLine == Length` less its doors. `OtherFloorLine == OtherGross/Height` less its doors: the mitred span, not `Length`.
  - `TestBuildWallLayers_FloorLinePerLayer` in `build_layers_test.go`. Layers `[0.1, 0.2]` with the same door. Every layer reports 3.1 on both faces.
  - In `build_fuzz_test.go`, next to the joins invariant: `0 ≤ MeasuredFloorLine ≤ Length` and `0 ≤ OtherFloorLine ≤ OtherGross/Height`, within 1e-9 relative.
- **Run:** `go test ./geometry -run 'FloorLine' -count=1`. Before: it fails to compile, because `MeasuredFloorLine` is undefined. After: it passes.
- **Do:**
  - Add `MeasuredFloorLine, OtherFloorLine float64 // m, each face along the base, less doors and the partitions standing there` to `WallQuantities`.
  - In `wallQuantities(span, height, thickness, ops)`, sum `o.Width` over the openings with `o.Sill == 0` into `doors`. Set `MeasuredFloorLine: span.length - doors` and `OtherFloorLine: other - doors`.
- **Done when:** the three tests pass, and `go test ./geometry` passes.
- **Commit:** `feat(geometry): report each wall face's floor-line length, net of doors`

## Task 2: a partition standing on a face takes its width off that floor line

- **Files:** `geometry/join.go`, `geometry/join_test.go`.
- **Test first:** `TestJoinFloorLine` in `join_test.go`, through `BuildPlan`. A 10 m through wall, base 0, 2.5 m high, 0.2 m thick. A 0.1 m partition joins its measured face at its end. The test asserts:
  - the through wall's `MeasuredFloorLine == 9.9`, and its `OtherFloorLine == 10`;
  - with a layered through wall `[0.1, 0.1]`, only layer 0's `MeasuredFloorLine` is 9.9;
  - a partition with `Base: 1` (it stands on the face, but above the floor) leaves `MeasuredFloorLine == 10`, while `MeasuredJoins` still holds its strip.
- **Run:** `go test ./geometry -run TestJoinFloorLine -count=1`. Before: it fails with `MeasuredFloorLine 10, want 9.9`. After: it passes.
- **Do:** in `subtractStrips`, next to each `Joins += a`, take `s.hi - s.lo` off the same face's floor line when `s.bottom <= s.face.base + minDim`.
- **Done when:** `TestJoinFloorLine` passes, and `go test ./geometry` passes.
- **Commit:** `feat(geometry): take a partition's width off the floor line it stands on`

## Task 3: the wasm answer carries both lengths

- **Files:** `internal/wasmapi/build.go`, `internal/wasmapi/build_test.go`, `internal/wasmapi/testdata/build/*.answer.json`.
- **Test first:** in the existing joins test of `build_test.go` (line 147), decode `measuredFloorLine` and `otherFloorLine`. Assert that each equals the Go value that `geometry.BuildPlan` gives for the same request, and that the joined face's `measuredFloorLine` is its length less the strip width.
- **Run:** `go test ./internal/wasmapi -count=1`. Before: the new assertion fails (0 ≠ the length), and after the field is added the goldens differ. After `go test ./internal/wasmapi -update`, and after reading the golden diff (only the two new keys are added), it passes.
- **Do:** add `MeasuredFloorLine float64 \`json:"measuredFloorLine"\`` and `OtherFloorLine float64 \`json:"otherFloorLine"\`` to `wallQuantities`, and set them in `toWallQuantities`.
- **Done when:** `go test ./internal/wasmapi` passes, and the golden diff holds only the new keys.
- **Commit:** `feat(wasm): answer each wall face's floor-line length`

## Task 4: the guides and the changelog name the lengths

- **Files:** `docs/guides/builders.md`, `docs/guides/wasm.md`, `CHANGELOG.md`.
- **Test first:** none: no test can see prose. The check is that `make lint` passes, and the docs build passes in CI (Build Docs, Links).
- **Do:**
  - In builders.md's quantities table, add one row: `MeasuredFloorLine`, `OtherFloorLine`, each face along the wall's base, in m, less the doors (`Sill: 0`) and the partitions that stand on that line. A window takes nothing off.
  - In builders.md's layer notes, add one line: every layer reports its own pair, and a partition comes off the face it meets.
  - In wasm.md, where the wall quantities are listed, add `measuredFloorLine` and `otherFloorLine`.
  - In CHANGELOG.md, under `## Unreleased`, `### Added`: "Walls report each face's floor-line length, net of doors and of the partitions standing on it, to price skirting per metre: `MeasuredFloorLine` and `OtherFloorLine` in Go, `measuredFloorLine` and `otherFloorLine` in the wasm answer. (#174)"
- **Done when:** `make lint` passes, and a grep of the docs finds both names.
- **Commit:** `docs: describe the floor-line lengths of wall faces`

## Task 5: review the whole change

- Run `builder:review` on the branch against the spec `docs/superpowers/specs/2026-11-23-floor-line-design.md`, with `make test`.
- **Done when:** the review has no open Critical or Important finding.

## Lens findings

- Engineering: the spec's anti-goal "nothing counts twice" needs no union of spans. Openings may not overlap (`overlaps`), strips may not overlap (`joins_overlap`), and an opening may not touch a strip (`in_join`, `openingInStrip`). No change, because these refusals exist today. Task 2 tests the one case they do not cover: a strip above the floor.
- Engineering: a room's layers mitre, so each layer's opposite span differs. Task 1 takes `other` from the layer's own span, and the room test checks the mitred value.
- Engineering: a niche at `Sill == 0` is refused today, so a door always cuts through, and both faces lose it. No change.
- Design: the names follow the `Measured…`/`Other…` pairs, and the docs say in plain words what comes off. The person meets the names in the answer and the guide.
- Architecture: the wasm answer only gains keys, so a consumer that reads it today goes on working. No change.

## Check against the spec

| Spec line | Task |
|---|---|
| A 4 m wall with a 0.9 m door reports 3.1 m on both faces | Task 1: `TestWallFloorLine` |
| A window takes nothing off | Task 1: `TestWallFloorLine` |
| A mitred room corner gives the opposite face its own span, less its doors | Task 1: `TestRoomWallFloorLine` |
| Every layer reports its own pair | Task 1: `TestBuildWallLayers_FloorLinePerLayer` |
| A partition at the base takes its strip width off, on that layer only; one standing higher takes nothing off | Task 2: `TestJoinFloorLine` |
| The wasm answer carries both lengths, equal to the Go numbers | Task 3 |
| Anti-goal: nothing counted twice, a window never counted | Task 1 (window); the existing refusals; Lens findings |

## Cold read

Read as a builder with only this plan and the repository. Each task names its function and its test with values. The one gap was "which layer gets the strip": Task 2 now says layer 0's measured face, or the last layer's opposite face, as `subtractStrips` does today.

## Changes during build

- Task 2: the test uses the existing join fixtures (`joinT`, 4 m, with `joinFixture` and `otherP`) instead of a new 10 m wall. The values become 3.9 and 4. It also covers the opposite face and both layer cases. Reason: the fixtures already pin the strip at x ∈ [1.9, 2], so the test reads the same numbers as the area tests.
