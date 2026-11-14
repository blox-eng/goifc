# Running in the browser

goifc runs in a browser as two WebAssembly files that share one JSON
contract:

- **`goifc-build.wasm`** builds walls, the walls of a room, and slabs from
  numbers. One request holds a whole plan; the answer has the quantities,
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

Check them before you serve them:

```sh
sha256sum -c SHA256SUMS
gh attestation verify goifc-build.wasm --repo blox-eng/goifc
```

The second command proves the release workflow built the file from the
tag. Keep the six files in one folder, and serve `.wasm` as
`Content-Type: application/wasm`.

## Build walls and slabs

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

A request has three lists, `walls`, `rooms` and `slabs`, and `preview`.
Each item's fields are the Go builder's parameters (see
[Building from numbers](builders.md)), in meters:

- **wall:** `id`, `edge` (two `[x, y]` points), `base`, `height`,
  `thickness` or `layers`, `side` (`"left"` or `"right"`), `openings`.
- **room:** `id`, `outline` (the closed outline's points), `base`,
  `height`, `thickness` or `layers`, `side`, `openings` (one list per
  outline edge; `null` or `[]` for none).
- **slab:** `id`, `outline`, `z` (the top), `thickness` or `layers`.
- **opening:** `offset`, `sill`, `width`, `height`, and `depth` for a
  niche (0 or left out cuts through).

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

`answer.walls` lists the walls of `walls` and `rooms`, in request order;
`answer.slabs` lists the slabs.

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
| `psets` | Every property set, by `GlobalId`. |
| `storeyPlans` | Each storey's floor plan outlines. |
| `typeLayers` | The layer build-up of each type, by type `GlobalId`. |

A part you did not ask for is absent from the answer. A value the file
does not have is `null`, never 0. `answer.warnings` lists what goifc could
not read, for example an element with no geometry.

## When something is refused

A refused request has `"ok": false` and lists every refusal at once:

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
and `opening` the index in its openings. Match on `code`; `message` is for
a person.

| Code | What to do |
|---|---|
| `too_wide` | The opening reaches or passes an end of the wall. Make it narrower, or move it in. |
| `overlaps` | The opening touches or overlaps an earlier one. Move them apart. |
| `above_wall` | The opening reaches or passes the top of the wall. Lower its sill or height. |
| `in_corner` | The opening cuts into a corner join of the room. Move it away from the corner. |
| `layer_sliver` | A niche's back lies under 1 mm from a layer boundary. Put it on the boundary or further from it. |
| `invalid_input` | The request or an item is malformed. `message` names the field. |
| `invalid_ifc` | The file is not IFC goifc can read. `message` says why. |
| `too_large` | A limit is exceeded. `message` names the size and the limit. |
| `nothing_to_draw` | The preview had no elements. Add one, or leave `preview` out. |
| `worker_failed` | From `goifc.js` only: the worker died, for example out of memory. The next request starts a fresh worker. |

`goifc.js` rejects its Promise with an `Error` whose `code` is
`worker_failed`, or `too_large` for a file over the read limit. Every
other refusal is an answer with `"ok": false`.

## Limits

- A build request: 16 MiB of JSON and 10,000 items. Each item keeps the Go
  builders' limits.
- An IFC file: 48 MiB. Reading takes about 35 times the file's size in
  memory, and a browser tab gives WebAssembly at most 4 GiB. Read bigger
  files with the Go library on a server.

## Versions

Every answer carries `goifcVersion`. The contract follows goifc's version:
a breaking change to it is a breaking goifc release. Load `goifc.js`,
`wasm_exec.js` and both `.wasm` files from the same release.
