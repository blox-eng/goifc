<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/assets/goifc-mark-dark.svg">
  <img alt="goifc" src=".github/assets/goifc-mark.svg" width="76">
</picture>

# goifc

[![Go Reference](https://pkg.go.dev/badge/github.com/blox-eng/goifc.svg)](https://pkg.go.dev/github.com/blox-eng/goifc)
[![CI](https://github.com/blox-eng/goifc/actions/workflows/ci.yml/badge.svg)](https://github.com/blox-eng/goifc/actions/workflows/ci.yml)
[![Go 1.25](https://img.shields.io/badge/go-1.25-00ADD8.svg)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-black.svg)](LICENSE)

Read an IFC model from Go. No CGO, no IfcOpenShell, no OCCT, no Python sidecar.
One `go get`, one static binary.

## Why this exists

I needed quantities out of IFC files inside a Go service. Every path led back to
IfcOpenShell — which is very good, and which is a C++ toolchain, a Python runtime,
and a container three times the size of the service using it.

So the question was never "is IfcOpenShell better." It is. The question was how
much of it I actually needed. The answer turned out to be: parse the file, walk
the semantics, tessellate enough to get numbers. That fits in 6,000 lines of Go.

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
			continue // no volume for this element — see "Quantities" below
		}
		fmt.Printf("%s\t%.3f m³\t(%s)\n", e.Name, *e.Qto.Volume, e.QuantitySource)
	}

	var glb bytes.Buffer
	a.Scene.WriteGLB(&glb) // proxy geometry for a viewer
}
```

The package is named `ifc`, not `goifc` — alias the import as above.

## The pipeline

`Assemble` runs three stages, each usable on its own:

| Package    | Does                                                                       |
|------------|----------------------------------------------------------------------------|
| `step`     | STEP/SPF (ISO 10303-21) tokenizer and entity graph. Schema-agnostic — no EXPRESS. |
| `model`    | Entity graph → semantic `[]Element`. Ports ifcopenshell's `util.element`, `util.placement`, `util.unit`. |
| `geometry` | Proxy tessellation, derived quantities, and a Y-up GLB whose node names are `GlobalId`s. |

`step` is genuinely standalone. It parses any STEP file, IFC or not.

## Quantities carry their provenance

Every element reports where its numbers came from, because the two sources are
not equally trustworthy:

| `QuantitySource` | Means                                                        |
|------------------|--------------------------------------------------------------|
| `"qto"`          | Authored `IfcElementQuantity`. Net, exact, from the modeller. |
| `"geometry"`     | Derived from the proxy mesh. Gross — a bounding estimate.     |
| `"none"`         | Neither available. Never a fabricated `0.0`.                  |

Authored quantities always win. A missing quantity stays missing. If you only
trust one tier, filter on the tag — that is what it is for.

## Limitations

**Geometry-derived volume is gross, not net.** On the `"geometry"` tier the
extrude path reports the solid, un-subtracted volume. A wall with a window and
a door over-reports against IfcOpenShell's net figure. Netting openings out of
extruded geometry is out of scope — the tag exists so you can tell these apart
and treat them as bounds.

**Tessellated geometry is proxy geometry.** The meshes in the GLB are simplified
representations for visualization. They are not a substitute for a B-rep kernel.
Do not clash-detect with them.

**No EXPRESS schema.** The semantic layer reads positional attribute indices that
are stable across IFC2X3 and IFC4 for core entities. Exotic or schema-specific
attributes are not reachable this way.

## Status

163 tests, 82–92% statement coverage, one runtime dependency
([`qmuntal/gltf`](https://github.com/qmuntal/gltf)). CI pins `CGO_ENABLED=0`, so
the no-CGO claim is enforced rather than asserted.

Used in production by Blox. The API is unstable pre-1.0 — expect breaking
changes on minor versions. No support SLA.

## Contributing

Issues and PRs welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Commits follow
[Conventional Commits](https://www.conventionalcommits.org/); CI enforces it.

## License

MIT — see [LICENSE](LICENSE).
