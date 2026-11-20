# Pitched Roofs in Layers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `geometry.BuildRoofLayers` builds a layered pitched roof over any simple outline, with stated gable ends, from Go, `BuildPlan` and the wasm build (#161).

**Architecture:** A pure-Go weighted straight skeleton (sloped edges speed 1, gable edges speed 0) of the plumb line (the outline offset by `Overhang`) gives one plane per sloped edge. Every layer is that roof surface dropped vertically by `t / cos(Pitch)` per layer above, with vertical sides on the plumb line. Quantities are closed forms, and the mesh is checked against them.

**Tech Stack:** Go (package `geometry`, `internal/wasmapi`), JSON goldens, Node `node --test` for the wasm contract.

**Spec:** `docs/superpowers/specs/2026-11-20-pitched-roof-design.md`

## Global Constraints

- Package `geometry` imports no cgo, `os` or `net`.
- Synthetic fixtures only; no private names in code, tests, docs or commits.
- Every commit: a signed Conventional Commit, gated:
  `! git diff --cached | rg -i '<private names>' | rg -v 'blox-eng/goifc' && git commit -S ...`
  (the controller holds the name list; never write it into the repo). Never `--no-verify`.
- Pitch in (0°, 75°]; Overhang in [0, 3] m; outline: `checkOutline` plus at most 256 points.
- Edge i runs `outline[i] → outline[i+1]` (wrapping). Layers are top first.
- Lowest layer's underside is at `Eaves` above the outline line; every layer ends on the plumb line.
- Quantities per layer: `PlanArea` = plumb-line area; `SlopeArea` = `PlanArea / cos(Pitch)`; `Volume` = `PlanArea × t / cos(Pitch)`; line lengths in 3D, the same on every layer.
- Lines: `Eaves` (sloped plumb-line edges), `Ridge` (horizontal arcs between two sloped planes), `Hip` (sloping arcs folding outward), `Valley` (folding inward), `Verge` (sloping top edges on gable ends), `Top` (level top edges on gable ends). Arcs between planes of collinear edges are not counted.
- Refusals return an error and nothing else; refusal codes (wasm): `pitch_out_of_range`, `gable_out_of_range`, `gable_repeated`, `no_sloped_edge`, `roof_step`, `gable_collinear`, `overhang_out_of_range`, `overhang_closes_outline`, `roof_too_complex`.

## Review Focus

1. A square outline, hip: all four hips meet in one point (a pyramid). Expect `Ridge` = 0, four hips, no refusal. (Task 2)
2. A nearly straight corner (179.9°) or a 1 mm edge beside long ones: near-simultaneous events. Expect a closed shell with the closed-form quantities, or `roof_too_complex`; never a wrong mesh. (Task 2, Task 4 fuzz)
3. A clockwise outline: the same roof and quantities as its counter-clockwise twin, gables still indexed in the given order. (Task 3)
4. Pitch exactly 75° builds; 75.0001° and 0° are refused; a 50 m span at 75° does not overflow the maximum dimension silently. (Task 1, Task 3)
5. A sharp 10° tip with a 3 m overhang: the mitred corner runs far; expect a build or `overhang_closes_outline`, never a self-crossing plumb line. (Task 1)

## Hand values used below

`s = tan 30°`, `c = cos 30°`. Overhang 0, `Eaves` 3, one layer of 0.2, unless a test says otherwise.

- **R** (rectangle): `(0,0) (10,0) (10,8) (0,8)`.
  - Hip, no gables: 4 planes; Ridge 2 (from (4,4) to (6,4)); Hip 4·√(32+16s²); Eaves 36; PlanArea 80.
  - Gables {1, 3}: 2 planes; Ridge 10; Verge 4·4/c; Eaves 20; Hip 0.
  - Mono, gables {1, 2, 3}: 1 plane; Top 10; Verge 2·8/c; Eaves 10; Ridge 0; the top's high edge at z = 3 + 0.2/c + 8s.
