# Outward-facing classification

Design for issue #4. Status: draft — **one decision open**, see "The outward sign".

## Problem

Grouping a building's walls by which elevation they face is a routine takeoff task, and
the library gives no help with it. Issue #4 measured two independent wrong answers on a
real 29 MB ArchiCAD IFC2X3 export (87 exterior walls, footprint ≈ 20.6 × 26.4 m,
non-convex with wings) before reaching a right one. Both wrong answers *looked* fine:

1. **Summing signed face normals cancels.** A wall has two large opposite faces — inner
   and outer. Area-weighted signed normals nearly cancel, so the winner is decided by
   floating-point noise. All 87 walls collapsed into two of the four bins.
2. **Mixing frames silently.** `Element.Verts` are element-local while `BBoxMin`/`BBoxMax`
   are world. Deriving normals from `Verts` and comparing against `BBox` compares two
   different spaces; in the local frame most walls extrude along the same local axis, so
   **every wall binned identically**. Nothing failed — the numbers were simply all wrong.

The second is already fixed upstream: issue #5 closed and PR #7 shipped
`Element.WorldVerts()` and `Element.WorldNormal(local)`. This design uses them and does
not re-derive world transforms.

That leaves the split this design is built around: **the axis is easy, the sign is not.**

## Non-goals

- Deciding elevations for a whole building. This classifies one element at a time (with
  its neighbours as context); binning and naming elevations is the consumer's.
- Exact geometry. Facings come off the proxy mesh, with all the caveats that carries.
- Netting openings out of anything. Out of scope here as everywhere else in this library.

## API

```go
// Facing is an element's outward direction in world space: the area-weighted
// dominant normal of its exterior-facing vertical faces.
type Facing struct {
    Normal     [3]float64 // unit, world space
    FaceArea   float64    // m² of faces voting for this direction
    Confidence float64    // 0..1; low when no face family dominates, or the sign is doubtful
}

// FacingOf returns the outward direction of e, or ok=false when the element has no
// dominant vertical face family (a column, a slab, a degenerate mesh).
func FacingOf(e Element) (Facing, bool)

// BuildFacings classifies every element against the others. Prefer it over FacingOf in
// a loop: outwardness is not a property of one element, so a lone element cannot have
// its sign decided and reports low confidence.
func BuildFacings(elems []Element) map[string]Facing

// Azimuth returns the compass bearing of f in degrees clockwise from trueNorth, in
// [0, 360). Pass model.TrueNorth(file) for a real bearing, {0,1} for a model-space one.
func (f Facing) Azimuth(trueNorth [2]float64) float64
```

Plus, in `model`, the piece nothing reads today:

```go
// TrueNorth is the model's north direction in world XY, unit length. It lives on
// IfcGeometricRepresentationContext attribute 5. Absent, malformed and zero-length
// norths all yield (0,1).
func TrueNorth(f *step.File) [2]float64
```

`Confidence` is deliberately one number covering two independent doubts — how decisively
one face family won, and how sure the sign is. A consumer that wants to show
"unclassified" rather than file a wall under the wrong elevation thresholds on it. In a
quantity context a wrong bin is a wrong invoice, so the honest failure mode matters more
than coverage.

## Implementation

**Axis.** Accumulate triangle area into *unsigned* direction buckets — fold antipodes
together before bucketing — and take the largest. Unsigned is the whole point: it is what
makes the cancellation failure above impossible rather than unlikely. Faces with
`|n.z| > cos(75°)` are roof or floor and are excluded. If the winning bucket holds less
than ~60% of total vertical face area, decline: a square column splits ~50/50 across two
axes and has no facade.

Normals come from `WorldVerts`, so a rotated `Placement` rotates the answer.

**Determinism.** Bucket merging iterates a slice, never a map, and ties keep the earlier
bucket. Identical input must yield bit-identical normals.

## The outward sign — THE OPEN DECISION

Which of the two opposite directions points away from the building is the hard half, and
a building-centroid rule is not a correct answer to it: on a non-convex footprint,
courtyard-facing and re-entrant walls sit on the far side of the centroid from the
direction they actually face. Issue #4 reproduced one elevation exactly (330.6 m² against
an independently derived 331 m²) and got the other three wrong for precisely this reason.

