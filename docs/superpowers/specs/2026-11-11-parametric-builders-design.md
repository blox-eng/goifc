# Parametric builders: walls, room walls and slabs from numbers (#91)

Status: draft for approval, 2026-11-11.

## Intent

An app that drafts rooms and facades has no IFC file. It has numbers: a wall
edge, a height, a thickness, the openings. It must get the same quantities for
a drafted wall as for the same wall read from IFC, and a mesh that the GLB
export can write.

goifc has the path mesh → quantities → GLB for `geometry.Element`, but no
function makes an `Element` from numbers. This design adds three builders and
exports the 2D helpers a caller needs.

The same builders also run in a browser as WebAssembly, so the browser and the
server give the same numbers.

## Goals

- `BuildWall`, `BuildRoomWalls` and `BuildSlab` return an `Element` and exact
  quantities, computed from the parameters.
- The mesh agrees with the quantities: the mesh volume equals NetVolume for
  every builder, and for `BuildWall` on a world axis `DerivedQuantities` Area
  equals MeasuredNet. (Room walls are not checked on Area: their mitred end
  caps lean toward the axis, and `maxSideArea` counts them.)
- Walls from one closed outline join at mitred corners. The core at a corner
  counts one time.
- Each opening is checked. A refusal names the opening and one reason:
  `too_wide`, `overlaps`, `above_wall` or `in_corner`. Nothing is clamped.
- Hostile numbers never panic. The work is bounded.
- CI proves that goifc compiles with `GOOS=js GOARCH=wasm`.
- A semver minor release with a CHANGELOG entry.

## Non-goals (follow-up issues)

- T-joins: a wall that meets the middle of another wall needs an explicit join.
- Niches: an opening with a depth smaller than the wall. Here every opening
  cuts through the full wall; `Opening` has no depth field, so a niche cannot
  be passed in and silently measured as a hole.
