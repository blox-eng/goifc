# Limitations

The honest edges. None of these are undiagnosed bugs — each is a boundary that
was chosen, and each has a test or a tag pinning it.

## Geometry-derived volume is gross, not net

On the `"geometry"` tier the extrude path reports the solid, un-subtracted
volume. A wall with a window and a door over-reports against IfcOpenShell's net
figure.

Netting openings out of extruded geometry is out of scope — it needs a real B-rep
kernel. The [provenance tag](concepts/quantities.md) exists so you can tell these
apart and treat tier 2 as a bound.

## Tessellated geometry is proxy geometry

The meshes in the GLB are simplified representations for visualization. They are
not a substitute for a B-rep kernel.

Do not clash-detect with them.

## No EXPRESS schema

The semantic layer reads positional attribute indices that are stable across
IFC2X3 and IFC4 for core entities. Exotic or schema-specific attributes are not
reachable this way.

The `step` layer below it is honest about the same boundary: `IsA` and `ByType`
are exact-type only, and there is no subtype expansion or attribute-by-name.

## A section plane lying exactly along a solid's edges emits nothing

A triangle only contributes a crossing segment when it has a vertex strictly
above and one strictly below. A face that merely touches the plane cannot close
a ring.

This is a known gap with a test pinning it. See
[sections and floor plans](guides/sections.md).

## Storeys with no contained mesh produce no plan

`BuildImport` omits them from `StoreyPlans` rather than guessing a cut plane.
See [storey plans](guides/storey-plans.md).

## Scope of hardening

The well-trodden path is `BuildImport` on architectural IFC exports, because
that is what Blox's import pipeline exercises. Off that path — MEP-heavy models,
structural-analysis exports, non-architectural STEP — expect to find edges.

Finding one is a useful bug report. See
[compatibility](compatibility.md) for what is and is not promised.