- **L**: `(0,0) (18,0) (18,6) (12,6) (12,10) (0,10)`, PlanArea 156.
  - Hip: Ridge 8 ((15,3)→(9,3), (5,5)→(7,5)); Valley √(18+9s²) ((12,6)→(9,3)); Hip √(8+4s²) + 3·√(50+25s²) + 2·√(18+9s²); Eaves 56.
  - Gable {2}: refused `roof_step`, edge 2.
  - Mono, gables {1, 2, 3, 4, 5}: 1 plane, no refusal.
- **E** (equal-arm L): `(0,0) (18,0) (18,6) (6,6) (6,14) (0,14)`, gables {1, 4}, PlanArea 156: Ridge 26 (y=3 from x=18 to 3; x=3 from y=14 to 3); Valley √(18+9s²); Hip √(18+9s²); Verge 4·3/c; Eaves 52; Top 0.

---

### Task 1: Roof input checks and the plumb line

**Files:**
- Create: `geometry/roof_check.go`
- Test: `geometry/roof_check_test.go`

**Interfaces:**
- Produces:
  - `type RoofSpec struct { Pitch float64; Gables []int; Overhang float64; Eaves float64 }` (the spec's doc comments).
  - `type RoofReason string` with constants `ReasonPitchOutOfRange = "pitch_out_of_range"`, `ReasonGableOutOfRange = "gable_out_of_range"`, `ReasonGableRepeated = "gable_repeated"`, `ReasonNoSlopedEdge = "no_sloped_edge"`, `ReasonRoofStep = "roof_step"`, `ReasonGableCollinear = "gable_collinear"`, `ReasonOverhangOutOfRange = "overhang_out_of_range"`, `ReasonOverhangClosesOutline = "overhang_closes_outline"`, `ReasonRoofTooComplex = "roof_too_complex"`.
  - `type RoofError struct { Reason RoofReason; Edge, Other int }` (`Edge` = the gable or edge; `Other` = a second edge, -1 when none), with `Error()` naming both, like `HoleError`.
  - `func checkRoof(outline [][2]float64, spec RoofSpec, layers []float64) (gable []bool, total float64, err error)`: every refusal of the spec's list except `roof_step` and `roof_too_complex` (those come from the skeleton). Outline over 256 points: `tooLarge`. A drop `total / cos(Pitch)` over `maxLocalExtent`: `tooLarge`.
  - `func plumbLine(outline [][2]float64, overhang float64) ([][2]float64, error)`: the outline as a counter-clockwise local ring (relative to `outline[0]`), each edge moved outward by `overhang` along its own normal, corners mitred; same point count and edge order, so edge i stays edge i. Refuses with `*RoofError{ReasonOverhangClosesOutline, Edge: i, Other: j}` when the result fails `checkOutline`'s crossing or pinch tests; i and j are the two edges.
  - The ring is reversed when the outline is clockwise; keep an edge mapping so `Gables` always index the caller's edges (`TestPlumbLineOffsetsEveryEdge` checks it on a clockwise R).

- [ ] **Step 1: Write the failing tests** `TestCheckRoofRefusals` (table: pitch 0, -1, 75.0001, NaN → `pitch_out_of_range`; 75 → ok; gable 4 on R → `gable_out_of_range`; {1, 1} → `gable_repeated`; {0,1,2,3} → `no_sloped_edge`; overhang -0.1, 3.0001, NaN → `overhang_out_of_range`; 3 → ok; an outline with a collinear point `(0,0) (5,0) (10,0) (10,8) (0,8)` and gables {0} → `gable_collinear` Edge 0 Other 1; same with no gables → ok; 257 points → `errors.Is(err, ErrTooLarge)`), `TestPlumbLineOffsetsEveryEdge` (R at 0.5 → `(-0.5,-0.5) (10.5,-0.5) (10.5,8.5) (-0.5,8.5)` relative to `(0,0)`, area 99; R clockwise gives the same polygon with the edge mapping), `TestPlumbLineRefusesAClosedNotch` (a U outline `(0,0) (10,0) (10,8) (6,8) (6,2) (4,2) (4,8) (0,8)`, overhang 1.5 → `overhang_closes_outline`, edges 3 and 5), `TestPlumbLineSharpTip` (triangle `(0,0) (20,0) (0,3.5)` (tip about 10°), overhang 3 → either a ring that passes `checkOutline` or `overhang_closes_outline`).
- [ ] **Step 2: Run** `go test ./geometry -run 'TestCheckRoof|TestPlumbLine'`. Expected: FAIL, undefined `checkRoof`.
- [ ] **Step 3: Implement** both functions in `geometry/roof_check.go`. Reuse `checkOutline`, `checkOrigin("eaves", …)`, `checkLayers`, `tooLarge`.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): check roof inputs and offset the plumb line (#161)`.

### Task 2: Weighted straight skeleton

**Files:**
- Create: `geometry/skeleton.go`
- Test: `geometry/skeleton_test.go`

**Interfaces:**
- Consumes: the counter-clockwise ring from `plumbLine`; `budget` from `geometry/budget.go`.
- Produces:
  - `type skelFace struct { Edge int; Poly [][3]float64 }`: the region swept by edge `Edge`, a counter-clockwise plan polygon whose points carry their height as `z = time × tan(Pitch)` above the ring (the face starts with the edge itself at z = 0).
  - `type skelArc struct { A, B [3]float64; Left, Right int }`: an inner arc between the faces of edges `Left` and `Right`.
  - `func weightedSkeleton(ring [][2]float64, speed []float64, slope float64, b *budget) ([]skelFace, []skelArc, error)` with `speed[i]` 1 for a sloped edge and 0 for a gable; `slope = tan(Pitch)`. It returns faces only for sloped edges. A gable face with plan area over `minDim²` is refused `*RoofError{ReasonRoofStep, Edge: gable, Other: -1}`. The work (each event, each queue step) is charged to `b`; running out is `*RoofError{ReasonRoofTooComplex, Edge: -1, Other: -1}`.
- Method: Felkel–Obdržálek with weights. Wavefront vertices move with velocity from their two edges' speeds; edge events (an edge shrinks to 0) and split events (a reflex vertex hits an opposite edge). Process events with equal time (within a tolerance relative to the outline's size) together, so a rectangle's two ridge ends and a square's apex resolve as one event. A vertex between two collinear sloped edges moves along their shared normal.

