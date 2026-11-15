# Changelog Promotion Without a Bot PR Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Release workflow stops opening changelog PRs; `make promote` does the promotion in the next PR, and a required check fails while the newest tag has no changelog heading.

**Architecture:** Two bash scripts under `.github/scripts/` (a guard and a promote wrapper over the existing `promote-changelog.sh` and `announce-release.sh`), a Makefile target, one step in CI's Conventional Commits Check job, and the promote step deleted from `release.yml`. Tests are Go tests that run the scripts, as the existing ones do.

**Tech Stack:** bash, awk, git, GitHub Actions YAML, Go `testing`.

**Spec:** `docs/superpowers/specs/2026-11-15-changelog-promote-design.md`

## Global Constraints

- Synthetic fixtures only; nothing private in the repo.
- Signed Conventional Commits; never `--no-verify`.
- No new secrets, tokens, repo settings or ruleset changes.
- Heading format: `## vX.Y.Z — YYYY-MM-DD` (em dash U+2014), as `promote-changelog.sh` writes it.
- Guard failure text names the fix: `run make promote, then commit CHANGELOG.md and www/index.html`.

## Review Focus

1. A tag `v0.1.1` while the changelog has only `## v0.1.10 — …`: the guard fails (no prefix match).
2. A pre-release tag `v1.0.0-rc.1`: the guard matches `## v1.0.0-rc.1 — …`.
3. A clone with no `v*` tags (a fork): the guard passes.
4. `make promote` run twice: the second run refuses and leaves both files unchanged.
5. A release with no headline: promote succeeds, `www/index.html` is unchanged, the message is printed.

---

### Task 1: The guard, and CI runs it

**Files:**
- Create: `.github/scripts/changelog-guard.sh`
- Modify: `.github/workflows/ci.yml` (Conventional Commits Check job)
- Test: `internal/release/scripts_test.go`

**Interfaces:**
- Produces: `changelog-guard.sh [tag] [changelog]`. `tag` is `vX.Y.Z[-pre]`, default the newest `v*` tag (`git tag --list 'v*' --sort=-v:refname`); `changelog` defaults to `CHANGELOG.md` at the repo root. Exit 0 with `OK: CHANGELOG.md has a heading for <tag>` or `OK: no v* tag, nothing to check`; exit 1 otherwise.

- [ ] **Step 1: Write the failing test** `TestChangelogGuard`, table-driven over `released` (the existing fixture, which has v0.4.0 and v0.3.0):
  - `v0.4.0` → pass; `v0.5.0` → fail, output contains `v0.5.0` and `make promote`;
  - changelog with only `## v0.1.10 — 2026-01-01`, tag `v0.1.1` → fail;
  - changelog with `## v1.0.0-rc.1 — 2026-01-01`, tag `v1.0.0-rc.1` → pass.
  Add `TestChangelogGuardNoTags`: run the script with no tag argument, `cmd.Dir` set to a fresh `git init` temp repo that holds `released` as CHANGELOG.md → pass, output contains `no v* tag`.
- [ ] **Step 2: Run** `go test ./internal/release/ -run ChangelogGuard` → FAIL (script missing).
- [ ] **Step 3: Implement the script.** Match a line equal to `## <tag> —` followed by a space or the end of the line, never a prefix. Resolve the changelog default against `git rev-parse --show-toplevel` when no argument is given, so the no-tags test works from its temp repo.
- [ ] **Step 4: Run** the same command → PASS.
- [ ] **Step 5: Add a CI step.** In `conventional-commits`, add `actions/checkout` (same pinned SHA as the other jobs, `fetch-depth: 0`, `persist-credentials: false`), then a step named `Changelog names the latest release` that runs `.github/scripts/changelog-guard.sh`. Add a comment: the job is required and runs on docs-only PRs, unlike Lint.
- [ ] **Step 6: Commit** `ci: fail a PR while CHANGELOG.md has no heading for the latest tag (#140)`.

### Task 2: `make promote`

**Files:**
- Create: `.github/scripts/promote-release.sh`
- Modify: `Makefile` (target `promote`, and add it to `.PHONY`)
- Test: `internal/release/scripts_test.go`

