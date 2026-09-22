# Regenerating the oracles

`testdata/oracle/*.json` map GlobalID to the world-space AABB IfcOpenShell
computes, in meters. They are committed data: CI reads them and never
regenerates them, so nothing in the normal build needs IfcOpenShell.

Regenerate only when the oracle itself should change — a new corpus model, or a
deliberate move to a new IfcOpenShell version (which is a behaviour change worth
its own commit message).

    make oracle

**That target never writes `testdata/oracle/`.** It runs IfcOpenShell in Docker
against every public model, into a `mktemp -d` directory it removes on exit, and
`diff`s the result against the committed files. It prints the diff and exits
non-zero if anything moved; the committed oracle is left exactly as it was.
Promoting a regeneration is a deliberate human act — see "Promoting a verified
regeneration" below.

It checks every step and fails on the first one that breaks, so a docker run
that dies, or writes half a file, cannot leave `make` reporting success.

## Status: currently blocked

**`make oracle` does not work today.** Verified 2026-09-23 against
`aecgeeks/ifcopenshell:latest` (digest
`sha256:dc337e1711820ef3cb644fc2ed5392cf244c3406d7b5ad98ff9a193a79c3586b` at
that time): the image's default `python3` resolves to Python 3.8.10, but the
`ifcopenshell` wheel baked into the image is built for Python 3.10, and no
`python3.10` interpreter exists anywhere in the image. Running
`dump_oracle.py` inside it fails immediately on `import ifcopenshell`:

    ImportError: libpython3.10.so.1.0: cannot open shared object file: No such file or directory
    ...
    ImportError: IfcOpenShell not built for 'linux/64bit/python3.8'

The target and `dump_oracle.py` are correct as far as they go — the blocker is
the published image, not this code. Before relying on this path, a maintainer
must pin a working image (a specific digest known to have a matching
interpreter and `ifcopenshell` build, not floating `:latest`) or find the
correct tag, and update the `oracle` target in the `Makefile` accordingly. A
documented path nobody has verified recently is worse than no path: trust the
"Status" line above over the surrounding instructions until this note is
removed by someone who has re-verified it.

## Provenance of the committed oracles

Gate 1's whole point is that a third party can check goifc's bounds claim
themselves. They cannot do that without knowing what the gate is measured
against, so here is what is actually knowable about these bytes — and what is
not. Metadata deliberately lives here and not inside the JSON:
`parity.LoadOracle` unmarshals into `map[string]AABB`, so a `_meta` key in one
of those files would decode as a zero-volume box and be compared as if it were
an element.

Stamped 2026-09-23. Re-stamp this section in the same commit that changes any
of these files.

| File | Entries | SHA-256 |
|---|---|---|
| `testdata/oracle/ifcopenhouse.json` | 34 | `043224f04268b2af52afea75e2f40a803e2eb74fdc945d6c7e5f7a90b7a38f18` |
| `testdata/oracle/duplex_a.json` | 215 | `04d278373d786ffbefce5955c8448e3b5d37f9f06d8da7e216b55d2a50007558` |
| `testdata/oracle/fzk_haus.json` | 82 | `e3058aef91d358d4e3b69c3b1dad7e4b58fa38aa5d1a3dbc46166c77f3544106` |

What the content is: keys are IfcOpenShell's `shape.guid` (the IFC GlobalID);
each value is the axis-aligned min/max of `shape.geometry.verts` in meters,
world-space.

**The producing IfcOpenShell version, image digest and generation date are
unknown, and this file does not guess them.** All three files entered this
repository in one commit, this branch's Gate 1 commit (`d984959`,
`test(parity): assert goifc's bounds contain ifcopenshell's`), inherited from
the Blox monorepo where they were originally produced. Regeneration is blocked
(see "Status" above), so the producing version cannot be recovered by
re-running anything either.

`dump_oracle.py` in this directory is the *intended* path forward, not a
record of what produced these bytes — and there is positive evidence it is not
the producer: it writes `json.dump(..., indent=1, sort_keys=True)`, while the
committed files have no indentation at all and their keys are unsorted. Their
values do carry full float64 repr (`0.41700000000000015`), so they are not
rounded. `USE_WORLD_COORDS=True`, which `dump_oracle.py` sets, is consistent
with the committed numbers being world-space, but that is inference from the
data, not a record from the run.

What this means for a reader: Gate 1 is reproducible against *these bytes* —
anyone can clone and run it, and the two `knownViolations` shortfalls are
measured to nine decimals against them. It is not yet reproducible from
IfcOpenShell source. Closing that gap needs a working image pinned by digest
(Status above), after which a regeneration verified by `make oracle` can be
promoted and this section stamped with the real version.

## Before trusting a regeneration

**Never point a regeneration at `testdata/oracle/` casually, and never trust
its output without diffing it first.** `parity/knownviolations.go` records two
Gate 1 shortfalls to nine decimal places, measured against the exact bytes
committed in `testdata/oracle/`. A regeneration under a different IfcOpenShell
version, a different `aecgeeks/ifcopenshell` image tag, or different
`ifcopenshell.geom` settings (`USE_WORLD_COORDS` in particular) will shift
those numbers and either break Gate 1 or silently invalidate the allowlist and
the published coverage page together — with no visible error, since a shifted
oracle just produces a different-but-plausible AABB.

`make oracle` is built to enforce exactly that: temporary directory, diff, no
write. Run it and read the diff.

If a regeneration disagrees materially with what's committed, do not copy it
over the working files — keep the committed oracle and record the discrepancy
here (or in the PR) instead of overwriting a working gate with a
freshly-guessed one.

## Promoting a verified regeneration

Only when the diff is understood and wanted, and by hand — nothing in the build
does this for you:

1. Pin the image by digest in the `oracle` target, so the run is reproducible.
2. `make oracle` and read the whole diff. An unexplained change in existing
   numbers means the oracle moved under you, and Gate 1's meaning moved with
   it.
3. Generate once more into a directory you keep — `make oracle` deletes its
   own — and copy those files over `testdata/oracle/` yourself:

       mkdir -p /tmp/oracle-promote
       gzip -dc testdata/ifcopenhouse.ifc.gz > /tmp/oracle-promote/ifcopenhouse.ifc
       docker run --rm -v /tmp/oracle-promote:/data -v "$PWD/oracle":/src:ro \
           aecgeeks/ifcopenshell@sha256:<the digest you pinned> \
           python3 /src/dump_oracle.py /data/ifcopenhouse.ifc /data/ifcopenhouse.json
       cp /tmp/oracle-promote/ifcopenhouse.json testdata/oracle/ifcopenhouse.json

4. Re-measure the allowlist. `cd parity && go test ./... -run TestGate1` fails
   on any allowlisted violation that has widened or stopped reproducing, and
   names the newly measured shortfall; update `knownviolations.go`'s
   nine-decimal numbers to those, or delete the entries that are gone. A stale
   allowlist against a fresh oracle is the failure mode this whole file exists
   to prevent — and note that a shortfall can shift by less than
   `allowlistMargin` without failing anything, so re-read the numbers rather
   than trusting a green run.
5. `make parity-report` and commit the regenerated `docs/coverage.md` with it.
6. Re-stamp "Provenance of the committed oracles" above: new hashes, entry
   counts, the pinned image digest, the IfcOpenShell version it reports, and
   the date.

All of that belongs in one commit whose message says which IfcOpenShell version
the oracle moved to and why.
