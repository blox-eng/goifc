# Layered builders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `BuildWallLayers`, `BuildRoomWallLayers` and `BuildSlabLayers` return one `Wall` or `Slab` per layer, with exact per-layer quantities that add up to the single-solid element.

**Architecture:** A layer of a wall is a wall in its own right: its faces lie at depths d_k and d_k+1, and its span is the whole wall's span interpolated linearly in depth (the mitre point is linear in depth). Placement is checked once on the whole wall; the wall is then split into per-layer parts (span, shift, openings as they cut that layer) and each part goes through the existing `meshWall` / `wallQuantities`. Slabs reuse one triangulation for every layer.

**Tech Stack:** Go, standard library only; package `geometry`.

**Spec:** `docs/superpowers/specs/2026-11-13-layered-builders-design.md`

## Global Constraints

- Units: SI metres everywhere; layers are thicknesses in metres, `[]float64`, measured face first (walls) or top first (slabs).
- 1 to 16 layers (`maxLayers = 16`); each layer and their sum pass `checkDim` (finite, 1 mm to 1,000 m). Errors name the layer: `layer 2: thickness 0.0004 m is under the 1 mm minimum`.
- Placement refusals of a layered wall are exactly those of the single-solid builder with thickness = the sum, plus `layer_sliver` (last in the refusal order).
- "On a boundary" means within 1e-9 m.
- `maxMeshCells` bounds the sum of grid cells over all layers.
- One layer gives the same quantities, verts and tris as the single-solid builder.
- No cgo, no `os`/`net` in the geometry path (the wasm CI build must stay green).
- Fixtures are synthetic numbers. Signed Conventional Commits; grep each staged diff for private names before committing.
- Release v0.17.0 is cut by CI from the `feat:` commits; the CHANGELOG entry goes under `## Unreleased`.

## Review Focus

1. A niche whose depth is typed as a sum of layers (0.015 + 0.175) lands within float noise of a boundary — it must cut those layers through and put the back in the next layer, never `layer_sliver`. (Task 2, `TestBuildWallLayers_NicheOnBoundary`.)
2. A room on the SideRight (inward-growing) side: inner layers get shorter, and a short wall could reach a layer whose faces vanish — the whole-wall check must already refuse it, never a layer with negative length. (Task 3, `TestBuildRoomWallLayers_InwardLayersShrink`.)
3. 16 layers with many openings must not slip past the mesh cap 16-fold. (Task 2, `TestBuildWallLayers_MeshCapCountsEveryLayer`.)
4. A layer thinner than the 1 mm minimum hidden among thick ones (0.0005 between two 0.2 layers). (Task 1, `TestLayersInputErrors`.)
5. NaN or Inf as a layer thickness. (Task 1, `TestLayersInputErrors`, and every fuzz target.)

---

### Task 1: Layer input checks and `BuildSlabLayers`

**Files:**
- Create: `geometry/build_layers.go`, `geometry/build_layers_test.go`
- Modify: `geometry/build_slab.go`

**Interfaces:**
- Produces: `const maxLayers = 16`; `func checkLayers(layers []float64) (total float64, err error)`; `func BuildSlabLayers(outline [][2]float64, z float64, layers []float64) ([]Slab, error)`.

- [ ] **Step 1: Write the failing tests** in `build_layers_test.go`:
  - `TestLayersInputErrors`: table over `BuildSlabLayers(square4, 3.0, layers)` — `nil` and `[]float64{}` → error containing `no layers`; 17 layers of 0.01 → error containing `16`; `{0.2, 0.0005, 0.2}` → error containing `layer 1`; `{0.2, math.NaN()}` and `{math.Inf(1)}` → error containing `layer 1` / `layer 0`; `{600, 600}` → error about `thickness` over the maximum.
  - `TestBuildSlabLayersOracle`: `square4 = {{0,0},{4,0},{4,4},{0,4}}`, z 3.0, layers `{0.05, 0.20}` → 2 slabs; each `Area` 16, `Perimeter` 16; `Volume` 0.8 and 3.2; `Top.Origin[2]` 3.0 and 2.95; each mesh passes `meshAgrees` with its `Volume`.
  - `TestBuildSlabLayers_OneLayerIsBuildSlab`: `BuildSlabLayers(o, z, {t})[0]` deep-equals `BuildSlab(o, z, t)` (quantities, `Top`, `Element.Verts`, `Element.Tris`).
