# SVG Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `geometry.WritePreviewSVG(w, walls, slabs) (Detail, error)` draws built walls and slabs as a small, class-themed isometric SVG, simplified to boxes over 1 MiB.

**Architecture:** One new file, `geometry/preview.go`. Each element's mesh goes to world in float64, back faces are culled, coplanar kept triangles merge into shapes (outline loops from boundary edges), shapes are classed and painter-sorted, openings join the sort on their wall's seen face, and the document is written into a buffer that goes to `w` only when it fits the cap. Boxes mode replaces each mesh with a plan-hull prism and runs the same pipeline.

**Tech Stack:** Go 1.25, standard library only (`bytes`, `io`, `math`, `sort`, `strconv`; tests add `encoding/xml`, `flag`).

**Spec:** `docs/superpowers/specs/2026-11-14-svg-preview-design.md`

## Global Constraints

- Package `geometry`, file `geometry/preview.go`. No cgo, no `os`, no `net` in it (wasm target).
- View `d = (1, 1, −1)/√3`, right `r = (1, −1, 0)/√2`, up `s = (1, 1, 2)/√6`; draw at `x = p·r`, `y = −p·s`.
- World positions in float64: `applyMat(e.Placement, v3{…})` per vertex, never `WorldVerts` (float32 loses mm at origins up to 1e7 m).
- Cull: keep a triangle only if `n·d < −1e-9`.
- Merge: same element, unit normals differ by under 1e-6, plane offsets by under 1e-4 m.
- Classes: `top` if `n.z ≥ 0.5`; else `left` if `n·r < 0`; else `right`; openings `opening`.
- Coordinates rounded to 1 mm, three decimals at most, no trailing zeros (`strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)`; write `-0` as `0`).
- viewBox = bounds of everything drawn plus 2 % of the larger side on each edge; no `width`/`height`.
- Allowed tags: `svg`, `g`, `polygon`, `path`. Allowed attributes: `xmlns`, `viewBox`, `data-detail`, `class`, `points`, `d`, `fill-rule`. No input text in the output.
- `maxPreviewBytes = 1 << 20`; `maxPreviewElements = 100_000`; `maxPreviewTriangles = 4_000_000`.
- On any error `w` gets nothing and the returned `Detail` is `""`.
- Fixtures synthetic, built in the test from the builders. No private names.
- Signed Conventional Commits; never `--no-verify`.

## Review Focus

1. A wall at a large origin (1e6 m): the drawing must match the same wall at the origin, shifted, to 1 mm — float32 world verts would not.
2. A mitred room corner: the corner faces of two walls share an edge on screen; painter order must not flip them at the shared edge.
3. A door (`Sill == 0`) notches the outline instead of making a hole: the merged shape is one loop with the notch, and the door's opening polygon still draws.
4. A layered wall seen from its opposite side: each layer's through-opening must draw on the layer face the viewer sees, and the inner layers' opening polygons must be covered by the outer layer.
5. Identical inputs in a different slice order: the bytes may differ, but the visible-surface test must still pass.

Tests for 1 and 3 are in Task 1; 2 and 4 in Task 3; 5 in Task 3.

---

### Task 1: Shapes from one element

**Files:**
- Create: `geometry/preview.go`
- Test: `geometry/preview_test.go`

