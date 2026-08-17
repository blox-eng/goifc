# Changelog

Notable changes to goifc. The API is unstable pre-1.0 — breaking changes land on
minor versions, as the README states. Releases before v0.2.0 predate this file.

## Unreleased

### Changed

- `Assembled` gained an unexported field recording the `*step.File` it was
  built from, so `BuildImportFrom` can reject an assembly paired with a
  different file rather than silently joining data across models. **Breaking
  for unkeyed composite literals** — `Assembled{r, s}` no longer compiles;
  callers using keyed fields (`Assembled{Result: r, Scene: s}`) are
  unaffected.

### Added

- `geometry.Facing` — an element's outward direction in world space: the
  area-weighted dominant normal of its vertical faces, signed to point at the
  exposed side, with an `Exposure` and a `Confidence`.
- `geometry.Exposure` and its three values — `ExposureExterior` (the side
  reaches open air outside the building, and the only exposure that belongs on
  a compass elevation), `ExposureEnclosed` (a void the building encloses with
  nothing overhead: a courtyard, a lightwell — weather-exposed but on no
  elevation), `ExposureInterior` (no exposed side; an internal partition).
- `geometry.FacingOf(e)` — the facing of one element, or `ok == false` when it
  has no dominant vertical face family (a column, a slab, a degenerate mesh).
  With no neighbours both sides reach open air, so an element classified alone
  gets its axis, `ExposureExterior`, an ARBITRARY sign and low confidence.
- `geometry.BuildFacings(elems)` — classifies every element against the others,
  keyed by `GlobalID`. This is what lets the sign be decided at all; prefer it
  to `FacingOf` whenever the neighbours are available. Elements with no facade
  are absent from the map rather than present with a zero value.
- `(geometry.Facing).Azimuth(trueNorth)` — the compass bearing in degrees
  clockwise from `trueNorth`, in `[0, 360)`. Pass `model.TrueNorth(file)` for a
  real bearing, or `{0,1}` for a model-space one.
- `geometry.LayerAxis(e, ls)` — the world direction a layer stack runs along,
  from the first declared layer toward the last. Compare it against a
  `Facing.Normal` to learn whether the declared order already runs from the
  exposed face inward: a negative dot product means the first declared layer is
  the outermost. The library reports the direction; reordering is the
  consumer's decision.
- `model.TrueNorth(f)` — the model's north direction in world XY, unit length,
  off `IfcGeometricRepresentationContext` attribute 5. Absent, malformed and
  zero-length norths all yield `(0,1)`.

### Known limitations

- **Grid resolution is the failure mode.** The outward sign comes from a
  horizontal-slice occupancy grid at a fixed 10 cm cell, so a gap narrower than
  one cell seals and an element thinner than one cell vanishes. Rasterization
  is conservative — a cell the cross-section touches counts as occupied — so
  the failure leans toward sealing, which reads as `ExposureInterior` at low
  confidence rather than leaking open air into a room. A slice taken through a
  fully glazed storey with no modelled mullions finds no occupancy to enclose
  it and reports its walls freestanding, again at low confidence. Threshold on
  `Confidence`: below ~0.5 the sign is a guess, and in a quantity context a
  wrong bin is a wrong invoice.
- **`Facing.FaceArea` is the GROSS facade area of one side.** It is measured
  against the resolved `Normal` after the sign is known, so summing it over the
  elements binned to one elevation gives that elevation's area. Openings are
  not subtracted — for net quantities use `NetArea`. It is the only place this
  package reads triangle winding: a mesh wound inward throughout reports the
  element's inner face, equal on a plain wall and smaller on a stepped one.
- The sign is probed outward from the element's bounding-box **centre**, so an
  L-shaped or strongly curved element whose centre falls outside its own body
  degrades to low confidence.
- `BuildFacings` costs O(bands × elements), where a band is a distinct quantized
  element mid-height rather than a storey. Peak memory is one grid regardless of
  the band count, but the time is not bounded that way.

## v0.2.0 — 2026-08-13

### Breaking

- `geometry.LoopBelow` is renamed to `geometry.LoopSilhouette`. The constant's
  string value is UNCHANGED (`"below"`), so drawing data already on disk stays
  valid and renderers matching the literal need no change. The fix is source-only:
  rename the identifier at the call site.

  The old name described the only plane the package could cut — a horizontal one,
  always seen from above. Now that any plane can be cut, "below" is no longer
  what the role means, but it is still what the wire format says.

### Added

- `geometry.Plane` — a cutting plane as an origin plus an orthonormal
  right-handed basis (`U`, `V` span the plane; `N` is the normal).
- `geometry.HorizontalPlane(cutZ)` — the plane `Footprint` has always cut.
- `geometry.PlaneFromNormal(origin, n)` — derives a valid basis from a normal.
  `ok == true` guarantees `Valid()`. The in-plane orientation of `U` is
  deterministic but unspecified; build the `Plane` yourself if you need a
  particular one.
- `geometry.Plane.Valid()` — reports whether a basis is finite, orthonormal and
  right-handed. `SectionOn` and `FootprintOn` emit no rings for a plane that
  fails it, so a caller that hand-builds a basis can check up front and tell a
  bad basis apart from a plane that genuinely missed the mesh.
- `(geometry.Element).SectionOn(p)` — the closed cut rings where an element's
  mesh crosses `p`, in `p`'s UV coordinates. Cut rings only: it never falls back
  to a silhouette or a bounding box, so a caller building a section learns that
  the plane missed rather than receiving a fabricated outline.
- `geometry.FootprintOn(e, p)` — `Footprint` generalized to any plane, keeping
  the cut → silhouette → bounding-box fallback ladder.

### Changed

- `geometry.Footprint(e, cutZ)` keeps its signature and behaviour for every
  finite `cutZ`; it now delegates through `HorizontalPlane`. A non-finite `cutZ`
  yields nil, where it previously returned a ring built around NaN.

### Fixed

- `PlaneFromNormal` no longer reports success for a finite normal whose squared
  length overflows to infinity, which left an all-zero basis behind.
- The bounding-box fallback no longer emits NaN corners for an element whose
  bounds were never measured.

### Known limitations

- A plane that contains two edges of a solid while bisecting it yields no cut
  rings. A triangle contributes a crossing segment only when it has a vertex
  strictly above and one strictly below, so faces meeting the plane along an
  edge contribute nothing and the ring cannot close. Through `FootprintOn` this
  degrades further: the cut is returned tagged `LoopSilhouette` rather than
  `LoopCut`.
- For a non-closed mesh the silhouette depends on which way `N` points, and one
  of the two directions can fall through to the bounding-box fallback. For a
  closed solid the outline is invariant under flipping `N`.