- [ ] **Step 1: Write the failing tests** with a test helper `skelLines(faces, arcs)` that returns the arcs as plan segments with heights:
  - `TestSkeletonRectangleHip`: R, all speed 1, slope s: 4 faces; face areas sum to 80; one horizontal arc (4,4)→(6,4) at z 4s; four arcs from the corners.
  - `TestSkeletonSquarePyramid`: 8 × 8 square: 4 faces, all arcs end at (4,4) z 4s, no horizontal arc (Review Focus 1).
  - `TestSkeletonL`: L, all speed 1: the arcs of "Hand values" (ridges, valley, hips) with their end points, to 1e-9.
  - `TestSkeletonEqualArmCrossGable`: E with speed 0 on edges 1 and 4: the ridges y=3 and x=3 meet at (3,3); valley (6,6)→(3,3); hip (0,0)→(3,3); faces only for edges 0, 2, 3, 5.
  - `TestSkeletonMono`: R with speed 0 on 1, 2, 3: one face, the rectangle, its far edge at z 8s; L mono likewise one face of area 156.
  - `TestSkeletonRefusesAStep`: L with speed 0 on edge 2: `roof_step`, Edge 2.
  - `TestSkeletonNearlyStraightCorner`: R with a point `(5, 0.0001)` added on edge 0: a result whose face areas sum to the ring area within 1e-9 relative, or `roof_too_complex` (Review Focus 2).
  - `TestSkeletonBudget`: a 256-point zigzag outline with a budget of 100 units: `roof_too_complex`; with the element budget: builds.
  - Every successful case also checks: each face point's z equals `slope ×` its distance to its edge line (planarity), and face plan areas sum to the ring's area.
