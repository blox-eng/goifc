# goifc

Read IFC in Go. Parse the file, walk the semantics, tessellate the geometry, get
the numbers — each one labelled with where it came from.

```bash
go get github.com/blox-eng/goifc
```

[Get started](getting-started.md){ .md-button .md-button--primary }
[API reference](https://pkg.go.dev/github.com/blox-eng/goifc){ .md-button }

## What you get

- **Schemas** — IFC2X3 and IFC4 core entities.
- **Elements** — semantics, placements, units, materials, storeys.
- **Geometry** — proxy meshes, bounding boxes, plane sections, GLB.
- **Quantities** — authored (`qto`), or derived from the mesh (`geometry`).
  Openings are not netted out, so a derived volume is gross.

Where goifc's numbers are bounds rather than truth, it says so in the data —
see [quantities and provenance](concepts/quantities.md) — and the edges it does
not cover are written down in [limitations](limitations.md).

## Where to go next

<div class="grid cards" markdown>

-   **[Getting started](getting-started.md)**

    Install, parse a file, and get elements with quantities out of it.

-   **[The pipeline](concepts/pipeline.md)**

    Three stages — `step`, `model`, `geometry` — each usable on its own.

-   **[Sections and floor plans](guides/sections.md)**

    Cut the tessellated model with any plane and get closed 2D rings back.

-   **[The `step` package](step.md)**

    A schema-agnostic STEP/SPF parser that works on any STEP file, IFC or not.

</div>

## API reference

Package documentation lives on pkg.go.dev, which is generated from the source and
always matches the release you are importing:

[`ifc`](https://pkg.go.dev/github.com/blox-eng/goifc) ·
[`step`](https://pkg.go.dev/github.com/blox-eng/goifc/step) ·
[`model`](https://pkg.go.dev/github.com/blox-eng/goifc/model) ·
[`geometry`](https://pkg.go.dev/github.com/blox-eng/goifc/geometry)

This site covers the things godoc cannot: how the pieces fit together, what the
numbers mean, and where the edges are.

How it measures up against IfcOpenShell and web-ifc on the same job and the same
cores — faster and leaner, with less exact geometry — is in [benchmarks](benchmarks.md).

A question that is not an issue belongs on
[Discord](https://discord.gg/mcDPQECCy).
