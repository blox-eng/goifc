# Promote the changelog automatically after a release

Status: approved 2026-11-20 (a small change: this short design).

## Intent

A merge of a `feat:` or `fix:` commit releases goifc by itself, but
`CHANGELOG.md` and the goifc.org news line move only when someone runs
`make promote` in a pull request. v0.21.0 shipped on 2026-11-19 and the
landing page still named v0.20.0 the next day. The promotion should follow
every release with nobody in the loop.

## Goals and non-goals

- Goal: after each release, a pull request promotes `## Unreleased` to the
  version and writes the news line, then merges itself once its checks pass.
- Non-goals: changing how the version is picked, the release notes or the
  wasm files; pushing to `main` directly.
- Anti-goal: a promotion pull request that gets no CI and never merges, as
  the bot pull request that #160 removed did.

## Approaches

1. **A pull request opened with a repository token, auto-merged (picked).**
   A token other than the built-in one starts CI on the
   pull request, so it can pass its checks and merge.
2. The built-in token: its pull request starts no workflow (#160).
3. A direct push to `main`: the repository takes only pull requests.

## Design

- `.github/scripts/open-promote-pr.sh VERSION` runs on the released commit:
  `promote-release.sh VERSION`, a commit
  `docs: promote Unreleased to vVERSION in CHANGELOG.md` on
  `docs/promote-vVERSION`, a push, a pull request (found again on a re-run)
  and `gh pr merge --squash --auto`. Nothing to promote: it does nothing.
  A refusal of `promote-release.sh` fails the job with its message.
- `release.yml` gets a job "Promote Changelog" after "Create Release". Its
  `GITHUB_TOKEN` keeps `contents: read`; the script uses the secret
  `RELEASE_TOKEN`, a fine-grained token for blox-eng/goifc only (Contents and
  Pull requests, read and write). Without the secret the job fails and says
  how to add it. A second step updates the pull request's branch while it is
  behind `main`, for up to 40 minutes, since auto-merge never does.
- The repository allows auto-merge (set 2026-11-20). GitHub signs the squash
  commit on `main`.
- `make promote` stays as the fallback; CONTRIBUTING.md says when to use it.

## Tests

`internal/release`: the script with a stub `gh` and a local remote pushes the
promoted branch, opens one pull request and asks for auto-merge; a re-run
opens no second one; a release already promoted does nothing; a refusal
fails. The workflow test checks the job uses `RELEASE_TOKEN`, not the
built-in token, for the pull request.