**Interfaces:**
- Produces:
  - `type Detail string`; `const DetailExact Detail = "exact"`, `DetailBoxes Detail = "boxes"`.
  - `type previewShape struct { class string; loops [][][3]float64; normal [3]float64; offset float64; depth float64; elem, seq int }` — loops in world metres; `depth` = centroid·d; `seq` = order inside the element.
  - `func previewWorld(e Element) ([][3]float64, error)` — float64 world vertices; error on a non-finite vertex or placement entry.
  - `func elementShapes(e Element, elem int) ([]previewShape, error)` — cull, merge, loops, class, depth; error on an index out of range.
  - `func previewClass(n [3]float64) string`
  - `func previewPoint(p [3]float64) [2]float64` — screen `(p·r, −p·s)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestPreviewClass(t *testing.T) {
	cases := map[string][3]float64{"top": {0, 0, 1}, "left": {-1, 0, 0}, "right": {0, -1, 0}}
	for want, n := range cases {
		if got := previewClass(n); got != want {
			t.Errorf("previewClass(%v) = %s, want %s", n, got, want)
		}
	}
}

func TestElementShapes_SquareWallShowsThreeFaces(t *testing.T) {
	// 3 m wall along +X, measured face south (SideRight), no openings.
	w := mustWall(t, [2][2]float64{{0, 0}, {3, 0}}, 0, 2.5, 0.2, SideRight, nil)
	got, err := elementShapes(w.Element, 0)
	// want exactly 3 shapes: one top, one right (south face), one left (west end);
	// each one loop of 4 points.
}

func TestElementShapes_WindowMakesAHole(t *testing.T) {
	// same wall, window Offset 1, Sill 0.9, Width 1, Height 1.2:
	// the south face shape has 2 loops; the reveal faces that face the viewer
	// (window head's underside is culled; its sill top and west jamb face are kept)
	// are separate shapes.
}

func TestElementShapes_DoorNotchesTheOutline(t *testing.T) {
	// door Offset 1, Sill 0, Width 0.9, Height 2.1: the south face is one loop of 8 points.
}

func TestElementShapes_FarOriginMatchesShiftedNear(t *testing.T) {
	// the same wall at (1e6, 1e6) and at (0, 0): every loop point differs by
	// (1e6, 1e6, 0) within 1e-3.
}

func TestElementShapes_RefusesBadMesh(t *testing.T) {
	// NaN vertex -> error naming "vertex"; index 99 on a 3-vertex mesh -> error naming "index".
}
```

`mustWall` is a test helper over `BuildWall` that fails the test on error.

- [ ] **Step 2: Run to verify they fail**

Run: `ctx-wire run go test ./geometry/ -run 'TestPreviewClass|TestElementShapes' -count=1`
Expected: FAIL to build: `undefined: previewClass`, `undefined: elementShapes`.

- [ ] **Step 3: Implement the Task 1 interfaces in `geometry/preview.go`**

Loop chaining: weld vertices on a 1e-4 m grid key; count each undirected edge per plane group; keep edges used once, directed as in their triangle; chain by following each edge's end vertex. Start each loop at its lowest welded-vertex id, and order loops by that id, so output is deterministic. Drop zero-area triangles (cross product length under 1e-12).

- [ ] **Step 4: Run to verify they pass**

Run: `ctx-wire run go test ./geometry/ -run 'TestPreviewClass|TestElementShapes' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add geometry/preview.go geometry/preview_test.go
git commit -S -m "feat(geometry): turn a mesh into classed shapes for a preview"
```

### Task 2: Openings on the seen face

**Files:**
- Modify: `geometry/preview.go`
- Test: `geometry/preview_test.go`

