package ifc_test

import (
	"bytes"
	"fmt"

	"github.com/blox-eng/goifc"
	"github.com/blox-eng/goifc/step"
)

// ExampleAssemble shows the end-to-end entry: raw IFC bytes -> parse -> Assemble
// -> per-element quantities + a GLB export, all from one orchestration call.
func ExampleAssemble() {
	f, err := step.ParseBytes([]byte(boxIFC))
	if err != nil {
		fmt.Println("parse:", err)
		return
	}
	a, err := ifc.Assemble(f)
	if err != nil {
		fmt.Println("assemble:", err)
		return
	}

	fmt.Println("elements:", len(a.Result.Elements))
	for i := range a.Result.Elements {
		e := a.Result.Elements[i]
		h := 0.0
		if e.Qto.Height != nil {
			h = *e.Qto.Height
		}
		fmt.Printf("%s source=%s height=%.2fm\n", e.Name, e.QuantitySource, h)
	}

	var glb bytes.Buffer
	fmt.Println("glb written:", a.Scene.WriteGLB(&glb) == nil && glb.Len() > 0)

	// Output:
	// elements: 1
	// Box source=geometry height=1.00m
	// glb written: true
}
