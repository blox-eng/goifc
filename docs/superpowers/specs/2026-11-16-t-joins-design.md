# T-joins: a partition that ends against another wall

Status: approved 2026-11-16.

## Intent

A partition often ends against the middle of another wall, the through
wall. Today goifc cannot build that: every builder sees one element, so the
partition's end overlaps the through wall's core and the through wall still
counts the face the partition hides. A tiler does not finish that strip, so
the finish quantities are wrong by the strip's area at every T in a plan.

goifc builds T-joins so that a plan's walls give finish areas a contractor
can price: the strip a partition covers is subtracted from the through
wall's net face area (decided 2026-11-12: the tiler's convention).

The main consumer is a browser 3D editor, through `goifc-build.wasm`. Go users
get the same call.

## Goals

- A wall declares which of its ends meets which wall. goifc checks the
  declaration and refuses one that does not match the geometry.
- The through wall's net face loses the strip on the face the partition
  touches.
- The partition stops exactly on the through wall's face, at any angle in
  the allowed range, so no volume is counted twice.
- Go and the browser compute through one function, `geometry.BuildPlan`.
- Every refusal names the item, the join and the reason.

## Non-goals

- Finding joins automatically from geometry (rejected: a near miss silently
  stays unjoined).
- X-joins (a wall that crosses another). Build it as two partitions.
- A partition that cuts into the through wall's layers. It stops at the
  finished face (decided 2026-11-16).
- Joins on room walls as the partition. Room walls end at corners; only a
  straight wall item can be a partition. A room wall can be a through wall.
- Changing the existing builders (`BuildWall`, `BuildRoomWalls`, the layer
  variants). They keep their signatures and results.

## Anti-goals

- A join that is half applied: the partition trimmed but the strip not
  subtracted, or the reverse.
- A number in the browser that Go cannot reproduce.
- A join accepted silently when the end is not on the face.

## Approaches

1. **One Go call for the whole plan (picked, 2026-11-16).** `BuildPlan`
   takes every wall, room and slab plus the joins and returns every element.
   One call applies both sides of a join; the wasm builder becomes a JSON
   adapter over it. Costs new public input types that repeat the existing
   builders' parameters.
2. **Two Go helpers** (trim a partition; subtract a strip). Small API, but
   a caller who runs one of them gets wrong numbers with no error.
3. **Joins only in the wasm contract.** No Go API change, but Go users
   cannot build T-joins and the browser's numbers have no Go equivalent.

## Design

### Declaring a join

A wall item gets an optional `Joins` list, at most two (one per end):

```go
type End int // EndA (edge[0]) or EndB (edge[1])

type Join struct {
	End  End
	Wall string // a wall item's ID, or a room wall "r1/2" (room ID, "/", wall index)
}
```

In JSON: `"joins": [{"end": "b", "wall": "w1"}]`.

A join names an item, never a layer: for a layered through wall goifc picks
the layer whose face the partition touches (the outermost layer on that
side).

### The geometry check

A join is valid only if all of these hold, else it is refused:

1. `Wall` names a wall item or a room wall in the same plan, and not the
   partition itself. Else `ReasonUnknownWall`.
2. The partition's end point lies on the plane of one face of the through
   wall, within 1 mm (`minDim`). The end point is the partition's
   measured-face corner, as given in its edge. Else `ReasonNotOnFace`.
3. The partition meets that face from outside the through wall: its body
   runs away from the through wall. Else `ReasonNotOnFace`.
4. The angle between the two walls' axes is between 15° and 165°. Else
   `ReasonTooShallow`.
5. The strip (below) lies inside that face's span along the through wall,
   at least 1 mm from each end, and inside the face's height range. For a
   room wall's opposite face, the span is the mitred one
   (`otherStart`..`otherEnd`). Else `ReasonOffWall`.
6. No other join's strip on the same face overlaps or touches it. Else
   `ReasonJoinsOverlap` (on the later join, in request order).

