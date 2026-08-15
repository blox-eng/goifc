# Sections and floor plans

Cut the tessellated model with any plane and get closed 2D rings back, in the plane's own UV coordinates and metres.

```
p := geometry.HorizontalPlane(3.0) // cut at z = 3 m
for _, e := range a.Scene.Elements {
    for _, loop := range e.SectionOn(p) {
        _ = loop.Points // [][2]float64, outer rings CCW, holes CW
    }
}
```

For a plane that is not horizontal, `PlaneFromNormal` builds one from an origin and a normal:

```
p, ok := geometry.PlaneFromNormal([3]float64{0, 0, 3}, [3]float64{1, 0, 0})
if !ok {
    panic("degenerate normal")
}
```

`PlaneFromNormal` does not promise a particular UV orientation

Its `U` is deterministic for a given normal, but *which* in-plane orientation it picks is unspecified and may change between versions — including for a `+Z` normal, where it is **not** guaranteed to match `HorizontalPlane`. If you persist `Points`, or need to match an external convention, build the `Plane` yourself rather than rely on this choice.

## `SectionOn` versus `FootprintOn`

These two differ in exactly one way: what they do when the plane does not cross the solid.

|                                 | `e.SectionOn(p)`            | `geometry.FootprintOn(e, p)`                                |
| ------------------------------- | --------------------------- | ----------------------------------------------------------- |
| Plane crosses the solid         | cut rings, tagged `LoopCut` | cut rings, tagged `LoopCut`                                 |
| Plane misses                    | `nil`                       | silhouette of faces opposing `p.N`, tagged `LoopSilhouette` |
| Mesh degenerate / no silhouette | `nil`                       | bounding-box rectangle, tagged `LoopSilhouette`             |

**That fallback is a plan-view feature and a section-view bug.** A floor plan wants context geometry for everything on the storey, whether or not the cut height happens to pass through it. A building section wants to know the plane missed — a fabricated rectangle in a section drawing is a lie that renders convincingly.

## Loop roles

```
const (
    LoopCut        LoopRole = "cut"   // section poché — the plane crosses the solid
    LoopSilhouette LoopRole = "below" // light context
)
```

The string values are a serialization contract

`LoopSilhouette`'s value is `"below"`, not `"silhouette"`. The constant was renamed in v0.2.0; the wire value deliberately was not, so drawing data already on disk stayed valid. Renderers matching the literal `"below"` need no change. Do not change these values without a coordinated consumer migration.

## Winding and holes

Outer rings are wound CCW. Hole rings — the inner boundary of a hollow or annular section — are wound CW and **share the outer ring's `Role`**. An even-odd or nonzero polygon fill therefore renders them as cutouts with no extra field and no nesting metadata to interpret.

Coordinates come out in the IFC-native orientation. A renderer whose Y axis points down applies its own flip.

## A plane that lies along a solid's edges emits nothing

A triangle only contributes a crossing segment when it has a vertex strictly above *and* one strictly below the plane. A face that merely touches the plane along an edge contributes nothing, so the cut ring cannot close.

This is a known gap with a test pinning it, not an undiagnosed bug. In practice it bites when you cut exactly at a slab's top face — nudge the plane by a millimetre if you hit it.
