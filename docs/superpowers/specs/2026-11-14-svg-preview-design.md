# SVG preview: a small drawing of built shapes

Status: approved, 2026-11-14.

## Intent

An app that drafts walls and slabs from numbers shows its models in a
gallery. Each card needs a picture the person recognises at a glance: their
room, their house. The app renders on a server or in a worker, not in a live
3D view, and it themes every picture with its own colours.

goifc already builds the shapes. This design adds one function that draws
them as a small SVG: a fixed isometric view, with no colours, that the host
styles through four classes.

## Goals

- `WritePreviewSVG` draws the walls and slabs that the builders return, with
  the walls' openings, as one SVG document.
- A window or door shows on whichever face of its wall the viewer sees.
- One fixed view from the south-west, so every card in a gallery looks the
  same way.
- Every drawn shape has exactly one class: `top`, `left`, `right` or
  `opening`. The host gives the colours.
- The same input gives the same bytes.
- The output stays small. Over the cap, the drawing is simplified and says
  so (maintainer, 2026-11-14: "simplify"). It is never silently worse.
- Hostile input never panics. The work and the output are bounded.
- A semver minor release (v0.18.0) with a CHANGELOG entry.

## Non-goals

- Drawing elements read from an IFC file (maintainer, 2026-11-14: built
  shapes only). The function takes any `Element`, but it is tested and
  documented for builder output only.
- A choice of view, zoom, camera or projection.
- Colours, stroke widths, fonts, labels, dimensions, a legend or a
  background. The host's CSS does this.
- A raster image (PNG, WebP).
- A perfect hidden-surface pass. Painter order is the limit (see Known
  limit).
- More than two detail levels.

## Anti-goals

- Any text from the input in the output. No GlobalID, no name. The SVG holds
  only fixed tag and class names and numbers, so it cannot carry markup or
  script from a caller.
- A simplified drawing that does not say it is simplified.
- A drawing that shows a face which points away from the viewer, or an
  opening on such a face.
- Any private or downstream data in the repository. Fixtures are synthetic.

## Approaches

**A. Draw from the meshes, back to front (chosen, 2026-11-14).** Project each
element's triangles into the view, drop the ones that face away, merge the
rest into one polygon per flat face, class each polygon by its facing, and
draw all polygons from the farthest to the nearest. Openings come from each
`Wall`'s measured face and are drawn on the visible face of that wall. Over the cap, the same steps run on a
simple prism per element.

**B. Draw only from the `Face` data.** Rejected: a `Wall` carries only its
measured face. The top, the ends and the opposite face are not there, so the
drawing would show flat sheets, not walls.

**C. A raster image from a 3D renderer.** Rejected: it needs a GPU renderer,
and it loses the no-colour SVG that the host themes through classes.

## Design

### API

```go
// Detail is how exactly a preview draws its elements.
type Detail string

const (
	DetailExact Detail = "exact" // every flat face, with its openings
	DetailBoxes Detail = "boxes" // one prism per element, no openings
)

// WritePreviewSVG draws walls and slabs, with the walls' openings, as one
// SVG document in a fixed isometric view from the south-west.
func WritePreviewSVG(w io.Writer, walls []Wall, slabs []Slab) (Detail, error)
```

- It lives in package `geometry`, file `preview.go`. It uses no cgo and no
  `os` or `net`, so it builds for wasm.
- The caller passes the builders' results as they are. A layered wall is
  one `Wall` per layer. Order does not matter to the picture.
- Build ruling, 2026-11-14: the agreed draft signature took `elems []Element`
  and `faces []Face`. A `Face` does not say which element it belongs to, so a
  through-opening could not be drawn on its wall's other face. Taking `Wall`
  and `Slab` keeps each measured face with its own mesh. Elements read from
  IFC are out of scope (built shapes only), so nothing is lost. It does matter to the bytes: ties in the sort break
  by input index, so the same input gives the same bytes.
- It builds the whole document in memory first. On an error, `w` gets
  nothing.
