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

## Should you use it

Pick goifc when deployment cost dominates and bounding numbers are good enough.
Pick IfcOpenShell when the geometry has to be exact — it is the better library,
and it is also a C++ toolchain, a Python runtime, and a container several times
the size of the service using it. goifc is the subset you need to parse the
file, walk the semantics, and tessellate enough to get numbers.

The [feature-by-feature comparison](https://blox-eng.github.io/goifc/latest/)
is on the docs site.

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
[getting started](https://blox-eng.github.io/goifc/latest/getting-started/).

## The numbers are labelled, and some of them are bounds

Every element reports where its quantities came from — `"qto"` for an authored
`IfcElementQuantity` (net, from the modeller), `"geometry"` for one derived from
the proxy mesh (**gross** — a wall over-reports by its windows and doors), and
`"none"` where neither exists, never a fabricated `0.0`.

That tag is the most important thing to understand before trusting a total:
[quantities and provenance](https://blox-eng.github.io/goifc/latest/concepts/quantities/).

The meshes are proxy geometry for visualization, not a B-rep substitute — do not
clash-detect with them. The rest of the edges, stated plainly, are in
[limitations](https://blox-eng.github.io/goifc/latest/limitations/).

## More

- [The pipeline](https://blox-eng.github.io/goifc/latest/concepts/pipeline/) — `step`,
  `model` and `geometry`, each usable on its own.
- [Sections and floor plans](https://blox-eng.github.io/goifc/latest/guides/sections/) —
  cut the model with any plane, get closed 2D rings back.
- [Storey plans](https://blox-eng.github.io/goifc/latest/guides/storey-plans/) — what
  `BuildImport` pre-bakes per `IfcBuildingStorey`.
- [Local and world frames](https://blox-eng.github.io/goifc/latest/concepts/frames/) —
  meshes are local, bounding boxes are world, and mixing them is wrong without
  erroring.
- [The `step` package](https://blox-eng.github.io/goifc/latest/step/) — parses any STEP
  file, IFC or not.

## Compatibility

The API is unstable pre-1.0 — expect breaking changes on minor versions, and pin
a version. Used in production by Blox, whose import pipeline is the only consumer
this has been hardened against, so the well-trodden path is `BuildImport` on
architectural IFC exports; off that path, expect to find edges.

Both of those have a fuller answer, including the serialization contracts that
hold steady even when the Go API does not, in the
[compatibility policy](https://blox-eng.github.io/goifc/latest/compatibility/).

## Contributing

Issues and PRs welcome — the [open issues](https://github.com/blox-eng/goifc/issues)
are the roadmap. See [CONTRIBUTING.md](CONTRIBUTING.md). Commits follow
[Conventional Commits](https://www.conventionalcommits.org/); CI enforces it.

## License

MIT — see [LICENSE](LICENSE).
