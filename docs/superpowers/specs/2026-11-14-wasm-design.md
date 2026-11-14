# WebAssembly: goifc-build.wasm and goifc-read.wasm

Status: approved, 2026-11-14.

## Intent

A browser editor needs to build walls, rooms and slabs while a person
drafts, and needs to read an IFC file a person drops in, with no server
round trip. The numbers must match what the Go library gives on a
server.

goifc ships a JavaScript API as two WebAssembly files that share one
JSON contract:

- `goifc-build.wasm` runs in a Web Worker. It takes a whole plan and
  returns the built elements: their quantities, their faces, a GLB of
  their meshes and, on request, the SVG preview.
- `goifc-read.wasm` loads only when a person opens an IFC file. It
  returns the import tree, a GLB and, on request, more parts.

An editor that only drafts never downloads the reader.

The first user is a browser plan editor. The API stays general:
nothing in it knows about any one consumer.

## Goals

- One JSON contract for both files, owned by goifc. It is goifc's own
  format, never a consumer's.
- The error codes are the Go refusal reasons plus three contract codes.
  Every error names the item it belongs to.
- Every answer carries the goifc version that made it.
- The build answer for a request is byte-identical to the Go answer for
  the same request, and CI proves it.
- Each GitHub release ships the files, their checksums and signed build
  provenance.

## Non-goals

- An npm package. Release assets only, until someone outside asks.
- TinyGo. Standard Go makes the size target (see "Measured size").
- New geometry. The wasm files call the existing builders and reader
  and add no semantics. The one `geometry` change is two exported error
  values (see "geometry error values"). T-joins come after this, as their own design.
- A higher-level plan document (storeys, rooms laid out by goifc).
- A public Go package for the contract. It stays `internal/` until a
  Go user needs the same JSON on a server.

## Anti-goals

- A consumer's type names, ids or field names in the contract.
- A silent empty answer: a part the caller did not ask for is absent,
  and a part that failed carries an error, never an empty list.
- A tab that crashes on a big file with no error code.
- A file in a release that nobody can verify CI built from the tag.

## Measured size

Measured 2026-11-14 with Go 1.25.12, `-trimpath -ldflags="-s -w"`, from
throwaway entry points (not in the repo):

| File | Raw | gzip -9 |
|---|---|---|
| goifc-build.wasm (room walls, slab, preview, JSON) | 3.6 MB | 1.01 MB |
| goifc-read.wasm (parse, assemble, GLB, JSON) | 4.2 MB | 1.15 MB |

Go's `wasm_exec.js` adds about 17 KB.

## Approaches

Decided with the maintainer, one question at a time:

1. **What build returns for the 3D view.** Picked: one GLB plus a JSON
   answer. Rejected: mesh arrays inside the JSON (about 3 times bigger,
   slow to parse); typed arrays next to the JSON (fastest, but every
   caller wires a raw mesh layout by hand). A GLB loads in any viewer,
   and read and build give the same mesh format.
2. **What one build call takes.** Picked: one request builds the whole
   scene, with lists of walls, rooms and slabs and a caller id on each.
   Rejected: one call per builder (N calls, N GLBs, and no whole-plan
   preview); a higher-level plan document (new semantics, out of
   scope).
3. **How people get the files.** Picked: GitHub release assets with
   `SHA256SUMS` and signed build provenance. Rejected for now: npm (a
   token in the repo and a second registry to secure); source only
   (every user needs Go, and nobody can verify the browser file).
4. **What read returns.** Picked: the caller chooses parts, with a
   small default. Rejected: the import tree only; the flat elements
   with every property set (many MB on a real file).
5. **The read size cap.** Picked: 256 MiB, refused with `too_large`.
   Changed to 48 MiB on 2026-11-14 after measuring (see "Build notes").
   Rejected: no cap (the tab dies with no code); a caller-set cap (one
   more option before anyone needs it).
