# Status Sync and Release News Line Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** README, docs and goifc.org agree on what goifc does, Builders included, and goifc.org names the newest release in a news line the release workflow writes.

**Architecture:** Piece 1 (Tasks 1–2) is docs on branch `feat/91-parametric-builders` (PR #147), before merge. Piece 2 (Tasks 3–4) is a new branch `feat/news-line` from `main` after #147 merges: two bash scripts ported from matchblox, Go tests that drive them, and wiring into `www/`, the Landing workflow and the Release workflow.

**Tech Stack:** Go 1.25 tests, bash + awk, MkDocs Material (strict), static HTML/CSS.

**Spec:** `docs/superpowers/specs/2026-11-12-status-and-news-line-design.md`

## Global Constraints

- Synthetic figures only; no private names. Grep the staged diff for private names before each commit.
- Signed Conventional Commits (`git commit -S`), never `--no-verify`.
- `www/` keeps no build step, loads nothing from another origin, and stays one screen with no scroll.
- The landing page's feature list matches `README.md` and `docs/index.md` (`www/README.md`).
- The module keeps one dependency; tests add none.

## Review Focus

- A headline with `&`, `<`, `"` or backticks: the page gets escaped text, never markup.
- A release section whose first bullet is not bold but a later one is: the script must take the first bold bullet of the section only, and stop at the next `## ` heading.
- `## Unreleased` above the releases: the guard must skip it and take the first `## vX.Y.Z` heading.
- The landing page at 400 px wide with the news line: still one screen, the line wraps under its tag.
- The release workflow when the section has no headline: the release still succeeds and the changelog PR says the line is stale.

---

### Task 1: Builders guide backed by a compiled example

**Files:**
- Create: `geometry/example_builders_test.go`
- Create: `docs/guides/builders.md`
- Modify: `mkdocs.yml` (nav, Guides), `docs/index.md` (a card under "Where to go next")

**Interfaces:**
- Consumes: `geometry.BuildWall`, `BuildRoomWalls`, `BuildSlab`, `Opening`, `SideLeft`, `PlacementError`, `ReasonTooWide` (PR #147).
- Produces: nothing code-facing.

- [ ] **Step 1: Write `ExampleBuildWall`**: a 2.40 × 2.60 × 0.10 wall along (0,0)→(2.40,0), `SideLeft`, door `{Offset: 0.50, Sill: 0, Width: 0.80, Height: 2.00}`. Print `MeasuredNet`, `OtherNet`, `NetVolume` with `%.2f`/`%.3f`. Write `// Output:` with deliberately wrong values first.
- [ ] **Step 2: Run** `go test ./geometry -run Example -v`. Expected: FAIL, got/want differ.
- [ ] **Step 3: Fix the Output block** to `4.64`, `4.64`, `0.464`. Add `ExampleBuildWall_refusal`: the same wall with a door at Offset 2.00, Width 0.80; print each `*PlacementError` via `errors.As` walk → `wall 0 opening 0: too_wide`.
- [ ] **Step 4: Run** `go test ./geometry -run Example -v`. Expected: PASS.
- [ ] **Step 5: Write `docs/guides/builders.md`**: what builders are for (no IFC), the wall example (same code as the Example), the quantities table, refusals (the four reasons in check order, one line each), room walls and mitres, slabs, niches (`Depth`, `NicheBack`), `Side`, `base` for storeys, and "not yet: T-joins". Add it to nav after "Measuring silhouettes" and a card on `docs/index.md`.
- [ ] **Step 6: Run** `make docs`. Expected: "Documentation built", no warnings.
- [ ] **Step 7: Commit** `docs(geometry): guide the builders, with a compiled example (#91)`.

### Task 2: Status sync and the release headline

**Files:**
- Modify: `docs/index.md`, `www/index.html`, `docs/limitations.md`, `CHANGELOG.md`

- [ ] **Step 1:** `docs/index.md` "What you get" gains the README's Builders bullet, word for word, between Quantities and Untrusted files.
- [ ] **Step 2:** `www/index.html` `.does` list gains `<li><b>Builders</b> walls, room walls and slabs from numbers, no IFC</li>` in the same position.
- [ ] **Step 3:** `docs/limitations.md` gains a Builders entry: walls that meet mid-wall (T-joins) are not built yet; build them as separate walls.
- [ ] **Step 4:** `CHANGELOG.md` Unreleased: the first bullet starts `- **Build walls, room walls and slabs from numbers, with no IFC.**`.
- [ ] **Step 5: Run** `make docs` and `.github/scripts/check-links.sh www`. Expected: both pass. Screenshot `www/index.html` at 1280×800 and 400×800. Expected: one screen, no scroll.
- [ ] **Step 6: Commit** `docs: list the builders on the landing page and the docs home (#91)`.

### Task 3: announce-release.sh and news-guard.sh

Branch `feat/news-line` from `main` after #147 merges.

**Files:**
- Create: `.github/scripts/announce-release.sh`, `.github/scripts/news-guard.sh`
- Create: `internal/release/scripts_test.go` (package `release`, test-only)

**Interfaces:**
- Produces: `announce-release.sh VERSION [changelog] [page]` (exit 0 writes the line; exit 1 leaves the page and prints `announce-release: <why>`); `news-guard.sh [changelog] [page]` (takes the first `## vX.Y.Z` heading; exit 1 prints has/want and the fix command).

- [ ] **Step 1: Write the tests** with a helper that runs a script in `t.TempDir()` copies:
  - `TestAnnounceWritesTheLine`: section `## v0.4.0 — 2026-01-01` with `- **Faster parsing.** More text` → page contains `<a class="news" href="https://github.com/blox-eng/goifc/releases/tag/v0.4.0"><span class="tag">New</span> <span>v0.4.0 · Faster parsing</span> <span class="go">Release notes →</span></a>`, indentation kept.
  - `TestAnnounceWrappedHeadline`: bold run over two lines → joined with one space.
  - `TestAnnounceEscapes`: headline ``Older <news> & `more` "x"`` → `Older &lt;news&gt; &amp; more &quot;x&quot;`.
  - `TestAnnounceRefuses`: no bold bullet in the section (a bold one under the next section must not count); page with zero or two news lines; version `0.4.0"><script>` → exit 1, page bytes unchanged.
  - `TestNewsGuard`: passes after announce; fails on a hand-edited line with output naming `announce-release.sh 0.4.0`; skips `## Unreleased`.
- [ ] **Step 2: Run** `go test ./internal/release/ -v`. Expected: FAIL, scripts missing.
- [ ] **Step 3: Write both scripts**, ported from matchblox's, for goifc's heading `## vVERSION — date` (match the prefix `## vVERSION ` or the line `## vVERSION`) and goifc's release URL. Guard derives the version with `awk '/^## v[0-9]/ {sub(/^## v/,""); print $1; exit}'`.
- [ ] **Step 4: Run** `go test ./internal/release/ -v` and `shellcheck .github/scripts/*.sh`. Expected: PASS, no findings.
- [ ] **Step 5: Commit** `ci: write and guard the landing page's release news line`.

### Task 4: Wire the news line

**Files:**
- Modify: `www/index.html`, `www/styles.css`, `www/README.md`, `.github/workflows/landing.yml`, `.github/workflows/release.yml`, `CONTRIBUTING.md`

- [ ] **Step 1:** Add `<a class="news" …>` as the first child of `.stage > div`, above the h1; run `announce-release.sh <newest>` to fill it. Port matchblox's `.news` CSS onto goifc's tokens (`--muted`, `--fg`, the accent, the mono and sans stacks).
- [ ] **Step 2:** `landing.yml`: add `CHANGELOG.md` and `.github/scripts/news-guard.sh` to both `paths` lists; add step `News line` running `.github/scripts/news-guard.sh`.
- [ ] **Step 3:** `release.yml` promote step: after `promote-changelog.sh`, run `announce-release.sh "$RELEASE_VERSION"`; on failure append to the PR body: "**The site's news line still names the last release:** v$V has no headline (a `- **...**` line) in CHANGELOG.md. Add one, run `.github/scripts/announce-release.sh $V` on this branch, and push." `git add www/index.html` with CHANGELOG.md.
- [ ] **Step 4:** `CONTRIBUTING.md`: each release's changelog entry set opens with one `- **headline**` bullet, which goifc.org shows. `www/README.md`: one paragraph on the news line and who writes it.
- [ ] **Step 5: Run** `check-links.sh www`, `news-guard.sh`, `actionlint`, `go test ./...`; screenshot 1280×800 and 400×800. Expected: all pass; one screen.
- [ ] **Step 6: Commit** `feat(www): announce the newest release on goifc.org`.
