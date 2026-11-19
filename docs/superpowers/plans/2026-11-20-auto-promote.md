# Auto-promote Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** After each release, a pull request promotes the changelog and news line and merges itself.

**Architecture:** One script, `.github/scripts/open-promote-pr.sh`, wraps `promote-release.sh` with git and gh; a new job in `release.yml` runs it with `RELEASE_TOKEN`.

**Tech Stack:** bash, GitHub Actions, Go tests in `internal/release`.

**Spec:** `docs/superpowers/specs/2026-11-20-auto-promote-design.md`

## Global Constraints

- Branch `docs/promote-vX.Y.Z`, commit and PR title `docs: promote Unreleased to vX.Y.Z in CHANGELOG.md` (what `make promote` users write).
- The job's `GITHUB_TOKEN` is `contents: read`; only `RELEASE_TOKEN` writes. No direct push to `main`.
- Actions pinned by SHA, as the rest of `release.yml`.

## Review Focus

1. A re-run of the job: no second PR, no failure.
2. A release already promoted by hand: the job does nothing.
3. `RELEASE_TOKEN` missing or expired: the job fails and names the fix.
4. `main` moves while the PR waits: the branch is updated, or the job warns after 40 min.
5. The token never appears in a log line the script writes.

### Task 1: open-promote-pr.sh, the job, the docs

**Files:** Create `.github/scripts/open-promote-pr.sh`. Modify `.github/workflows/release.yml`, `internal/release/scripts_test.go`, `CONTRIBUTING.md`, `CHANGELOG.md`.

- [ ] Step 1: Tests: `TestOpenPromotePR` (stub gh, bare remote: branch pushed with the v0.5.0 heading; gh called `pr create` once and `pr merge --squash --auto`), `TestOpenPromotePRRerun` (pr view finds it: no create), `TestOpenPromotePRAlreadyPromoted` (no push, no gh), `TestOpenPromotePRRefusal` (promote refuses: non-zero, message kept); replace `TestReleaseWorkflowOpensNoPR` with `TestReleaseWorkflowPromotesWithReleaseToken`.
- [ ] Step 2: `go test ./internal/release/` → FAIL.
- [ ] Step 3: Write the script and the job; update CONTRIBUTING.md "After a release" and CHANGELOG Unreleased/Changed.
- [ ] Step 4: `go test ./internal/release/`, `make lint`, actionlint if present → PASS.
- [ ] Step 5: Commit `ci: open and auto-merge the changelog promotion after a release`.