**Interfaces:**
- Consumes: `promote-changelog.sh <version> <date>` (works on `./CHANGELOG.md`), `announce-release.sh VERSION [changelog] [page]`, and `changelog-guard.sh` from Task 1.
- Produces: `promote-release.sh [version] [date]`, run from the repo root. `version` has no `v`, default the newest `v*` tag without its `v`; `date` defaults to that tag's commit date in UTC (`TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%d <tag>`). `make promote` runs it with no arguments.

- [ ] **Step 1: Write the failing tests.** Each test runs in a temp dir holding CHANGELOG.md (a fixture whose `## Unreleased` has `- **Shiny thing.**`) and `www/index.html` (`site`), with `cmd.Dir` set there:
  - `TestPromoteRelease`: args `0.5.0 2026-11-15` → CHANGELOG.md equals the fixture with `## Unreleased\n\n## v0.5.0 — 2026-11-15` in place of `## Unreleased`; the page's news line names `v0.5.0 · Shiny thing`; `changelog-guard.sh v0.5.0 CHANGELOG.md` passes.
  - `TestPromoteReleaseTwiceRefuses`: a second run exits 1, output contains `already has`, and both files are byte-equal to after the first run.
  - `TestPromoteReleaseNoHeadline`: Unreleased holds `- Plain fix.` → exit 0, page unchanged, output contains `no headline`.
- [ ] **Step 2: Run** `go test ./internal/release/ -run PromoteRelease` → FAIL (script missing).
- [ ] **Step 3: Implement.** Refuse when `changelog-guard.sh v<version> CHANGELOG.md` already passes (message: `CHANGELOG.md already has a heading for v<version>`). Then run `promote-changelog.sh`, then `announce-release.sh <version> CHANGELOG.md www/index.html`. If announce fails, print its stderr plus `www/index.html keeps the last headline` and exit 0. Call the sibling scripts by `$(dirname "$0")`.
- [ ] **Step 4: Run** the same command → PASS.
- [ ] **Step 5: Add** a Makefile target, `promote: ## Promote Unreleased to the latest tag in CHANGELOG.md and goifc.org (run in the PR after a release)`, that calls `.github/scripts/promote-release.sh`.
- [ ] **Step 6: Commit** `ci: add make promote for the PR after a release (#140)`. Use `ci`, not `feat`: this is maintainer tooling, and a `feat` would cut a minor release.

### Task 3: The Release workflow stops opening PRs

**Files:**
- Modify: `.github/workflows/release.yml` (permissions of `release`; delete the step "Promote Unreleased changelog section"; adjust the notes step's comment, which says the promote step renames the section)
- Modify: `CONTRIBUTING.md` (the news-line paragraph, plus a short "After a release" paragraph)
- Test: `internal/release/scripts_test.go`

- [ ] **Step 1: Write the failing test** `TestReleaseWorkflowOpensNoPR`: read `../../.github/workflows/release.yml`. It must not contain `gh pr create`, `pull-requests: write` or `promote-changelog.sh`, and it must still contain `release-notes.sh Unreleased`.
- [ ] **Step 2: Run** `go test ./internal/release/ -run ReleaseWorkflow` → FAIL.
- [ ] **Step 3: Edit release.yml.** Delete the step and drop `pull-requests: write` together with its comment. The notes-step comment now says: the next PR promotes the section with `make promote`, and CI's changelog guard enforces that.
- [ ] **Step 4: Edit CONTRIBUTING.md.** "The release workflow writes it" becomes "`make promote` writes it". Add: after a release, the next PR runs `make promote` before adding its own entries, and commits CHANGELOG.md and www/index.html. Until then, the Conventional Commits Check fails and names the command.
- [ ] **Step 5: Run** `go test ./internal/release/` and `actionlint` if it is installed → PASS.
- [ ] **Step 6: Commit** `ci: stop the Release workflow opening changelog PRs (#140)`.

Last: add an `## Unreleased` entry to CHANGELOG.md in Task 3's commit: `- Releases no longer open a changelog PR. Run make promote in the next PR; CI fails until you do. (#140)`. Use a plain bullet with no bold run, so it never becomes goifc.org's headline.