- [ ] **Step 2: Run** `go test ./geometry/ -run 'Layers' -v` → FAIL (undefined).
- [ ] **Step 3: Implement** `checkLayers` in `build_layers.go`: 0 layers → `errors.New("no layers: a layered element needs at least one layer")`; more than 16 → error naming 16; each layer `checkDim(fmt.Sprintf("layer %d: thickness", k), t)`; then `checkDim("thickness", sum)`. Split `BuildSlab` so the outline check and triangulation run once (`slabCap(outline) (ring, cap, err)`) and a `slabAt(outline, ring, cap, z, thickness) Slab` builds one layer; `BuildSlab` = one call of each. `BuildSlabLayers`: `checkLayers`, `checkOrigin("z", z)`, then layer k at `z − d_k` with thickness `layers[k]`.
- [ ] **Step 4: Run** `go test ./geometry/ -run 'Slab|Layers' -v` → PASS (existing slab tests too).
- [ ] **Step 5: Add** `FuzzBuildSlabLayers` beside `FuzzBuildSlab` (same outline inputs plus three layer thicknesses, zeros dropped); property: every layer passes `meshAgrees`, Σ `Volume` = `Area × Σ layers` within 1e-9 relative.
- [ ] **Step 6: Commit** `feat(geometry): build a slab in layers`.

### Task 2: `BuildWallLayers`

**Files:**
- Modify: `geometry/build_layers.go`, `geometry/build_wall.go` (reason constant, extract checks), `geometry/build_layers_test.go`, `geometry/build_fuzz_test.go`

