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

## Documentation

The site at <https://docs.goifc.org/latest/> is built from `docs/` with
MkDocs. To preview a change locally:

```bash
make docs-serve
```

The build runs with `--strict`, so a broken internal link fails rather than
publishing a dead one. `make docs` does a one-shot build if you only want the
check.

Two pages are *included* from their canonical file rather than copied —
`docs/step.md` from `step/README.md`, and `docs/changelog.md` from
`CHANGELOG.md`. Edit the source file, not the page, or your change will be
overwritten by the include.

Docs deploy automatically on merge to `main` and on `v*` tags. It is deliberately
not a required check: a Pages outage should never block a merge.

The landing page at <https://goifc.org> is a separate site: hand-written HTML in
`www/`, no build step, deployed by Cloudflare Pages. Preview it with
`python3 -m http.server --directory www`, and see [`www/README.md`](www/README.md)
before changing it. It never links into the docs by relative path.

Its news line names the newest release that has a headline: the first bold
bullet of its changelog section. `make promote` writes it. A change worth
announcing opens its `## Unreleased` entry with one:

```markdown
- **Build walls, room walls and slabs from numbers, with no IFC.** (#91)
  The rest of the entry.
```

A release without a headline, such as most fixes, leaves the line on the last
release that had one. The Landing check fails when the line drifts from that
rule; the failure prints the command that fixes it.

### After a release

A release publishes `## Unreleased` as its notes. The Release workflow's
Promote Changelog job then opens a pull request,
`docs: promote Unreleased to vX.Y.Z in CHANGELOG.md`, that runs `make promote`
on the released commit and merges itself once its checks pass. Until it
merges, the Conventional Commits Check fails on every other pull request and
names the command.

The job opens the pull request with the repository secret `RELEASE_TOKEN`: a
fine-grained token for this repository only, with Contents and Pull requests
read and write. A pull request opened with the built-in token starts no CI
and never merges. When the token is missing or expired the job fails and
says so; make a new token, save it as `RELEASE_TOKEN`, and re-run the job.

If the job fails or its pull request does not merge, promote by hand, in a
pull request of its own, from `main`:

```sh
git fetch --tags
git switch -c docs/promote-vX.Y.Z origin/main
make promote
git commit -am "docs: promote Unreleased to vX.Y.Z in CHANGELOG.md"
```

Then rebase your open pull requests onto `main`; their own entries stay
under `## Unreleased`. `make promote` refuses when `## Unreleased` holds
entries the release did not ship, such as your branch's own entries, so it
never credits them to the wrong release.

Wait for the Release run to finish before you merge the next pull request.
A pull request whose checks passed before the tag existed can still merge.
Then `## Unreleased` on `main` holds more than the release shipped, and:

- the next release keeps GitHub's generated notes, and the workflow warns;
- `make promote` refuses. Add the `## vX.Y.Z — date` heading by hand, above
  the entries that `git show vX.Y.Z:CHANGELOG.md` lists under
  `## Unreleased`, and keep the later entries under `## Unreleased`.

If two releases were cut before anyone promoted, the guard names the older
one and `make promote` refuses. Split `## Unreleased` by hand: one heading
per release, each above the entries its tag added.

## Pull requests

Keep PRs focused on one change. Include a test for any bug fix or new
behavior. If you're changing parsing or geometry output, add or update a
fixture under `testdata/`.

## Code of conduct

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
