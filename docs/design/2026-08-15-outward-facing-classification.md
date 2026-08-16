# Outward-facing classification

Design for issue #4. Status: **decided** — see "The outward sign".

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

### The sign has a second consumer

Elevation binning is the visible motivation, but it is not the only thing blocked on the
sign. `MaterialLayers` reads `IfcMaterialLayerSetUsage.DirectionSense` and deliberately
does **not** reorder the layer list, because the sense says which way the stack runs from
the reference line and that is not enough on its own to say which end is outside — that
needs the element's placement too (`model/materiallayers.go`). So any consumer that wants
a build-up ordered from the exposed face inward — the near-universal convention for
describing a wall — currently has to take the declared file order, which is a coin flip
per element. A reversed stack silently swaps the outer cladding with the inner finish.

The same fact answers a third question: whether an element sits on the weather boundary
at all, which is what separates an external envelope element from an internal partition.

**So the primitive worth shipping is not "which way does this face" but "which side of
this element is exposed".** Elevation binning, build-up ordering and envelope-vs-interior
classification are three consumers of that one fact.

## Non-goals

- Deciding elevations for a whole building. This classifies one element at a time (with
  its neighbours as context); binning and naming elevations is the consumer's.
- Exact geometry. Facings come off the proxy mesh, with all the caveats that carries.
- Netting openings out of anything. Out of scope here as everywhere else in this library.
- Mapping `Exposure` onto a product vocabulary. The library reports what the geometry
  says; naming it is the consumer's job, as with `LayerSet.Direction`.

## API

```go
// Exposure is what the Facing.Normal side of an element reaches.
type Exposure string

const (
    // ExposureExterior — the side reaches open air outside the building.
    ExposureExterior Exposure = "exterior"
    // ExposureEnclosed — the side reaches a void the building encloses: a
    // courtyard, a lightwell, a shaft. Weather-exposed, but on no elevation.
    ExposureEnclosed Exposure = "enclosed"
    // ExposureInterior — no exposed side was found; an internal partition.
    ExposureInterior Exposure = "interior"
)

// Facing is an element's outward direction in world space: the area-weighted
// dominant normal of its vertical faces, signed to point at the exposed side.
type Facing struct {
    Normal   [3]float64 // unit, world space; points at the exposed side
    VoteArea float64    // m² of vertical face that VOTED for this axis — not facade area
    Exposure Exposure
    // Confidence is 0..1. The sign is resolved by probing outward from the
    // element's BBox centre, so an L-shaped or strongly curved element whose
    // centre falls outside its own body degrades to low confidence.
    Confidence float64
}

// FacingOf returns the facing of e, or ok=false when the element has no dominant
// vertical face family (a column, a slab, a degenerate mesh). Outwardness is not a
// property of one element: with no neighbours both sides reach open air, so an
// element classified alone gets its axis, ExposureExterior, an arbitrary sign and
// low confidence. Prefer BuildFacings whenever the neighbours are available.
func FacingOf(e Element) (Facing, bool)

// BuildFacings classifies every element against the others, which is what lets the
// sign be decided at all.
func BuildFacings(elems []Element) map[string]Facing

// Azimuth returns the compass bearing of f in degrees clockwise from trueNorth, in
// [0, 360). Pass model.TrueNorth(file) for a real bearing, {0,1} for a model-space one.
func (f Facing) Azimuth(trueNorth [2]float64) float64

// LayerAxis returns the world direction ls stacks along, from the first declared
// layer toward the last, or ok=false when the usage carries no resolvable
// direction. Compare it against a Facing.Normal to learn whether the declared
// order already runs from the exposed face inward: a negative dot product means
// the first declared layer is the outermost. This library reports the direction;
// reordering is the consumer's decision.
func LayerAxis(e Element, ls model.LayerSet) ([3]float64, bool)
```

Plus, in `model`, the piece nothing reads today:

```go
// TrueNorth is the model's north direction in world XY, unit length. It lives on
// IfcGeometricRepresentationContext attribute 5. Absent, malformed and zero-length
// norths all yield (0,1).
func TrueNorth(f *step.File) [2]float64
```

`VoteArea` is named for what it is. The axis vote folds antipodal faces together — that
folding is the whole point of the axis stage — so a wall's inner and outer face both
count toward the winning axis and a 6 m × 3 m free-standing wall reports 36 m², roughly
twice its facade. No fixed divisor recovers the real number: it is 2× free-standing and
somewhere between 1× and 2× when the element abuts a neighbour. The field is a diagnostic
for how decisively the axis won, not a quantity. Facade area is `NetArea`'s job.

`Confidence` stays a single number. An earlier draft split it into axis and sign
components; naming `Exposure` explicitly makes that unnecessary, because the field a
caller would have branched on is now the enum rather than a threshold. One number, one
question: how much to trust this row.

A consumer that would rather show "unclassified" than file a wall under the wrong
elevation thresholds on `Confidence` and checks `Exposure`. In a quantity context a wrong
bin is a wrong invoice, so the honest failure mode matters more than coverage.

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

## The outward sign

Which of the two opposite directions points away from the building is the hard half, and
a building-centroid rule is not a correct answer to it: on a non-convex footprint,
courtyard-facing and re-entrant walls sit on the far side of the centroid from the
direction they actually face. Issue #4 reproduced one elevation exactly (330.6 m² against
an independently derived 331 m²) and got the other three wrong for precisely this reason.

**Decision: a horizontal-slice occupancy grid with a flood fill from outside.**

For the Z band around an element's mid-height:

