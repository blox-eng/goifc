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

This runs the linter and the tests — the same two gates CI applies to every
pull request. A PR won't merge if either fails.

`make all` additionally formats and vets. Run it before you push if your
editor does not format on save; CI will not reformat for you.

## Pull requests

Keep PRs focused on one change. Include a test for any bug fix or new
behavior. If you're changing parsing or geometry output, add or update a
fixture under `testdata/`.

## Code of conduct

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