6. **How it gets built.** One sensible way: a pure-Go contract package
   and two thin `syscall/js` entry points. TinyGo adds a second compiler
   with its own `reflect` and `encoding/json` gaps, and the measurement
   shows it is not needed. One file with both halves makes a drafting
   editor download the reader. Hand-written JS bindings per Go type
   drift from the Go types; the JSON contract is the single owner.

## Layout

- `internal/wasmapi`: the contract, in plain Go, with no `syscall/js`.
  - `Build(version string, request []byte) (answer []byte, glb []byte)`
  - `Read(version string, ifc []byte, include []string) (answer []byte, glb []byte)`
  - The caller passes the version (see "Version"), so a Go test and the
    `.wasm` give the same bytes for the same version.
  - The contract types (DTOs) with explicit camelCase `json` tags. The
    contract never serializes a geometry or model type directly, so a
    Go-side rename cannot change the JSON without a failing golden test.
- `cmd/goifc-build` and `cmd/goifc-read`: `//go:build js && wasm` main
  packages. Each registers one global function that copies bytes in,
  calls `wasmapi`, and copies the answer and GLB out. No logic.
- `wasm/goifc.js`: an ES module that starts a Worker, loads
  `wasm_exec.js` and one `.wasm`, and exposes Promises:

  ```js
  import { createBuilder, createReader } from "./goifc.js";

  const builder = await createBuilder({ base: "/static/goifc/" });
  const { answer, glb } = await builder.build(request); // glb: ArrayBuffer

  const reader = await createReader({ base: "/static/goifc/" });
  const out = await reader.read(file, { include: ["nodes", "glb"] });
  ```

  One worker per file. Requests queue in order. The GLB comes back as a
  transferred `ArrayBuffer`, so it is not copied.

  If the worker dies (out of memory, a Go runtime fatal error, `exit`),
  `goifc.js` rejects the request in flight with an `Error` whose `code`
  is `worker_failed` and whose message says what died. Requests queued
  behind it go to a fresh worker. `worker_failed` is a JS-side code: Go
  never returns it, because a dead runtime cannot answer.
- `wasm/goifc-contract.schema.json`: a hand-written JSON Schema (draft
  2020-12) for the request and both answers.

## The build contract

### Request

```json
{
  "walls": [{
    "id": "w1",
    "edge": [[0, 0], [4, 0]],
    "base": 0, "height": 2.7,
    "thickness": 0.2,
    "side": "left",
    "openings": [{"offset": 1, "sill": 0, "width": 0.9, "height": 2.1, "depth": 0}]
  }],
  "rooms": [{
    "id": "r1",
    "outline": [[0, 0], [5, 0], [5, 4], [0, 4]],
    "base": 0, "height": 2.7,
    "layers": [0.02, 0.2, 0.08],
    "side": "left",
    "openings": [[], [{"offset": 1, "sill": 0.9, "width": 1.2, "height": 1.2}], [], []]
  }],
  "slabs": [{
    "id": "s1",
    "outline": [[0, 0], [5, 0], [5, 4], [0, 4]],
    "z": 0,
    "thickness": 0.2
  }],
  "preview": true
}
```

- Each item has `thickness` or `layers`, never both. `thickness` calls
  `BuildWall`, `BuildRoomWalls` or `BuildSlab`. `layers` calls the
  `...Layers` builder. Fields map one-to-one to the Go parameters.
- `side` is the text form of `geometry.Side` (`Side.MarshalText`).
- `openings`, `depth` and `preview` may be left out. Left out means none,
  0 and false. Every other field is required.
- Unknown fields are refused (`invalid_input`), so a typo never builds
  something silently different.
- An `id` matches `^[A-Za-z0-9_.:-]{1,64}$` and is unique across the
  request. No `/`, because element ids are derived with `/`.

### Element ids

Every built element gets an id, used both in the JSON and as the GLB
node name:

| Item | Element id |
|---|---|
| wall, thickness | `w1` |
| wall, layers | `w1/0`, `w1/1`, ... (measured face first) |
| room, thickness | `r1/0`, `r1/1`, ... (wall i runs outline[i] to outline[i+1]) |
| room, layers | `r1/0/0`, `r1/0/1`, ... (wall, then layer) |
| slab, thickness | `s1` |
| slab, layers | `s1/0`, `s1/1`, ... (top first, as `BuildSlabLayers`) |

