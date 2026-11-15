#!/usr/bin/env bash
# Fails while CHANGELOG.md has no heading for the newest release tag. The
# Release workflow publishes "## Unreleased" as the notes and commits
# nothing, so the PR after a release must promote that section; otherwise
# the next release's notes would repeat this one's (#140).
#
# Usage: changelog-guard.sh [tag] [changelog]
#   tag       - e.g. v0.19.0; default: the newest v* tag in git, where
#               v1.0.0 is newer than v1.0.0-rc.1
#   changelog - default: CHANGELOG.md at the repository root
set -euo pipefail

tag="${1:-}"
changelog="${2:-}"
if [[ -z "$tag" ]]; then
  tag="$(git -c versionsort.suffix=- tag --list 'v*' --sort=-v:refname | head -n 1)"
  if [[ -z "$tag" ]]; then
    echo "OK: no v* tag, nothing to check"
    exit 0
  fi
fi
if [[ -z "$changelog" ]]; then
  changelog="$(git rev-parse --show-toplevel)/CHANGELOG.md"
fi

if awk -v head="## $tag" '$0 == head || index($0, head " ") == 1 { found = 1; exit } END { exit !found }' "$changelog"; then
  echo "OK: CHANGELOG.md has a heading for $tag"
  exit 0
fi
echo "changelog-guard: CHANGELOG.md has no heading for $tag, the latest release." >&2
echo "Run make promote, then commit CHANGELOG.md and www/index.html." >&2
exit 1