- On success it returns the detail it drew. On an error it returns `""`.

### The view

- The viewer looks along `d = (1, 1, −1)/√3`: from the south-west, from
  above. World Z is up, +X east, +Y north.
- Screen right is `r = (1, −1, 0)/√2`. Screen up is `s = (1, 1, 2)/√6`, world up projected onto the screen.
  A world point `p` draws at `x = p·r`, `y = −p·s` (SVG y points down).
- The `viewBox` is the bounding box of everything drawn, in metres, with a
  margin of 2 % of its larger side on every edge. There is no `width` or
  `height` attribute: the host sizes the picture.
- Every coordinate is rounded to 1 mm (three decimals), with no trailing
  zeros.

### What is drawn

- A triangle is drawn only if its world normal `n` gives `n·d < 0` (it faces
  the viewer). A triangle with `|n·d| ≤ 1e-9` is edge-on and dropped.
- In one element, the kept triangles that share a plane are merged into one
  shape. Two triangles share a plane when their unit normals differ by under
  1e-6 and their plane offsets by under 1e-4 m (the mesh grid snap). The
  shape's outline is the triangles' boundary edges: edges that only one
  triangle of the group uses. A group with more than one outline loop (a
  wall face with a window hole) is one `path` with `fill-rule="evenodd"`.
  A group with one loop is a `polygon`.
- A shape's class comes from its normal `n`:
  - `top` when `n.z ≥ 0.5` (within 60° of straight up);
  - else `left` when `n·r < 0` (it faces west, more or less);
  - else `right` (it faces south, more or less).
- Each opening of a wall adds one `opening` polygon: the opening's rectangle
  at `Measured.Origin + u·U + v·V`, on a face that the viewer sees.
  - When `Measured.Normal` faces the viewer, the rectangle is on the
    measured face.
  - Otherwise, a through-opening (`Depth == 0`) is drawn on the opposite
    face. Its plane is the plane of the wall's drawn shape whose normal is
    `−Measured.Normal`; the rectangle moves along `−Measured.Normal` onto
    that plane, with the same `u` and `v`.
  - A niche on a measured face that points away is not drawn: the other face
    is whole.
- Slabs have no openings.

### Painter order

- Each shape's depth is its centroid's `·d` (larger is farther). Shapes are
  drawn from the largest depth to the smallest. Ties break by element index,
  then by shape order in the element.
- An opening polygon sorts just after the shape of its own wall that it lies
  on, so the next nearer shape can cover it. If that wall has no drawn shape
  on that plane, the opening sorts by its own centroid.

**Known limit.** Sorting by centroid can draw a long wall over a short one
that is in front of part of it. The painter-order test below is fixed and is
not relaxed to pass. If centroid order fails it on a fixture, the sort is
improved: each pair of shapes whose screen outlines overlap is ordered by
exact depth at a point in the overlap, then a topological sort, with
centroid order breaking any cycle (build ruling, 2026-11-14). A layout
outside the fixtures can still draw with a wrong overlap, not a crash or a
missing shape.

