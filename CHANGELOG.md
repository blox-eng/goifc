# Changelog

Notable changes to goifc. The API is unstable pre-1.0 — breaking changes land on
minor versions, as the README states. Releases before v0.2.0 predate this file.

## v0.2.0 (unreleased)

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
