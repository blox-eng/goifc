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
[![tests: 198](https://img.shields.io/badge/tests-198-00ADD8.svg)](https://github.com/blox-eng/goifc/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/blox-eng/goifc/branch/main/graph/badge.svg)](https://codecov.io/gh/blox-eng/goifc)
[![govulncheck](https://img.shields.io/badge/govulncheck-enforced-00ADD8.svg)](.github/workflows/ci.yml)

Read an IFC model from Go. No CGO, no IfcOpenShell, no OCCT, no Python sidecar.
One `go get`, 6,500 lines, one static binary.

**Docs:** [`ifc`](https://pkg.go.dev/github.com/blox-eng/goifc) ·
[`step`](https://pkg.go.dev/github.com/blox-eng/goifc/step) ·
[`model`](https://pkg.go.dev/github.com/blox-eng/goifc/model) ·
[`geometry`](https://pkg.go.dev/github.com/blox-eng/goifc/geometry)

## Why this exists

I needed quantities out of IFC files inside a Go service. Every path led back to
IfcOpenShell — which is very good, and which is a C++ toolchain, a Python runtime,
and a container three times the size of the service using it.

So the question was never "is IfcOpenShell better." It is. The question was how
much of it I actually needed. The answer turned out to be: parse the file, walk
the semantics, tessellate enough to get numbers. That fits in 6,500 lines of Go.

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
			continue // no volume for this element — see "Quantities" below
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

## The pipeline

`Assemble` runs three stages, each usable on its own:

| Package    | Does                                                                       |
|------------|----------------------------------------------------------------------------|
| `step`     | STEP/SPF (ISO 10303-21) tokenizer and entity graph. Schema-agnostic — no EXPRESS. |
| `model`    | Entity graph → semantic `[]Element`. Ports ifcopenshell's `util.element`, `util.placement`, `util.unit`. |
| `geometry` | Proxy tessellation, derived quantities, plane sections, and a Y-up GLB whose node names are `GlobalId`s. |

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

## Sections and floor plans

Cut the tessellated model with any plane and get closed 2D rings back, in the
plane's own UV coordinates and metres:

```go
p, ok := geometry.PlaneFromNormal([3]float64{0, 0, 3}, [3]float64{0, 0, 1})
if !ok {
	panic("degenerate normal")
}
for _, e := range a.Scene.Elements {
	for _, loop := range e.SectionOn(p) { // or geometry.FootprintOn(e, p)
		_ = loop.Points // [][2]float64, outer rings CCW, holes CW
	}
}
```

`geometry.HorizontalPlane(z)` is the common case. `SectionOn` returns the true
cut rings and returns nil rather than inventing an outline when the plane misses;
`FootprintOn` is the forgiving sibling that falls back to a silhouette, which is
what you want for a plan view and not what you want for a section.

`BuildImport` uses this to pre-bake `StoreyPlans` — one `StoreyPlan` per
`IfcBuildingStorey`, cut 1.2 m above the floor, with each entity's loops tagged
by `GlobalID` and IFC class. That is a renderable floor plan without a second
pass over the geometry.

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

**A section plane lying exactly along a solid's edges emits nothing.** A triangle
only contributes a crossing segment when it has a vertex strictly above and one
strictly below, so a face that merely touches the plane cannot close a ring. This
is a known gap with a test pinning it, not an undiagnosed bug.

## Status

6,532 lines of non-test Go. The 198 tests are 190 unit, 6 runnable examples and
2 fuzz targets; the one dependency is
[`qmuntal/gltf`](https://github.com/qmuntal/gltf). The fuzz jobs are a 60-second
smoke test per push — where a new crasher shows up first, not assurance on their
own.

Used in production by Blox, whose import pipeline is the only consumer this has
been hardened against — so the well-trodden path is `BuildImport` on architectural
IFC exports. Off that path, expect to find edges.

The API is unstable pre-1.0 — expect breaking changes on minor versions. v0.2.0
renamed `geometry.LoopBelow` to `LoopSilhouette`, a source-only break: the
serialized string stayed `"below"`, so stored data was unaffected. Breaks are
kept that shape where possible, but not promised. No support SLA.

## Roadmap

The open issues are the roadmap, and both are about getting more out of the
geometry that is already tessellated:

- [#4](https://github.com/blox-eng/goifc/issues/4) — classify an element's
  outward-facing direction, so facades can be binned per elevation.
- [#6](https://github.com/blox-eng/goifc/issues/6) — `ElevationView`: project a
  facade onto a vertical plane, with openings.

Netting openings out of extruded volume is deliberately **not** on it. That needs
a real B-rep kernel, and the honest answer there is IfcOpenShell.

## Contributing

Issues and PRs welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Commits follow
[Conventional Commits](https://www.conventionalcommits.org/); CI enforces it.

## License

MIT — see [LICENSE](LICENSE).
