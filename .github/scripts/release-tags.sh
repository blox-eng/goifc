#!/usr/bin/env bash
# Prints the release tags reachable from HEAD, newest first: vX.Y.Z and
# vX.Y.Z-pre only, with v1.0.0 newer than v1.0.0-rc.1. A tag on another
# branch, or one that is not a version, is not a release of this branch.
#
# Usage: release-tags.sh
set -euo pipefail

# A repository with no commit yet has no releases.
git rev-parse -q --verify HEAD >/dev/null || exit 0
git -c versionsort.suffix=- tag --merged HEAD --list 'v[0-9]*' --sort=-v:refname |
  grep -E '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$' || true
