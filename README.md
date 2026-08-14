<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/assets/goifc-mark-dark.svg">
  <img alt="goifc" src=".github/assets/goifc-mark.svg" width="76">
</picture>

# goifc

[![Go Reference](https://pkg.go.dev/badge/github.com/blox-eng/goifc.svg)](https://pkg.go.dev/github.com/blox-eng/goifc)
[![CI](https://github.com/blox-eng/goifc/actions/workflows/ci.yml/badge.svg)](https://github.com/blox-eng/goifc/actions/workflows/ci.yml)
[![CodeQL](https://github.com/blox-eng/goifc/actions/workflows/codeql.yml/badge.svg)](https://github.com/blox-eng/goifc/actions/workflows/codeql.yml)
[![Go 1.25](https://img.shields.io/badge/go-1.25-00ADD8.svg)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-black.svg)](LICENSE)

[![cgo: disabled](https://img.shields.io/badge/cgo-disabled-00ADD8.svg)](.github/workflows/ci.yml)
[![dependencies: 1](https://img.shields.io/badge/dependencies-1-00ADD8.svg)](go.mod)
[![codecov](https://codecov.io/gh/blox-eng/goifc/branch/main/graph/badge.svg)](https://codecov.io/gh/blox-eng/goifc)
[![govulncheck](https://img.shields.io/badge/govulncheck-enforced-00ADD8.svg)](.github/workflows/ci.yml)

Read an IFC model from Go. No CGO, no IfcOpenShell, no OCCT, no Python sidecar.
One `go get`, one static binary.

**[Documentation](https://blox-eng.github.io/goifc/)** — guides, concepts and the
compatibility policy.

**API reference:** [`ifc`](https://pkg.go.dev/github.com/blox-eng/goifc) ·
[`step`](https://pkg.go.dev/github.com/blox-eng/goifc/step) ·
[`model`](https://pkg.go.dev/github.com/blox-eng/goifc/model) ·
[`geometry`](https://pkg.go.dev/github.com/blox-eng/goifc/geometry)

## Why this exists

I needed quantities out of IFC files inside a Go service. Every path led back to
IfcOpenShell — which is very good, and which is a C++ toolchain, a Python runtime,
and a container three times the size of the service using it.

So the question was never "is IfcOpenShell better." It is. The question was how
much of it I actually needed. The answer turned out to be: parse the file, walk
the semantics, tessellate enough to get numbers. That fits in a few thousand
lines of Go.

This is that subset, extracted and made honest about its edges.

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

## Install

```bash
go get github.com/blox-eng/goifc
```

## What you get

Everything starts with `step.ParseBytes`. From there, two entry points — pick by
whether you want a flat list or a tree:

| Call                | Returns        | Use when                                                        |
|---------------------|----------------|-----------------------------------------------------------------|
| `ifc.Assemble(f)`   | `*Assembled`   | You want the elements and their numbers. A flat `[]model.Element` with quantities back-filled, plus the `*geometry.Scene` they came from. |
| `ifc.BuildImport(f)`| `*ImportModel` | You are importing the model into your own store. Spatial containers and physical elements in ONE parents-first tree (`ParentIndex` always points backwards), plus per-type material layers and per-storey floor plans. |

`BuildImport` calls `Assemble` and adds the structure around it. It is the call
Blox actually ships — the flat list is rarely what an importer wants, because
a wall means little without the storey it sits on.

Both hand back a `*geometry.Scene`, and `Scene.WriteGLB(w)` writes a Y-up GLB
whose node names are `GlobalId`s — so a viewer's picked node is a database key,
with no side table to maintain.

## Quickstart

```go
package main

import (
	"bytes"
	"fmt"
	"os"

	ifc "github.com/blox-eng/goifc"
	"github.com/blox-eng/goifc/step"
)

func main() {
	src, err := os.ReadFile("model.ifc")
	if err != nil {
		panic(err)
	}

	f, err := step.ParseBytes(src)
	if err != nil {
		panic(err)
	}

	a, err := ifc.Assemble(f)
	if err != nil {
		panic(err)
	}

	for i := range a.Result.Elements {
		e := a.Result.Elements[i]
		if e.Qto.Volume == nil {
			continue // no volume for this element — see the next section
		}
		fmt.Printf("%s\t%.3f m³\t(%s)\n", e.Name, *e.Qto.Volume, e.QuantitySource)
	}

	var glb bytes.Buffer
	a.Scene.WriteGLB(&glb) // proxy geometry for a viewer
}
```

The package is named `ifc`, not `goifc` — alias the import as above.

Walking the tree instead:

```go
m, err := ifc.BuildImport(f)
if err != nil {
	panic(err)
}
for _, n := range m.Nodes {
	depth := 0
	for p := n.ParentIndex; p != nil; p = m.Nodes[*p].ParentIndex {
		depth++
	}
	fmt.Printf("%*s%s (%s)\n", depth*2, "", n.Name, n.IFCClass)
}
```

Because nodes are emitted parents-first, you can create each row as you walk and
resolve `ParentIndex` against ids you have already written. No second pass.

## The numbers are labelled, and some of them are bounds

Every element reports where its quantities came from — `"qto"` for an authored
`IfcElementQuantity` (net, from the modeller), `"geometry"` for one derived from
the proxy mesh (**gross** — a wall over-reports by its windows and doors), and
`"none"` where neither exists, never a fabricated `0.0`.

That tag is the most important thing to understand before trusting a total:
[quantities and provenance](https://blox-eng.github.io/goifc/concepts/quantities/).

The meshes are proxy geometry for visualization, not a B-rep substitute — do not
clash-detect with them. The rest of the edges, stated plainly, are in
[limitations](https://blox-eng.github.io/goifc/limitations/).

## More

- [The pipeline](https://blox-eng.github.io/goifc/concepts/pipeline/) — `step`,
  `model` and `geometry`, each usable on its own.
- [Sections and floor plans](https://blox-eng.github.io/goifc/guides/sections/) —
  cut the model with any plane, get closed 2D rings back.
- [Storey plans](https://blox-eng.github.io/goifc/guides/storey-plans/) — what
  `BuildImport` pre-bakes per `IfcBuildingStorey`.
- [Local and world frames](https://blox-eng.github.io/goifc/concepts/frames/) —
  meshes are local, bounding boxes are world, and mixing them is wrong without
  erroring.
- [The `step` package](https://blox-eng.github.io/goifc/step/) — parses any STEP
  file, IFC or not.

## Compatibility

The API is unstable pre-1.0 — expect breaking changes on minor versions, and pin
a version. Used in production by Blox, whose import pipeline is the only consumer
this has been hardened against, so the well-trodden path is `BuildImport` on
architectural IFC exports; off that path, expect to find edges.

Both of those have a fuller answer, including the serialization contracts that
hold steady even when the Go API does not, in the
[compatibility policy](https://blox-eng.github.io/goifc/compatibility/).

## Contributing

Issues and PRs welcome — the [open issues](https://github.com/blox-eng/goifc/issues)
are the roadmap. See [CONTRIBUTING.md](CONTRIBUTING.md). Commits follow
[Conventional Commits](https://www.conventionalcommits.org/); CI enforces it.

## License

MIT — see [LICENSE](LICENSE).