### Answer

```json
{
  "goifcVersion": "v0.19.0",
  "ok": true,
  "walls": [{"id": "w1", "quantities": {...}, "measured": {...}}],
  "slabs": [{"id": "s1", "quantities": {...}, "top": {...}}],
  "preview": {"detail": "exact", "svg": "<svg ...>"}
}
```

- `walls` lists every wall element, from `walls` and `rooms` items, in
  request order. `slabs` lists every slab element.
- `quantities` carries every `WallQuantities` or `SlabQuantities` field;
  `measured` and `top` carry the `Face`. Meters, m² and m³, as in Go.
- The GLB holds one node per element, named by its element id, made by
  `Scene.WriteGLB` from a scene of the built elements.
- A preview refusal does not fail the build: `preview` then holds
  `{"error": {...}}` in place of `detail` and `svg`.

### Errors

A refused request returns `"ok": false`, an `errors` list, no element
lists and no GLB:

```json
{
  "goifcVersion": "v0.19.0",
  "ok": false,
  "errors": [
    {"code": "too_wide", "item": "r1", "wall": 1, "opening": 0, "message": "..."},
    {"code": "invalid_input", "item": "s1", "message": "..."}
  ]
}
```

- Every item is built, so the answer lists every refusal in the request
  at once, not just the first.
- `code` is one of:
  - the `PlacementReason` values: `too_wide`, `overlaps`, `above_wall`,
    `in_corner`, `layer_sliver`. `wall` and `opening` are the
    `PlacementError` indexes; `wall` is 0 for a wall item.
  - `invalid_input`: the request or an item is malformed. `message` is
    the Go error text or the decode error.
  - `invalid_ifc`: read only. The file does not parse as IFC.
  - `too_large`: a cap is exceeded: a contract cap, or a Go error that
    wraps `geometry.ErrTooLarge`. `message` names the size and the cap.
  - `nothing_to_draw`: preview only, for a Go error that wraps
    `geometry.ErrNothingToDraw` (a request with no elements).
  - `worker_failed`: JS only (see "Layout").
- `item` is absent only for an error that belongs to no item (bad JSON,
  a request cap).
- `message` is English text for a person. Code matches on `code`.

### Caps

- Request: at most 16 MiB of JSON and 10,000 items.
- Each item: the Go builders' existing caps.
- Preview: the existing `WritePreviewSVG` caps.

### geometry error values

Decided with the maintainer, 2026-11-14 (picked over mapping every Go error to
`invalid_input`): `geometry` exports two error values, so the wasm layer
and Go callers tell a broken limit from bad input with `errors.Is`.

- `ErrTooLarge` is wrapped by every existing limit error: a dimension
  over 1,000 m, a point over 1,000 m from the first point, a position
  over 1e7 m from the origin, more than 256 openings, more than 16
  layers, and the preview caps (bytes, elements, triangles, vertices,
  coordinates).
- `ErrNothingToDraw` is wrapped by the preview's "nothing to draw".
- The message text of every error stays the same. Adding them breaks no
  caller. It gets its own CHANGELOG entry and a test per wrapped error.
- Errors about the shape of the input (under 1 mm, not finite, zero
  area, crossing edges, a mitre too sharp) stay plain: they are
  `invalid_input`.

## The read contract

```js
read(ifcBytes, { include: ["nodes", "glb"] })   // the default
```

| Part | Carries | Default |
|---|---|---|
| `nodes` | The `ImportModel` tree: project, site, storeys, elements; each with class, name, quantities and their source, net area, opening deduction and perimeter, material, type and bounding box | on |
| `glb` | The meshes, one node per element, named by its `GlobalId` | on |
| `psets` | All property sets, keyed by `GlobalId` | off |
| `storeyPlans` | `ImportModel.StoreyPlans` | off |
| `typeLayers` | `ImportModel.TypeLayers` | off |

