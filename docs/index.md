# goifc

Read an IFC model from Go. No CGO, no IfcOpenShell, no OCCT, no Python sidecar.
One `go get`, one static binary.

```bash
go get github.com/blox-eng/goifc
```

[Get started](getting-started.md){ .md-button .md-button--primary }
[API reference](https://pkg.go.dev/github.com/blox-eng/goifc){ .md-button }

## Why this exists

I needed quantities out of IFC files inside a Go service. Every path led back to
IfcOpenShell — which is very good, and which is a C++ toolchain, a Python runtime,
and a container three times the size of the service using it.

So the question was never "is IfcOpenShell better." It is. The question was how
much of it I actually needed. The answer turned out to be: parse the file, walk
the semantics, tessellate enough to get numbers. That fits in a few thousand
lines of Go.

## Which one you want

|                       | goifc                            | IfcOpenShell                    |
|-----------------------|----------------------------------|---------------------------------|
| Deploy                | `go get`, static binary          | C++ toolchain, Python runtime   |
| Solids                | proxy meshes                     | exact B-rep (OCCT)              |
| Quantities            | authored, or derived + labelled  | authored, or exact from solids  |
| Openings netted out   | no — gross volume                | yes — net volume                |
| Schema coverage       | IFC2X3 / IFC4 core entities      | full EXPRESS schema             |
| Runs in a Go process  | yes                              | via subprocess or bindings      |

Pick goifc when deployment cost dominates and bounding numbers are good enough.
Pick IfcOpenShell when the geometry has to be exact.

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
