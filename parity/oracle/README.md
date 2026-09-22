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

**Before trusting a regeneration, diff it against the committed files rather
than overwriting them in place.** `parity/knownviolations.go` records two Gate
1 shortfalls to nine decimal places, measured against the exact committed
oracle in `testdata/oracle/`. A regeneration under a different IfcOpenShell
version, a different `aecgeeks/ifcopenshell` image tag, or different
`ifcopenshell.geom` settings (`USE_WORLD_COORDS` in particular) will shift
those numbers and either break Gate 1 or silently invalidate the allowlist and
the published coverage page together. If a regeneration disagrees materially
with what's committed, do not copy it over the working files — keep the
committed oracle and record the discrepancy here (or in the PR) instead.