- An unknown name in `include` returns `invalid_input` that names it.
- A part that is off is absent from the answer.
- The answer lists the parts it holds: `"parts": ["nodes", "glb"]`.
- A file over 48 MiB (50,331,648 bytes) returns `too_large` before it
  is parsed. `goifc.js` checks the size before it copies the file into
  the worker.
- The scene's warnings come back as `"warnings": [...]`, never dropped.

The answer:

```json
{
  "goifcVersion": "v0.19.0",
  "ok": true,
  "parts": ["nodes", "glb"],
  "nodes": [{"globalId": "...", "parentIndex": null, ...}],
  "warnings": []
}
```

- `nodes` is the flat, parents-first list of `ImportModel.Nodes`, with
  `parentIndex` as in Go (null for the root). Each node carries every
  `ImportNode` field, camelCase; a Go nil stays `null`, never 0.
- `psets` is an object keyed by `GlobalId`; `storeyPlans` and
  `typeLayers` mirror their Go fields.
- A part never fails alone. Any failure makes the whole answer
  `"ok": false` with `errors`, no parts and no GLB.
- A file that `step.ParseBytes`, `Assemble` or `BuildImportFrom`
  rejects returns `invalid_ifc` with the Go error text.
- A non-finite number in a property set (which JSON cannot hold) is
  written as `null`, and the answer adds a warning that names the
  element and the property.

## Version

The `cmd` entry points read `goifcVersion` from
`runtime/debug.ReadBuildInfo`: the tag on a release build, a
pseudo-version or `(devel)` otherwise, and pass it to `wasmapi`. The release job
checks that the version in a built file's answer equals the tag. The
contract's version is goifc's semver: a breaking contract change is a
breaking goifc change.

## Release

The Release workflow gains a job that runs after go-semantic-release
cuts a tag, in the same workflow (a tag pushed with `GITHUB_TOKEN` does
not start another workflow). It:

1. Checks out the tag and builds both files with `GOOS=js GOARCH=wasm
   CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`, Go from `go.mod`.
2. Copies `wasm_exec.js` from `$(go env GOROOT)/lib/wasm/`.
3. Writes `SHA256SUMS` for the five files: both `.wasm`, `wasm_exec.js`,
   `goifc.js` and `goifc-contract.schema.json`.
4. Attests all five with `actions/attest-build-provenance`, pinned by
   commit SHA. Only this job gets `id-token: write` and
   `attestations: write`.
5. Uploads the six files (five plus `SHA256SUMS`) to the release.

If this job fails, the tag and release already exist without assets.
The job fails loudly, and it can also run by hand
(`workflow_dispatch` with a `tag` input) to attach the assets to an
existing tag. Building before the tag would need the next version
computed ahead of semantic-release; the CI `wasm` job already builds
and checks the same commit on main, so the job rarely fails for a new
reason.

No new secret. A user verifies with `gh attestation verify <file>
--repo blox-eng/goifc` and `sha256sum -c SHA256SUMS`.

## CI

The `wasm` job, which today runs `go build ./...` for `js/wasm`, also:

- builds both files and fails if `goifc-build.wasm` is over 1.2 MB
  gzipped (`gzip -9`), printing both sizes;
- runs a parity test under Node: the same requests through the real
  `.wasm` and through `wasmapi` in Go must give byte-identical answers
  and GLBs, with the same version passed to both. The requests are the
  golden requests below. Numbers are full-precision float64. If parity
  shows a last-digit difference (for example `math.Hypot` uses assembly
  on amd64 and pure Go on wasm), the fix goes into `geometry` so both
  agree; the contract never rounds to hide it.
- measures peak memory while `goifc-read.wasm` reads a synthetic
  IFC at the 48 MiB cap under Node every night, and fails over 2 GiB.

## Tests

- Golden requests and answers in `internal/wasmapi/testdata/`, with an
  `-update` flag as in the preview goldens. Synthetic fixtures only.
  They cover: a wall, a layered wall, a room, a layered room with
  openings, a slab, a layered slab, a refused opening, every error code,
  a preview refusal.
