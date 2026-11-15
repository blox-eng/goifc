# Changelog promotion without a bot PR (#140)

Approved 2026-11-15.

## Intent

A release's notes list only what that release shipped, and nobody merges a
bot PR to keep it so. Today the Release workflow opens an unsigned
`github-actions[bot]` PR after each release. GitHub runs no CI on it, so it
never merges, `## Unreleased` keeps every entry, and each release's notes
repeat the ones before it (#123, #125, #128, #133, #138, #158).

## Goals

- The Release workflow commits nothing and opens no PR.
- A person promotes the release in the next PR with one command, `make promote`.
- CHANGELOG.md cannot fall a release behind without a required check failing,
  and the failure names the command that fixes it.

## Non-goals

- Auto-merge, GitHub App tokens, new secrets, ruleset changes.
- Predicting the next version in the PR that causes the release.

## Approaches

1. **No bot PR; CI forces the promotion (picked).** No secret, no repo
   setting, and the bot loses `pull-requests: write`. Costs one small commit
   in the PR after each release.
2. **Signed bot PR that merges itself.** Commit through the API (signed),
   dispatch CI on the branch, auto-merge. Needs repo-wide auto-merge, CI that
   runs outside a PR, and a reworked Conventional Commits check.
3. **GitHub App token.** Adds a long-lived write key to an OSS repo. Ruled out.

Picked 1: fewest moving parts, nothing new to secure, and drift is a visible
failure instead of a warning.

## Design

- **Why the notes stay right.** Every squash-merge to main cuts at most one
  release, and the guard stops any PR merging while the latest tag has no
  heading. So at release time `## Unreleased` holds only that release's
  entries, and the workflow keeps publishing it as the notes.
  One race is left (found in review): a PR whose checks passed before the
  tag existed can still merge. The notes step then finds no heading for the
  previous tag, warns, and keeps the generated notes instead of repeating
  entries. CONTRIBUTING.md says to wait for the Release run before merging
  the next PR.
- **`.github/workflows/release.yml`.** Delete the "Promote Unreleased
  changelog section" step. Drop `pull-requests: write`. Keep the notes step.
- **`.github/scripts/changelog-guard.sh [tag] [changelog]`.** `tag` defaults
  to the newest `v*` tag in git (by version order), `changelog` to
  CHANGELOG.md. Passes when no tag exists or when the changelog has a
  `## vX.Y.Z —` heading for it. Otherwise fails with: CHANGELOG.md has no
  heading for vX.Y.Z; run `make promote` and commit CHANGELOG.md and
  www/index.html.
- **Where the guard runs.** A step in the Conventional Commits Check job of
  `ci.yml`, after a checkout that fetches tags. That job is required and runs
  on every PR, docs-only ones included; Lint is skipped on docs-only PRs, and
  a `fix:` PR touching only docs still cuts a release.
- **`make promote`.** Takes the newest `v*` tag and its commit date in UTC,
  runs `promote-changelog.sh` and `announce-release.sh`. Refuses when the
  heading already exists. When announce fails (no headline), prints its
  message, leaves www/index.html as it is, and still succeeds: the Landing
  check keeps the last headline. Run it before adding new entries.
- **Docs.** CONTRIBUTING.md says the next PR after a release runs
  `make promote`; the news-line paragraph no longer says the workflow writes it.

## Tests

Go tests in `internal/release/scripts_test.go`, synthetic changelogs only:
the guard passes with the heading, fails naming `make promote` without it,
passes with no tag; `make promote` output matches what the bot produced.

## Left to the builder

Script internals, the checkout depth, and test names.
