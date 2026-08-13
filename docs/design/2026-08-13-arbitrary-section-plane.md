# Section geometry on an arbitrary plane

Design for issue #3. Status: approved, ready to plan.

## Problem

`geometry.Footprint` and its `sectionRings` helper cut only a horizontal plane. The Z
axis is hardcoded into the distance term (`geometry/section.go:88`):

```go
ds := [3]float64{verts[0][2] - cutZ, verts[1][2] - cutZ, verts[2][2] - cutZ}
```

and the emitted rings drop to world XY (`section.go:57`, `section.go:130-132`). That
serves a floor plan and nothing else. A caller wanting a vertical section — an
elevation, a cross-section, a cut on a skewed plane — must reimplement the pipeline,
even though the hard parts are already here and are plane-agnostic:
`stitchParityRings` (parity cancellation for shared internal faces), `nestEvenOdd`
(hole nesting), `canonicalRing`, and the deterministic ordering.

Only the distance computation and the 3D→2D projection care which plane it is.

## Non-goals

- Elevation views. Projecting a facade with occlusion and punched openings is issue
  #6; this issue supplies the UV basis that work projects into, nothing more.
- Outward-facing classification (issue #4). Nothing here needs to know which way a
  wall faces.
- Any change to how rings are stitched, nested, wound, or ordered.

## API

```go
// Plane is a cutting plane: a point on it plus an orthonormal basis. U and V
// span the plane and define the 2D coordinates of emitted rings; N is the
// normal. The basis MUST be orthonormal and right-handed (N == U x V) — see
// "Basis validity" below.
type Plane struct {
    Origin  [3]float64
    U, V, N [3]float64
}

// HorizontalPlane returns the z = cutZ plane with U=+X, V=+Y, N=+Z — the plane
// Footprint cuts today.
func HorizontalPlane(cutZ float64) Plane

// PlaneFromNormal derives a right-handed orthonormal basis for the plane
// through origin with normal n, choosing U deterministically. Returns ok=false
// for a zero-length or non-finite n.
func PlaneFromNormal(origin, n [3]float64) (Plane, bool)

// SectionOn returns the closed CUT rings where e's mesh crosses p, in p's UV
// coordinates, hole-nested and tagged LoopCut. Empty when the plane misses the
// mesh or p's basis is invalid. Determinism and winding guarantees match the
// horizontal path exactly.
func (e Element) SectionOn(p Plane) []Loop

// FootprintOn is Footprint generalized to p: cut rings if the plane crosses the
// solid, else the silhouette of faces facing away from N, else the element's
// AABB projected into p's UV.
func FootprintOn(e Element, p Plane) []Loop
```

`Footprint(e, cutZ)` keeps its signature and delegates to
`FootprintOn(e, HorizontalPlane(cutZ))`. Nothing downstream changes.

`SectionOn` is cut-rings-only while `FootprintOn` carries the three-tier fallback
ladder, because a caller building an elevation wants to know the plane missed —
silently receiving an AABB rectangle instead would be a fabricated outline.

## Implementation

Three substitutions, all above the reusable machinery:

| Today | Becomes |
|---|---|
| `ds[i] = verts[i][2] - cutZ` (`section.go:88`) | `ds[i] = dot(verts[i] - p.Origin, p.N)` |
| drop Z: `{v[0], v[1]}` (`section.go:57`, `61-64`, `130-132`) | `{dot(v-p.Origin, p.U), dot(v-p.Origin, p.V)}` |
| `n[2]/l >= -1e-6` (`section.go:127`) | `dot(n, p.N)/l >= -1e-6` |

Everything downstream of `ds` works on signs and interpolation parameters, so it
carries over untouched: `triCrossing`, `stitchParityRings`, `walkFace`, `nextEdge`,
`canonicalRing`, `nestEvenOdd`, `representativePoint`, `ringSelfIntersects`.

`sectionOnPlaneEps` and `sectionWeldQuantum` stay as they are — both are already in
meters, and a dot-product distance is in meters too.

## Basis validity

Right-handedness is load-bearing in four places that currently assume the horizontal
basis without saying so:

- `ensureCCW` and the `polygonArea2D(poly) <= sectionAreaEps` sign gate (`section.go:344-347`)
- `nestEvenOdd`'s hole re-winding at odd containment depth (`section.go:204`)
- `representativePoint`'s "unit left normal `(-dy, dx)` points into a CCW ring's
  interior" (`section.go:226`)

Hand `Plane` a left-handed basis — `U=+X, V=+Y, N=-Z`, which reads as perfectly
reasonable to a caller wanting to look up rather than down — and every ring winds
backwards, so outer rings and holes swap roles. No error is raised; the drawing is
just wrong. This is the same failure shape as issue #5: a real frame rule, documented
nowhere, silently corrupting every result.

Decision: **validate, and degrade honestly.** `SectionOn` and `FootprintOn` check the
basis before doing any work:

- each of U, V, N is unit length to within 1e-9
- pairwise dot products are zero to within 1e-9
- `dot(N, cross(U, V)) > 0`

A plane failing any check yields no rings (`SectionOn` returns nil; `FootprintOn`
returns nil rather than falling back to an AABB, since the AABB projection needs the
same basis). This matches how the package already treats input it cannot trust —
`NetArea` returns a nil `Net` with a reason, `meshVolume` returns `ok=false` — rather
than panicking on caller input, which is out of character here.

`PlaneFromNormal` exists so that callers rarely hand-build a basis at all. It picks
the world axis least parallel to `n` as a seed, so its choice of U is deterministic
and never degenerate.

## AABB fallback

`Footprint` falls back to `aabbRing(e.BBoxMin, e.BBoxMax)` twice (`section.go:166`,
`section.go:175`), which builds a rectangle from world X and Y. Under an arbitrary
plane that is a rectangle in the wrong coordinate system, emitted silently as though
it were the element's outline.

`aabbRing` is replaced by a projection: transform the AABB's eight corners into UV via
`(dot(c-Origin,U), dot(c-Origin,V))`, then emit their 2D bounding rectangle, CCW. For
`HorizontalPlane` this reduces to exactly today's rectangle, so the existing tests
hold byte-identically.

The result is a bounding rectangle, not a silhouette — the same approximation the
horizontal path already makes, now merely expressed in the right frame.

## Naming

Two names in the touched file stop being true once the plane is arbitrary:

- `weldXY` (`section.go:22`) welds UV, not XY. Unexported → renamed to `weldUV`, free.
- `LoopBelow` (`section.go:147`) means "the silhouette of faces facing away from N".
  For a vertical plane that is not "below", it is the far side.

`LoopBelow` is exported and consumed downstream, so its rename is split in two:

- **The Go identifier is renamed** to `LoopSilhouette`, with dependent consumers
  updated in the same change set.
- **The string value stays `"below"`.** `LoopRole` is a string type, and consumers
  serialize it: it travels through drawing JSON, is persisted alongside generated
  plans, and appears as a literal union member in renderer code. Changing the value
  would invalidate drawings already on disk and force a coordinated renderer, server,
  and data migration in exchange for a naming improvement.

The doc comment carries the reconciliation: the constant is named for what it means,
the value is retained for compatibility with drawings already on disk.

## Testing

The regression gate matters more than the new tests: **the entire existing section
suite must pass byte-identically through the `HorizontalPlane` delegation.**
`TestSectionRingsCubeMidCut`, `TestFootprintHoleNesting`, `TestSectionRingsTJunction`,
`TestSectionRingsOnPlaneFace`, and `TestSectionRingsCornerTouch` are unchanged —
they are simply passed `HorizontalPlane(...)` at their call sites rather than
being replaced.

New cases:

- **Rotation invariance** — cut a known box horizontally, then cut the same box
  rotated 90 degrees about X with the correspondingly rotated plane. Ring areas match
  within float tolerance. This is the test that catches a wrong basis.
- **Vertical cut with a void** — a plane with N=+X through a box carrying a
  through-hole yields one outer ring and one nested hole, `LoopCut`, correct winding.
- **Oblique plane** — a 45 degree plane through a unit cube yields a ring of area
  sqrt(2), proving the UV projection is not silently assuming an axis.
- **Left-handed basis** — `U=+X, V=+Y, N=-Z` returns no rings rather than
  role-swapped ones. Guards the failure mode described above.
- **Non-orthonormal basis** — a non-unit or skewed basis likewise returns no rings.
- **`PlaneFromNormal`** — produces a valid right-handed basis for axis-aligned,
  oblique, and near-degenerate normals; `ok=false` for zero-length and non-finite.
- **Silhouette is invariant under flipping N** — the same closed box, cut with two
  opposed planes clear of the mesh, yields rings of the same area (the two planes have
  different U/V bases, so the rings cannot be byte-identical; the test compares areas
  only). This pins a property
  that is easy to assume is a hazard and is not: for a closed solid the parity boundary
  of the front-facing set equals that of the back-facing set, because edges shared by
  two front faces cancel while each silhouette edge is shared by one front and one back
  face and so survives in either set. Recorded as a test so a future reader does not
  "fix" a flip that was never wrong.
- **AABB fallback frame** — an element with no usable rings, cut on a vertical plane,
  yields a rectangle in UV whose extent matches the AABB projected onto that plane,
  not the world XY rectangle.
- **Determinism** — identical input yields byte-identical output, asserted on a
  non-axis-aligned plane where the ring start-vertex choice is least obvious.
- **Degenerate** — plane misses the mesh entirely (empty, no nil panic); plane
  coplanar with a face (no spurious ring, matching existing on-plane-face behaviour).

## What this does and does not get the caller

This issue is pure enablement — it produces nothing a user sees. It supplies the UV
basis that an elevation view (issue #6) projects into, and the same generalization
serves any vertical section, such as a wall build-up detail.

Two limits are inherited rather than introduced, and belong to the consumer to
document:

- The silhouette path stitches a parity boundary, which approximates a true projected
  polygon union. A non-convex solid whose faces sit at different depths — a facade
  with recessed balconies is exactly that shape — may retain internal boundary edges
  instead of one filled outline.
- The AABB fallback is a bounding rectangle, not an outline. It is a last resort for
  an element that yields no usable rings at all.

Neither is a regression: both are how the horizontal path already behaves.

One thing this issue explicitly does NOT guard, and the consumer must: `belowRings`
keeps faces with `dot(n, N) < -1e-6`, i.e. faces pointing away from N. Because the
silhouette outline is invariant under flipping N on a closed solid (see Testing), a
consumer that aims its view direction the wrong way gets no error and no visible
difference at this layer. Whether the drawing shows the near face or the far one is
decided by the consumer's face selection and depth sort, so that is where a flipped
view direction must be caught.

## Risks

- **Downstream lockstep.** `LoopBelow` is exported, so renaming the identifier is a
  source-breaking change for any consumer referencing it by name. Consumers must be
  updated together with this change, or they fail to build. The retained `"below"`
  string value means no runtime or data compatibility is at stake — only compilation.
- **Silent frame errors are the whole hazard of this change.** The validation above,
  plus the left-handed and non-orthonormal tests, are the mitigation, and they are not
  optional extras.
