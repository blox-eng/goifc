package geometry_test

import (
	"errors"
	"fmt"
	"strings"

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

// ExampleWritePreviewSVG draws a 4 × 3 m room as a small isometric SVG.
// Style it with CSS on the classes top, left, right and opening.
func ExampleWritePreviewSVG() {
	outline := [][2]float64{{0, 0}, {4, 0}, {4, 3}, {0, 3}}
	walls, err := geometry.BuildRoomWalls(outline, 0, 2.70, 0.20, geometry.SideLeft, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	var svg strings.Builder
	detail, err := geometry.WritePreviewSVG(&svg, walls, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(detail)
	fmt.Println(strings.SplitN(svg.String(), "\n", 2)[0])
	// Output:
	// exact
	// <svg xmlns="http://www.w3.org/2000/svg" viewBox="-2.514 -5.336 5.736 5.609" data-detail="exact">
}

// ExampleBuildPlan builds a 4 m through wall and a 0.1 m partition that meets
// its face. The strip the partition covers is taken off the through wall's net
// area.
func ExampleBuildPlan() {
	through := geometry.PlanWall{ID: "T", Edge: [2][2]float64{{0, 0}, {4, 0}}, Height: 2.5, Thickness: 0.2, Side: geometry.SideLeft}
	partition := geometry.PlanWall{
		ID: "P", Edge: [2][2]float64{{2, 3}, {2, 0}}, Height: 2.5, Thickness: 0.1, Side: geometry.SideLeft,
		Joins: []geometry.Join{{End: geometry.EndB, Wall: "T"}},
	}
	built, err := geometry.BuildPlan(geometry.Plan{Walls: []geometry.PlanWall{through, partition}})
	if err != nil {
		fmt.Println(err)
		return
	}
	q := built.Walls[0].Wall.Quantities
	fmt.Printf("measured joins %.2f m²\n", q.MeasuredJoins)
	fmt.Printf("measured net %.2f m²\n", q.MeasuredNet)
	// Output:
	// measured joins 0.25 m²
	// measured net 9.75 m²
}

// ExampleBuildSlabPolygon builds a 10 m by 8 m floor with a 2.5 m by 1.2 m
// stair opening in it. The hole's walls count toward the slab, and the area
// is the floor less the opening.
func ExampleBuildSlabPolygon() {
	s, err := geometry.BuildSlabPolygon(geometry.Polygon2D{
		Outer: [][2]float64{{0, 0}, {10, 0}, {10, 8}, {0, 8}},
		Holes: [][][2]float64{{{2, 2}, {4.5, 2}, {4.5, 3.2}, {2, 3.2}}},
	}, 3, 0.20)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("area %.2f m²\n", s.Quantities.Area)
	fmt.Printf("hole perimeter %.2f m\n", s.Quantities.HolePerimeter)
	// Output:
	// area 77.00 m²
	// hole perimeter 7.40 m
}
