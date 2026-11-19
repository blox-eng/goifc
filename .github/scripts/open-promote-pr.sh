#!/usr/bin/env bash
# Opens the pull request that promotes CHANGELOG.md and the goifc.org news
# line to a new release, and asks GitHub to merge it once its checks pass.
# The Release workflow runs it on the released commit, after the release.
#
# Usage: open-promote-pr.sh VERSION   (0.21.0, without the v)
# Environment:
#   GH_TOKEN  - RELEASE_TOKEN: a pull request opened with the built-in token
#               starts no CI, so it would never merge (#140)
#   GH_REPO   - owner/name
#   PUSH_URL  - where to push; default: GH_REPO on github.com with GH_TOKEN
# A re-run finds its pull request again. A release already promoted needs
# nothing.
set -euo pipefail

version="${1:?usage: open-promote-pr.sh VERSION}"
here="$(dirname "$0")"

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "open-promote-pr: GH_TOKEN is empty. Add the repository secret RELEASE_TOKEN: a fine-grained token for this repository with Contents and Pull requests read and write (CONTRIBUTING.md, After a release)." >&2
  exit 1
fi
push_url="${PUSH_URL:-https://x-access-token:${GH_TOKEN}@github.com/${GH_REPO:?GH_REPO is empty}.git}"

if "$here/changelog-guard.sh" "v$version" CHANGELOG.md >/dev/null 2>&1; then
  echo "CHANGELOG.md already has a heading for v$version: nothing to promote."
  exit 0
fi

"$here/promote-release.sh" "$version"

branch="docs/promote-v$version"
title="docs: promote Unreleased to v$version in CHANGELOG.md"
git switch -q -c "$branch"
git -c user.name='github-actions[bot]' \
  -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
  -c commit.gpgsign=false \
  commit -q -am "$title"

# A re-run pushes the same change again. Nothing the push prints may show the
# token.
if ! git push -q --force "$push_url" "HEAD:refs/heads/$branch" 2>&1 | sed "s#${GH_TOKEN}#***#g"; then
  echo "open-promote-pr: pushing $branch failed: RELEASE_TOKEN may have expired (CONTRIBUTING.md, After a release)." >&2
  exit 1
fi

if ! gh pr view "$branch" >/dev/null 2>&1; then
  gh pr create --base main --head "$branch" --title "$title" --body "Opened by the Release workflow after v$version: \`make promote\` moves the \`## Unreleased\` entries under v$version and points the goifc.org news line at the release when it has a headline. It merges itself once its checks pass; until then, the Conventional Commits Check fails on other pull requests."
fi
gh pr merge "$branch" --squash --auto