**Interfaces:**
- Consumes: `previewShape`, `elementShapes` (Task 1).
- Produces: `func wallOpenings(w Wall, shapes []previewShape, elem int) []previewShape` — class `opening`, one loop of 4 world points; `seq` = the seq of the shape it lies on (so Task 3's sort puts it right after), or −1 when no shape of the wall lies on that plane.

- [ ] **Step 1: Write the failing tests**

```go
func TestWallOpenings_OnTheMeasuredFaceWhenItFacesTheViewer(t *testing.T) {
	// south wall measured outside (SideRight), window Offset 1, Sill 0.9, W 1, H 1.2:
	// one opening, corners at y = 0 (the measured plane).
}

func TestWallOpenings_OnTheOtherFaceWhenMeasuredInside(t *testing.T) {
	// same window, wall measured inside (SideLeft, thickness 0.2, edge (3,0)->(0,0)
	// so the measured face points north): the opening's corners lie at the
	// south face's plane, and match the SideRight opening's corners within 1e-3.
}

func TestWallOpenings_NicheOnAHiddenFaceIsNotDrawn(t *testing.T) {
	// niche Depth 0.05 on a measured face pointing north: no opening.
}

func TestWallOpenings_OnFaceInFrontOfFaceSeq(t *testing.T) {
	// the opening's seq equals the seq of the south face shape.
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `ctx-wire run go test ./geometry/ -run TestWallOpenings -count=1`
Expected: FAIL to build: `undefined: wallOpenings`.

- [ ] **Step 3: Implement `wallOpenings`**

Opposite plane: the shape of this wall whose normal is within 1e-6 of `−Measured.Normal`; the rectangle moves along `−Measured.Normal` by the plane distance. A face "faces the viewer" when `Normal·d < −1e-9`.

- [ ] **Step 4: Run to verify they pass**

Run: `ctx-wire run go test ./geometry/ -run 'TestWallOpenings|TestElementShapes' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add geometry/preview.go geometry/preview_test.go
git commit -S -m "feat(geometry): draw a wall's openings on the face the viewer sees"
```

### Task 3: Painter order, the document and `WritePreviewSVG` (exact)

**Files:**
- Modify: `geometry/preview.go`
- Test: `geometry/preview_test.go`, `geometry/testdata/preview/{room,layered-wall,l-house}.svg`

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces:
  - `func paintOrder(shapes []previewShape)` — sorts in place: depth descending; ties by `elem`, then `seq`; an opening right after the shape with the same `elem` and `seq`.
  - `func writePreview(buf *bytes.Buffer, shapes []previewShape, detail Detail)`
  - `func WritePreviewSVG(w io.Writer, walls []Wall, slabs []Slab) (Detail, error)` — exact only in this task (boxes and the cap come in Task 4). Element index: walls first, then slabs.
  - Input checks before any work: `nothing to draw`; element count over 100,000; triangles over 4,000,000 (each names the count and the limit); errors from Task 1 name `wall %d` or `slab %d`.

- [ ] **Step 1: Write the failing tests**

Fixtures (helpers in the test file):
- `previewRoom`: `BuildRoomWalls({{0,0},{4,0},{4,3},{0,3}}, 0, 2.7, 0.2, SideLeft, …)` with a door on wall 0 (Offset 0.5, Sill 0, W 0.9, H 2.1) and a window on wall 3 (Offset 1, Sill 0.9, W 1.2, H 1.4), plus `BuildSlab` of the outline at z 0, thickness 0.2.
- `previewLayeredWall`: `BuildWallLayers({{0,0},{4,0}}, 0, 2.7, {0.015, 0.20, 0.10}, SideLeft, …)` with a window and a niche (Depth 0.01).
- `previewLHouse`: `BuildRoomWalls({{0,0},{8,0},{8,4},{4,4},{4,7},{0,7}}, 0, 2.7, 0.25, SideLeft, …)` with one window on each of walls 0 and 5, plus the L slab.

```go
var updatePreview = flag.Bool("update", false, "rewrite geometry/testdata/preview golden files")

func TestWritePreviewSVG_Golden(t *testing.T)          // each fixture vs its golden file, byte for byte; detail exact
func TestWritePreviewSVG_VisibleSurface(t *testing.T)  // room, layered wall (viewed with its measured face north too), L-house
func TestWritePreviewSVG_InputOrderStillVisible(t *testing.T) // L-house walls reversed: VisibleSurface still passes
func TestWritePreviewSVG_OnlyAllowedMarkup(t *testing.T)
func TestWritePreviewSVG_SameBytesTwice(t *testing.T)
func TestWritePreviewSVG_NoFaceAwayFromViewer(t *testing.T)
func TestWritePreviewSVG_Refusals(t *testing.T)        // nothing to draw; 100,001 elements; 4,000,001 triangles; NaN vertex names "wall 0"; NaN Measured.Normal or opening Width names "wall 0"; on each: Detail "" and w empty
func TestWritePreviewSVG_EmptyElementIsSkipped(t *testing.T) // a slab with no triangles beside a room draws the room; only empty elements -> "nothing to draw"
```

`VisibleSurface` oracle: sample the viewBox on a 2 cm grid. Skip a sample within 1 cm of any drawn edge. At each sample, the last drawn shape containing it (even-odd over its loops, in screen space) must be the one whose plane is nearest the viewer along `d` at that screen point; a depth tie within 1e-6 m (an opening on its face) accepts the later one. Parse shapes back from the SVG for this, and keep a parallel list of their world planes from the same call through an unexported hook `previewShapesFor(walls, slabs) ([]previewShape, error)` that `WritePreviewSVG` itself uses.

`OnlyAllowedMarkup`: decode with `encoding/xml`; every element in {svg, g, polygon, path}; every attribute in the allow-list; root `data-detail` = returned Detail; `g` class `goifc-preview`; every shape's class in {top, left, right, opening}.

- [ ] **Step 2: Run to verify they fail**

Run: `ctx-wire run go test ./geometry/ -run TestWritePreviewSVG -count=1`
Expected: FAIL to build: `undefined: WritePreviewSVG`.

- [ ] **Step 3: Implement `paintOrder`, `writePreview`, `previewShapesFor`, `WritePreviewSVG`**

Document shape, exactly (newline after each tag, no indentation):

```
<svg xmlns="http://www.w3.org/2000/svg" viewBox="X Y W H" data-detail="exact">
<g class="goifc-preview">
<polygon class="C" points="x y x y …"/>
<path class="C" fill-rule="evenodd" d="Mx yLx y…Z Mx y…Z"/>
</g>
</svg>
```

One loop → `polygon`; more → `path`.

- [ ] **Step 4: Generate the golden files and look at them**

Run: `ctx-wire run go test ./geometry/ -run TestWritePreviewSVG_Golden -update -count=1`, then open the three files as one artifact page for the maintainer to see.
Expected: a room, a layered wall and an L-shaped house, nothing overlapping wrongly.

- [ ] **Step 5: Run all preview tests**

Run: `ctx-wire run go test ./geometry/ -run 'Preview|ElementShapes|WallOpenings' -count=1`
Expected: PASS. If `VisibleSurface` fails on a fixture: do not relax the oracle or the fixture (spec, Known limit). Replace the centroid sort with pairwise ordering of screen-overlapping shapes by exact depth at a point in the overlap, a topological sort, and centroid order to break cycles; ledger it as a ruling.

- [ ] **Step 6: Commit**

```bash
git add geometry/preview.go geometry/preview_test.go geometry/testdata/preview
git commit -S -m "feat(geometry): write a preview of built walls and slabs as SVG"
```

### Task 4: Boxes and the cap

**Files:**
- Modify: `geometry/preview.go`
- Test: `geometry/preview_test.go`

**Interfaces:**
- Consumes: Task 3.
- Produces: `func boxShapes(e Element, elem int) ([]previewShape, error)` — plan convex hull of world vertices, prism from min to max Z, top + sides that face the viewer; `WritePreviewSVG` draws exact, and over `maxPreviewBytes` draws boxes; over again returns `preview is %s bytes as boxes, over the %s-byte cap` (thousands separators: `1,204,511`, `1,048,576`).

- [ ] **Step 1: Write the failing tests**

```go
func TestBoxShapes_ObliqueWallIsItsFootprint(t *testing.T) // wall (0,0)->(3,3), T 0.2: top loop has 4 points, the wall's footprint corners within 1e-3
func TestWritePreviewSVG_BigModelDrawsBoxes(t *testing.T)  // previewMeasuredModel: DetailBoxes; data-detail="boxes"; no class="opening"; len ≤ 1<<20; exact size logged with t.Logf
func TestWritePreviewSVG_TooBigEvenAsBoxes(t *testing.T)   // 10,000 BuildWall walls on a grid: error contains "as boxes" and "1,048,576"; Detail ""; w empty
```

`previewMeasuredModel` rebuilds the spec's measurement: 154 rooms on a 12-wide grid (5 m × 4.5 m pitch), outline 4.5 × 4 m, `BuildRoomWallLayers` with `{0.015, 0.20, 0.10}`, height 2.7, `SideLeft`, openings per wall: door (0.5, 0, 0.9, 2.1); window (1.2, 0.9, 1.2, 1.4); window (1.0, 0.9, 1.5, 1.4); niche (1.0, 1.0, 0.6, 0.4, Depth 0.01); plus `BuildSlab` per room, z 0, thickness 0.2 — 2,002 elements.

- [ ] **Step 2: Run to verify they fail**

Run: `ctx-wire run go test ./geometry/ -run 'TestBoxShapes|BigModel|TooBig' -count=1`
Expected: FAIL to build: `undefined: boxShapes`; once stubbed, BigModel fails with detail `exact` and a size over the cap.

- [ ] **Step 3: Implement `boxShapes` and the two-level cap in `WritePreviewSVG`**

- [ ] **Step 4: Run to verify they pass, and record the measured sizes**

Run: `ctx-wire run go test ./geometry/ -run 'Preview|BoxShapes' -count=1 -v 2>&1 | rg 'exact size|PASS|FAIL'`
Expected: PASS; the logged exact size is over 1,048,576. If the real exact drawing of the measured model is under the cap, the model is not over the cap: rule in the ledger and add openings to the fixture until it is, without changing the cap.

- [ ] **Step 5: Commit**

```bash
git add geometry/preview.go geometry/preview_test.go
git commit -S -m "feat(geometry): simplify a preview to boxes over 1 MiB"
```

### Task 5: Fuzz, example, docs, release notes and CI

**Files:**
- Modify: `geometry/build_fuzz_test.go`, `geometry/example_builders_test.go`, `geometry/doc.go`, `README.md`, `docs/guides/builders.md`, `CHANGELOG.md`, `.github/workflows/ci.yml`, `.github/workflows/fuzz-nightly.yml`

**Interfaces:**
- Consumes: `WritePreviewSVG` (Tasks 3–4).

- [ ] **Step 1: Write the fuzz target and the example**

```go
func FuzzWritePreviewSVG(f *testing.F) // args: x1, y1, x2, y2, h, th float64, side uint8, o, s, w, oh, depth float64
// builds a 4-corner room (0,0),(x1,0),(x2,y2),(0,y1) with one opening on wall 0;
// builder refusal -> return; else WritePreviewSVG must not panic, and either
// returns an error with an empty writer or output that decodes with encoding/xml
// and has data-detail equal to the returned Detail.

func ExampleWritePreviewSVG() // a 4 x 3 m room, no openings, prints the Detail and the first line of the SVG
// Output:
// exact
// <svg xmlns="http://www.w3.org/2000/svg" viewBox="…" data-detail="exact">
```

Fill the example's `viewBox` from the first real run.

- [ ] **Step 2: Run them**

Run: `ctx-wire run go test ./geometry/ -run 'ExampleWritePreviewSVG' -count=1 && .github/scripts/fuzz-smoke.sh ./geometry/ FuzzWritePreviewSVG 60s`
Expected: PASS; fuzz finds no failure in 60 s.

- [ ] **Step 3: Docs and release notes**

- `README.md`, Builders bullet: add the sentence "`WritePreviewSVG` draws them as a small isometric SVG that your CSS colours."
- `docs/guides/builders.md`: new `## Preview` section before `## Not yet`: the call, the four classes, the `data-detail` attribute and the 1 MiB rule, and this CSS:

```css
.goifc-preview .top     { fill: #e8e6e1; }
.goifc-preview .left    { fill: #b9b5ad; }
.goifc-preview .right   { fill: #d3d0c9; }
.goifc-preview .opening { fill: #7fa7c9; }
@media (prefers-color-scheme: dark) {
  .goifc-preview .top     { fill: #4a4845; }
  .goifc-preview .left    { fill: #2e2d2b; }
  .goifc-preview .right   { fill: #3c3a37; }
  .goifc-preview .opening { fill: #3d5a73; }
}
```

- `geometry/doc.go`: one paragraph after the layered builders: the preview, its fixed view, its classes, and that it draws boxes over 1 MiB.
- `CHANGELOG.md` under `## Unreleased`: `### Added` with one bullet: "**SVG preview.** `geometry.WritePreviewSVG` draws built walls and slabs as an isometric SVG from the south-west, with classes `top`, `left`, `right` and `opening` and no colours. Windows and doors show on the face you see. Over 1 MiB it draws each element as a box and says so in `data-detail` and the returned `Detail`."

- [ ] **Step 4: CI**

- `.github/workflows/ci.yml`: after "Fuzz the layered slab builder", add a step "Fuzz the SVG preview" with `CGO_ENABLED: '0'` and `run: .github/scripts/fuzz-smoke.sh ./geometry/ FuzzWritePreviewSVG 60s`.
- `.github/workflows/fuzz-nightly.yml`: add `- target: FuzzWritePreviewSVG` / `package: ./geometry/` after `FuzzBuildSlabLayers`.

- [ ] **Step 5: Full suite, wasm build and vet**

Run: `ctx-wire run go test ./... -count=1 && GOOS=js GOARCH=wasm ctx-wire run go build ./geometry/ && ctx-wire run go vet ./...`
Expected: all PASS; the wasm build succeeds.

- [ ] **Step 6: Commit**

```bash
git add geometry/build_fuzz_test.go geometry/example_builders_test.go geometry/doc.go README.md docs/guides/builders.md CHANGELOG.md .github/workflows/ci.yml .github/workflows/fuzz-nightly.yml
git commit -S -m "docs(geometry): document, fuzz and announce the SVG preview"
```
