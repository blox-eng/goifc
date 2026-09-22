# Regenerating the oracles

`testdata/oracle/*.json` map GlobalID to the world-space AABB IfcOpenShell
computes, in meters. They are committed data: CI reads them and never
regenerates them, so nothing in the normal build needs IfcOpenShell.

Regenerate only when the oracle itself should change — a new corpus model, or a
deliberate move to a new IfcOpenShell version (which is a behaviour change worth
its own commit message).

    make oracle

This runs IfcOpenShell in Docker against every model in `testdata/` and rewrites
the JSON. Review the diff: an unexplained change in existing numbers means the
oracle moved under you, and the parity gate's meaning moved with it.

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

The safe practice: run the tooling against a **temporary directory**, never
in place over `testdata/oracle/`, and diff the result against the committed
files by hand before copying anything over:

    mkdir -p /tmp/oracle-check
    gzip -dc testdata/ifcopenhouse.ifc.gz > /tmp/oracle-check/ifcopenhouse.ifc
    docker run --rm -v /tmp/oracle-check:/data -v "$PWD/oracle":/src \
        aecgeeks/ifcopenshell:latest \
        python3 /src/dump_oracle.py /data/ifcopenhouse.ifc /data/ifcopenhouse.json
    diff testdata/oracle/ifcopenhouse.json /tmp/oracle-check/ifcopenhouse.json

If a regeneration disagrees materially with what's committed, do not copy it
over the working files — keep the committed oracle and record the discrepancy
here (or in the PR) instead of overwriting a working gate with a
freshly-guessed one.
