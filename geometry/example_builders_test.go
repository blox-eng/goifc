package geometry_test

import (
	"errors"
	"fmt"

	"github.com/blox-eng/goifc/geometry"
)

// ExampleBuildWall builds a 2.40 m wall, 2.60 m high and 0.10 m thick, with a
// door, and reads its quantities. No IFC is involved.
func ExampleBuildWall() {
	edge := [2][2]float64{{0, 0}, {2.40, 0}}
	door := geometry.Opening{Offset: 0.50, Sill: 0, Width: 0.80, Height: 2.00}
	w, err := geometry.BuildWall(edge, 0, 2.60, 0.10, geometry.SideLeft, []geometry.Opening{door})
	if err != nil {
		fmt.Println(err)
		return
	}
	q := w.Quantities
	fmt.Printf("measured net %.2f m²\n", q.MeasuredNet)
	fmt.Printf("other net    %.2f m²\n", q.OtherNet)
	fmt.Printf("net volume   %.3f m³\n", q.NetVolume)
	// Output:
	// measured net 4.64 m²
	// other net    4.64 m²
	// net volume   0.464 m³
}

// ExampleBuildWall_refusal places a door that runs past the wall's end. The
// builder refuses it, and the refusal names the opening and the reason.
func ExampleBuildWall_refusal() {
	edge := [2][2]float64{{0, 0}, {2.40, 0}}
	door := geometry.Opening{Offset: 2.00, Sill: 0, Width: 0.80, Height: 2.00}
	_, err := geometry.BuildWall(edge, 0, 2.60, 0.10, geometry.SideLeft, []geometry.Opening{door})
	fmt.Println(err)
	var pe *geometry.PlacementError
	if errors.As(err, &pe) {
		fmt.Println("opening", pe.Opening, "is", pe.Reason)
	}
	// Output:
	// wall 0 opening 0: too_wide
	// opening 0 is too_wide
}

// ExampleBuildWallLayers builds a 3 m wall of plaster, brick and insulation
// with a window. Each layer has its own quantities; the window cuts all three.
func ExampleBuildWallLayers() {
	edge := [2][2]float64{{0, 0}, {3.00, 0}}
	window := geometry.Opening{Offset: 1.00, Sill: 0.90, Width: 1.20, Height: 1.20}
	layers := []float64{0.015, 0.175, 0.10} // plaster, brick, insulation
	ws, err := geometry.BuildWallLayers(edge, 0, 2.50, layers, geometry.SideLeft, []geometry.Opening{window})
	if err != nil {
		fmt.Println(err)
		return
	}
	for k, w := range ws {
		fmt.Printf("layer %d: measured net %.2f m², net volume %.4f m³\n", k, w.Quantities.MeasuredNet, w.Quantities.NetVolume)
	}
	// Output:
	// layer 0: measured net 6.06 m², net volume 0.0909 m³
	// layer 1: measured net 6.06 m², net volume 1.0605 m³
	// layer 2: measured net 6.06 m², net volume 0.6060 m³
}