1. Rasterize into a 2D bitset, in world XY, every element triangle crossing that band.
2. Flood-fill *unoccupied* cells inward from the grid border. The reached set is open air.
3. Probe a cell one step off the element along `+n` and `-n`.

| probe | result |
|---|---|
| one side open air, the other not | sign resolved, `ExposureExterior` |
| a side unoccupied but unreachable from the border | `ExposureEnclosed` — a void the building encloses |
| both sides open air | freestanding; `ExposureExterior`, arbitrary sign, low confidence |
| neither side open air | `ExposureInterior` |

Two earlier candidates were rejected:

| Strategy | Why not |
|---|---|
| Centroid | Wrong on any non-convex footprint — the failure that motivated the issue |
| Ray casting to "escape" | Under-specified: for any wall the inward ray also escapes, just later, so the rule collapses into "which direction has less material along it" — the centroid heuristic wearing a local disguise, and it still fails on a thin wing. It also cannot distinguish an enclosed void from the outdoors at all |
| `IfcRelSpaceBoundary` | Authoritative where present, but spatial nodes carry no geometry today (`model/spatial.go`) and the relation is unparsed, so it is a much larger build. Deferred — it can be layered in behind the same `Facing` return with no API change |

The flood fill is chosen because it answers the actual question — *is this side connected
to the outdoors?* — rather than approximating it. Non-convexity is simply irrelevant to a
connectivity search, and `ExposureEnclosed` falls out of the same pass for free. It needs
no `IfcSpace`, no polygon boolean library, and no floating-point boundary cases.

Slicing per Z band rather than projecting the whole building matters: a roof slab
projected flat would seal every courtyard, and an upper-storey overhang would corrupt the
storey beneath it. Bands are cached per distinct slice so a storey is rasterized once.

An exact variant — union the per-element `sectionRings` into outer rings plus holes, then
test point-in-polygon — was considered and rejected. It is the same idea with no
resolution artifact, but robust polygon booleans are a classic source of subtle geometry
bugs, the library has no such code today, and the exactness buys nothing at the 10 cm
scale real building data is authored at.

## Testing

All of these must exist before this is done:

- **Opposite-face cancellation** — a wall long in X, thin in Y returns ±Y, asserted on
  both winding orders so the result cannot depend on triangle orientation.
- **Local vs world** — the same mesh under a 90° `Placement` rotation returns a normal
  rotated by 90°. This is the regression test for the frame bug; without it the function
  passes while being uniformly wrong.
- **Four-elevation partition** — a closed rectangular room: every wall in exactly one of
  four bins, all `ExposureExterior`, all bins non-empty, summed face area equal to total
  exterior vertical face area. This proves the fill *resolves*, as opposed to degrading to
  low confidence everywhere.
- **Non-convex footprint** — a U-shaped plan where a naive centroid rule misassigns the
  walls flanking the recess. They must resolve correctly, never a confident wrong bin.
- **Enclosed void** — a ring of walls around a closed courtyard: the inward-facing walls
  report `ExposureEnclosed`, not `ExposureExterior`, and so do not inflate any elevation.
- **Interior partition** — a wall inside a closed room reports `ExposureInterior`.
- **Layer axis** — a wall whose declared stack runs inward returns a `LayerAxis` with a
  negative dot against its `Facing.Normal`; mirroring the placement flips the sign.
- **Non-applicable geometry** — column, slab and empty mesh return `ok=false`.
- **Azimuth** — a +X-facing wall with `trueNorth=(0,1)` gives 90°; a rotated TrueNorth
  shifts it by exactly that rotation.
- **Determinism** — repeated calls on identical input return bit-identical normals.

Tests live in-package (`package geometry`) so they can use `v3`, `elemBox` and
`boxMeshWorld`, matching `geometry/section_test.go`. Fixtures are synthetic: the
behaviour under test is geometric, so a hand-built U-shape is both sufficient and
readable, and no third-party model needs to ship in the repo.

## What this does and does not get the caller

It gets: elevation grouping that stops being a per-consumer reimplementation of a problem
with two well-camouflaged failure modes; build-ups that can be ordered from the exposed
face instead of from declared file order; an envelope-versus-interior signal; a confidence
signal to degrade honestly on; and a real compass bearing instead of a guessed one.

It does not get: a correct answer on every wall of every model. Freestanding elements and
geometry below the grid resolution report low confidence, by design, because the
alternative is a confident wrong answer.

## Risks

- **Grid resolution is the failure mode.** A gap narrower than one cell seals; an element
  thinner than one cell vanishes. Rasterization is conservative — a cell the footprint
  touches counts as occupied — so the failure leans toward sealing, which reads as
  interior at low confidence rather than leaking open air into a room. The cell size is a
  documented constant, not a caller knob, until a real model argues otherwise.
- **A model open at the slice height reads as freestanding.** A band taken through a fully
  glazed storey with no modelled mullions finds no occupancy to enclose it. Those elements
  land in the low-confidence bucket, which is the honest answer, but it is a real coverage
  gap on curtain-walled models.
- **Cost is linear in cells, not elements.** A 20 × 26 m footprint at 10 cm is ~54k cells
  per band — trivial — but a large site model at the same resolution is not, and bands are
  per distinct mid-height. Cache by band and revisit only if it shows up in a profile.

## Follow-on

Issue #6 (`ElevationView`) depends on this: it needs to know which direction each facade
view is built for. Note that #6's stated build step 1 — generalise `belowRings` from −Z to
an arbitrary direction — is **already done**; `belowRings` has taken a `Plane` since PR #3
(`geometry/section.go:127`). That issue predates the change.

`IfcRelSpaceBoundary` remains the authoritative source where a model carries spaces, and
is the natural next increment behind the unchanged `Facing` return.