- Every golden answer is checked against the schema by a test, with no
  new module dependency. The builder picks how (for example a small
  checker for the schema subset the file uses).
- Fuzz targets `FuzzBuildRequest` and `FuzzReadInclude`: no panic, every
  answer is valid JSON, `ok` is false exactly when `errors` is not
  empty, every error `code` is in the list above. Added to ci.yml and
  fuzz-nightly.yml.
- Read: the existing IFC fixtures through `Read` with each part on and
  off.

## Docs

- `docs/guides/wasm.md`: loading the files, the worker API, the
  contract with one example of each answer, the error codes, and how to
  verify a release.
- README bullet, a `CHANGELOG.md` Unreleased entry.

## Calls made by Claude (for the maintainer to check)

- Error codes: the five `PlacementReason` values, plus `invalid_input`,
  `invalid_ifc`, `too_large`, `nothing_to_draw` and the JS-only
  `worker_failed`. `invalid_ifc` is new: a file that does not parse is
  the file's fault, not the request's.
- Which existing Go errors wrap `ErrTooLarge` (the list above).
- If the worker dies, `goifc.js` rejects with `worker_failed` and starts
  a fresh worker for queued requests.
- The read cap was 256 MiB, measured to not fit, and is now 48 MiB;
  then the maintainer hears the number first.
- Build parity passes the same version to both sides and compares full
  float64; a last-digit difference is fixed in `geometry`.
- The read answer is all-or-nothing, and `nodes` is the flat list with
  `parentIndex`.
- A failed release job can be re-run by hand to attach the assets.
- Build caps: 16 MiB of request JSON and 10,000 items. Items keep the Go
  caps.
- An element id scheme with `/`, and caller ids that may not contain it.
- A preview refusal returns the built elements with a preview error,
  instead of failing the build.
- Unknown request fields are refused.
- A JSON Schema file ships with each release.
- CI fails over 1.2 MB gzipped for `goifc-build.wasm`.
- The contract version is goifc's semver.
- The contract package stays `internal/`.

## Left to the builder

- DTO names and file split inside `internal/wasmapi`.
- How `goifc.js` finds the worker script (one file with an inline
  worker, or a second file).
- The Node harness for the parity test.
- How the schema test checks the golden answers.

## Effect on consumers

The first consumer codes against this. Once approved, it gets: the file names and
where they live (release assets), the `goifc.js` API, the request and
answer shapes, the element id scheme and the error codes.

## Build notes

- 2026-11-14, the maintainer: the read cap is 48 MiB, not 256 MiB. Measured in
  Node, Go 1.25 wasm, `runtime.MemStats.Sys` after one `Read` of a
  synthetic file of dense walls (about 1 KB each, one property set each):
  16 MiB → 585 MiB, 32 → 1,159, 48 → 1,669, 64 → 2,295, 96 → 3,070; 128
  and 256 MiB run out of memory at the 4 GiB wasm32 limit. Memory is about
  35 times the file. 48 MiB keeps under 2 GiB, for headroom in a tab and
  on weaker devices. Picked over 96 MiB (about 3.1 GiB, which only fits
  where a tab can use almost 4 GiB) and over keeping 256 MiB and letting
  big files fail as `worker_failed`.
- 2026-11-14: `goifc-build.wasm` was 1,281,483 bytes gzipped, over the
  1.2 MB gate. The `regexp` package was 127 KB of it; the id check is now
  written out, and the file is 1,154,220 bytes.
- 2026-11-14: the whole contract suite, compiled to wasm and run under
  Node, gives the same bytes as on amd64, floats included. No `geometry`
  change was needed for parity.
- 2026-11-14: the Node test compares parsed answers and GLB bytes; byte
  identity of the JSON is proven by the contract suite run as wasm against
  the same goldens. `wasm/test/version.mjs` prints the built file's
  version for the release job's tag check.
- 2026-11-14: a number JSON cannot hold (not finite) becomes `null` with a
  warning in node quantities and net areas too, not only in property sets.
