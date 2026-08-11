# goifc

A pure-Go IFC parser, semantic extractor and tessellator. No CGO, no
IfcOpenShell, no OCCT — just a Go module.

## Install

```bash
go get github.com/blox-eng/goifc
```

## Usage

```go
package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/blox-eng/goifc"
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
		e := a.Result.Elements[i] // e.Qto, e.QuantitySource ("qto"|"geometry"|"none")
		fmt.Println(e.Name, e.QuantitySource)
	}

	var glb bytes.Buffer
	a.Scene.WriteGLB(&glb) // proxy geometry for a viewer
}
```

`step.ParseBytes` parses an in-memory STEP/SPF (ISO 10303-21) document into a
navigable entity graph. `ifc.Assemble` runs the pipeline — STEP parse →
semantic extraction → proxy tessellation + derived quantities — and returns
both the semantic elements and a scene you can export to GLB.

## Limitations

**Geometry-derived volume is gross, not net.** For elements where quantities
are backfilled from geometry rather than an authored IFC quantity set (tagged
`QuantitySource == "geometry"`), the extrude path reports the solid
(un-subtracted) volume. A wall with window and door openings will over-report
volume compared to IfcOpenShell, which nets openings out. The
`quantity_source="geometry"` tag exists specifically so a consumer can tell
these apart from authored quantities and treat them as bounding estimates,
not exact figures. Netting openings out of extruded geometry is deliberately
out of scope.

**Tessellated geometry is proxy geometry, not exact solids.** The meshes
written to GLB are simplified representations suitable for visualization,
not a substitute for a full boundary-representation (B-rep) kernel. Do not
use them for anything that requires exact solid geometry (e.g. precise
clash detection).

## Stability

Used in production by Blox. The API is unstable pre-1.0: expect breaking
changes on minor versions. No support SLA.

## License

MIT — see [LICENSE](LICENSE).
