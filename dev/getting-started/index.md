# Getting started

## Install

```
go get github.com/blox-eng/goifc
```

The package is named `ifc`, not `goifc`. Alias the import:

```
import ifc "github.com/blox-eng/goifc"
```

## Two entry points

Everything starts with `step.ParseBytes`. From there, pick by whether you want a flat list or a tree:

| Call                 | Returns        | Use when                                                                                                                                                                                                               |
| -------------------- | -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ifc.Assemble(f)`    | `*Assembled`   | You want the elements and their numbers. A flat `[]model.Element` with quantities back-filled, plus the `*geometry.Scene` they came from.                                                                              |
| `ifc.BuildImport(f)` | `*ImportModel` | You are importing the model into your own store. Spatial containers and physical elements in ONE parents-first tree (`ParentIndex` always points backwards), plus per-type material layers and per-storey floor plans. |

`BuildImport` calls `Assemble` and adds the structure around it. It is the call Blox actually ships — the flat list is rarely what an importer wants, because a wall means little without the storey it sits on.

Both hand back a `*geometry.Scene`, and `Scene.WriteGLB(w)` writes a Y-up GLB whose node names are `GlobalId`s — so a viewer's picked node is a database key, with no side table to maintain.

## Quickstart: elements and quantities

```
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
            continue // no volume for this element — see Quantities
        }
        fmt.Printf("%s\t%.3f m³\t(%s)\n", e.Name, *e.Qto.Volume, e.QuantitySource)
    }

    var glb bytes.Buffer
    a.Scene.WriteGLB(&glb) // proxy geometry for a viewer
}
```

`e.QuantitySource` says whether the number is the modeller's or a bound derived from the proxy mesh. A tier-2 volume is gross — read [quantities and provenance](https://blox-eng.github.io/goifc/latest/dev/concepts/quantities/index.md) before you put these numbers in front of anyone.

## Quickstart: walking the tree

```
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

Because nodes are emitted parents-first, you can create each row as you walk and resolve `ParentIndex` against ids you have already written. No second pass.

## Next

- [The pipeline](https://blox-eng.github.io/goifc/latest/dev/concepts/pipeline/index.md) — what each of the three stages does, and how to use one on its own.
- [Sections and floor plans](https://blox-eng.github.io/goifc/latest/dev/guides/sections/index.md) — cutting the model with a plane.
- [Limitations](https://blox-eng.github.io/goifc/latest/dev/limitations/index.md) — the edges, stated plainly.
