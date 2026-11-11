# goifc

Read IFC in Go. Parse the file, walk the semantics, tessellate the geometry, get the numbers — each one labelled with where it came from.

```
go get github.com/blox-eng/goifc
```

[Get started](https://docs.goifc.org/latest/getting-started/index.md) [API reference](https://pkg.go.dev/github.com/blox-eng/goifc)

## What you get

- **Schemas** — IFC2X3 and IFC4 core entities, read by a STEP parser that takes any STEP file.
- **Elements** — semantics, placements, units, materials and their layers, storeys.
- **Geometry** — proxy meshes, bounding boxes, GLB, plane sections, storey plans, elevations, and which side of each wall faces outside.
- **Quantities** — authored (`qto`), or derived from the mesh (`geometry`). goifc does not subtract openings from a derived volume, though a mesh the file supplies may already have them cut; a wall's net elevation area, openings subtracted, comes from `Scene.NetAreas`.
- **Untrusted files** — tessellation, extraction and storey plans are bounded by the file's size, and a budget that runs out says so.

Where goifc's numbers are bounds rather than truth, it says so in the data — see [quantities and provenance](https://docs.goifc.org/latest/concepts/quantities/index.md) — and the edges it does not cover are written down in [limitations](https://docs.goifc.org/latest/limitations/index.md).

## Where to go next

- **[Getting started](https://docs.goifc.org/latest/getting-started/index.md)**

  Install, parse a file, and get elements with quantities out of it.

- **[The pipeline](https://docs.goifc.org/latest/concepts/pipeline/index.md)**

  Three stages — `step`, `model`, `geometry` — each usable on its own.

- **[Sections and floor plans](https://docs.goifc.org/latest/guides/sections/index.md)**

  Cut the tessellated model with any plane and get closed 2D rings back.

- **[Measuring silhouettes](https://docs.goifc.org/latest/guides/measuring-silhouettes/index.md)**

  An element's outline on any plane, and the area several outlines cover, with the overlap counted once.

- **[The `step` package](https://docs.goifc.org/latest/step/index.md)**

  A schema-agnostic STEP/SPF parser that works on any STEP file, IFC or not.

## API reference

Package documentation lives on pkg.go.dev, which is generated from the source and always matches the release you are importing:

[`ifc`](https://pkg.go.dev/github.com/blox-eng/goifc) · [`step`](https://pkg.go.dev/github.com/blox-eng/goifc/step) · [`model`](https://pkg.go.dev/github.com/blox-eng/goifc/model) · [`geometry`](https://pkg.go.dev/github.com/blox-eng/goifc/geometry)

This site covers the things godoc cannot: how the pieces fit together, what the numbers mean, and where the edges are.

How it measures up against IfcOpenShell and web-ifc on the same job and the same cores — faster and leaner, with less exact geometry — is in [benchmarks](https://docs.goifc.org/latest/benchmarks/index.md).

A question that is not an issue belongs on [Discord](https://discord.gg/x8XSW8RD4s).
