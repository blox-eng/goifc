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

Read IFC in Go. Parse the file, walk the semantics, tessellate the geometry, get
the numbers — each one labelled with where it came from.

**[goifc.org](https://goifc.org)** ·
**[Documentation](https://docs.goifc.org/latest/)** — guides, concepts and the
compatibility policy ·
**[Discord](https://discord.gg/x8XSW8RD4s)**

**API reference:** [`ifc`](https://pkg.go.dev/github.com/blox-eng/goifc) ·
[`step`](https://pkg.go.dev/github.com/blox-eng/goifc/step) ·
[`model`](https://pkg.go.dev/github.com/blox-eng/goifc/model) ·
[`geometry`](https://pkg.go.dev/github.com/blox-eng/goifc/geometry)

## Install

```bash
go get github.com/blox-eng/goifc
```

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

`Assemble` gives you a flat list. `ifc.BuildImport(f)` is the other entry point:
the same elements as a parents-first tree with spatial containers, per-type
material layers and pre-baked floor plans — the call Blox actually ships. See
[getting started](https://docs.goifc.org/latest/getting-started/).

## The numbers are labelled, and some of them are bounds

Every element reports where its quantities came from — `"qto"` for an authored
`IfcElementQuantity` (net, from the modeller), `"geometry"` for one derived from
the proxy mesh (**gross** — a wall over-reports by its windows and doors), and
`"none"` where neither exists, never a fabricated `0.0`.

That tag is the most important thing to understand before trusting a total:
[quantities and provenance](https://docs.goifc.org/latest/concepts/quantities/).

The meshes are proxy geometry for visualization, not a B-rep substitute — do not
clash-detect with them. The rest of the edges, stated plainly, are in
[limitations](https://docs.goifc.org/latest/limitations/).

How much of a real model is a bound rather than a tessellated shape is measured,
not asserted: a gate asserts goifc's bounds contain an independent reference
implementation's on a public IFC corpus, and the fallback rate per model is
published in
[coverage](https://docs.goifc.org/latest/coverage/).

## How it compares

Benchmarked against IfcOpenShell and web-ifc on seven models from 0.1 to 56 MB,
doing the same job (no tool cuts openings) on the same cores. The 49 MB
Schependomlaan housing model, one core, medians:

| | goifc | web-ifc | IfcOpenShell |
|---|---|---|---|
| Parse + walk + tessellate | **0.68 s** | 2.5 s | 17.1 s |
| Parse | **0.30 s** | 0.38 s | 0.74 s |
| Walk: properties, quantities, containers | **0.24 s** | 1.9 s | 0.76 s |
| Tessellate | **0.14 s** | 0.23 s | 15.6 s |
| Triangles | 254k | 262k | 262k |
| Peak memory | **398 MiB** | 627 MiB | 458 MiB |
| On all 26 cores, whole job | **0.22 s** | — | 4.3 s |
| Cold start | **3 ms** | 266 ms | 314 ms |
| Install | **3.2 MB binary** | 24 MB | 230 MB |

Across the real models, on one core, the whole job is 2.7–4.1× faster than
web-ifc, reading properties 3–11× faster, and parse about level. Where goifc is
worse is geometry: it does not cut openings out of walls, and even on the same
job a few walls per model come out with loose bounding boxes where web-ifc
matches IfcOpenShell exactly. Method, fairness caveats and every table:
[benchmarks](https://docs.goifc.org/latest/benchmarks/).

## More

- [The pipeline](https://docs.goifc.org/latest/concepts/pipeline/) — `step`,
  `model` and `geometry`, each usable on its own.
- [Sections and floor plans](https://docs.goifc.org/latest/guides/sections/) —
  cut the model with any plane, get closed 2D rings back.
- [Storey plans](https://docs.goifc.org/latest/guides/storey-plans/) — what
  `BuildImport` pre-bakes per `IfcBuildingStorey`.
- [Local and world frames](https://docs.goifc.org/latest/concepts/frames/) —
  meshes are local, bounding boxes are world, and mixing them is wrong without
  erroring.
- [The `step` package](https://docs.goifc.org/latest/step/) — parses any STEP
  file, IFC or not.

## Compatibility

The API is unstable pre-1.0 — expect breaking changes on minor versions, and pin
a version. Used in production by Blox, whose import pipeline is the only consumer
this has been hardened against, so the well-trodden path is `BuildImport` on
architectural IFC exports; off that path, expect to find edges.

Both of those have a fuller answer, including the serialization contracts that
hold steady even when the Go API does not, in the
[compatibility policy](https://docs.goifc.org/latest/compatibility/).

## Contributing

Issues and PRs welcome — the [open issues](https://github.com/blox-eng/goifc/issues)
are the roadmap. See [CONTRIBUTING.md](CONTRIBUTING.md). Commits follow
[Conventional Commits](https://www.conventionalcommits.org/); CI enforces it.
For a question that is not an issue, there is a
[Discord](https://discord.gg/x8XSW8RD4s).

## License

MIT — see [LICENSE](LICENSE).
