# Contributing

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org/).
The type must be one of:

`feat, fix, cicd, chore, patch, release, test, docs, refactor, ci, dev`

Example: `fix: correct wall volume for zero-height storeys`

## Before pushing

Run the full check suite locally:

```bash
make ci
```

This runs formatting, vetting, and tests. CI runs the same target on every
pull request; a PR won't merge if it fails.

## Pull requests

Keep PRs focused on one change. Include a test for any bug fix or new
behavior. If you're changing parsing or geometry output, add or update a
fixture under `testdata/`.
