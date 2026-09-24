# Tessellation gaps and the placement DoS

Status: **decided**, not yet built. Second sub-project in the sequence the
[parity and coverage harness](2026-09-21-parity-and-coverage-harness.md)
opened: that harness ranked goifc's geometry gaps by frequency on real files,
and this doc spends the ranking.

Closes [#54](https://github.com/blox-eng/goifc/issues/54) (a denial of service
in a released library), then the top two entries of the measured gap list, one
of which is [#52](https://github.com/blox-eng/goifc/issues/52).

## Problem

Three defects, found by two different instruments, sharing one root cause
between them: goifc reasons about IFC types by exact keyword, and recurses
through file-controlled structure without always bounding itself.

### `model` overflows the stack on a cyclic placement

`model/placement.go:28` composes the `IfcLocalPlacement.PlacementRelTo` chain
by recursion, with no cycle detection and no depth bound — unlike `geometry/`,
which guards its own walks with `maxMapDepth` and `maxWalkDepth`.

A file whose placement refers to itself therefore recurses until the goroutine
stack limit, producing `fatal error: stack overflow`. That is not a panic:
`recover()` cannot catch it, and it takes the process down. It is reachable
from `goifc.Assemble` on arbitrary input, so any consumer that imports a file
it did not author can be killed by a 30-byte entity. The defect shipped in
v0.11.0 and in every release before it.

Reduced reproducer, from the nightly fuzzer:

```
#12=IFCLOCALPLACEMENT(,#12);
```

Confirmed against `aa9b353`: every frame in the trace holds the same instance
pointer, so the cycle is direct rather than a long chain. Frames are ~680
bytes, which puts the 1 GB stack ceiling at roughly 1.5 million of them.

This is the reason Fuzz Nightly has been red on `main` since 22 September. The
run that found it is not a regression from the harness work — the same tree
passed on the 21st, and the fuzzer's corpus simply reached the input.

### `IfcFaceBasedSurfaceModel` has no dispatch case

`tessellateItemDepth` (`geometry/geometry.go:91`) has no case for it, so every
element built from one degrades to a bounding box. The harness ranks it first
at 65 occurrences, and in `duplex_a` it hides 235 `IfcConnectedFaceSet`s that
no other path can reach.

### The polygonal half-space path is unreachable

This one is not a missing feature. `clip.go:65` already implements bounded
clipping, and `clip.go:93`'s `clipTrianglesByBoundedPlane` already reasons
about the footprint. But `clip.go:46` gates the whole boolean on
`second.IsA("IfcHalfSpaceSolid")`, and `step.Instance.IsA` is exact-keyword
only — there is no EXPRESS schema behind it, as
[limitations](../limitations.md) states. `IfcPolygonalBoundedHalfSpace` is a
*subtype*, so it fails that test. All 11 instances in the corpus decline, and
the code below is dead.

It is Revit's standard mitred wall/slab/beam join, so the cost is paid on
ordinary architectural models, not exotic ones.

## What was measured

Entity counts across the whole public corpus, taken directly from the files
rather than from the harness, so the supertype audit below rests on data:

| Entity | `ifcopenhouse` | `duplex_a` | `fzk_haus` |
|---|---|---|---|
| `IFCFACEBASEDSURFACEMODEL` | 1 | 40 | 0 |
| `IFCCONNECTEDFACESET` | 0 | 235 | 0 |
| `IFCPOLYGONALBOUNDEDHALFSPACE` | 0 | 7 | 4 |
| `IFCHALFSPACESOLID` (plain) | 2 | 0 | 2 |
| `IFCBOXEDHALFSPACE` | 0 | 0 | 0 |
| `IFCFACESURFACE` | 0 | 0 | 0 |
| `IFCEDGELOOP` | 0 | 0 | 0 |

The 11 polygonal half-spaces match the harness's reported occurrence count
exactly. The 41 face-based surface models report as 65 occurrences because an
`IfcMappedItem` is resolved once per element that reaches it, which is the
intended behaviour of that diagnostic.

### The `IsA` supertype defect is one bug, not a class

[#52](https://github.com/blox-eng/goifc/issues/52) looked like an instance of a
broader pattern — every `IsA(<supertype>)` call in `geometry/` and `model/`
being silently wrong — so every call site was audited. The pattern is real but
almost entirely latent:

- **`IfcHalfSpaceSolid`** — the only supertype with live subtypes in this
  corpus. Two call sites: `clip.go:46` and `unhandled.go:205`.
- **`IfcFace`** (`brep.go:53`) would miss `IfcFaceSurface` — zero occurrences.
- **`IfcPolyLoop`** (`brep.go:85`) drops `IfcEdgeLoop` faces — zero
  occurrences. This is a deliberate narrowing, not a supertype error.
- **`IfcConnectedFaceSet`** (`geometry.go:110`) already names all three of its
  relevant subtypes explicitly, and is correct.
- Everything else — `IfcSIUnit`, `IfcPropertySet`, `IfcLocalPlacement`,
  `IfcCartesianTransformationOperator3DNonUniform` — is either a leaf type or
  a deliberate exact-match narrowing.

So the audit's finding is that one supertype matters, and building machinery
for the rest would be speculative. That conclusion is the reverse of what the
issue assumed, and it is why this doc chooses a three-line predicate over a
subtype registry.

### A correction to the earlier ranking

The harness design doc named `IfcTriangulatedFaceSet` and `IfcPolygonalFaceSet`
as known gaps. Both appear **zero** times in the corpus. They remain unhandled
and may matter for IFC4 models this corpus does not contain, but they are not
what is costing accuracy today, and neither is in scope here. This is precisely
the guesswork the harness was built to replace.

## Decisions

### Cycle detection *and* a depth cap, not either alone

The bug is a cycle, and IFC bounds placement depth nowhere, so a depth cap
alone would answer a question nobody asked: it would silently truncate a legal
40-deep chain into a wrong transform rather than reject it.

But cycle detection alone leaves a second door open. A crafted *acyclic* chain
of ~1.5 million distinct placements still exhausts the stack, and against a
linearly-scanned seen-set it is also O(n²) — a CPU hang even where the stack
survives. goifc's primary consumer ingests customer-supplied IFC server-side,
so that file is a thing someone can upload.

The two guards make each other affordable. The cap bounds the seen-set, so the
linear scan is provably cheap and no map is allocated per element; detection
keeps the cap from having to be tight enough to be wrong.

**Rejected:** a `map[int]bool` seen-set, matching `geometry/bbox.go`. It
allocates once per element on the import hot path to guard against an input
that is vanishingly rare in practice. With a bounded chain, a slice scan is
cheaper and allocates nothing for well-formed files.

### A local predicate, not a subtype registry

```go
// isHalfSpaceSolid reports whether inst is an IfcHalfSpaceSolid or one of its
// two subtypes. step.IsA is exact-keyword only — there is no EXPRESS schema —
// so a supertype test must enumerate. See docs/limitations.md.
func isHalfSpaceSolid(inst *step.Instance) bool {
	return inst.IsA("IfcHalfSpaceSolid") ||
		inst.IsA("IfcPolygonalBoundedHalfSpace") ||
		inst.IsA("IfcBoxedHalfSpace")
}
```

`geometry/bbox.go:77` already enumerates these three by hand. Folding it into
the predicate leaves the package with one answer to "is this a half space"
instead of two that have already drifted apart once.

`IfcBoxedHalfSpace` has zero occurrences and recognising it is therefore
speculative — but `bbox.go` already names it, and a predicate that disagreed
with the site it replaced would be worse than a speculative branch.

**Rejected, with a trigger recorded:** a curated supertype→subtype map behind a
new `step.IsKindOf`. It is the right design if the audit had found six sites;
it found one. It would also add public API and contradict a documented
limitation in order to cover a single supertype. **Revisit if a third consuming
site appears, or if a corpus model introduces `IfcFaceSurface` or
`IfcEdgeLoop`** — at that point enumeration stops being cheaper than a table.

### A new Gate 1 violation blocks

Both tessellation fixes replace a conservative OBB superset with a tight mesh,
so either can expose an element that passed containment only because its box
was loose. Such a violation is root-caused before the PR lands. It may be
allowlisted only once its cause is understood and filed as its own issue, with
a `note` explaining what it is, matching how the `#53` entries are written.

`parity/knownviolations.go` is a record of diagnosed bugs. It is not where PRs
go to get unblocked.

## PR 1 — `fix(model)`: cycle detection in the placement chain

`localPlacementMatrix` becomes a wrapper over a helper carrying the ancestry.
`LocalPlacement`'s signature does not change.

```go
// maxPlacementDepth bounds the IfcLocalPlacement.PlacementRelTo chain. Cycle
// detection is what fixes the bug this guards; this cap closes the second
// door, an ACYCLIC chain of distinct placements long enough to exhaust the
// stack on its own (~1.5M frames at this frame size). IFC bounds this depth
// nowhere, so the value is not a schema limit — it is far above any real
// building's spatial nesting and far below what a stack can survive.
const maxPlacementDepth = 1024
```

The helper carries the set of express IDs already on the chain and returns
`Identity()` on a repeated ID or on exceeding the cap. Returning identity
rather than a partial compose keeps a malformed placement indistinguishable
from a missing one, which is what `placement.go:22` already does for a
non-`IfcLocalPlacement` value — the degenerate path stays uniform.

Note what that identity is and is not. It is the value the ANCESTOR call
returns at the point the cycle closes; the callers below it keep composing
their own relative placements on the way back out. So the transform a cyclic
chain finally yields is generally NOT the identity — a self-cycle on a
placement that translates +1 yields a +1 translation, not identity. The
contract is that the walk terminates with a bounded, well-defined transform,
not that the answer is identity.

**Tests**, written first, each failing before the fix:

- A direct self-cycle, as a synthetic fixture under `model/testdata/synthetic/`,
  asserting the composed transform rather than a crash — for a placement that
  translates +1, that is a +1 translation, since only the ancestor call returns
  identity. The unit test states the bug; a fuzz seed alone only asserts "did
  not crash".
- A two-node mutual cycle (`A → B → A`), which a parent-only check would miss.
- A legal chain just under the cap, asserting the transform still composes —
  pinning that the cap does not truncate real work. Generated in the test
  rather than committed as a fixture: a thousand-entity file would be
  unreadable, and the depth is the only thing under test.
- The reproducer committed to `testdata/fuzz/FuzzAssemble/cca694515baa1bef`, in
  this same commit. Committing it earlier turns `main` red.

No parity impact: no corpus model contains a placement cycle, so
`docs/coverage.md` and `baseline.json` are untouched. Verified rather than
assumed.

### One file, not two

`geometry/testdata/fuzz/FuzzUnionArea2D/a23f727e8ca49c9c` was believed to be a
second, untriaged crasher. It is not. It is an existing regression seed,
committed in `cea89b7` (PR #47), and it passes on `aa9b353` including under
`-race`.

It was mistaken for a finding because `fuzz-nightly.yml`'s failure path uploads
`path: '**/testdata/fuzz/**'` — the entire committed corpus, not just the
newly-written crasher. The `FuzzAssemble` job failing on #54 therefore produced
an artifact containing every seed in the repository, `geometry/`'s included.

Narrowing that glob so the next crasher arrives unambiguous is worth doing and
is **not** in scope here; it is noted so the next person does not re-triage a
passing seed.

## PR 2 — `feat(geometry)`: dispatch `IfcFaceBasedSurfaceModel`

`FbsmFaces` is attribute 0, a SET of `IfcConnectedFaceSet`. `SbsmBoundary` is
attribute 0, a SET of `IfcShell`. Different schema entities, identical
traversal, and `brepMesh` already tessellates a bare `IfcConnectedFaceSet`. One
shared helper, two named constants:

```go
attrSbsmBoundary = 0 // IfcShellBasedSurfaceModel.SbsmBoundary
attrFbsmFaces    = 0 // IfcFaceBasedSurfaceModel.FbsmFaces

// surfaceModelMesh unions the faces of every shell or face set in a surface
// model's boundary attribute. Shared by IfcShellBasedSurfaceModel and
// IfcFaceBasedSurfaceModel: different entities with the same shape, each a
// SET of things brepMesh already handles. The attr is a parameter rather than
// assumed because the two constants agreeing at 0 is a fact about the schema,
// not a rule.
func surfaceModelMesh(m *step.Instance, attr int) ([]float32, []uint32, bool)
```

Separate dispatch cases, so both keywords are visible to `dispatchedTypes`, and
`"IfcFaceBasedSurfaceModel"` added to `handledItemTypes` — without it
`TestHandledItemTypesMatchesDispatch` fails the build, which is that harness
working as designed.

**Risk.** This routes `duplex_a`'s 235 face sets through `brepMesh` for the
first time. `brepMesh` ignores inner bounds and reads only `IfcPolyLoop` outer
loops; for AABB purposes that is harmless, since an ignored hole does not move
a bounding box. If this PR needs debugging, it will be in face handling, not in
the dispatch case.

**Test:** a synthetic `IfcFaceBasedSurfaceModel` fixture asserting
`SourceBrep` and real geometry rather than a box.

## PR 3 — `fix(geometry)`: resolve half-space subtypes

The predicate above, applied at `clip.go:46` and `unhandled.go:205`, with
`bbox.go:77` folded in.

**Both consuming sites move in the same commit.** Fixing only `clip.go` would
make the clip work while `UnhandledItemTypes` kept reporting
`IFCPOLYGONALBOUNDEDHALFSPACE | 11` as an unhandled gap — a generated,
byte-compared page asserting something false.

**Risk, and it is the largest in this plan.** `clipTrianglesByBoundedPlane` has
never executed against real data. Its `AgreementFlag` reasoning
(`clip.go:112–117`) is *derived* from the plain half-space path, which the
comment at `clip.go:60` records as oracle-validated — the bounded path itself
is not. If that sign convention is inverted, the clip removes the wrong side
and the AABB shrinks wrongly, which Gate 1 catches as a new violation. This is
the most likely place in this plan for real debugging to appear, and per the
decision above it is root-caused, not allowlisted.

**Tests:** a synthetic `DIFFERENCE(box, IfcPolygonalBoundedHalfSpace)`
asserting the clip actually cuts, and a `UnhandledItemTypes` assertion that a
polygonal half-space is no longer counted as a gap.

## Measurement and verification

Both tessellation PRs run the same loop. It is written out because it is the
part most likely to be cut short under time pressure.

1. **Record the before-state.** `make parity` on a clean tree, capturing Gate
   1's violation set and the three OBB rates. Without this, "no new
   violations" is an assertion rather than a comparison.
2. **Apply the fix, re-run `make parity`.** Gate 2 is *expected to fail* here:
   it fails in both directions by design, and a closed gap moves the rate. A
   Gate 2 **pass** at this step means the rate did not move — which is a
   prompt to investigate, not a diagnosis. The dispatch case may never be
   reached, or it may be reached and then decline inside its mesh helper (a
   partial tessellation, a boundary the gate refuses), which is a legitimate
   outcome that leaves the rate alone. What establishes that the new path
   RUNS is the synthetic geometry test asserting a non-OBB source on a fixture
   built for it; Gate 2 measures what that path is worth across the corpus.
3. **Diff Gate 1.** Any `GlobalID` violating that was not violating before is
   root-caused. Both `#53` stair entries must still be present and still
   violating; if the change incidentally fixes them, the stale-entry check
   fails and both entries come out in the same commit.
4. **`make parity-report`, then `make parity-baseline`**, then `make parity`
   again to confirm green. Report, baseline and code land in one commit, or
   CI's byte-compare of `docs/coverage.md` rejects it.
5. **`make ci`** — lint, test, vulncheck, parity — before opening the PR.

### Predictions, to be checked against rather than aimed at

PR 2 should move `duplex_a` from 32.1% toward roughly 13%, if the ~58%
attribution holds, and may move `ifcopenhouse` — which the baseline calls clean
— through its single face-based surface model. PR 3 should remove the
`IFCPOLYGONALBOUNDEDHALFSPACE` row entirely and may take `fzk_haus` to 0%,
since its 4 half-spaces sit against exactly 2 fallback elements. A number far
from these is a finding to explain in the PR body, not a number to accept
quietly.

### The generated page stays honest on its own

`parity/report.go:270`'s `unattributedNote` already switches prose when a model
has fallback elements but no named unhandled type, naming the model and saying
that nothing accounts for its fallbacks. So closing these gaps cannot leave
`docs/coverage.md` asserting something false, and that branch firing is a
useful signal that a *dispatched* path is now declining mid-attempt.

`report.go:117` will still cite `IFCFACEBASEDSURFACEMODEL` as an example of key
casing after the row disappears. It illustrates a naming convention rather than
claiming a gap, so it stays: churning a byte-compared document for a cosmetic
point is not worth the diff.

### Sequencing

PR 1 is independent. PRs 2 and 3 both rewrite `baseline.json` and
`docs/coverage.md`, so they serialise: whichever lands second rebases onto
`main` and re-runs steps 1–4 from scratch rather than resolving a conflict by
hand. A merged baseline is a number nobody measured.

`make oracle` remains blocked on the upstream image (`parity/oracle/README.md`)
and is not needed. The oracle JSON is committed and frozen, and none of these
changes alters what IfcOpenShell would produce — only goifc's side of the
comparison moves.

## Not in scope

- **[#53](https://github.com/blox-eng/goifc/issues/53)**, the `IfcStairFlight`
  ~9.9 mm Y shortfall. An unexplored extrusion-path bug; its cost is unknown
  and does not belong inside this plan's estimate.
- **Narrowing `fuzz-nightly.yml`'s crasher upload glob**, as described above.
- **`IfcTriangulatedFaceSet` and `IfcPolygonalFaceSet`** — zero occurrences in
  this corpus. Revisit when a corpus model contains one.
- **`step.IsKindOf`** — deferred with the trigger recorded above.
