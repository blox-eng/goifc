# Limitations

The edges, stated plainly. Each is a chosen boundary, not an undiagnosed bug.

## Geometry-derived volume is gross, not net

Tier-2 volumes are un-subtracted solids: a wall over-reports by its windows and doors. See [quantities and provenance](https://blox-eng.github.io/goifc/latest/dev/concepts/quantities/index.md) — the tag exists so you can tell tier 1 from tier 2 and treat tier 2 as a bound.

## Tessellated geometry is proxy geometry

The meshes in the GLB are simplified representations for visualization. They are not a substitute for a B-rep kernel.

Do not clash-detect with them.

## No EXPRESS schema

The semantic layer reads positional attribute indices that are stable across IFC2X3 and IFC4 for core entities. Exotic or schema-specific attributes are not reachable this way.

The `step` layer below it is honest about the same boundary: `IsA` and `ByType` are exact-type only, and there is no subtype expansion or attribute-by-name.

## A section plane lying exactly along a solid's edges emits nothing

A triangle only contributes a crossing segment when it has a vertex strictly above and one strictly below. A face that merely touches the plane cannot close a ring.

This is a known gap with a test pinning it. See [sections and floor plans](https://blox-eng.github.io/goifc/latest/dev/guides/sections/index.md).

## Storeys with no contained mesh produce no plan

`BuildImport` omits them from `StoreyPlans` rather than guessing a cut plane. See [storey plans](https://blox-eng.github.io/goifc/latest/dev/guides/storey-plans/index.md).

## Scope of hardening

The well-trodden path is `BuildImport` on architectural IFC exports, because that is what Blox's import pipeline exercises. Off that path — MEP-heavy models, structural-analysis exports, non-architectural STEP — expect to find edges.

Finding one is a useful bug report. See [compatibility](https://blox-eng.github.io/goifc/latest/dev/compatibility/index.md) for what is and is not promised.
