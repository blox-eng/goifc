# Roof precision: drafting noise and the mesh weld (#169, #170): plan

Spec: docs/superpowers/specs/2026-11-25-roof-precision-design.md
Spec approved: 2026-11-25
Test command: make test
Plan approved: no
Next: builder:build

## Files

- `geometry/build_roof.go` (change): `joinCollinear` joins by an absolute distance; `PlanArea` comes from the joined ring; the cover check; the weld threshold in `roofShellOf`.
- `geometry/build_roof_test.go` (change): the regression tests; `roofTrue` loses `gridSnap`; `TestRoofNearCollinearEavesPoint` wants the joined roof.
- `geometry/testdata/fuzz/FuzzBuildRoofLayers/a8d904e55433258a`, `…/0df01d4429048d88` (create): the #169 seeds, verbatim from the issue.
- `CHANGELOG.md` (change): one Fixed line per issue.

## Task 1: a roof over a µm jog has planes that cover it (#169)

- **Files:** `geometry/build_roof.go`, `geometry/build_roof_test.go`, the two seed files, `CHANGELOG.md`.
- **Test first:**
  - Add the two seed files, byte for byte as in the #169 issue body.
  - `TestRoofJogJoins`: outline `(-87,0),(10,0),(10,4),(10.00001,8),(0,8),(0,1)`, gables `0,3,4,5`, pitch 30, overhang 0.5, one 0.2 m layer. Assert `roofTrue(t, r, ringSize(outline), 30)` passes, and that the planes' plan areas sum to `PlanArea` within `1e-9 × PlanArea`.
  - Change `TestRoofNearCollinearEavesPoint`: a point 1e-7 m off the edge is now noise and joins. Want 4 planes (was 5). The other assertions stay.
- **Run:** `go test ./geometry -run 'FuzzBuildRoofLayers|TestRoofJogJoins|TestRoofNearCollinearEavesPoint' -count=1`. Before: both seeds fail in `roofTrue` ("planes cover …, slope area …"), `TestRoofJogJoins` fails the same way, and `TestRoofNearCollinearEavesPoint` gets 5 planes. After: all pass.
- **Do:**
  - In `joinCollinear`, drop ring point `k` between two sloped edges when it lies between its neighbours (today's dot test) and its distance from the line `p`–`k+1` is under `gridSnap`: `|cross2D(a,b,c)| < gridSnap × |c−a|`. Drop the `area` parameter, and update its caller in `skeleton_test.go:427`. Rewrite the comment to name the rule: a point under the mesh grid off the line is drafting noise.
  - `onLine` stays as it is: `checkRoof`'s gable test still uses it.
  - In `BuildRoofLayers`, compute `planArea := polygonArea2D(ring)` after `joinCollinear`.
  - CHANGELOG, Unreleased/Fixed: "A roof over an outline with a point less than 0.1 mm off a straight edge now has planes that cover it; the point counts as drafting noise."
- **Done when:** the Run command passes, and `make test` passes.
- **Commit:** `fix(geometry): join a roof edge's sub-0.1 mm jog, so its planes cover it (#169)`

## Task 2: a roof whose faces do not cover its outline is refused

- **Files:** `geometry/build_roof.go`, `geometry/build_roof_test.go`.
- **Test first:** `TestRoofFacesCover`: the `roofR` rectangle's ring and its `weightedSkeleton` faces. `facesCover(ring, faces)` is nil. With the last face removed, it is a `*RoofError` with `ReasonRoofTooComplex`.
- **Run:** `go test ./geometry -run TestRoofFacesCover -count=1`. Before: it does not compile, because `facesCover` is undefined. After: it passes.
- **Do:**
  - Add `facesCover(ring [][2]float64, faces []skelFace) error`. It sums each face's plan area (`polygonArea2D` of its x, y) and compares the sum with `polygonArea2D(ring)`.
  - The tolerance is the one `roofTrue` uses for planes: `1e-8 × area + skelPointTol × size × perimeter`, where `size` is the ring's bounding-box diagonal (the skeleton's `s.size`, `skeleton.go:140`; `ringSize` is a test helper and stays in the test) and `perimeter` is the faces' total edge length.
  - Outside the tolerance it returns `errSkelTooComplex()`.
  - Call it in `BuildRoofLayers` right after `weightedSkeleton`. Its error goes through `callerEdges` as the skeleton's does.