Three strategies, in rough order of cost:

| Strategy | Needs | Strength | Weakness |
|---|---|---|---|
| `IfcRelSpaceBoundary` | `IfcSpace` entities in the model | Authoritative — the model states which side faces which space | Absent from many exports |
| Ray casting | Nothing beyond the other elements | Robust on non-convex plans | Approximate; cost scales with element count |
| Centroid | Nothing | Trivial | Wrong on any non-convex footprint |

**Proposed: ray casting primary, centroid as an explicitly-degraded fallback,
`IfcRelSpaceBoundary` deferred.** Cast from the element's centre along both candidate
directions against the other elements' world AABBs; the direction that escapes is
outward. AABBs rather than triangles because this is a sign decision, not a visibility
computation — "escapes the building" versus "runs into more building" is all that is
being asked. When both directions are equally blocked — a courtyard wall, or a lone
element with no neighbours — return the axis at low confidence rather than a coin flip
dressed as an answer.

Rationale: ray casting needs no `IfcSpace`, and the failure case that motivated the issue
was a non-convex footprint, which is exactly what it fixes and centroid does not.
`IfcRelSpaceBoundary` is better where present and can be layered in later behind the same
`Facing` return without an API change.

**This is not settled.** Anyone picking this up should confirm the strategy before
implementing the sign; the axis work (and `TrueNorth`, and `Azimuth`) is independent of
the answer and can proceed regardless.

## Testing

Seven cases, from the issue, all of which must exist before this is done:

- **Opposite-face cancellation** — a wall long in X, thin in Y returns ±Y, asserted on
  both winding orders so the result cannot depend on triangle orientation.
- **Local vs world** — the same mesh under a 90° `Placement` rotation returns a normal
  rotated by 90°. This is the regression test for the frame bug; without it the function
  passes while being uniformly wrong.
- **Non-convex footprint** — a U-shaped plan where a naive centroid rule misassigns the
  courtyard walls. They must resolve correctly or report low confidence, never a
  confident wrong bin.
- **Four-elevation partition** — a closed rectangular room: every wall in exactly one of
  four bins, all bins non-empty, summed face area equal to total exterior vertical face
  area. This is the test that proves ray casting *works*, as opposed to degrading to low
  confidence everywhere.
- **Non-applicable geometry** — column, slab and empty mesh return `ok=false`.
- **Azimuth** — a +X-facing wall with `trueNorth=(0,1)` gives 90°; a rotated TrueNorth
  shifts it by exactly that rotation.
- **Determinism** — repeated calls on identical input return bit-identical normals.

Tests live in-package (`package geometry`) so they can use `v3`, `elemBox` and
`boxMeshWorld`, matching `geometry/section_test.go`.

## What this does and does not get the caller

It gets: elevation grouping that stops being a per-consumer reimplementation of a problem
with two well-camouflaged failure modes, a confidence signal to degrade honestly on, and
a real compass bearing instead of a guessed one.

It does not get: a correct answer on every wall of every model. Courtyards and
re-entrant geometry will report low confidence, by design, because the alternative is a
confident wrong answer.

## Risks

- **Ray casting against AABBs over-blocks.** A wall whose AABB is crossed by an unrelated
  element's AABB reads as blocked when a triangle-level test would escape. Mitigated by
  it being a two-way comparison — both directions suffer the same inflation — but a dense
  model may push more elements into the low-confidence bucket than necessary.
- **`Confidence` conflates two doubts.** Axis decisiveness and sign certainty multiply
  into one number, so a caller cannot tell which is weak. Splitting the field later is a
  breaking change; splitting it now costs nothing. Worth deciding alongside the strategy.
- **Cost is O(n²) in the naive form.** Every element against every other element's AABB.
  A real model is ~1,400 elements, so ~2M box tests — fine, but not free, and worth a
  spatial index if it shows up in a profile.

## Follow-on

Issue #6 (`ElevationView`) depends on this: it needs to know which direction each facade
view is built for. Note that #6's stated build step 1 — generalise `belowRings` from −Z to
an arbitrary direction — is **already done**; `belowRings` has taken a `Plane` since PR #3
(`geometry/section.go:127`). That issue predates the change.
