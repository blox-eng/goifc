# Running in the browser

goifc runs in a browser as two WebAssembly files that share one JSON
contract:

- **`goifc-build.wasm`** builds walls, the walls of a room, slabs and pitched roofs
  from numbers. One request holds a whole plan; the answer has the quantities,
  the faces, a GLB of the meshes and, on request, the SVG preview.
- **`goifc-read.wasm`** reads an IFC file. The answer has the import tree,
  a GLB and the parts you ask for.

Each runs in a Web Worker, so the page stays responsive. Load the reader
only when someone opens a file: an editor that only drafts never downloads
it. The numbers are the same as the Go library's: CI runs the whole
contract suite compiled to WebAssembly and checks it gives the same bytes.

## Get the files

Every [release](https://github.com/blox-eng/goifc/releases) has six files:

| File | What it is |
|---|---|
| `goifc-build.wasm` | The builders (about 1.2 MB gzipped) |
| `goifc-read.wasm` | The IFC reader (about 1.2 MB gzipped) |
| `wasm_exec.js` | Go's WebAssembly loader, for the Go version that built them |
| `goifc.js` | The worker API below |
| `goifc-contract.schema.json` | The request and answers as JSON Schema |
| `SHA256SUMS` | Checksums of the five files above |

Check them before you serve them. First prove `SHA256SUMS` came from the
release workflow, run on the tag's commit; then check the other five files
against it:

```sh
tag=v0.19.0
gh attestation verify SHA256SUMS --repo blox-eng/goifc \
  --signer-workflow blox-eng/goifc/.github/workflows/release.yml \
  --source-digest "$(gh api "repos/blox-eng/goifc/commits/$tag" --jq .sha)"
sha256sum -c SHA256SUMS
```

Checking `SHA256SUMS` alone proves nothing: anyone who changes a file can
write new sums. Each of the six files has its own attestation too, so the
same `gh attestation verify` works on any one of them. Keep the six files
in one folder, and serve `.wasm` as `Content-Type: application/wasm`.

## Build walls, slabs and roofs

```js
import { createBuilder } from "/static/goifc/goifc.js";

const builder = await createBuilder({ base: "/static/goifc/" });
const { answer, glb } = await builder.build({
  slabs: [{ id: "s1", outline: [[0, 0], [5, 0], [5, 4], [0, 4]], z: 0, thickness: 0.2 }],
});
```

`answer` is:

```json
{
  "goifcVersion": "v0.19.0",
  "ok": true,
  "walls": [],
  "slabs": [
    {
      "id": "s1",
      "quantities": { "area": 20, "perimeter": 18, "volume": 4 },
      "top": {
        "origin": [0, 0, 0], "u": [1, 0, 0], "v": [0, 1, 0], "normal": [0, 0, 1],
        "outline": [[0, 0], [5, 0], [5, 4], [0, 4]],
        "openings": []
      }
    }
  ]
}
```

`glb` is an `ArrayBuffer`: load it with any glTF loader. Each element is a
node named by its element id.

A request has four lists, `walls`, `rooms`, `slabs` and `roofs`, and `preview`.
Each item's fields are the Go builder's parameters (see
[Building from numbers](builders.md)), in meters:

- **wall:** `id`, `edge` (two `[x, y]` points), `base`, `height`,
  `thickness` or `layers`, `side` (`"left"` or `"right"`), `openings`,
  `joins`.
- **room:** `id`, `outline` (the closed outline's points), `base`,
  `height`, `thickness` or `layers`, `side`, `openings` (one list per
  outline edge; `null` or `[]` for none).
- **slab:** `id`, `outline`, `holes` (optional: one list of points per hole,
  for a stair or a shaft), `z` (the top), `thickness` or `layers`.
- **roof:** `id`, `outline`, `pitch` (degrees, over 0 and up to 75), `gables`
  (optional: the outline edges that are vertical gable ends, counted from 0),
  `overhang` (optional, metres, 0 to 3, 0 when left out), `eaves` (the height
  of the underside at the outline line), `thickness` or `layers`.
- **opening:** `offset`, `sill`, `width`, `height`, and `depth` for a
  niche (0 or left out cuts through).
- **join:** `end` (`"a"` for `edge[0]`, `"b"` for `edge[1]`) and `wall`, the
  id of the wall the end meets mid-wall: a wall item (`"w1"`) or a room wall
  (`"r1/2"`). A wall has at most one join per end.

Give `thickness` or `layers`, never both. A field the contract does not
know is refused, so a typo never builds something different. Set
`"preview": true` to get `answer.preview.svg`, the
[SVG preview](builders.md#preview) of the whole plan.

### Element ids

An `id` is 1 to 64 letters, digits or `_ . : -`, unique in the request.
Each built element gets an id from it:

| Item | Element ids |
|---|---|
| wall | `w1` |
| wall in layers | `w1/0`, `w1/1`, … (measured face first) |
| room | `r1/0`, `r1/1`, … (wall i runs from point i to point i+1) |
| room in layers | `r1/0/0`, `r1/0/1`, … (wall, then layer) |
| slab | `s1` |
| slab in layers | `s1/0`, `s1/1`, … (top first) |
| roof | `r1` |
| roof in layers | `r1/0`, `r1/1`, … (top first) |

### Walls that meet mid-wall

A partition's end that meets another wall's face (a T) is declared in the
partition's `joins`. goifc cuts the end flush with the face and takes the
strip it covers off that face's net area. The through wall's
`measuredJoins` and `otherJoins` hold the area taken off its measured and
opposite faces, and `measuredNet` and `otherNet` already exclude it.

```json
{
  "walls": [
    { "id": "T", "edge": [[0, 0], [4, 0]], "base": 0, "height": 2.5, "thickness": 0.2, "side": "left" },
    { "id": "P", "edge": [[2, 3], [2, 0]], "base": 0, "height": 2.5, "thickness": 0.1, "side": "left",
      "joins": [{ "end": "b", "wall": "T" }] }
  ]
}
```

Here T's `measuredJoins` is 0.25 and its `measuredNet` 9.75.

`answer.walls` lists the walls of `walls` and `rooms`, in request order;
`answer.slabs` lists the slabs; `answer.roofs` lists the roofs, and is absent
when the request has none.

### Slabs with holes

A hole is a ring of 3 or more points inside the outline, at least 1 mm from
it and from the other holes. A slab takes up to 64 holes and 2,048 points in
all.

```json
{ "slabs": [{ "id": "s1", "outline": [[0, 0], [10, 0], [10, 8], [0, 8]],
    "holes": [[[2, 2], [4.5, 2], [4.5, 3.2], [2, 3.2]]], "z": 3, "thickness": 0.2 }] }
```

The slab's `area` and `volume` are net of its holes, `perimeter` is the
outline's, and `holePerimeter` is the holes' edges together (7.4 here, area
77). `top.holes` lists the holes in the top face's `u`, `v`; it is absent
for a slab without holes.

### Pitched roofs

```json
{ "roofs": [{ "id": "r1", "outline": [[0, 0], [10, 0], [10, 8], [0, 8]],
    "pitch": 30, "eaves": 3, "thickness": 0.2 }] }
```

Every edge slopes up and inward at `pitch`, except the `gables`. Each roof
in `answer.roofs` has `quantities` and `planes`. The quantities are
`slopeArea`, `planArea`, `volume`, and the lengths `eaves`, `ridge`, `hip`,
`valley`, `verge` and `top` (here `ridge` is 2 and `planArea` 80). `planes`
holds one face per sloped plane of the top, in the faces' `u`, `v`, like a
slab's `top`. A roof in layers gives one entry per layer, each with its own
`volume`; the areas and lengths are the same in all.

The preview draws a roof like a slab, with its ridge, hip and valley lines.

A roof goifc cannot build is refused with one of these codes, with `item`
and, where an edge is at fault, `edge` (and `other` for a second edge):
`pitch_out_of_range`, `gable_out_of_range`, `gable_repeated`,
`no_sloped_edge`, `roof_step`, `gable_collinear`, `overhang_out_of_range`,
`overhang_closes_outline`, `roof_too_complex`. For example, on an L outline
`roof_step` with `edge` 2 means the gable edge 2 leaves a step between two
planes.

three.js's `GLTFLoader` removes `[ ] . : /` from node names, so `w1/0`
loads as `w10`. Read the element id from `node.userData.name`: it keeps
the name as the GLB has it.

## Read an IFC file

```js
import { createReader } from "/static/goifc/goifc.js";

const reader = await createReader({ base: "/static/goifc/" });
const { answer, glb } = await reader.read(file); // a File, Blob, ArrayBuffer or Uint8Array
```

By default the answer has `nodes`, and `glb` holds the meshes. Ask for
more with `include`:

```js
await reader.read(file, { include: ["nodes", "glb", "psets", "storeyPlans", "typeLayers"] });
```

| Part | What it holds |
|---|---|
| `nodes` | The project, site, storeys and elements, parents first. Each has its class, name, quantities and where they came from, net area, material, type and bounding box. `parentIndex` points at an earlier node, `null` for the root. |
| `glb` | The meshes. Each node is named by the element's `GlobalId`. |
| `psets` | Each element's property sets, by the element's `GlobalId`. Storeys and other spatial nodes are not included. |
| `storeyPlans` | Each storey's floor plan outlines. |
| `typeLayers` | The layer build-up of each type, by type `GlobalId`. |

A part you did not ask for is absent from the answer. A value the file
does not have is `null`, never 0. `answer.warnings` lists what goifc could
not read, for example an element with no geometry.

## When something is refused

A refused request has `"ok": false` and lists every geometry refusal at once.
A malformed field (a `null` or missing value, a join with no `end` or `wall`,
a `thickness` of 0, or both `thickness` and `layers`) is reported alone,
before any geometry runs:

```json
{
  "goifcVersion": "v0.19.0",
  "ok": false,
  "errors": [
    { "code": "overlaps", "item": "w1", "wall": 0, "opening": 1, "message": "wall 0 opening 1: overlaps" },
    { "code": "above_wall", "item": "w1", "wall": 0, "opening": 2, "message": "wall 0 opening 2: above_wall" }
  ]
}
```

`item` is the id you gave. `wall` is the outline edge (0 for a wall item)
and `opening` the index in its openings. A refused join has `item` (the
partition) and `join`, the index in its `joins`, instead of `wall` and
`opening`; a refused hole has `item` (the slab) and `hole`, the index in its
`holes`:

```json
{ "code": "not_on_face", "item": "P", "join": 0, "message": "P join 0: not_on_face" }
```

Match on `code`; `message` is for a person.

| Code | What to do |
|---|---|
| `too_wide` | The opening reaches or passes an end of the wall. Make it narrower, or move it in. |
| `overlaps` | The opening touches or overlaps an earlier one. Move them apart. |
| `above_wall` | The opening reaches or passes the top of the wall. Lower its sill or height. |
| `in_corner` | The opening cuts into a corner join of the room. Move it away from the corner. |
| `layer_sliver` | A niche's back lies under 1 mm from a layer boundary. Put it on the boundary or further from it. |
| `in_join` | The opening overlaps or touches a strip a partition's join covers on that face. Move it away from the join. |
| `unknown_wall` | A join names no wall or room wall of the request, or the partition itself. Check the `wall` id. |
| `not_on_face` | The joined end is not on a face of the wall it names, or the partition runs into the wall. Move the end onto the face, or turn the partition. |
| `too_shallow` | The two walls meet at under 15° or over 165°. Change the angle. |
| `off_wall` | The strip the partition covers leaves the face. Move the partition in from the wall's ends or edges. |
| `joins_overlap` | The strip overlaps or touches an earlier strip on the same face. Move the partitions apart. |
| `hole_outside` | A hole point lies outside the slab's outline, or a hole edge runs on or within 1 mm of it. Move the hole in. |
| `holes_overlap` | The hole touches (within 1 mm) or overlaps an earlier hole, or lies inside it. Move them apart. |
| `hole_self_crossing` | The hole's edges cross, overlap or pinch. Fix its points. |
| `invalid_input` | The request or an item is malformed. `message` names the field. |
| `invalid_ifc` | The file is not IFC goifc can read. `message` says why. |
| `too_large` | A limit is exceeded. `message` names the size and the limit. |
| `nothing_to_draw` | The preview had no elements. Add one, or leave `preview` out. |
| `worker_failed` | From `goifc.js` only: the worker died, for example out of memory. The next request starts a fresh worker. |

`goifc.js` rejects its Promise, without asking the worker, with an `Error`
whose `code` is:

- `worker_failed` when the worker died or was closed;
- `too_large` for a file over the read limit;
- `invalid_input` when `read` gets something that is not a file, or an
  `include` that is not an array of part names.

Every other refusal is an answer with `"ok": false`.

## Limits

- A build request: 16 MiB of JSON, 10,000 items and 100,000 elements
  (each layer and each room wall counts). Each item keeps the Go builders'
  limits.
- An IFC file: 48 MiB. Reading takes about 35 times the file's size in
  memory, and a browser tab gives WebAssembly at most 4 GiB. Read bigger
  files with the Go library on a server.

## Versions

Every answer carries `goifcVersion`. The contract follows goifc's version:
a breaking change to it is a breaking goifc release. Load `goifc.js`,
`wasm_exec.js` and both `.wasm` files from the same release.