**Interfaces:**
- Consumes: `checkLayers` (Task 1).
- Produces:
  - `const ReasonLayerSliver PlacementReason = "layer_sliver"` (godoc: the niche's back lies less than 1 mm from a layer boundary but not on it, which would leave a layer sliver under 1 mm).
  - `type layerPart struct { span wallSpan; shift, depth, thickness float64; ops []Opening; backs float64 }` — `shift` is where the layer's measured face starts in the whole wall's u; `depth` = d_k; `ops` are the openings as they cut this layer, offsets in the layer's own u; `backs` is the niche-back area that ends exactly on this layer's measured face.
  - `func splitLayers(span wallSpan, layers []float64, ops []Opening) []layerPart`
  - `func checkLayerSlivers(wall int, layers []float64, ops []Opening) error` — joined `*PlacementError`s.
  - `func wallLayers(f wallFrame, origin [3]float64, span wallSpan, height float64, layers []float64, ops []Opening) []Wall`
  - `func BuildWallLayers(edge [2][2]float64, base, height float64, layers []float64, side Side, openings []Opening) ([]Wall, error)`

- [ ] **Step 1: Write the failing tests** (fixture `layers3 = {0.015, 0.175, 0.10}`, T = 0.29; edge `{{0,0},{3,0}}`, height 2.50, SideLeft):
  - `TestBuildWallLayersOracle`: window `{Offset 1.00, Sill 0.90, Width 1.20, Height 1.20}` → 3 walls; every layer `Length` 3.00, `MeasuredGross`/`OtherGross` 7.50, `MeasuredNet`/`OtherNet` 6.06, `RevealPerimeter` 4.80; `GrossVolume` 7.5·t_k, `NetVolume` 6.06·t_k; SideLeft on +X grows the wall to −Y, so `Measured.Origin` of the layers is `(0,0,0)`, `(0,−0.015,0)`, `(0,−0.19,0)`; `Measured.Normal` = (0, 1, 0) for all.
  - `TestBuildWallLayers_NicheInsideALayer`: niche `{Offset 1.0, Sill 1.0, Width 0.5, Height 0.5, Depth 0.10}` → layer 0 lists it with `Depth 0`, `MeasuredNet` = `OtherNet` = 7.25, `NicheBack` 0; layer 1 lists it with `Depth` 0.085 (1e-12), `MeasuredNet` 7.25, `OtherNet` 7.50, `NicheBack` 0.25; layer 2 lists no opening, all faces 7.50.
  - `TestBuildWallLayers_NicheOnBoundary`: same niche with `Depth 0.015 + 0.175` → layers 0, 1 list it with `Depth 0`; layer 2 lists none, `MeasuredNet` 7.25, `OtherNet` 7.50, `NicheBack` 0.25, `NetVolume` 0.75 (uncut).
  - `TestBuildWallLayers_LayerSliver`: `Depth` 0.0155 and `Depth` 0.1895 → `errors.As` gives `*PlacementError{Wall 0, Opening 0, Reason "layer_sliver"}`.
  - `TestBuildWallLayers_Invariants` over the oracle and niche cases: layer 0 `MeasuredNet` and last `OtherNet` equal `BuildWall(…, T, …)`'s; Σ `NetVolume` and Σ `NicheBack` equal the whole wall's; layer k `OtherGross` = layer k+1 `MeasuredGross`; each layer passes `meshAgrees` with its `NetVolume`.
  - `TestBuildWallLayers_OneLayerIsBuildWall`: for each `wallOracle()` case, `BuildWallLayers(…, {t}, …)[0]` deep-equals `BuildWall(…, t, …)`.
  - `TestBuildWallLayers_SameRefusalsAsWholeWall`: a too-wide door plus an overlapping window → `err.Error()` equals `BuildWall`'s with thickness T.
  - `TestBuildWallLayers_MeshCapCountsEveryLayer`: openings that use a bit over 1/16 of `maxMeshCells` on one layer (use `gridCells` to size them), 16 layers → `errTooMuchMesh`; the same wall with 1 layer builds.
- [ ] **Step 2: Run** `go test ./geometry/ -run 'WallLayers' -v` → FAIL.
- [ ] **Step 3: Implement.**
  - Extract from `BuildWall` a `checkWall(edge, base, height, thickness, side, openings) (wallSpan, error)` doing everything up to and including `checkOpenings`; `BuildWall` keeps its cell check and calls `wallLayers(..., []float64{thickness}, ...)[0]` so both paths share the core.
  - `splitLayers`: with T = Σ layers and d_k the prefix sums, the measured-side span at depth d is `s(d) = span.otherStart·d/T`, `e(d) = span.length + (span.otherEnd − span.length)·d/T`. Layer k: `shift = s(d_k)`, `span = wallSpan{length: e(d_k)−s(d_k), otherStart: s(d_k+1)−s(d_k), otherEnd: e(d_k+1)−s(d_k)}`. For each opening with cut depth c (`o.cut(T)`): if c ≥ d_k+1 − 1e-9 the layer gets it with `Depth 0`; else if c > d_k + 1e-9 it gets `Depth c − d_k`; else if k > 0 and |c − d_k| ≤ 1e-9 add `Width·Height` to `backs`; otherwise skip. Offsets become `Offset − shift`. Use exact values for k = 0 and the last layer (d = 0, d = T) so one layer reproduces the whole span bit for bit.
  - `checkLayerSlivers`: for each niche (Depth ≠ 0) and each interior boundary d_k (0 < k < n), refuse when `1e-9 < |Depth − d_k| < minDim`.
  - `wallLayers`: per part, `meshWall(part.span, height, part.thickness, part.ops)`, add `part.shift` to u and `part.depth` to w of every point, `placeWall` with the element origin unchanged; quantities from `wallQuantities(part.span, height, part.thickness, part.ops)` then `MeasuredNet −= backs`, `NicheBack += backs`; `Measured` = `measuredFace(origin + shift·u + depth·g, f, part.span.length, height, part.ops)`.
  - `BuildWallLayers`: `checkLayers`, `checkWall` with T, `checkLayerSlivers` (its refusals appended after the whole-wall ones, all joined flat), Σ `gridCells(part.span, height, part.ops)` ≤ `maxMeshCells`, then `wallLayers`.
- [ ] **Step 4: Run** `go test ./geometry/ -v` → PASS (all existing wall tests too).
- [ ] **Step 5: Add** `FuzzBuildWallLayers`: the `FuzzBuildWall` inputs with `th` replaced by three layer thicknesses (zeros dropped); property: each layer `meshAgrees`, Σ `NetVolume` and Σ `NicheBack` match `BuildWall` with the sum (1e-9 relative + snap slack), and an error from one means an error from the other unless it is `layer_sliver` or a layer input error.
- [ ] **Step 6: Commit** `feat(geometry): build a wall in layers`.

### Task 3: `BuildRoomWallLayers`

**Files:**
- Modify: `geometry/build_layers.go`, `geometry/build_room.go` (extract checks), `geometry/build_layers_test.go`, `geometry/build_fuzz_test.go`

**Interfaces:**
- Consumes: `splitLayers`, `checkLayerSlivers`, `wallLayers` (Task 2).
- Produces: `func checkRoom(outline [][2]float64, base, height, thickness float64, side Side, openings [][]Opening) ([]wallSpan, error)`; `func BuildRoomWallLayers(outline [][2]float64, base, height float64, layers []float64, side Side, openings [][]Opening) ([][]Wall, error)` indexed `[wall][layer]`.

- [ ] **Step 1: Write the failing tests** (`square4` CCW, height 2.50, `roomLayers = {0.015, 0.20, 0.10}`, T = 0.315):
  - `TestBuildRoomWallLayersOracle` SideLeft (grows outward): for every wall, layer measured `Length` 4.00 / 4.03 / 4.43, other-face length 4.03 / 4.43 / 4.63, so `MeasuredGross` 10.0 / 10.075 / 11.075 and `OtherGross` 10.075 / 11.075 / 11.575; `GrossVolume` 0.1505625 / 2.115 / 1.1325 (Σ 3.3980625 = the whole wall's); wall 0 layer 1 `Measured.Origin` = (−0.015, −0.015, 0).
  - `TestBuildRoomWallLayers_InwardLayersShrink` SideRight: measured lengths 4.00 / 3.97 / 3.57, other 3.97 / 3.57 / 3.37; and a 0.70 × 0.70 room with layers `{0.2, 0.2}` is refused by the whole-wall corner check (same error as `BuildRoomWalls` with 0.4), never built with a negative length.
  - `TestBuildRoomWallLayers_OpeningKeepsItsPlace`: a door at `Offset 1.0` on wall 0, SideLeft → layer k's `Measured.Openings[0].Offset` = 1.0 + d_k, and its world start `Measured.Origin + Offset·U` is x = 1.0 in every layer.
  - `TestBuildRoomWallLayers_OneLayerIsBuildRoomWalls` and `…_SameRefusalsAsWholeRoom` (an `in_corner` opening), as in Task 2.
  - Invariants as in Task 2, per wall.
- [ ] **Step 2: Run** `go test ./geometry/ -run 'RoomWallLayers' -v` → FAIL.
- [ ] **Step 3: Implement.** Extract `checkRoom` from `BuildRoomWalls` (everything up to and including the joined refusals; returns spans). `BuildRoomWalls` calls `checkRoom`, its cell check, and `wallLayers(…, []float64{thickness}, …)[0]` per wall. `BuildRoomWallLayers`: `checkLayers`, `checkRoom` with T, `checkLayerSlivers` per wall (wall index = outline edge), Σ cells over every wall and layer ≤ `maxMeshCells`, then `wallLayers` per wall.
- [ ] **Step 4: Run** `go test ./geometry/ -v` → PASS.
- [ ] **Step 5: Add** `FuzzBuildRoomWallLayers` from `FuzzBuildRoomWalls` with three layers; same property as Task 2 per wall.
- [ ] **Step 6: Commit** `feat(geometry): build the walls of a room in layers`.

### Task 4: Docs, units and the release note

**Files:**
- Modify: `README.md`, `geometry/doc.go`, `docs/guides/builders.md`, `docs/limitations.md` (if it lists builder limits), `geometry/example_builders_test.go`, `CHANGELOG.md`, `Makefile` (fuzz lists only if the builder fuzz targets are already listed there; they are not today, so leave it)

- [ ] **Step 1: Add** `ExampleBuildWallLayers` (plaster 0.015, brick 0.175, insulation 0.10 on a 3 m wall with a window; prints each layer's `MeasuredNet` and `NetVolume` to 3 decimals). Run `go test ./geometry/ -run Example -v` → PASS.
- [ ] **Step 2: Write** the units line in README ("What you get" or just under it) and `doc.go`: goifc works in SI metres everywhere — builder input and output, quantities (m, m², m³) and GLB; IFC files in other units are converted to metres when they are read. Add a "Layers" section to `docs/guides/builders.md`: the three functions, per-layer quantity meanings, the niche rules (inside a layer, on a boundary, `layer_sliver`), `RevealPerimeter` repeats per layer and is not additive.
- [ ] **Step 3: Write** the CHANGELOG entry under `## Unreleased` → `### Added`: the three builders, `ReasonLayerSliver`, the units statement.
- [ ] **Step 4: Run** the gates: `go test ./...`, `go test -race ./geometry/`, `make lint`, `make parity`, `GOOS=js GOARCH=wasm go build ./...`, `make docs`; a 120 s fuzz of each new target (`go test ./geometry/ -run '^$' -fuzz FuzzBuildWallLayers -fuzztime 120s`, likewise Room and Slab). All green.
- [ ] **Step 5: Commit** `docs: layered builders and the SI unit rule`.
