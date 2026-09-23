# Design docs

Design docs record *why* a non-trivial change was made — the alternatives weighed
and why one won. They are dated snapshots of a decision, not living
documentation; where one disagrees with the rest of this site, the site is
current and the doc is history.

- [Section geometry on an arbitrary plane](2026-08-13-arbitrary-section-plane.md)
  — generalising the horizontal-only footprint cut to any plane (issue #3,
  shipped in v0.2.0).
- [Outward-facing classification](2026-08-15-outward-facing-classification.md)
  — deciding which way a building element faces, and which of its two sides is
  the exposed one (issue #4).
- [Parity and coverage harness](2026-09-21-parity-and-coverage-harness.md)
  — a public IFC corpus, a gate asserting goifc's bounds really bound
  IfcOpenShell's answer, and a frequency-ranked list of the geometry gaps.
- [Tessellation gaps and the placement DoS](2026-09-24-tessellation-gaps-and-placement-dos.md)
  — spending that ranking: a stack overflow reachable on untrusted input
  (issue #54), a missing surface-model dispatch case, and a clipping path made
  unreachable by exact-keyword type matching (issue #52).
