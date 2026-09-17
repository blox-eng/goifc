# Measuring silhouettes

`Element.SilhouetteOn` gives you the outline of one element on one plane.
`UnionArea2D` measures **several** of those outlines as the one surface they
cover, with the overlap counted once.

```go
p, ok := geometry.ElevationPlane([3]float64{0, -1, 0}) // looking north
if !ok {
	panic("no plane")
}

var polys []geometry.Polygon2D
for _, e := range face { // the elements you have decided clad this plane
	loops := e.SilhouetteOn(p)
	if len(loops) == 0 {
		continue // an absent outline, reported absent
	}
	polys = append(polys, polygonFromLoops(loops))
}

area, ok := geometry.UnionArea2D(polys)
if !ok {
	// No figure. Do NOT substitute 0 — see "When it refuses" below.
	return
}
fmt.Printf("%.2f m² of facade\n", area)
```

## The question it answers

An element's own area is a tier-2 quantity: gross, per element, and summed by
whoever reads it. Sum two elements that overlap and you have counted the
overlap twice.

A facade does this constantly. A cladding band runs in front of the wall behind
it; a pilaster stands proud of the plane it sits on; a parapet and the slab
under it share a strip. Those are **one surface** to anyone estimating,
painting, cladding or invoicing it, and the union is the only figure that says
so.

The same reasoning already runs inside the library: net area deducts the
*union* of a host's openings rather than their sum, so two overlapping openings
are not double-deducted. This is that operation, exported, for outlines the
caller is holding rather than elements the library is holding.

## The shape it takes

```go
type Polygon2D struct {
	Outer [][2]float64   // the outer ring
	Holes [][][2]float64 // the voids inside it
}
```

Rings are implicitly closed — do not repeat the first point as the last — and
coordinates are metres in **one plane's (u, v) frame**.

`SilhouetteOn` returns a flat `[]Loop`, hole-nested by winding rather than by
structure. Splitting those rings into an outer and its holes is the one
conversion between the two APIs, and it is yours to make: the ring with the
largest absolute area is the outer one, every other ring a hole.

Winding decides nothing here. The rings are filled even-odd, exactly as
[`Loop`](sections.md) says a renderer may treat them, so an outline that has
been stored, re-serialized and read back still measures correctly even if
something along the way had its own opinion about orientation.

!!! warning "One frame, or nothing"
    The polygons must already be expressed in the **same** plane's frame.
    `PlaneFromNormal`'s in-plane basis is deterministic but explicitly
    unspecified, and it differs per normal — two elements whose normals differ
    by a rounding error can get two different frames and union into a figure
    that means nothing. For a facade, derive one frame for the whole plane
    (`ElevationPlane` does, with world up as `v`) and project every member
    onto it. Nothing in this function can detect a frame mismatch.

## The perimeter

`UnionMeasure2D` returns the area and the boundary length from **one** walk of
**one** boundary:

```go
area, perimeter, ok := geometry.UnionMeasure2D(polys)
```

Taking the two from separate walks risks an area and a perimeter that describe
different shapes, which is why there is no separate perimeter function.

The perimeter is the whole boundary of the union, not only its outer
silhouette: a void the union still has is edge too. What it excludes is a
**seam** — where two polygons merge, the line between them is interior and
carries no boundary, exactly as the shared area is counted once.

## When it refuses

`ok == false` means there is **no figure**, not a figure of zero. Zero is a
measurement; this is the absence of one. Substituting `0` puts a number nothing
downstream can tell apart from a real empty facade.

It refuses when:

- there are no polygons at all;
- a ring has fewer than three points, or encloses no area;
- any coordinate is `NaN` or infinite;
- the rings do not describe the surface they claim — a hole that is not inside
  its outer ring, two holes that overlap each other, a boundary that crosses
  itself. "Outer minus holes" and the region the rings actually enclose are
  then two different numbers, and there is no way to know which one was meant;
- the union boundary did not close.

That last one is the same refusal `SilhouetteOn` already makes, and for the
same reason: the area integral is taken about the world origin, so an unclosed
boundary does not return a slightly wrong number — it returns the residual
multiplied by the model's distance from the origin. A 0.58 m² panel 47 m out
reports 30.69 m².

## A bridged outline is not a quantity source

`SilhouetteBridgedOn` can close an outline across one short gap so a *drawing*
is not torn. That outline is right to draw and wrong to measure: a segment no
face in the mesh accounts for was invented, and the area invented with it is up
to the gap length times the local extent.

Do not feed a bridged outline to the union. Measure with `NetAreas` or
`Facing.FaceArea`, or take the silhouette from `SilhouetteOn`, which refuses
rather than repairs.

## Cost

The decomposition behind the union is a vertical sweep, quadratic in the vertex
count of the largest single polygon, and then a boundary walk over the pieces.
A facade outline is tens of vertices. Nothing refuses a polygon for being
large — there is no cap — but a machine-generated outline with thousands of
vertices is worth simplifying before it gets here.
