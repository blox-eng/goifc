#!/usr/bin/env bash
# Fails while CHANGELOG.md has no heading for the newest release tag, or for
# the one before it. The Release workflow publishes "## Unreleased" as the
# notes and commits nothing, so the PR after a release must promote that
# section; otherwise the next release's notes would repeat this one's (#140).
# Two releases cut before a promotion leave the older one with no heading,
# so it is checked too.
#
# Usage: changelog-guard.sh [tag] [changelog]
#   tag       - check this tag only, e.g. v0.19.0; default: the two newest
#               tags that release-tags.sh lists
#   changelog - default: CHANGELOG.md at the repository root
set -euo pipefail

here="$(dirname "$0")"
changelog="${2:-}"
if [[ -z "$changelog" ]]; then
  changelog="$(git rev-parse --show-toplevel)/CHANGELOG.md"
fi
if [[ -n "${1:-}" ]]; then
  tags=("$1")
else
  mapfile -t tags < <("$here/release-tags.sh" | head -n 2)
  if [[ ${#tags[@]} -eq 0 ]]; then
    echo "OK: no release tag, nothing to check"
    exit 0
  fi
fi

for tag in "${tags[@]}"; do
  if ! awk -v head="## $tag" '$0 == head || index($0, head " ") == 1 { found = 1; exit } END { exit !found }' "$changelog"; then
    echo "changelog-guard: CHANGELOG.md has no heading for $tag, a release on this branch." >&2
    echo "Run make promote on a branch from main, then commit CHANGELOG.md and www/index.html (CONTRIBUTING.md, After a release)." >&2
    exit 1
  fi
done
echo "OK: CHANGELOG.md has a heading for ${tags[*]}"
