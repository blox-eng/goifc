# Parity and coverage harness

Status: **decided**, not yet built. First of several sub-projects scoping goifc
toward a native-Go alternative to IfcOpenShell for quantity takeoff.

## Problem

goifc was extracted from a private monorepo, and its validation stayed behind.

The library's own comments cite an IfcOpenShell parity oracle it does not ship:
`geometry/clip.go:60` ("verified against the ifcopenshell parity oracle on a
real…"), `model/extract.go:14`, `model/spatial.go:12`. The public repository
holds one real IFC file — `testdata/two_storey_spanning.ifc`, 3 KB — plus 17
synthetic fixtures, 76 KB in total.

So [compatibility](../compatibility.md) is accurate when it says Blox's import
pipeline is the only consumer this library has been hardened against, and a
reader can do nothing with that. The evidence is unreachable, unrunnable and
unextendable. For a library whose central claim is that its numbers are
trustworthy *bounds*, the proof of the claim lives somewhere else.

### The gaps are unranked, and the degradation is silent

`tessellateItemDepth` dispatches on representation-item type and falls through
to `obbFromItem` (`geometry/geometry.go:131`). An unsupported item therefore
produces a bounding box, not an error. `IfcTriangulatedFaceSet` and
`IfcPolygonalFaceSet` appear nowhere in the module; `IfcRevolvedAreaSolid` has
no tessellation path, as `geometry/testdata/synthetic/obb_revolved.ifc` records.
Elements built from any of them become boxes and say so only through
`Element.Source == SourceOBB`.

A box is not a slightly worse solid. A pitched roof's OBB can hold twice its
true volume. The share of a model that degrades this way is the library's real
accuracy figure, and it is currently unmeasured — so every decision about which
geometry gap to close next rests on guesswork about which IFC constructs
exporters actually emit.

## Goal

Make the bounds claim executable, and rank the gaps by frequency on real files.

Three outputs:

1. A public corpus any contributor can clone and run.
2. A gate asserting that goifc's bounds really do bound IfcOpenShell's answer.
3. A frequency-ranked list of the representation-item types that fall back.

## Non-goals

NURBS and `IfcAdvancedBrep` evaluation, and IFC write support, stay out of
scope. Both remain documented boundaries in [limitations](../limitations.md)
rather than roadmap items: the target user reads IFC to measure it inside a Go
service, and neither gap blocks that.

Running IfcOpenShell itself in-process — compiled to WebAssembly under wazero —
was weighed and rejected before this design. It yields a cgo-free distribution,
not a Go library: OCCT depends on C++ exceptions and threads that wazero does
not implement, the embedded artifact would dwarf the service importing it, and
LGPL-3 geometry inside an MIT module is a licence question every corporate user
would have to answer. The honest subset is the product. This harness measures
how large the subset is.

## The corpus

| File | Raw | Gzipped | Exporter | Ships |
|---|---|---|---|---|
| `ifcopenhouse.ifc` | 113 KB | 28 KB | IfcOpenShell 0.5.0-dev | yes |
| `duplex_a.ifc` | 2.4 MB | 463 KB | Revit 2011 | yes |
| `fzk_haus.ifc` | 2.6 MB | 488 KB | ArchiCAD | yes |
| `office_a.ifc` | 4.1 MB | — | Revit + Solibri | no, private |
| `kb645.ifc` | 29.6 MB | — | ArchiCAD | no, private |

The three shipped files are redistributable published samples: IfcOpenShell's
own IfcOpenHouse, buildingSMART's Duplex Apartment common building model, and
KIT Karlsruhe's FZK-Haus. They commit gzipped, 979 KB together.

A fourth file, `rhino_house.ifc` (5.2 MB, Asuni ASIFC 2.15), was dropped. Its
exporter is commercial, it is not a buildingSMART sample, and it carries no
licence statement, so its provenance cannot be established. It was the only file
in the set likely to hold `IfcAdvancedBrep` geometry — which costs nothing here,
because NURBS is out of scope. Should that change, the corpus needs a sample
model whose licence is known, not this one.

The two private files load from `$GOIFC_PRIVATE_CORPUS` when it is set. They
sharpen the gap ranking locally and never gate CI, so a contributor without them
sees every public gate pass.

## Why a nested module

A Go module zip contains every file in the repository. Committing the corpus at
the root would add roughly a megabyte to `go get github.com/blox-eng/goifc` for
every user, permanently — against a project that opens by promising one `go get`
and one static binary.

Go prunes any subdirectory holding its own `go.mod` from the parent's module zip.
So `parity/` becomes a separate module, and library users download none of it
while contributors get it in the same clone.

```
parity/
  go.mod              module github.com/blox-eng/goifc/parity
                      require github.com/blox-eng/goifc v0.0.0
                      replace github.com/blox-eng/goifc => ../
  corpus.go           gzip.NewReader -> step.Parse; $GOIFC_PRIVATE_CORPUS
  parity_test.go      gate 1
  coverage_test.go    gate 2
  cmd/coverage/       writes docs/coverage.md
  testdata/
    *.ifc.gz
    oracle/*.json     GlobalID -> {min, max}, from IfcOpenShell
    baseline.json     committed coverage numbers
```

The `replace` directive points at the working tree, so the harness always tests
uncommitted local changes. Fixtures decompress through `gzip.NewReader` into the
existing `step.Parse(io.Reader)` (`step/parse.go:21`); the library needs no
change to read them.

Two alternatives lost. Committing at the repository root is simpler — one
module, `go test ./...` reaches everything — but pays the module-zip cost
forever. A separate `goifc-corpus` repository isolates the data best, and then
the harness and the library version independently and no contributor can run the
gate from a single clone, which defeats the purpose of publishing it.

## Gate 1: the bounds must bound

For every element the oracle knows, goifc's world AABB must **contain**
IfcOpenShell's, within tolerance.

Containment, not equality. An OBB fallback box is legitimately larger than the
solid it stands for; that is the design, and `GeomSource` labels it. A goifc box
*smaller* than IfcOpenShell's is always a bug, because it means a bound that
under-reports — the one failure mode a consumer cannot defend against.

This converts a prose claim into an assertion anyone can run. The docs currently
say that where goifc's numbers are bounds rather than truth, it says so in the
data. After this gate, that sentence is tested.

### The looseness ratio

Alongside pass or fail, the harness records goifc's AABB volume divided by the
oracle's, per element, and reports the distribution per model.

That ratio is the product metric. "Closing the gap to IfcOpenShell" means
driving it toward 1.0, and unlike a feature checklist it is a single number that
moves when the geometry genuinely improves. Each closed gap gets a release note
with a before and after.

## Gate 2: coverage must not regress

Element-level OBB rate per model, compared against `baseline.json`. A rise fails
CI. A fall is the point, and `make parity-baseline` rewrites the file.

Element-level rates need no new API: `Element.Source` already carries
`extrude`, `brep` or `obb` (`geometry/geometry.go:12-16`).

### One API addition

Ranking the gaps needs the *representation-item type* that hit the fallback, and
`tessellateItemDepth` discards it — every unsupported item returns `SourceOBB`
alike. Without that attribution the harness can report "37% of elements are
boxes" but not "because 34% of them are `IfcTriangulatedFaceSet`", and the
ranking is the reason to measure first.

`parity` is a separate module and cannot reach unexported helpers, so `geometry`
gains a diagnostic reporting the unhandled item types in a file with their
counts. Having the harness keep its own copy of the handled-type list was
rejected: the copy drifts from the switch it mirrors, silently, and a coverage
harness that lies is worse than none.

The addition is public surface and needs a changelog entry. It earns its place
beyond this harness by answering a question users already ask — why did this
element come back as a box.

## CI and the published number

A new blocking `parity` job runs `cd parity && go test ./...`. Being a separate
module, it is invisible to the existing `test` job's `./...` and to Codecov;
that is correct, since harness coverage should not dilute the library's figure.
`make parity` runs it locally, and joins `make ci` so that target keeps matching
CI's blocking set (`Makefile:94`). Lint covers `parity/` too.

`docs/coverage.md` publishes the result: per model, element count, OBB rate, the
most frequent unhandled item types, and the looseness distribution. It is
generated by `go run ./parity/cmd/coverage -md` and committed. CI regenerates and
diffs it, failing when it is stale, because a published accuracy number that has
quietly drifted is worse than no number at all.

Regenerating the oracles needs IfcOpenShell, through a documented `make oracle`
running it in Docker. That is a maintainer dependency at development time only.
Users still install nothing, and CI never runs it — the oracle JSON is committed
data.

## What this unblocks

The ranked output decides the next sub-project rather than this document doing
it. The expected order, to be confirmed by data:

1. Close the highest-frequency tessellation gaps — likely
   `IfcTriangulatedFaceSet`, `IfcPolygonalFaceSet`, `IfcIndexedPolyCurve`, and
   the missing parameterised profiles.
2. Net volume, extending the trust-gate pattern `geometry/netarea.go` already
   applies to elevational area.
3. True solid-solid CSG, only if net volume's trust gates reject too many hosts
   to be useful.

Each gets its own design doc, and each is measured by the same two gates.

## Risks

**The oracle encodes IfcOpenShell's own choices.** Which entity classes count as
elements, how it treats openings, which representations it prefers. Gate 1
inherits every one of them. `model/extract.go:14-22` already documents this for
element selection and argues the direction of error is safe. The gate reports
elements missing from either side separately rather than failing on them, so a
selection difference reads as a diff instead of a red build.

**Tolerance choice is load-bearing.** Too tight and floating-point noise fails
the build; too loose and a real under-report slips through. The initial value
comes from measuring the actual distribution on the three files, and is recorded
here once measured rather than guessed now.

**Corpus skew.** Three published sample files are small and clean next to a 30 MB
production export. The gap ranking they produce is therefore provisional, which
is exactly why `$GOIFC_PRIVATE_CORPUS` exists.
