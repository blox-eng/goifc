# Parametric builders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `geometry.BuildWall`, `BuildRoomWalls` and `BuildSlab` build an `Element`, exact quantities and the measured face from plain numbers, with refusals that name the opening and the reason (#91).

**Architecture:** Quantities are computed from the parameters. The mesh is a cross-check: each face is cut on one global (u, v) grid around the openings, so every edge is shared by exactly two triangles and the shell is closed. Room walls get mitred ends from the crossing of neighbouring offset lines. New code only; the IFC import path is untouched.

**Tech Stack:** Go (module `github.com/blox-eng/goifc`), package `geometry`; GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-11-11-parametric-builders-design.md`

## Global Constraints

- Signatures, types, reason strings and rules exactly as in the spec's "The contract" and "Rules".
- Units are meters. Errors, never panics. Nothing is clamped.
- Bounds: ≤ 2,048 outline points; ≤ 256 openings per wall; every coordinate within 1,000 m of the first point; every first point within 1e7 m of the origin; height, thickness and every edge ≥ 1 mm.
- Synthetic fixtures only. No private or downstream names in code, tests, commits or CHANGELOG. Grep the staged diff for private names before each commit.
- Signed Conventional Commits (`git commit -S`), never `--no-verify`, no WIP commits.
- Pure Go, `CGO_ENABLED=0`; no `os`/`net` use in the new files; must build with `GOOS=js GOARCH=wasm`.
- Gates per task: `go test ./geometry/`; at the end `go test ./...`, `go test -race ./geometry/`, `make lint`, `make parity`.

## Review Focus

1. **A wall not on a world axis** (edge at 30°): quantities equal the axis-aligned wall's, mesh volume still equals NetVolume. Test in Task 4.
2. **Far from the origin** (edge start at (1e6, 1e6), base 3.0): mesh volume matches within tolerance, BBox z starts at 3.0. Test in Task 4.
3. **Openings given out of u order**: same quantities and same closed mesh as sorted input; refusal indices still point at the caller's indices. Test in Task 4.
4. **A clockwise room with SideRight** measures the interior the same as the CCW room with SideLeft. Test in Task 5.
5. **256 openings on one wall**: builds within the test timeout, mesh closed. Test in Task 4.

---

### Task 1: Exported 2D helpers

**Files:**
- Create: `geometry/polygon2d.go`, `geometry/polygon2d_test.go`

**Interfaces:**
- Produces: `PolygonArea2D(poly [][2]float64) float64` (unsigned), `PointInPolygon2D(p [2]float64, poly [][2]float64) bool`, `Triangulate2D(poly [][2]float64) ([]uint32, error)`; unexported `errTooManyPoints`, const `maxOutlinePoints = 2048`.

- [ ] **Step 1: Write the failing tests** `TestPolygonArea2DIsUnsigned` (CCW and CW 5×4 rectangle both 20.0; the L outline `(0,0),(4,0),(4,2),(2,2),(2,4),(0,4)` → 12.0), `TestPointInPolygon2D` (L outline: (1,1) true, (3,3) false), `TestTriangulate2DCoversTheArea` (L outline: Σ triangle areas == 12.0 within 1e-9), `TestTriangulate2DRefuses` (2 points, a bowtie, 2,049 points → non-nil error each; 2,048-point regular polygon → ok).
- [ ] **Step 2: Run** `go test ./geometry/ -run 'Polygon2D|PointInPolygon2D|Triangulate2D'` — Expected: FAIL, undefined.
- [ ] **Step 3: Implement** as thin wrappers over `polygonArea2D`, `pointInPolygon`, `triangulatePolygon(poly, newBudget())`; `ok == false` becomes an error that says the polygon could not be triangulated within the work budget. Doc comments say what each refuses.
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): export 2D polygon area, point-in-polygon and triangulation (#91)`

### Task 2: BuildSlab

**Files:**
- Create: `geometry/build.go` (shared types and input checks), `geometry/build_slab.go`, `geometry/build_slab_test.go`