- [ ] **Step 2: Run** `go test ./geometry -run TestSkeleton`. Expected: FAIL, undefined `weightedSkeleton`.
- [ ] **Step 3: Implement** `weightedSkeleton` in `geometry/skeleton.go`.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): weighted straight skeleton for roofs (#161)`.

### Task 3: BuildRoofLayers

**Files:**
- Create: `geometry/build_roof.go`
- Test: `geometry/build_roof_test.go`

**Interfaces:**
- Consumes: `checkRoof`, `plumbLine` (Task 1), `weightedSkeleton` (Task 2), `triangulatePolygon`, `newElement`, `isClosedManifold`, `meshVolume`.
- Produces:
  - `type RoofQuantities struct { SlopeArea, PlanArea, Volume float64; Eaves, Ridge, Hip, Valley, Verge, Top float64 }`
  - `type Roof struct { Element Element; Quantities RoofQuantities; Planes []Face }`
  - `func BuildRoofLayers(outline [][2]float64, spec RoofSpec, layers []float64) ([]Roof, error)` (doc comment in the style of `BuildSlabLayers`).
- Method: build the skeleton once, then each layer k: the top surface is the skeleton heights + `Eaves + (total − above_k)/c` − overhang drop (`Overhang × tan(Pitch)`, since the plumb line sits that far below the outline line); bottom = top − `t_k/c`. Mesh: each face polygon triangulated, top and bottom, plus vertical side quads on each plumb-line edge (a gable side follows the face edges along it). Vertices float32 local to `outline[0]`, placed with `newElement`. `Planes`: one `Face` per sloped face: `Origin` the face's start on its eaves edge, `U` along the eaves edge (ring direction), `V` up the slope, `Normal` = U × V (up), `Outline` in (u, v). Line lengths from the arcs: horizontal → `Ridge`; else fold direction from the two faces' normals (outward → `Hip`, inward → `Valley`); faces of collinear edges → skipped; gable plumb-line edges: the top's sloping pieces → `Verge`, level pieces → `Top`.

- [ ] **Step 1: Write the failing tests** (values from "Hand values", tolerance 1e-9 relative):
  - `TestRoofRectangleHip`, `TestRoofRectangleGable`, `TestRoofRectangleMono` (includes the high edge's top z), `TestRoofLHip`, `TestRoofEqualArmCrossGable`, `TestRoofLMono`, `TestRoofLStepRefused`.
  - `TestRoofOverhang`: R hip, overhang 0.5: PlanArea 99, SlopeArea 99/c, underside at z = 3 above the outline line (sample the bottom plane at (0,0)).
  - `TestRoofLayersSum`: R hip, layers {0.05, 0.15}: volumes sum to the one-layer 0.2 roof's volume; layer 1's top is layer 0's bottom; each layer's line lengths equal.
  - `TestRoofMeshAgrees`: for every case above, each layer's mesh `isClosedManifold` and its `meshVolume` agrees with `Volume` within float32 rounding (use `meshAgrees` from `build_fuzz_test.go`).
  - `TestRoofWindingAndGrid`: R and L clockwise give identical quantities and the same gables; R moved by (500000, 4600000) gives identical quantities and identical local vertices (Review Focus 3).
  - `TestRoofSteepPitch`: a 50 × 10 m rectangle hip at 75°: builds, Ridge 40 (Review Focus 4).
- [ ] **Step 2: Run** `go test ./geometry -run TestRoof`. Expected: FAIL, undefined `BuildRoofLayers`.
- [ ] **Step 3: Implement** in `geometry/build_roof.go`.
- [ ] **Step 4: Run** `go test ./geometry`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): build pitched roofs in layers (#161)`.

### Task 4: Fuzz target

**Files:**
- Modify: `geometry/build_fuzz_test.go` (add `FuzzBuildRoofLayers`)
- Create: `geometry/testdata/fuzz/FuzzBuildRoofLayers/` seeds (R, L, E)

**Interfaces:**
- Consumes: `BuildRoofLayers`, `meshAgrees`.

- [ ] **Step 1: Write** `FuzzBuildRoofLayers(f)` over six points (skip NaN and |c| > 1e3), a gable bitmask (uint8, low 6 bits), pitch and overhang. On success: each layer passes `meshAgrees`, `SlopeArea` = `PlanArea / cos(Pitch)` within 1e-9 relative, line lengths ≥ 0 and finite. On error: it is a `*RoofError` or an `ErrTooLarge` or an outline refusal, never a panic.
- [ ] **Step 2: Run** `go test ./geometry -run '^$' -fuzz FuzzBuildRoofLayers -fuzztime 120s`. Expected: no failure. Commit any crasher as a seed with its fix, each fix with a failing test first.
- [ ] **Step 3: Commit** `test(geometry): fuzz pitched roofs (#161)`.

### Task 5: BuildPlan, preview and docs

**Files:**
- Modify: `geometry/plan.go`, `geometry/preview.go`, `docs/guides/builders.md`, `CHANGELOG.md` (Unreleased / Added)
- Test: `geometry/plan_test.go` (or the existing plan test file), `geometry/preview_svg_test.go`, `geometry/testdata/preview/roof-l.svg`

**Interfaces:**
- Produces: `type PlanRoof struct { ID string; Outline [][2]float64; Pitch float64; Gables []int; Overhang, Eaves, Thickness float64; Layers []float64 }`; `Plan.Roofs []PlanRoof`; `type BuiltRoof struct { ID string; Roof Roof }`; `Built.Roofs []BuiltRoof`. IDs like slabs (`r1`, `r1/j` for layer j); roofs count toward `maxPlanItems` and `maxPlanElements`; IDs checked with the others; `Thickness` xor `Layers` (`errThicknessOrLayers`).

- [ ] **Step 1: Write the failing tests** `TestBuildPlanRoof` (one R hip roof item `r1` with two layers → `Built.Roofs` IDs `r1/0`, `r1/1`, GlobalIDs equal), `TestBuildPlanRoofRefusal` (L with gable 2 → `*ItemError{Item: "r1"}` wrapping `*RoofError{ReasonRoofStep}`), `TestBuildPlanRoofCounts` (items over the cap refused), and a preview golden `roof-l.svg` showing the plumb line and the ridge, hip and valley lines of the L hip roof.
- [ ] **Step 2: Run** `go test ./geometry -run 'TestBuildPlanRoof|TestPreview'`. Expected: FAIL.
- [ ] **Step 3: Implement** in `plan.go` and `preview.go`; document `BuildRoofLayers` and `PlanRoof` in `docs/guides/builders.md`; add the CHANGELOG entry.
- [ ] **Step 4: Run** `go test ./geometry`. Expected: PASS (regenerate goldens only through the repo's update flag, and read the diff).
- [ ] **Step 5: Commit** `feat(geometry): roofs in BuildPlan and the plan preview (#161)`.

### Task 6: wasm contract

**Files:**
- Modify: `internal/wasmapi/build.go`, `internal/wasmapi/answer.go`, `wasm/goifc-contract.schema.json`, `docs/guides/wasm.md`, the Node tests under `wasm/test/`
- Test: `internal/wasmapi/build_test.go`, `internal/wasmapi/testdata/build/roof.{request,answer}.json` + `roof.glb`, `roof-layers.*`, `refused-roof.*`

**Interfaces:**
- Consumes: `PlanRoof`, `Built.Roofs`, `RoofError`.
- Produces: request `"roofs"`: `{id, outline, pitch, gables, overhang, eaves, thickness | layers}`; answer `"roofs"`: `{id, quantities: {slopeArea, planArea, volume, eaves, ridge, hip, valley, verge, top}, planes: [faceOut]}`; a refused roof: `Error{Code: string(RoofError.Reason), Item, Edge (and Other when ≥ 0)}`.

- [ ] **Step 1: Write the failing tests**: goldens `roof` (R hip), `roof-layers` (E cross-gable, two layers), `refused-roof` (L gable 2 → `roof_step`, `edge` 2); a test that every `RoofReason` maps to its code; the schema test covers `roofs`; a Node test builds R and reads `ridge` 2.
- [ ] **Step 2: Run** `go test ./internal/wasmapi && make wasm-test`. Expected: FAIL.
- [ ] **Step 3: Implement** the adapter, the error mapping, the schema and `docs/guides/wasm.md`.
- [ ] **Step 4: Run** `go test ./... && make lint && make wasm-test && make wasm-growth`. Expected: PASS, 0 lint issues, growth under 50 KiB gzipped (over it: stop and ask the owner before using the `wasm-growth` label).
- [ ] **Step 5: Commit** `feat(wasm): build pitched roofs (#161)`.
