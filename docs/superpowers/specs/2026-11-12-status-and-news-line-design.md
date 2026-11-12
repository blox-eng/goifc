# Status sync and release news line — design

Date: 2026-11-12. Status: approved.

## Intent

A reader who lands on goifc.org, the README or the docs sees what goifc does
today and what shipped last. Two gaps stop that now:

1. The pages disagree. `www/README.md` requires the landing page's copy to
   match `README.md` and `docs/index.md`, and every claim on it to be true of
   `main`. The builders (#91) are in the README only.
2. The landing page says nothing about releases. matchblox's landing page
   carries one "New" line over the hero, written at release time and guarded
   in CI; goifc.org gets the same.

## Goals

- `README.md`, `docs/index.md` and `www/index.html` list the same features,
  Builders included.
- The docs explain the builders in a guide whose code compiles.
- goifc.org shows `New · vX.Y.Z · <headline> · Release notes →` for the
  newest release, written by the release workflow, never by hand.
- CI fails when the news line does not name the newest release.

## Non-goals

- Announcements other than releases.
- A news archive. GitHub Releases is the archive.
- Any change to the library's API.

## Anti-goals

- A page that fetches its news at run time: `_headers` sets
  `script-src 'self'`, and the page loads nothing from another origin.
- A build step for `www/`: the page is hand-written static HTML.

## Approaches

| Approach | Trade-off | Verdict |
|---|---|---|
| Port matchblox's release-time scripts | Static HTML, guarded, proven in matchblox | **Chosen** |
| Fetch GitHub's releases API from `app.js` | Blocked by the page's CSP; rate-limited; a failed fetch shows nothing | Rejected |
| Generate the line in a Pages build step | Breaks "no build step"; the line is invisible in review | Rejected |

## Piece 1: status sync (in #91's pull request, before merge)

- `docs/index.md` gains the README's Builders bullet, word for word.
- `www/index.html`'s feature list gains `Builders` — walls, room walls and
  slabs from numbers, no IFC. The headline "Read IFC in Go." stays.
- `docs/guides/builders.md`, in the nav and the index cards: a wall with a
  door and its quantities; the four refusal reasons and `PlacementError`;
  room walls and mitred corners; slabs; niches; `Side`; `base` for storeys.
  Synthetic figures only (the oracle values). An `ExampleBuildWall` in
  `geometry` compiles and checks the guide's main snippet.
- `docs/limitations.md` states that walls meeting mid-wall (T-joins) are not
  built yet.
- The Unreleased changelog section opens with the bold headline
  **Build walls, room walls and slabs from numbers, with no IFC.** It must be
  in before the release cuts, so v0.16.0 can announce itself.

## Piece 2: release news line (own pull request, after #91 merges)

- `.github/scripts/announce-release.sh VERSION [changelog] [page]`: reads the
  first `- **headline**` bullet under `## vVERSION — date` (it may wrap onto
  following lines), strips backticks and a trailing full stop, HTML-escapes
  it, and replaces the page's one `class="news"` line with a link to
  `https://github.com/blox-eng/goifc/releases/tag/vVERSION`. It refuses a
  malformed version, a section with no headline, and a page without exactly
  one news line, and leaves the page untouched each time.
- `.github/scripts/news-guard.sh`: finds the newest release in
  `CHANGELOG.md` that has a headline, writes the expected line to a temporary
  copy, and fails when the real page differs. Its message names the command
  that fixes it. It runs in the Landing workflow, which runs on every change
  to `www/` or `CHANGELOG.md`; the CI Lint job skips changes with no Go code.
  (Amended 2026-11-12, final review: was "the newest release", "in Lint".)
- `release.yml`: after promoting the changelog, runs `announce-release.sh` in
  the same changelog pull request. A release with no headline (most patch
  releases) keeps the last headline (maintainer decision, 2026-11-13), and the
  pull request's body says so.
- Go tests drive both scripts in a temporary directory: a headline, a wrapped
  headline, HTML in the headline, a missing headline, a missing news line,
  and a malformed version.
- `CONTRIBUTING.md`: each release section opens with one bold headline
  bullet.
- `www/styles.css`: matchblox's `.news` rules on goifc's tokens. The page
  stays one screen with no scroll, checked by screenshot at phone and desktop
  widths.

## Left to the builder

- The guide's section order and prose.
- Where the Go tests for the scripts live, following goifc's layout.