**Interfaces:**
- Consumes: Task 1 helpers.
- Produces: `Face`, `Slab`, `SlabQuantities` (spec); `BuildSlab(outline [][2]float64, z, thickness float64) (Slab, error)`; unexported `checkOutline(outline [][2]float64) error` (count, finite, bounds, repeated points, crossing edges, zero area), `checkDim(name string, v float64) error` (finite, ≥ 1 mm, ≤ 1,000 m), `newElement(verts []float32, tris []uint32, origin [3]float64) Element` (sets Placement translation, world BBox via `worldAABB`, `SourceExtrude`).

- [ ] **Step 1: Write the failing tests**
  - `TestBuildSlabOracle`, table rows with the formula as a comment: 5×4 rect, t 0.20 → Area 20.0 (5·4), Perimeter 18.0 (2·(5+4)), Volume 4.0 (20·0.2); L outline, t 0.25 → 12.0 (4·4 − 2·2), 16.0, 3.0. Tolerance 1e-9.
  - `TestBuildSlabMeshMatches`: `meshVolume` ok and equals Volume within 1e-6 relative; `isClosedManifold` true.
  - `TestBuildSlabPlacement`: outline offset by (1e6, 2e6), z = 6.0, t = 0.2 → BBoxMax z 6.0, BBoxMin z 5.8, Top face Origin = (1e6, 2e6, 6.0), U (1,0,0), V (0,1,0), Normal (0,0,1), Outline = outline minus outline[0].
  - `TestBuildSlabRefuses`: NaN, Inf, t 0, t 0.0005, < 3 points, repeated point, bowtie, a point 1,001 m from the first, first point at 1.1e7 → error, no panic.