All join refusals of a plan return at once, like opening refusals. Joins
are checked whenever both walls built; a join to a wall that failed to
build is not checked, because that wall's own refusal already fails the
plan (builder's call, after review). Every other refusal still returns in
the same round.

### Trimming the partition

The partition's end is cut flush with the through wall's face. Its
measured face and its opposite face then end at different points along the
partition when the angle is not 90°, exactly like a room's mitred corner
(`wallSpan.otherStart`/`otherEnd`). The partition's end point snaps onto
the face plane (it moved at most 1 mm).

The partition's quantities follow from the trimmed span through the
existing `wallQuantities`: length, face areas and volume. Its openings are
checked against the trimmed span, so an opening that reaches into the join
is refused as `ReasonInCorner`, as at a room corner.

### The strip on the through wall

- Along the through wall: from where the partition's two side faces meet
  the through wall's face. The width is the partition's total thickness
  divided by sin(angle).
- Vertically: the overlap of the partition's [base, base+height] with the
  through wall's. If they do not overlap, the join is `ReasonOffWall`.
- The strip is subtracted from the net area of the face it lies on:
  `MeasuredNet` when the partition touches the measured face, `OtherNet`
  when it touches the opposite face. For a layered through wall, the face
  is layer 0's measured face or the last layer's opposite face.
- Two new quantities make the subtraction visible:
  `MeasuredJoins, OtherJoins float64 // m², the strips partitions cover`.
  `MeasuredNet = MeasuredGross − holes − MeasuredJoins`, and likewise for
  the other face.
- The through wall's mesh, gross areas and volumes do not change.
- An opening in the through wall that overlaps or touches a strip on the
  face the strip lies on (or any through opening) is refused with a new
  placement reason, `ReasonInJoin` ("in_join"). A niche on the other face
  is allowed. The refusal is a `*PlacementError` on the through wall (its
  item, wall index and opening index), like every other opening refusal:
  the opening is what the person moves. (Settled after review.)

### `BuildPlan`

```go
type Plan struct {
	Walls []PlanWall
	Rooms []PlanRoom
	Slabs []PlanSlab
}

// Exactly one of Thickness and Layers is set.
type PlanWall struct {
	ID        string
	Edge      [2][2]float64
	Base, Height, Thickness float64
	Layers    []float64
	Side      Side
	Openings  []Opening
	Joins     []Join
}

type PlanRoom struct {
	ID        string
	Outline   [][2]float64
	Base, Height, Thickness float64
	Layers    []float64
	Side      Side
	Openings  [][]Opening
}

type PlanSlab struct {
	ID        string
	Outline   [][2]float64
	Z, Thickness float64
	Layers    []float64
}

type Built struct {
	Walls []BuiltWall // walls items, then room walls, in plan order
	Slabs []BuiltSlab
}
type BuiltWall struct { ID string; Wall Wall } // ID as below; Wall.Element.GlobalID = ID
type BuiltSlab struct { ID string; Slab Slab }

func BuildPlan(p Plan) (Built, error)
```

- Element IDs follow today's wasm rule, moved into geometry: `w1`,
  `w1/0`…, `r1/0`…, `r1/0/0`…, `s1`, `s1/0`….
- Item IDs follow today's rule (1–64 of letters, digits, `_ . : -`, unique).
- Errors: a malformed item returns `*ItemError{Item string; Err error}`,
  whose `Err` is today's error (plain, or `*PlacementError` joined). A
  join refusal is `*JoinError{Item string; Join int; Reason JoinReason}`.
  Every refusal across the plan returns at once with `errors.Join`.
- The existing caps hold: 10,000 items, 100,000 elements, each builder's
  own caps. A through wall's strips count toward its 256-opening cap.
- Order of work (settled after review):
  1. build each item as today;
  2. check joins 1 to 4, which depend only on the through walls' face
     planes, and compute every partition's trimmed span;
  3. compute every strip and check 5 and 6 against the trimmed spans of
     all walls, so a chain (a partition that is also a through wall) is
     checked against its final faces: at 45° a trim moves the opposite
     face's end by about the thickness, not 1 mm;
  4. check the through walls' openings against the strips (`in_join`);
  5. refuse if anything failed; else rebuild trimmed partitions and
     subtract the strips.

### The wasm contract

- `wallItem` gains `"joins": [{"end": "a"|"b", "wall": "<id>"}]`.
- `wallQuantities` gains `measuredJoins` and `otherJoins`.
- New error codes. `in_join` is an opening refusal: it carries `item`,
  `wall` and `opening`, like `overlaps`. The join refusals
  `unknown_wall`, `not_on_face`, `too_shallow`, `off_wall` and
  `joins_overlap` carry `item` (the partition) and `join` (its index in
  `joins`).
- `internal/wasmapi` maps JSON to `Plan` and `Built` to the answer; its own
  building code goes. The goldens are regenerated only where the new
  quantity fields appear; no existing number changes.
- `goifc-contract.schema.json` and `docs/guides/wasm.md` document all of it.
- Known consumers are told before the PR: the request and answer shapes change.

### Docs

`docs/guides/builders.md` gets a T-joins section (a worked example: a 4 m
through wall, a 0.1 m partition at 90°, 2.5 m high: the strip is 0.25 m²).
The "Not yet" note and the `docs/limitations.md` entry go. CHANGELOG
headline: `- **Build walls that meet mid-wall (T-joins).**`

## Tests (synthetic numbers only)

- 90° join on the measured face, and on the opposite face: exact strip,
  partition length and volume.
- 45° join: strip width = t / sin 45°, partition faces end at different u.
- A layered through wall and a layered partition.
- A room wall as the through wall, including its mitred opposite face.
- A partition joined at both ends.
- Each refusal: unknown wall, self join, end 2 mm off the face, end inside
  the wall, 10° angle, strip past the wall's end, partition above the
  through wall, two overlapping strips, an opening in the strip.
- `BuildPlan` without joins returns exactly what the existing builders
  return (property test over the fuzz corpus).
- Fuzz `BuildPlan` with joins: never panics, bounded work.
- The wasm goldens and the Node tests run the new fields.

## Left to the builder

The internal structure of `BuildPlan`, the exact message strings, the
golden file names, and how the fuzz targets generate joins.
