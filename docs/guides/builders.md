# Building geometry from numbers

The builders in `geometry` make walls, the walls of a room, and slabs from plain
numbers: an edge or an outline in plan, a height, a thickness, and the openings.
There is no IFC file. Each builder returns an `Element` (a closed mesh, ready for
GLB), exact quantities computed from the numbers, and the face it was measured
on, as data.

Use them when you know a building's dimensions but have no model: a sketch, a
survey, or a form someone filled in.

## A wall with a door

```go
edge := [2][2]float64{{0, 0}, {2.40, 0}}
door := geometry.Opening{Offset: 0.50, Sill: 0, Width: 0.80, Height: 2.00}
w, err := geometry.BuildWall(edge, 0, 2.60, 0.10, geometry.SideLeft, []geometry.Opening{door})
if err != nil {
	return err
}
q := w.Quantities
fmt.Printf("measured net %.2f m²\n", q.MeasuredNet) // 4.64
fmt.Printf("other net    %.2f m²\n", q.OtherNet)    // 4.64
fmt.Printf("net volume   %.3f m³\n", q.NetVolume)   // 0.464
```

This snippet is
[`ExampleBuildWall`](https://pkg.go.dev/github.com/blox-eng/goifc/geometry#example-BuildWall),
which the test suite runs.

The arguments, in order:

| Argument | Meaning |
|---|---|
| `edge` | The wall's measured face in plan, from a to b, in metres. |
| `base` | The elevation of the wall's bottom. A storey is its `base`. |
| `height`, `thickness` | In metres, each at least 1 mm. |
| `side` | `SideLeft` measures the face on the left of a→b, and the wall grows to the right. `SideRight` is the mirror. As text, `"left"` or `"right"`. |
| `openings` | Rectangles on the measured face: `Offset` along the edge from a, `Sill` up from the base, `Width`, `Height`. |

## The quantities

| Quantity | Meaning |
|---|---|
| `Length`, `Height` | The measured face. |
| `MeasuredGross`, `MeasuredNet` | The measured face, before and after openings, in m². |
| `OtherGross`, `OtherNet` | The opposite face. At a room corner it is longer or shorter than the measured face. |
| `RevealPerimeter` | The opening edges, in m. A door (`Sill: 0`) has no bottom edge. |
| `GrossVolume`, `NetVolume` | In m³. |
| `NicheBack` | The back panels of the niches, in m². |

These quantities are exact: they come from the numbers, not from the mesh. The
mesh agrees with them. Its volume is `NetVolume` within float32 rounding.

## Refusals

A misplaced opening is refused, not clipped. The builder checks every opening
and returns all the refusals at once, joined with `errors.Join`. Each refusal is
a `*PlacementError` that names the wall, the opening and the reason:

```go
door := geometry.Opening{Offset: 2.00, Sill: 0, Width: 0.80, Height: 2.00}
_, err := geometry.BuildWall(edge, 0, 2.60, 0.10, geometry.SideLeft, []geometry.Opening{door})
fmt.Println(err) // wall 0 opening 0: too_wide

var pe *geometry.PlacementError
if errors.As(err, &pe) {
	fmt.Println("opening", pe.Opening, "is", pe.Reason) // opening 0 is too_wide
}
```

Each opening gets the first rule it breaks, checked in this order:

| Reason | The opening |
|---|---|
| `too_wide` | reaches or passes an end of the measured face. |
| `above_wall` | reaches or passes the top of the wall. |
| `overlaps` | overlaps or touches an earlier opening. |
| `in_corner` | cuts into a room corner, where the opposite face stops short. |

"Touches" means closer than 1 mm. Wall thinner than that would not be wall.

Input that is not a placement problem, such as a negative height or a NaN, is
a plain error. A refusal list never contains such an error.

## The walls of a room

`BuildRoomWalls` takes a closed outline instead of an edge. Wall `i` runs from
`outline[i]` to `outline[i+1]`, and `openings[i]` holds wall `i`'s openings.
Neighbouring walls meet on a mitre, so the core of each corner counts once, and
each measured face stops at the outline's corner.

```go
room := [][2]float64{{0, 0}, {2.40, 0}, {2.40, 1.80}, {0, 1.80}}
walls, err := geometry.BuildRoomWalls(room, 0, 2.60, 0.10, geometry.SideLeft, nil)
```

A counter-clockwise outline with `SideLeft` measures the inside of the room.
Here the measured faces total 21.84 m², the outer faces 23.92 m², and the
volume 2.288 m³. A refusal's `Wall` field says which wall it belongs to.

## Slabs

```go
s, err := geometry.BuildSlab([][2]float64{{0, 0}, {5, 0}, {5, 4}, {0, 4}}, 0, 0.20)
// s.Quantities: Area 20 m², Perimeter 18 m, Volume 4 m³
```

The outline can wind either way and has no holes. The top face is at `z`, and
the slab extends down by the thickness.

## Niches

An opening with a `Depth` is a niche. It is cut from the measured face to that
depth, and the opposite face stays whole. A niche needs a sill, and its depth
must stay at least 1 mm inside the wall.

```go
niche := geometry.Opening{Offset: 1.00, Sill: 1.20, Width: 0.40, Height: 0.60, Depth: 0.08}
```

In the 2.40 × 2.60 × 0.10 wall, this niche leaves `MeasuredNet` at 6.00 m²,
`OtherNet` at 6.24 m² (whole), and `NicheBack` at 0.24 m².

## Layers

A wall or slab is often several materials. `BuildWallLayers`,
`BuildRoomWallLayers` and `BuildSlabLayers` take the layer thicknesses in
metres, measured face first for walls and top first for slabs, and return one
`Wall` or `Slab` per layer, in that order. The element's thickness is their
sum.

```go
layers := []float64{0.015, 0.175, 0.10} // plaster, brick, insulation
ws, err := geometry.BuildWallLayers(edge, 0, 2.50, layers, geometry.SideLeft, openings)
// ws[0] is the plaster, ws[2] the insulation
```

Each layer's quantities mean what they mean for a whole wall, for that layer
alone. `MeasuredGross` and `MeasuredNet` are its face toward the measured
side; `OtherGross` and `OtherNet` its face away from it. The layers' net
volumes add up to the whole wall's.

- **Corners.** In a room each layer mitres at its own depth, so it reports its
  own length: at an outside corner the outer layer is longer than the inner
  one.
- **Openings** are checked once, on the whole wall, so a layered wall refuses
  exactly what the same single wall refuses. A through opening cuts every
  layer, and each layer lists it in its own `Measured.Openings`.
- **Niches** cut the layers they pass through, are a niche in the layer where
  their back falls, and leave the rest whole. A back exactly on a layer
  boundary is the next layer's `NicheBack`. A back under 1 mm from a boundary
  is refused as `layer_sliver`.
- **`RevealPerimeter`** repeats in every layer an opening cuts; it does not
  add up across layers. A layer's reveal area is `RevealPerimeter` × its
  thickness.

At a corner that is almost straight, a layer's face end can snap by under
0.1 mm, like the whole wall's mitre does, so its mesh and numbers agree.

## Not yet

Walls that meet mid-wall (T-joins) are not built yet. Build the partition as a
separate wall for now; see [limitations](../limitations.md).