- [ ] **Step 2: Run** `go test ./geometry/ -run BuildSlab` — Expected: FAIL, undefined.
- [ ] **Step 3: Implement.** Top cap at local z 0, bottom at −thickness (Placement z = `z`); caps from `triangulatePolygon` (bottom reversed), one quad per outline edge for the sides; outward winding. Quantities from `polygonArea2D` and the outline length.
- [ ] **Step 4: Run** `go test ./geometry/ -run 'BuildSlab|Polygon2D'` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): build a slab from an outline, z and thickness (#91)`

### Task 3: BuildWall quantities and refusals

**Files:**
- Create: `geometry/build_wall.go`, `geometry/build_wall_test.go`, `geometry/side_test.go`
- Modify: `geometry/build.go` (Side, Opening, PlacementReason, PlacementError, WallQuantities, Wall)

**Interfaces:**
- Consumes: `checkDim`, `newElement` (Task 2).
- Produces: the contract's wall types; `(e *PlacementError) Error() string` → `wall 0 opening 1: too_wide`; `Side.MarshalText/UnmarshalText` ("left"/"right"); unexported `wallSpan struct{ length, otherStart, otherEnd float64; squareStart, squareEnd bool }` and `checkOpenings(wall int, span wallSpan, height float64, ops []Opening) error` (returns `errors.Join` of `*PlacementError` or the first input error), `wallQuantities(span wallSpan, height, thickness float64, ops []Opening) WallQuantities`. Task 5 reuses all three.

- [ ] **Step 1: Write the failing tests**
  - `TestBuildWallOracle` rows (formula in each row): the #91 door wall — edge (0,0)→(2.40,0), base 0, h 2.60, t 0.10, SideLeft, door {0.50, 0, 0.80, 2.00} → MeasuredGross 6.24 (2.40·2.60), MeasuredNet 4.64 (6.24 − 0.80·2.00), Other same, Reveal 4.80 (2·0.80 + 2·2.00 − 0.80), GrossVolume 0.624, NetVolume 0.464. Window wall — edge length 3.00, h 2.70, t 0.20, window {1.00, 0.90, 1.20, 1.20} → 8.10, 6.66, reveal 4.80 (four sides), 1.62, 1.332. Solid wall, no openings → net == gross.
  - `TestBuildWallRefusesEachReason`, one row per reason with `errors.As` asserting `Wall 0`, the opening index and the reason: `too_wide` {2.00, 0, 0.50, 2.0} on 2.40 (ends 2.50) and {1.60, 0, 0.80, 2.0} (touches the end) and {0, 0, 0.5, 2.0} (touches the start); `above_wall` {0.5, 0.9, 0.8, 1.70} (2.60 touches the top); `overlaps` two openings sharing an edge, and two crossing.
  - `TestBuildWallReturnsAllRefusals`: openings 0 and 2 bad, 1 good → `errors.Join` holds exactly two `*PlacementError` with indices 0 and 2, in that order.
  - `TestBuildWallInputErrors`: NaN offset, width 0, negative sill, height < 1 mm, zero-length edge, 257 openings, Side(0), Side(3) → plain error, not a `*PlacementError`, no panic.
  - `TestSideText` round-trips "left"/"right" and refuses "Left", "", "centre".
- [ ] **Step 2: Run** `go test ./geometry/ -run 'BuildWall|SideText'` — Expected: FAIL, undefined.
- [ ] **Step 3: Implement** the types, `checkOpenings` in the spec's order with strict "inside" tests (touching refuses as its reason; one named touch tolerance with its reason in a comment), `wallQuantities` per the spec formulas (GrossVolume = trapezoid plan area · height for mitres; for `BuildWall` the rectangle). `BuildWall` returns `Wall{Quantities, Measured}` and an empty `Element` for now; the mesh lands in Task 4.
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): measure a wall and refuse misplaced openings (#91)`

### Task 4: BuildWall mesh, face data and GLB

**Files:**
- Modify: `geometry/build_wall.go`
- Create: `geometry/wall_mesh.go`, `geometry/wall_mesh_test.go`

**Interfaces:**
- Consumes: Task 3 types, `wallSpan`.
- Produces: unexported `wallMesh(span wallSpan, height, thickness float64, ops []Opening) (verts []float32, tris []uint32)` in wall-local (u, w, v) then mapped to local (x, y, z) by `BuildWall`; Task 5 reuses it with mitred spans.

- [ ] **Step 1: Write the failing tests**
  - `TestWallMeshIsClosedAndMatches`: for every Task 3 oracle row, `isClosedManifold` true, `meshVolume` == NetVolume within 1e-6 relative.
  - `TestWallDerivedAreaMatches`: axis-aligned door wall in a `Scene` with GlobalID "w1" → `DerivedQuantities()["w1"].Area` == MeasuredNet 4.64 and `.Volume` == 0.464 within 1e-6.
  - `TestWallFaceData`: door wall, base 3.0 → Measured Origin (0,0,3.0), U (1,0,0), V (0,0,1), Normal points away from the body (SideLeft grows to −y... assert Normal · (body centre − Origin) < 0), Outline [[0,0],[2.40,0],[2.40,2.60],[0,2.60]], Openings == the input door; BBoxMin z 3.0, BBoxMax z 5.6.
  - `TestWallGLB`: scene of the door wall and the Task 2 slab → `WriteGLB` returns nil and the output starts with `glTF`.
  - Review Focus 1–3, 5: `TestWallRotated` (edge at 30°, same quantities, volume matches); `TestWallFarFromOrigin` (start (1e6,1e6), base 3.0); `TestWallOpeningOrder` (door + window given in reverse u order → same quantities, closed mesh, a refusal on the second input still reports index 1); `TestWallManyOpenings` (256 openings 0.05 wide, 0.10 apart on a 30 m wall → closed, matches).
- [ ] **Step 2: Run** `go test ./geometry/ -run Wall` — Expected: FAIL on mesh and face assertions.
- [ ] **Step 3: Implement `wallMesh`.** One global grid so no vertex sits inside a neighbour's edge:

  ```
  U = sorted unique {0, L, s, e, every opening's offset and offset+width}
  V = sorted unique {0, H, every opening's sill and sill+height}
  measured face (w=0):  every cell of U∩[0,L] × V not inside an opening → quad
  other face (w=t):     every cell of U∩[s,e] × V not inside an opening → quad
  reveals:              per opening, per grid interval along its edge → quad across w∈[0,t]
                        (no bottom strip when sill == 0)
  top (v=H), bottom (v=0): per u-interval of U, the region between the measured line
                        (if the interval is inside [0,L]) and the other line (if inside
                        [s,e]), bounded by the end lines → quad, or a triangle at a mitre end;
                        bottom skips u-intervals inside a door (sill == 0)
  end caps:             the quad from (0,0) to (s,t) and from (L,0) to (e,t), full height
  ```
  Share vertices through a map keyed by grid index and side, winding outward. Measured face Normal = −(growth direction).
- [ ] **Step 4: Run** `go test ./geometry/ -run 'Wall|BuildSlab'` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): mesh a wall around its openings as a closed shell (#91)`
- [ ] **Step 6: Note the sha.** BuildWall and BuildSlab both pass from this commit; record it in the ledger for downstream pinning.

### Task 5: BuildRoomWalls

**Files:**
- Create: `geometry/build_room.go`, `geometry/build_room_test.go`

**Interfaces:**
- Consumes: `checkOutline`, `checkOpenings`, `wallQuantities`, `wallMesh`, `newElement`.
- Produces: `BuildRoomWalls(outline [][2]float64, base, height, thickness float64, side Side, openings [][]Opening) ([]Wall, error)`; unexported `roomSpans(outline [][2]float64, thickness float64, side Side) ([]wallSpan, error)`.

- [ ] **Step 1: Write the failing tests**
  - `TestBuildRoomWallsOracle` (formula per row): rect room CCW (0,0),(2.40,0),(2.40,1.80),(0,1.80), h 2.60, t 0.10, SideLeft → Σ MeasuredGross 21.84 (2·(2.40+1.80)·2.60), Σ OtherGross 23.92 (2·(2.60+2.00)·2.60), Σ GrossVolume 2.288 ((2.60·2.00 − 2.40·1.80)·2.60). L room (Task 1 outline), h 2.50, t 0.10 → Σ MeasuredGross 40.0 (16·2.5), Σ OtherGross 42.0 (16.8·2.5), Σ GrossVolume 4.1 ((4.2² − 2² − 12)·2.5).
  - `TestBuildRoomWallsMeshMatches`: every wall closed, Σ `meshVolume` == Σ NetVolume within 1e-6 relative.
  - `TestBuildRoomWallsInCorner`: L room, wall 2 ((4,2)→(2,2), other face spans u ∈ [−0.10, 1.90]) with {1.80, 0.5, 0.15, 1.0} → `in_corner` (ends 1.95 > 1.90), Wall 2, Opening 0; {1.70, 0.5, 0.15, 1.0} → no error.
  - `TestBuildRoomWallsClockwise` (Review Focus 4): the rect room reversed with SideRight → same Σ quantities.
  - `TestBuildRoomWallsStraightThrough`: the rect room with an extra point (1.20, 0) → 5 walls, Σ quantities unchanged, the two pieces meet at a square cut, mesh closed.
  - `TestBuildRoomWallsRefuses`: an outline with an 8° spike (mitre > 10·t); a U-shaped notch 0.15 wide with t 0.10 (bodies overlap); `len(openings) != len(outline)`; outline errors from `checkOutline` → error, no panic.
- [ ] **Step 2: Run** `go test ./geometry/ -run BuildRoomWalls` — Expected: FAIL, undefined.
- [ ] **Step 3: Implement.** For each edge, offset its line by `thickness` to the growth side; `s`, `e` come from crossing it with the neighbours' offset lines, projected onto the edge's u; a parallel neighbour (180°) gives a square cut at that end. Refuse when the mitre point is > 10·t from the corner, when `e − s ≤ 0`, or when the opposite-face ring crosses itself (bounded by `maxOutlinePoints`, charged to a work budget). Each wall's Element is placed at outline[i] with z = base.
- [ ] **Step 4: Run** `go test ./geometry/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): build the walls of a room with mitred corners (#91)`

### Task 6: Fuzz targets

**Files:**
- Create: `geometry/build_fuzz_test.go`
- Modify: `.github/workflows/ci.yml` (fuzz job), `.github/workflows/fuzz-nightly.yml` (matrix)

**Interfaces:**
- Consumes: the three builders.

- [ ] **Step 1: Write** `FuzzBuildWall`, `FuzzBuildRoomWalls`, `FuzzBuildSlab` with the oracle rows as seeds. Property: no panic; on success every quantity is finite and ≥ 0, the mesh is closed and its volume equals NetVolume (Volume for a slab) within 1e-5 relative; on error, nothing returned is used. Inputs that break the bounds must error, not reach the property.
- [ ] **Step 2: Run** `go test ./geometry/ -run Fuzz -fuzz FuzzBuildWall -fuzztime 60s` (and the other two). Expected: PASS. A crasher is a bug in Tasks 2–5: fix it there with a regression test first.
- [ ] **Step 3: Wire CI.** One step per target in the `ci.yml` fuzz job (`.github/scripts/fuzz-smoke.sh ./geometry/ <Target> 60s`), three rows in the nightly matrix.
- [ ] **Step 4: Commit** `test(geometry): fuzz the wall, room and slab builders (#91)`

### Task 7: WebAssembly CI, CHANGELOG, docs

**Files:**
- Modify: `.github/workflows/ci.yml`, `CHANGELOG.md`, `README.md` (API list, if it lists geometry entry points), `geometry/doc.go`

- [ ] **Step 1: Add a `wasm` job** to `ci.yml`: same checkout/setup-go pins as `test`, `needs: [lint, changes]`, `if: needs.changes.outputs.code == 'true'`, `env: CGO_ENABLED: '0'`, run `GOOS=js GOARCH=wasm go build ./...`, with a comment giving the reason (a browser runs the same builders). Not a required check; say so in the PR.
- [ ] **Step 2: Verify locally** `GOOS=js GOARCH=wasm go build ./...` — Expected: exit 0. `actionlint` if installed.
- [ ] **Step 3: CHANGELOG** under `## Unreleased` → `### Added`: the three builders, `Face`, the refusal reasons, the Side convention, the 2D helpers, the wasm job; and what is not supported yet (T-joins, niches, storeys).
- [ ] **Step 4: Full gates** `go test ./...`, `go test -race ./geometry/`, `make lint`, `make parity` — Expected: all green.
- [ ] **Step 5: Commit** `ci: prove goifc builds for GOOS=js GOARCH=wasm (#91)` and `docs: changelog for the parametric builders (#91)`.

### Task 8: Niches (spec addendum)

**Files:**
- Modify: `geometry/build.go` (Opening.Depth), `geometry/build_wall.go` (checks, quantities, NicheBack), `geometry/wall_mesh.go` (depth layers, pocket), `geometry/build_fuzz_test.go`, `CHANGELOG.md`
- Create: `geometry/niche_test.go`

**Interfaces:**
- Consumes: `checkOpenings`, `wallQuantities`, `meshWall`, `wallSpan` (signatures unchanged; checkOpenings also takes thickness).
- Produces: `Opening.Depth`, `WallQuantities.NicheBack`.

- [ ] **Step 1: Write the failing tests** `TestNicheOracle` (the addendum row, both quantities and a closed mesh whose volume is 0.6048), `TestNicheInputErrors` (depth 0.0005, depth = thickness − 0.0005, NaN depth, sill 0 → plain errors), `TestNicheNearConcaveCorner` (L room wall 2: a niche 0.04 deep at {1.80, 0.5, 0.15, 1.0} builds — at w = 0.04 the wall reaches u = 1.96 — while the same through opening is `in_corner`), `TestNicheBesideThroughOpening` (door + niche on one wall: closed, volume matches).
- [ ] **Step 2: Run** `go test ./geometry/ -run Niche` — Expected: FAIL, unknown field Depth.
- [ ] **Step 3: Implement** per the addendum.
- [ ] **Step 4: Run** `go test ./geometry/` and fuzz each wall target 120 s — Expected: PASS.
- [ ] **Step 5: Commit** `feat(geometry): niches that stop short of the opposite face (#91)`

## After the last task

Final whole-branch review, then ask before: filing the three follow-up issues (T-joins, niches, storeys), pushing, opening the PR.