- Storeys. The caller passes a base elevation; goifc knows no storey.
- Non-rectangular openings, curved walls, sloped tops.
- Holes in the ear-clipper. Approach A below does not need them.
- An IFC writer (#97).

## Anti-goals

- A quantity that is silently wrong. A refusal is always better than a guess.
- A change to the IFC import path. The builders are new code beside it.
- Any private or downstream data in the repository. All fixtures are synthetic numbers.

## Approaches

**A. Cut each face into rectangles around the openings (chosen, 2026-11-11).**
Openings are rectangles on the wall's own axes. Each face is cut into columns
at the opening edges, and each column into the parts above and below the
openings. Add the reveal strips inside each opening and the two end caps.
Exact, needs no triangulation with holes, and each face keeps its own length,
so mitred ends need no extra step.

**B. Triangulate the elevation with the openings as holes, then extrude
through the thickness.** General, and hole support would also fix IFC profiles
with voids. Rejected: an extrusion through the thickness cannot make a mitred
end, so the corner rule breaks, and it changes the ear-clipper every IFC import
uses.

**C. Extrude the plan footprint vertically, then subtract the openings.**
Rejected: needs a mesh boolean, which goifc does not have.

## The contract (what the consumer codes against)

Package `geometry`. Units are meters. Plan coordinates are X, Y; Z is up.

```go
type Side int

const (
	SideLeft  Side = iota + 1 // the measured face is on the edge line; the wall grows to the right of a→b
	SideRight                 // the measured face is on the edge line; the wall grows to the left of a→b
)

// Opening is a rectangle on the measured face, in the wall's (u, v):
// u runs along the edge from its start, v up from the wall's base.
type Opening struct {
	Offset, Sill, Width, Height float64
}

type WallQuantities struct {
	Length                  float64 // the measured face, along u
	Height                  float64
	MeasuredGross, MeasuredNet float64 // m², the face on the edge line
	OtherGross, OtherNet       float64 // m², the opposite face
	RevealPerimeter         float64 // m, Σ opening edges, minus the bottom edge when Sill == 0
	GrossVolume, NetVolume  float64 // m³
}

type SlabQuantities struct {
	Area, Perimeter, Volume float64
}

type PlacementReason string

const (
	ReasonTooWide   PlacementReason = "too_wide"
	ReasonOverlaps  PlacementReason = "overlaps"
	ReasonAboveWall PlacementReason = "above_wall"
	ReasonInCorner  PlacementReason = "in_corner"
)

// PlacementError refuses one opening. Use errors.As.
type PlacementError struct {
	Wall    int // index of the wall (outline edge); 0 for BuildWall
	Opening int // index into that wall's openings
	Reason  PlacementReason
}

// Face is one planar face as data, for drawing and dimensioning without
// re-deriving the frame. Origin, U, V and Normal are world; U and V are unit;
// Outline and Openings are in (u, v) meters on that plane.
type Face struct {
	Origin, U, V, Normal [3]float64
	Outline              [][2]float64
	Openings             []Opening
}

type Wall struct {
	Element    Element
	Quantities WallQuantities
	Measured   Face // origin = edge start at the base; U along a→b; V up; Normal away from the wall
}

type Slab struct {
	Element    Element
	Quantities SlabQuantities
	Top        Face // origin = outline[0] at z; U = +X; V = +Y; Normal = +Z; Outline = the outline relative to outline[0]
}

func BuildWall(edge [2][2]float64, base, height, thickness float64, side Side, openings []Opening) (Wall, error)
func BuildRoomWalls(outline [][2]float64, base, height, thickness float64, side Side, openings [][]Opening) ([]Wall, error)
func BuildSlab(outline [][2]float64, z, thickness float64) (Slab, error)

func (s Side) MarshalText() ([]byte, error)   // "left" / "right"
func (s *Side) UnmarshalText(b []byte) error   // refuses anything else

func PolygonArea2D(poly [][2]float64) float64            // unsigned
func PointInPolygon2D(p [2]float64, poly [][2]float64) bool
func Triangulate2D(poly [][2]float64) ([]uint32, error)  // bounded; refuses what it cannot clip
```

**Changes to the shape in #91**, to tell the consumer:

1. Every builder also returns an `error`. #91's Done-when needs refusals and
   errors for degenerate input; the signatures in #91 had no way to return
   them.
2. `Side` is relative to the edge direction a→b: `SideLeft` or `SideRight`.
   For a room, a counter-clockwise outline with `SideLeft` measures the
   interior. The builders never reverse the outline, so wall and opening
   indices stay the caller's.
3. `BuildRoomWalls` takes `openings [][]Opening`: one list per outline edge,
   edge i running from outline[i] to outline[i+1 mod n].
4. `PlacementError` carries the wall and opening index with the reason, so
   the caller can show the reason next to the right field.
5. Each builder returns one result value (`Wall`, `[]Wall`, `Slab`) holding
   the `Element`, the quantities and the measured face as data (`Face`). The
   consumer stores the face for elevation drawings and reveal dimensions;
   goifc owns the frame convention, so the consumer never re-derives it.
6. Walls take a `base` elevation: the wall's bottom is at z = `base`.
7. `Side` has a text form, "left" and "right", for JSON and WebAssembly callers.

Changes 5–7 come from a downstream consumer review, 2026-11-11.

Everything else matches #91: `BuildWall`, `BuildRoomWalls`, `BuildSlab`, the
four reasons, the quantities named there.

## Rules

**The measured face** lies on the edge line and spans exactly [0, Length]. The
wall grows away from it by `thickness`.

**Corners.** In `BuildRoomWalls`, the opposite face of each wall is the edge
line offset by `thickness`. Adjacent opposite faces meet where their lines
cross (a mitre). The end cap is the plane through the outline corner and that
crossing. So the core at a corner is split between the two walls on the
mitre and counts one time. At a convex corner of the measured side the
opposite face is longer than the measured face; at a concave corner it is
shorter. `BuildWall` alone has square ends: both faces span [0, Length].

**Opening checks**, in this order, first refusal wins per opening. Every
opening of every wall is checked, and all refusals return together as
`errors.Join` of `*PlacementError`, in wall then opening order, so the caller
can mark every bad field at once. An input error (item 1) stops at the first
one, because the numbers after it are not trustworthy.

1. Not finite, or `Width <= 0`, `Height <= 0`, `Offset < 0` or `Sill < 0`:
   an input error, not a placement reason.
2. `too_wide`: `Offset + Width >= Length`, or `Offset == 0` on a square
   (`BuildWall`) end. The opening must leave wall at both ends.
3. `above_wall`: `Sill + Height >= height`. The opening must leave wall above.
4. `overlaps`: the rectangle overlaps or touches an earlier opening. Two
   openings side by side are one opening.

Touching counts as the reason it touches (maintainer ruling, 2026-11-11): every opening
edge then has wall behind it except the bottom at `Sill == 0`, so the reveal
formula is one rule and the mesh stays a closed shell. "Touching" uses one
named tolerance (left to the builder, with its reason).
5. `in_corner`: the opening's u-range is not inside the span both faces share.
   Through-cut openings need wall on both sides. Only a room wall at a
   concave corner can refuse this.

**Quantities** come from the parameters, not the mesh:

- MeasuredGross = Length × height; OtherGross = OtherLength × height.
- Net = Gross − Σ Width × Height, on both faces (every opening cuts through).
- RevealPerimeter = Σ (2 × Width + 2 × Height), minus Width for each opening
  with Sill == 0.
- GrossVolume = plan area of the wall (a trapezoid at mitres) × height;
  NetVolume = GrossVolume − Σ Width × Height × thickness.
- Slab: Area = |polygon area|, Perimeter = outline length, Volume = Area ×
  thickness.

**Placement.** `Verts` are local to the first point (the edge start, or
outline[0]) and `Placement` is the translation to it. This keeps float32
vertices precise far from the origin. `BBoxMin`/`BBoxMax` are world. A wall's
base is at z = `base`. A slab's top face is at `z` and it extends down by
`thickness`. `GlobalID` is empty: the caller sets it (`DerivedQuantities`
skips elements without one). `Source` is `SourceExtrude`.

**Degenerate input returns an error**, never a panic or a mesh: non-finite
numbers; `height`, `thickness` or an edge shorter than 1 mm; an outline with
fewer than 3 points, repeated points, crossing edges or zero area; a room
corner whose opposite face would have zero or negative length; a corner
whose mitre point lies more than 10 × thickness from the outline corner (an
interior angle under about 11°); room walls whose bodies overlap (a notch or
corridor narrower than 2 × thickness), found by checking the opposite-face
ring for crossings under the work budget; `Side` not one of the two values.
A straight-through vertex (180°) is allowed: the two walls meet at a square
cut, so a caller can split a wall to give each piece its own openings.

**Bounds.** At most 2,048 outline points and 256 openings per wall; more is
an error. Measured: the existing work budget triangulates a 3,000-point convex
outline and refuses a 4,096-point one, so 2,048 leaves headroom for concave
input. `Triangulate2D` runs under the existing work budget. Every coordinate
is within 1,000 m of the first point, and every first point within 10^7 m of
the origin; more is an error. This keeps float32 local vertices sub-millimetre
and keeps every sum finite. The fuzz properties hold inside these bounds.

## Proof

- **An oracle table** (`geometry/builders_oracle_test.go`), one row per case,
  with the formula written in the row:
  - the #91 wall: 2.40 × 2.60 × 0.10 with a door 0.80 × 2.00 → 6.24, 4.64, reveal 4.80;
  - the #91 room: 2.40 × 1.80 × 2.60, 0.10 walls, interior → measured 21.84 m²,
    volume 2.288 m³;
  - an L-shaped room (6 walls, one concave corner);
  - a window with a sill (reveal on four sides);
  - a rectangle slab and an L-shaped slab.
- **One test per refusal reason**, plus the input errors, each asserting the
  wall and opening index.
- **Mesh agrees with quantities:** for every oracle row, `meshVolume` equals
  NetVolume, and for an axis-aligned `BuildWall` `DerivedQuantities` Area equals
  MeasuredNet, within a stated float32 tolerance. The mesh is a closed shell: column cuts split every shared edge, so no vertex sits inside a neighbour's edge.
- **Face data:** for the #91 wall, `Measured` has U = the unit edge
  direction, V = +Z, Normal pointing away from the wall body, Outline
  [0, 2.40] × [0, 2.60], and the door rectangle; with `base` = 3.0 the
  Origin z and the BBox z both start at 3.0.
- **GLB:** a scene of built elements writes a valid GLB through `WriteGLB`.
- **Fuzz** (`FuzzBuildWall`, `FuzzBuildRoomWalls`, `FuzzBuildSlab`): no panic;
  every success has finite, non-negative quantities and a mesh whose volume
  matches NetVolume. Seeds run in `go test`; the nightly fuzz job runs them
  longer.
- **WebAssembly:** a CI job runs `GOOS=js GOARCH=wasm go build ./...`. It
  joins the Go jobs' docs-only skip. `go list -deps ./geometry` already pulls
  `os` through `fmt`, so a dependency ban is not a useful guard; the build is.

## Release

- One `feat(geometry)` PR for #91. go-semantic-release cuts v0.16.0 from it.
- A CHANGELOG entry under Unreleased: the new API, the Side convention, and
  what is not supported yet.
- Follow-up issues filed before the PR: T-joins, niches, storeys.
- Task order puts `BuildWall` and `BuildSlab` first, so the consumer can pin
  the branch commit where both pass before the release.

## Left to the builder

- The float tolerance for "touching" openings and for the mesh cross-checks,
  with the reason written next to it.
- File layout inside `geometry/`, following the package's existing style.
- How the existing `polygonArea2D`, `pointInPolygon` and `triangulatePolygon`
  sit behind the exported names (wrap, do not fork).

## Addendum: niches (approved 2026-11-11)

A niche is an opening that does not cut through: it is cut from the measured
face to a depth. It must not reduce the opposite face, or a niche in a shower
wall would remove tiles from the room behind it.

- `Opening` gains `Depth float64`: 0 cuts through (unchanged); otherwise the
  opening is a niche cut from the measured face to `Depth`.
- `WallQuantities` gains `NicheBack float64`: Σ Width × Height of the niches,
  the area of their back panels, which are finished too.
- Depth must be at least 1 mm and at least 1 mm under the thickness; a niche
  needs a sill (Sill ≥ 1 mm). Anything else is an input error. Floor-level
  niches are not supported yet.
- Placement rules are the same as for any opening. `in_corner` for a niche
  uses the span the wall has at the niche's depth: between the end lines at
  w = Depth, so a shallow niche may sit closer to a concave corner.
- MeasuredNet loses Width × Height of every opening; OtherNet only of the
  through openings. NetVolume loses Width × Height × Depth for a niche and
  × thickness for a through opening. RevealPerimeter is unchanged: the niche's
  four edges.
- The mesh adds a vertex layer at each niche depth: jambs, head and sill run
  from the face to the depth, a back panel closes the pocket, and the opposite
  face stays whole. The shell stays closed.
- Oracle row: a niche 0.40 × 0.60, 0.08 deep, in the 2.40 × 2.60 × 0.10 wall
  → OtherNet 6.24, MeasuredNet 6.00, NicheBack 0.24, NetVolume
  0.624 − 0.40·0.60·0.08 = 0.6048.
