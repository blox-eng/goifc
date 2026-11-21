#!/usr/bin/env bash
# Opens the pull request that promotes CHANGELOG.md and the goifc.org news
# line to a new release, and asks GitHub to merge it once its checks pass.
# The Release workflow runs it on main, after the release.
#
# Usage: open-promote-pr.sh VERSION   (0.21.0, without the v)
# Environment:
#   GH_TOKEN  - RELEASE_TOKEN: a pull request opened with the built-in token
#               starts no CI, so it would never merge (#140)
#   GH_REPO   - owner/name
#   PUSH_URL  - where to push; default: GH_REPO on github.com with GH_TOKEN
# Run it on main. A re-run rebuilds the branch from main and finds its pull
# request again. A release already promoted needs nothing, and a promotion
# someone pushed by hand to the branch is left alone.
set -euo pipefail

version="${1:?usage: open-promote-pr.sh VERSION}"
here="$(dirname "$0")"

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "open-promote-pr: GH_TOKEN is empty. Add the repository secret RELEASE_TOKEN: a fine-grained token for this repository with Contents and Pull requests read and write (CONTRIBUTING.md, After a release)." >&2
  exit 1
fi
push_url="${PUSH_URL:-https://x-access-token:${GH_TOKEN}@github.com/${GH_REPO:?GH_REPO is empty}.git}"

bot_email='41898282+github-actions[bot]@users.noreply.github.com'
branch="docs/promote-v$version"
title="docs: promote Unreleased to v$version in CHANGELOG.md"
redact() { sed "s#${GH_TOKEN}#***#g"; }

# Run on main, where a promotion that already merged shows.
if "$here/changelog-guard.sh" "v$version" CHANGELOG.md >/dev/null 2>&1; then
  echo "CHANGELOG.md already has a heading for v$version: nothing to promote."
  exit 0
fi
# A refusal (entries the release did not ship, two releases in one section)
# fails here, before anything is pushed.
"$here/promote-release.sh" "$version"

state="$(gh pr view "$branch" --json state -q .state 2>/dev/null || true)"
if [[ "$state" == MERGED ]]; then
  echo "The $branch pull request already merged: nothing to promote."
  exit 0
fi

# The branch may hold a person's promotion (CONTRIBUTING.md names the same
# branch): leave it to them. Only the bot's own commit is replaced.
expired="RELEASE_TOKEN may have expired (CONTRIBUTING.md, After a release)"
if ! remote_sha="$(git ls-remote "$push_url" "refs/heads/$branch" 2> >(redact >&2))"; then
  echo "open-promote-pr: reading $branch failed: $expired." >&2
  exit 1
fi
remote_sha="${remote_sha%%[[:space:]]*}"
if [[ -n "$remote_sha" ]]; then
  git fetch -q "$push_url" "$remote_sha" 2> >(redact >&2)
  if [[ "$(git log -1 --format=%ae FETCH_HEAD)" != "$bot_email" ]]; then
    echo "$branch holds a promotion made by hand ($remote_sha): leaving it and its pull request alone."
    # The workflow's wait loop switches to main next: drop this run's edits.
    git checkout -q -- .
    exit 0
  fi
fi

git switch -q -C "$branch"
git -c user.name='github-actions[bot]' -c user.email="$bot_email" -c commit.gpgsign=false \
  commit -q -am "$title"

# Nothing the push prints may show the token.
if ! git push -q --force-with-lease="refs/heads/$branch:$remote_sha" "$push_url" "HEAD:refs/heads/$branch" 2>&1 | redact; then
  echo "open-promote-pr: pushing $branch failed: $expired." >&2
  exit 1
fi

if [[ "$state" != OPEN ]]; then
  gh pr create --base main --head "$branch" --title "$title" --body "Opened by the Release workflow after v$version: \`make promote\` moves the \`## Unreleased\` entries under v$version and points the goifc.org news line at the release when it has a headline. It merges itself once its checks pass; until then, the Conventional Commits Check fails on other pull requests."
fi
gh pr merge "$branch" --squash --auto