### The document

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="-4.1 -3.2 9.4 6.8" data-detail="exact">
<g class="goifc-preview">
<polygon class="left" points="0 0 1.5 0.87 1.5 -1.63 0 -2.5"/>
<path class="right" fill-rule="evenodd" d="M… Z M… Z"/>
<polygon class="opening" points="…"/>
</g>
</svg>
```

- Elements used: `svg`, one `g`, `polygon`, `path`. Attributes used: `xmlns`,
  `viewBox`, `data-detail`, `class`, `points`, `d`, `fill-rule`. Nothing
  else: no `style`, no `fill`, no `stroke`, no `id`, no `script`, no
  `foreignObject`, no external reference.
- `data-detail` is always present, `exact` or `boxes`, and equals the
  returned `Detail`.
- The `g` has class `goifc-preview`, so a host can scope its CSS.

### Size and simplification

- The cap is `maxPreviewBytes = 1 << 20` (1 MiB) (maintainer, 2026-11-14:
  "your pick but let's measure first").
- Measured 2026-11-14 with a throwaway program on the builders: 154 rooms of
  three-layer walls plus a slab each, 2,002 elements, 120,736 triangles,
  1,540 openings. Exact: about 1.36 MB (677 bytes per element, 221 KB
  gzipped). Boxes: 511 KB (255 bytes per element, 61 KB gzipped). So a model
  that size draws as boxes, and boxes reach the cap near 4,000 elements.
- A gallery card is about 300 px wide. At that size the boxes drawing and the
  exact one look the same, and a card over 1 MB slows every page that shows
  it.
- Exact first. If the exact document is over the cap, the function draws
  again with `DetailBoxes`.
- `DetailBoxes` draws each element as a prism: the convex hull of its world
  vertices in plan (X, Y), extruded from its lowest to its highest Z. A
  wall's prism is its exact footprint; a slab's prism fills in any concave
  corner. Openings are not drawn. The same view, cull, class and painter
  rules apply.
- If the boxes document is still over the cap, the error names both sizes:
  `preview is 1,204,511 bytes as boxes, over the 1,048,576-byte cap`.

### Input checks

Each refusal is an error that says what is wrong. No partial output.

- `walls` and `slabs` both empty: `nothing to draw`.
- More than 100,000 elements, or more than 4,000,000 triangles in all:
  refused before any work, naming the count and the limit.
- A vertex, a placement entry, a face vector or an opening number that is
  not finite: refused, naming the wall or slab index.
- A triangle index out of range: refused, naming the wall or slab index.
- An element with no triangles draws nothing and is not an error.
- A degenerate triangle (zero area) is dropped.

## Tests

All fixtures are synthetic and built in the test from the builders.

- **Golden files** (`geometry/testdata/preview/*.svg`), compared byte for
  byte, regenerated with `-update`:
  - a 4 × 3 m room: four room walls with a door and a window, plus a slab;
  - one layered wall with a window and a niche;
  - an L-shaped house: six room walls and an L-shaped slab.
- **Painter order** on the room and the L-shaped house: for each pair of
  shapes whose screen outlines overlap, the nearer one (by the exact depth
  along `d` at a point in the overlap) is drawn later.
- **Cull:** no drawn shape has a normal that faces away.
- **Openings on the seen face:** a room measured inside (`SideLeft`, counter-
  clockwise): the south wall's window draws on its outer face; a niche on a
  measured face that points away is not drawn. The same room measured
  outside (`SideRight`) draws the same window polygon within 1 mm.
- **Classes:** a slab top is `top`; a wall facing west is `left`; a wall
  facing south is `right`; every shape has exactly one of the four classes.
- **Allow-list:** parse each output with `encoding/xml`; every element and
  attribute is in the list above.
- **Determinism:** two calls give the same bytes.
- **Simplify:** the measured 2,000-element model above returns `DetailBoxes`, has
  `data-detail="boxes"`, holds no `opening`, and is at most the cap.
- **Still too big:** a model past the boxes cap returns the named error and
  writes nothing.
- **Input checks:** one test per refusal above.
- **Fuzz:** `FuzzWritePreviewSVG` over a room's numbers and openings: no
  panic, output parses as XML or the call returns an error.
- **Example:** `ExampleWritePreviewSVG` draws a small room.

## Docs

- README, under Builders: one bullet for the preview.
- `docs/guides/builders.md`: a "Preview" section with the four classes and a
  sample CSS block for light and dark themes.
- `geometry/doc.go`: one paragraph.

## Left to the builder

- How the boundary edges of a merged group are chained into loops, and the
  start vertex of each loop (it must be deterministic).
- The welding tolerance used to match shared edges (the mesh grid snap,
  1e-4 m, is the starting point).
- The convex-hull algorithm for the boxes prism.
- How `-update` regenerates golden files.
- The wording of each error, inside the rules above.
