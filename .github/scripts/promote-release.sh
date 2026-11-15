#!/usr/bin/env bash
# Promotes CHANGELOG.md's "## Unreleased" section to the newest release and
# writes goifc.org's news line for it. Run it in the PR after a release,
# before adding new entries: the Release workflow commits nothing (#140),
# and changelog-guard.sh fails every PR until this is done.
#
# Usage: promote-release.sh [version] [date]   (make promote)
#   version - without the v (0.19.0); default: the newest v* tag
#   date    - YYYY-MM-DD; default: the tag's commit date in UTC
# Run from the repository root; rewrites CHANGELOG.md and www/index.html.
set -euo pipefail

here="$(dirname "$0")"
version="${1:-}"
if [[ -z "$version" ]]; then
  tag="$(git tag --list 'v*' --sort=-v:refname | head -n 1)"
  if [[ -z "$tag" ]]; then
    echo "promote-release: no v* tag to promote; run git fetch --tags" >&2
    exit 1
  fi
  version="${tag#v}"
fi
date="${2:-$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%d "v$version")}"

if "$here/changelog-guard.sh" "v$version" CHANGELOG.md >/dev/null 2>&1; then
  echo "promote-release: CHANGELOG.md already has a heading for v$version; nothing to do" >&2
  exit 1
fi

"$here/promote-changelog.sh" "$version" "$date"
echo "CHANGELOG.md: ## Unreleased is now ## v$version — $date"

if ! "$here/announce-release.sh" "$version" CHANGELOG.md www/index.html; then
  echo "www/index.html keeps the last headline. To announce v$version, add a \"- **...**\" line to its section and run .github/scripts/announce-release.sh $version." >&2
  exit 0
fi
echo "www/index.html: the news line names v$version"
