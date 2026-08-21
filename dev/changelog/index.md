Source

This page is [`CHANGELOG.md`](https://github.com/blox-eng/goifc/blob/main/CHANGELOG.md) included verbatim — edit that file, not this page.

# Changelog

Notable changes to goifc. The API is unstable pre-1.0 — breaking changes land on minor versions, as the README states. Releases before v0.2.0 predate this file.

## Unreleased

## v0.9.0 — 2026-08-19

### Added

- `Scene.ElevationsWith(f, r, planes, facings)` — `Scene.Elevations` over a classification the caller already holds. `BuildFacings` dominates the cost of drawing a facade set (on a ~1,900-element model, roughly 15s of a 17s call), and its result is worth more than the drawing: `Facing.FaceArea` binned by `Facing.Azimuth` is the sound way to total a facade, because summing the sheets is not — an element with two exposed sides is drawn on two perpendicular sheets, so per-sheet totals double-count it. A caller wanting both the drawings and the quantities previously had to classify the same scene twice. A nil or empty map is taken at face value, not as a request to build one: silently classifying there would restore the exact cost the call exists to avoid, invisibly, because the drawing would still look right.
- `ImportNode.OpeningDeduction` and `ImportNode.ProjectedGross` — the two halves `NetArea` is the difference of, on the import contract. `NetArea` alone cannot be aggregated: a host with no `IfcRelVoidsElement` is ABSENT from the reconciliation, so summing nets over a facade silently drops every solid wall. Netting a total means subtracting the DEDUCTION from the gross being totalled. `ProjectedGross` comes with it because both are measured on the host's winning projection axis, which foreshortens gross and deduction by the same factor — so a caller netting an unforeshortened gross (`Facing.FaceArea`) subtracts `OpeningDeduction * (FaceArea / ProjectedGross)`, not `OpeningDeduction` raw. Without `ProjectedGross` that bias is not merely uncorrected, it is invisible. Present exactly when `NetArea` is, so an untrusted host never publishes a zero deduction that would read as "this wall has no openings".
- `ImportNode.HasOpenings` — whether the element carries `IfcRelVoidsElement` openings at all. This is the fact the nil-able area fields cannot express: they are absent for two OPPOSITE reasons — the host has none, so its net equals its gross; or its reconciliation was refused, so its net is unknown. Reading absence as "no openings" reports a fully-glazed wall as solid; reading it as "unknown" drops every solid wall from a facade total. Always meaningful, so a plain bool rather than a third nil-able field.

## v0.8.1 — 2026-08-18

### Added

- `ImportNode.OpeningPerimeter` — the opening union's boundary length (m) on the import contract, so the measurement v0.8.0 added is actually reachable by consumers, which read `ImportNode` rather than `NetArea`. Present exactly when `NetArea` is: both are published from one trusted reconciliation, so a consumer can never read a confident perimeter beside an absent net.

## v0.8.0 — 2026-08-18

### Added

- `NetArea.OpeningPerimeter` — the boundary length, in metres, of the same opening union `OpeningDeduction` measures the area of. Facade trades bill reveals (the returns around a window or door) per linear metre, and the length they follow is the union's outline rather than the sum of the individual voids' perimeters: where two footprints merge, the seam between them is interior to the union, and it belongs to the outline no more than the shared area belongs to the deduction twice. It is `0` when the host is untrusted, exactly like `OpeningDeduction`, and both numbers come from one boundary walk so they can never describe different shapes. On a real ArchiCAD export it agrees to within 0.1 m with an independent per-void bounding-box measurement, and every host satisfies `perimeter² >= 4·pi·area`.

## v0.7.0 — 2026-08-17

### Changed

- `Assembled` gained an unexported field recording the `*step.File` it was built from, so `BuildImportFrom` can reject an assembly paired with a different file rather than silently joining data across models. **Breaking for unkeyed composite literals** — `Assembled{r, s}` no longer compiles; callers using keyed fields (`Assembled{Result: r, Scene: s}`) are unaffected.
- The projected-polygon union engine now reports a closed boundary or refuses the host outright, rather than silently returning the residual of an unclosed walk. `unionArea2D` and `unionBoundary` gained an `ok` result, `silhouetteRings` returns no rings instead of a wrong one, and `NetAreas` marks such a host untrusted with a reason — the mechanism it already had for exactly this. Fixes a defect where an unclosed boundary's area integral, taken about the world origin, returned the residual scaled by the model's distance from the origin rather than a slightly wrong number (a 0.58 m² panel 47 m out reported 30.69 m²).

### Added

- `(geometry.Element).SilhouetteOn(p)` — the projected-polygon union of the faces opposing `p.N`, as seen from the `+p.N` side, in `p`'s UV coordinates. Unlike `FootprintOn` it never falls back to a bounding box: a caller asking for a projection is asking a question a rectangle does not answer, so an absent outline is reported as absent.
- `geometry.ElevationPlane(dir)` — the vertical plane an elevation drawn along `dir` uses: world up stays up on the page, unlike `PlaneFromNormal`'s unspecified in-plane orientation. `ok` is false for a non-finite `dir` or one with no horizontal component (straight up or down is a plan, not an elevation).
- `geometry.ElevationView` / `ElevationEntity` — the orthographic facade view a plane projects: each entity's outline and punched-out openings in the plane's UV coordinates, plus its depth from the viewer. Membership is deliberately narrow — `ExposureExterior` only, outward normal facing the viewer — so a courtyard wall is excluded and a slab, roof or column (no dominant vertical face family, so no `Facing`) never appears. Outline area minus opening area reconciles with that host's `NetAreas` entry when both are measured on the same plane.
- `(*geometry.Scene).ElevationOn(f, r, p)` — projects the scene onto `p` and returns its elevation, classifying facings on every call.
- `(*geometry.Scene).Elevations(f, r, planes)` — one view per plane, classifying the scene's facings at most once (on the first valid plane) and sharing that work across the rest, since `BuildFacings` doesn't depend on the plane. An invalid plane yields its zero-value view in that position rather than being dropped, so the result stays index-aligned with `planes`.
- `ifc.BuildImportFrom(f, a)` — `BuildImport` split into its `Assemble` stage plus the import-contract assembly, so a caller that also wants the `Result` or `Scene` (for an elevation, a net-area check, a GLB) can reuse the tessellation `Assemble` already did instead of paying for it twice. `Assembled` must be paired with the same `*step.File` it was built from (checked by pointer identity) — `BuildImportFrom` rejects a mismatched pair rather than risk an `ExpressID` collision silently joining one model's data onto another's element.

## v0.6.0 — 2026-08-17

### Changed

- `geometry`: an opening now deducts its true projected silhouette from a host's net area, rather than its span. (#23)

## v0.5.0 — 2026-08-17

### Changed

- `geometry`: the silhouette is now computed as a true projected-polygon union, rather than approximated. (#22)

## v0.4.0 — 2026-08-17

### Changed

- `geometry`: net area now deducts the union of a host's opening footprints, rather than their sum — an opening overlapping another no longer gets double-deducted. (#20)

## v0.3.0 — 2026-08-17

### Added

- `geometry.Facing` — an element's outward direction in world space: the area-weighted dominant normal of its vertical faces, signed to point at the exposed side, with an `Exposure` and a `Confidence`.
- `geometry.Exposure` and its three values — `ExposureExterior` (the side reaches open air outside the building, and the only exposure that belongs on a compass elevation), `ExposureEnclosed` (a void the building encloses with nothing overhead: a courtyard, a lightwell — weather-exposed but on no elevation), `ExposureInterior` (no exposed side; an internal partition).
- `geometry.FacingOf(e)` — the facing of one element, or `ok == false` when it has no dominant vertical face family (a column, a slab, a degenerate mesh). With no neighbours both sides reach open air, so an element classified alone gets its axis, `ExposureExterior`, an ARBITRARY sign and low confidence.
- `geometry.BuildFacings(elems)` — classifies every element against the others, keyed by `GlobalID`. This is what lets the sign be decided at all; prefer it to `FacingOf` whenever the neighbours are available. Elements with no facade are absent from the map rather than present with a zero value.
- `(geometry.Facing).Azimuth(trueNorth)` — the compass bearing in degrees clockwise from `trueNorth`, in `[0, 360)`. Pass `model.TrueNorth(file)` for a real bearing, or `{0,1}` for a model-space one.
- `geometry.LayerAxis(e, ls)` — the world direction a layer stack runs along, from the first declared layer toward the last. Compare it against a `Facing.Normal` to learn whether the declared order already runs from the exposed face inward: a negative dot product means the first declared layer is the outermost. The library reports the direction; reordering is the consumer's decision.
- `model.TrueNorth(f)` — the model's north direction in world XY, unit length, off `IfcGeometricRepresentationContext` attribute 5. Absent, malformed and zero-length norths all yield `(0,1)`.

### Known limitations

- **Grid resolution is the failure mode.** The outward sign comes from a horizontal-slice occupancy grid at a fixed 10 cm cell, so a gap narrower than one cell seals and an element thinner than one cell vanishes. Rasterization is conservative — a cell the cross-section touches counts as occupied — so the failure leans toward sealing, which reads as `ExposureInterior` at low confidence rather than leaking open air into a room. A slice taken through a fully glazed storey with no modelled mullions finds no occupancy to enclose it and reports its walls freestanding, again at low confidence. Threshold on `Confidence`: below ~0.5 the sign is a guess, and in a quantity context a wrong bin is a wrong invoice.
- **`Facing.FaceArea` is the GROSS facade area of one side.** It is measured against the resolved `Normal` after the sign is known, so summing it over the elements binned to one elevation gives that elevation's area. Openings are not subtracted — for net quantities use `NetArea`. It is the only place this package reads triangle winding: a mesh wound inward throughout reports the element's inner face, equal on a plain wall and smaller on a stepped one.
- The sign is probed outward from the element's bounding-box **centre**, so an L-shaped or strongly curved element whose centre falls outside its own body degrades to low confidence.
- `BuildFacings` costs O(bands × elements), where a band is a distinct quantized element mid-height rather than a storey. Peak memory is one grid regardless of the band count, but the time is not bounded that way.

## v0.2.0 — 2026-08-13

### Breaking

- `geometry.LoopBelow` is renamed to `geometry.LoopSilhouette`. The constant's string value is UNCHANGED (`"below"`), so drawing data already on disk stays valid and renderers matching the literal need no change. The fix is source-only: rename the identifier at the call site.

The old name described the only plane the package could cut — a horizontal one, always seen from above. Now that any plane can be cut, "below" is no longer what the role means, but it is still what the wire format says.

### Added

- `geometry.Plane` — a cutting plane as an origin plus an orthonormal right-handed basis (`U`, `V` span the plane; `N` is the normal).
- `geometry.HorizontalPlane(cutZ)` — the plane `Footprint` has always cut.
- `geometry.PlaneFromNormal(origin, n)` — derives a valid basis from a normal. `ok == true` guarantees `Valid()`. The in-plane orientation of `U` is deterministic but unspecified; build the `Plane` yourself if you need a particular one.
- `geometry.Plane.Valid()` — reports whether a basis is finite, orthonormal and right-handed. `SectionOn` and `FootprintOn` emit no rings for a plane that fails it, so a caller that hand-builds a basis can check up front and tell a bad basis apart from a plane that genuinely missed the mesh.
- `(geometry.Element).SectionOn(p)` — the closed cut rings where an element's mesh crosses `p`, in `p`'s UV coordinates. Cut rings only: it never falls back to a silhouette or a bounding box, so a caller building a section learns that the plane missed rather than receiving a fabricated outline.
- `geometry.FootprintOn(e, p)` — `Footprint` generalized to any plane, keeping the cut → silhouette → bounding-box fallback ladder.

### Changed

- `geometry.Footprint(e, cutZ)` keeps its signature and behaviour for every finite `cutZ`; it now delegates through `HorizontalPlane`. A non-finite `cutZ` yields nil, where it previously returned a ring built around NaN.

### Fixed

- `PlaneFromNormal` no longer reports success for a finite normal whose squared length overflows to infinity, which left an all-zero basis behind.
- The bounding-box fallback no longer emits NaN corners for an element whose bounds were never measured.

### Known limitations

- A plane that contains two edges of a solid while bisecting it yields no cut rings. A triangle contributes a crossing segment only when it has a vertex strictly above and one strictly below, so faces meeting the plane along an edge contribute nothing and the ring cannot close. Through `FootprintOn` this degrades further: the cut is returned tagged `LoopSilhouette` rather than `LoopCut`.
- For a non-closed mesh the silhouette depends on which way `N` points, and one of the two directions can fall through to the bounding-box fallback. For a closed solid the outline is invariant under flipping `N`.
