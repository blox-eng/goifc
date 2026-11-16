# Slabs with Holes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build slabs with stated holes (stairs, shafts) with exact quantities and a closed mesh, from Go, `BuildPlan` and the wasm build.

**Architecture:** A hole-aware cap triangulation (bridge each hole into the outer ring, then the existing ear clipper) in `geometry/triangulate_holes.go`; hole checks and the two builders in `geometry/build_slab.go`; `PlanSlab.Holes` and the wasm `holes` field map onto them.

**Tech Stack:** Go (geometry: no cgo, os, net), Node tests for the wasm files.

**Spec:** `docs/superpowers/specs/2026-11-16-slab-holes-design.md` (issue #162)

## Global Constraints

- Synthetic fixtures only; no private names. The staged-diff grep gates every commit with `&&`. Signed Conventional Commits, never `--no-verify`. The PR squashes as `feat(geometry): build slabs with holes (#162)`.
- `BuildSlab` and `BuildSlabLayers` keep signatures and results; a `Polygon2D` with no holes builds exactly the same slab.
- Tolerance 1 mm (`minDim`). Caps: 64 holes; 2,048 points in total (`maxOutlinePoints`); work bounded by the existing budget.
- Refusal reasons: `outside`, `overlaps`, `self_crossing` (Go `HoleReason`); wasm codes `hole_outside`, `holes_overlap`, `hole_self_crossing`, with `item` and `hole`.
- The wasm size gate is 1,200,000 B gzipped with ~10 KB headroom: report the size after Task 2.

## Review Focus

1. A hole whose edge runs within 1 mm of the outer ring without crossing it: refused `outside`, never a sliver of slab.
2. A hole wound the same way as the outer ring, and one wound the other way: same result.
3. A hole with a repeated closing point or collinear points: built, not refused.
4. Holes with points far from the origin (a national grid, 1e6 m): same quantities as at the origin.
5. Many holes in a concave outline: bounded work, a closed shell whose mesh volume matches.

---

### Task 1: Holes in the slab builders

**Files:**
- Create: `geometry/triangulate_holes.go`, `geometry/triangulate_holes_test.go`, `geometry/build_slab_holes_test.go`
- Modify: `geometry/build_slab.go`, `geometry/build_layers.go` (`BuildSlabPolygonLayers`), `geometry/build.go` (`Face.Holes [][][2]float64`, `SlabQuantities.HolePerimeter float64`)

**Interfaces:**
- Produces: `func BuildSlabPolygon(p Polygon2D, z, thickness float64) (Slab, error)`, `func BuildSlabPolygonLayers(p Polygon2D, z float64, layers []float64) ([]Slab, error)`, `type HoleReason string` (`ReasonHoleOutside = "outside"`, `ReasonHolesOverlap = "overlaps"`, `ReasonHoleSelfCrossing = "self_crossing"`), `type HoleError struct{ Hole int; Reason HoleReason }` with `Error() = fmt.Sprintf("hole %d: %s", Hole, Reason)`; internal `triangulateWithHoles(outer [][2]float64, holes [][][2]float64, b *budget) (ring [][2]float64, tris []uint32, ok bool)` returning the merged vertex list and triangles into it.

Fixture: outer (0,0),(10,0),(10,8),(0,8); hole H (2,2),(4.5,2),(4.5,3.2),(2,3.2); z 3, thickness 0.2.

- [ ] **Step 1: Write the failing tests**:
  - `TestSlabHoleQuantities`: Area 77, HolePerimeter 7.4, Perimeter 36, Volume 15.4 (1e-9); the mesh's signed volume (sum of tetrahedra) equals Volume within float32 rounding (1e-4 relative); `Top.Holes` holds H in the face's u, v.
  - `TestSlabHoleNoHolesIsBuildSlab`: `BuildSlabPolygon(Polygon2D{Outer: outer})` is `reflect.DeepEqual` to `BuildSlab(outer, …)`; same for layers.
  - `TestSlabHoleLayers`: layers [0.05, 0.1, 0.05]: each layer has Area 77 and the same `Top.Holes`; volumes sum to 15.4.
  - `TestSlabHoleWinding`: H reversed gives the same quantities (Review Focus 2); H with its first point repeated at the end and an extra collinear point builds (Focus 3).
  - `TestSlabHoleTranslated`: everything shifted by (500000, 4600000): same quantities within 1e-9 (Focus 4).
  - `TestSlabHoleConcave`: an L outer (0,0),(18,0),(18,6),(12,6),(12,10),(0,10) with two holes in different arms: area = 156 − holes; closed shell (every edge used twice) (Focus 5).
  - `TestSlabHoleRefusals`, each a `*HoleError` (via `errors.As`) with that Hole index: a hole crossing x = 10 → 0 outside; a hole 0.5 mm inside the outer edge → outside (Focus 1); a second hole touching H's edge → 1 overlaps; a bowtie hole → self_crossing; two bad holes return both. 65 holes and 2,049 total points → too large.
  - `TestTriangulateWithHoles`: the triangles' total area equals outer − holes and no triangle centroid lies in a hole.
- [ ] **Step 2: Run** `go test ./geometry/ -run 'SlabHole|TriangulateWithHoles'` → FAIL (undefined).
- [ ] **Step 3: Implement.** Bridge each hole (rightmost hole point first) to a visible outer vertex, as earcut does; ear-clip the merged ring with `triangulatePolygon`'s clipper; charge the existing budget. Side faces for each hole face into it. Quantities from the numbers, not the mesh.
- [ ] **Step 4: Run** `go test ./geometry/` → PASS.
- [ ] **Step 5: Commit** `feat(geometry): build slabs with holes`.

### Task 2: BuildPlan, wasm and preview

**Files:**
- Modify: `geometry/plan.go` (`PlanSlab.Holes [][][2]float64`), `internal/wasmapi/build.go`, `internal/wasmapi/answer.go`, `wasm/goifc-contract.schema.json`, `docs/guides/wasm.md`, `internal/wasmapi/build_test.go`
- Create: `internal/wasmapi/testdata/build/slab-holes.{request,answer}.json` + `.glb`, `refused-hole.{request,answer}.json`
- Test: `geometry/plan_test.go`, `geometry/preview_test.go`

**Interfaces:**
- Consumes: Task 1's builders and `HoleError`.
- Produces: JSON slab `"holes": [[[x,y],…],…]`; answer `quantities.holePerimeter`, `top.holes`; error `{"code":"hole_outside","item":"s1","hole":0}`.

- [ ] **Step 1: Write the failing tests**: `TestBuildPlanSlabHoles` (the fixture through `BuildPlan`: Area 77; a refused hole is an `*ItemError` wrapping `*HoleError`); `TestBuild_SlabHoles` and `TestBuild_RefusedHole` against the new goldens; `TestBuild_RejectsBadRequests` rows for a null hole point and a hole with 2 points; `TestPreviewSlabHole`: the SVG of the fixture slab has its hole (the hole's corner (2,2) is not inside the drawn filled region: assert the path holds a second ring, as the existing preview tests read paths).
- [ ] **Step 2: Run** `go test ./geometry/ ./internal/wasmapi/` → FAIL.
- [ ] **Step 3: Implement**; holes on a layered slab go to `BuildSlabPolygonLayers`. Map `HoleError` to the three codes with `hole`.
- [ ] **Step 4: Run** `go test ./internal/wasmapi/ -update` once, check the old goldens only gain `"holePerimeter":0` (and nothing for `top.holes` if it is omitempty — make it `omitempty`), then `go test ./...` and `make wasm-test` → PASS; record the gzip size.
- [ ] **Step 5: Commit** `feat(wasm): slab holes in the build request`.

### Task 3: Fuzz, docs, changelog

**Files:**
- Modify: `geometry/build_fuzz_test.go` (`FuzzBuildSlabPolygon`), `geometry/example_builders_test.go` (`ExampleBuildSlabPolygon`), `docs/guides/builders.md`, `CHANGELOG.md`

- [ ] **Step 1: Write** `FuzzBuildSlabPolygon`: a rectangle outer and one fuzzed quad hole; no panic; on success Area = outer − hole area (1e-9 relative), the mesh is closed, and Volume = Area × thickness. Seed with the fixture.
- [ ] **Step 2: Run** `go test -run XXX -fuzz=FuzzBuildSlabPolygon -fuzztime=120s ./geometry/` → no failures.
- [ ] **Step 3: Write** `ExampleBuildSlabPolygon` printing `area 77.00 m²` and `hole perimeter 7.40 m`; the builders guide "Slabs with holes" section with the same numbers and the refusals; CHANGELOG under Unreleased/Added: `- **Build slabs with holes (a stair or shaft through a floor).** (#162)` with one line on `BuildPlan` and the wasm `holes` field.
- [ ] **Step 4: Run** `go test ./...`, `make lint`, `make docs`, `make parity` → PASS.
- [ ] **Step 5: Commit** `docs: slabs with holes in the builders guide`.