- **Done when:** `TestRoofFacesCover` passes, and `make test` passes (no current roof is refused).
- **Commit:** `fix(geometry): refuse a roof whose faces do not cover its outline`

## Task 3: every roof mesh point lies on its planes (#170)

- **Files:** `geometry/build_roof.go`, `geometry/build_roof_test.go`, `CHANGELOG.md`.
- **Test first:**
  - In `roofTrue`, set `planTol := round` (was `gridSnap + round`). Update its comment: the weld no longer moves a point.
  - `TestRoofWeldOnPlanes`: outline `(0,0),(10,0),(12,4),(10,8.000316),(0,8),(-2,4)`, no gables, pitch 75, one 0.2 m layer. Call `roofTrue(t, r, ringSize(outline), 75)`.
- **Run:** `go test ./geometry -run 'TestRoofWeldOnPlanes|TestRoof|FuzzBuildRoofLayers' -count=1`. Before: `TestRoofWeldOnPlanes` fails with "the mesh is 3.6e-4 off it" (about). After: all pass.
- **Do:**
  - In `roofShellOf`, weld points closer than `weld := 0x1p-20 × max(1, extent)` instead of `gridSnap`. Here `extent` is the largest `|x|`, `|y|` or `|z|` of the face points: 8 float32 steps at the roof's size. Use `weld` for both the grid cell and the distance.
  - Keep the comment's reason, the pinch guard, and say the threshold is what float32 cannot keep apart.
  - CHANGELOG, Unreleased/Fixed: "Every point of a roof's mesh now lies on its sloped planes within float32 rounding; before, a point could sit up to 0.37 mm off at 75°."
- **Done when:** the Run command passes, and `make test` passes (the fuzz corpus included).
- **Commit:** `fix(geometry): weld a roof mesh only where float32 cannot tell points apart (#170)`

## Task 4: review the whole change

Load `builder:review` on `fix/169-170-roof-precision` against `origin/main`. Give it the spec `docs/superpowers/specs/2026-11-25-roof-precision-design.md`, this plan and the test command `make test`. Also run `go test ./geometry -run FuzzBuildRoofLayers -fuzz FuzzBuildRoofLayers -fuzztime 5m` in the review worktree.

## Lens findings

- Engineering: `TestRoofNearCollinearEavesPoint` pinned the old rule, a 1e-7 m point keeping 5 planes. The approved ruling says such a point is noise. Task 1 changes the test and says so. No other change.
- Engineering: a gable a few µm off a sloped edge's line is neither joined nor refused (`checkRoof` uses the 1e-9 `onLine`). Task 2's cover check refuses such a roof if its faces fail to cover it. No change, because the spec keeps gables out of the join.
- Engineering: a tighter weld keeps points µm apart as two mesh points. The cap check in `roofShellOf` still refuses overlaps and pinches, so a bad mesh fails loudly. The fuzz run in Task 4 tests it at scale. No change.
- Engineering: the cover check costs O(points), next to the skeleton's cost. No change.
- Architecture (wasm contract): no field changes. `PlanArea` can shift by under 0.1 mm × the edge length on a noisy outline. The downstream viewer is told when this ships. No change.
- Design: no screen. The changelog says what a person sees. No change.

## Spec check

| Spec line | Task |
|---|---|
| Both #169 seeds build, and their planes cover `SlopeArea` | Task 1: the seeds and `TestRoofJogJoins` |
| The #170 hexagon has every mesh vertex on its planes within float32 rounding | Task 3: `TestRoofWeldOnPlanes` |
| A ring whose faces do not cover it is refused as `roof_too_complex` | Task 2: `TestRoofFacesCover` |
| The roof tests and the fuzz corpus pass with the tighter `roofTrue` | Task 3: the Run line; Task 4: the fuzz run |
| Anti-goal: a roof that builds with planes that miss or overlap its outline | Task 1, Task 2 |

## Cold read

Read as a builder with only this plan and the repository. Each name it uses exists: `joinCollinear`, `onLine`, `checkRoof`, `cross2D`, `gridSnap`, `skelPointTol`, `polygonArea2D`, `weightedSkeleton`, `errSkelTooComplex`, `callerEdges`, `roofShellOf`, `roofTrue`, `ringSize`, `roofR`, `skeleton_test.go:427`. One gap was closed: the plan now says where the seed files come from, and that `onLine` stays for the gable check.

## Changes during build
