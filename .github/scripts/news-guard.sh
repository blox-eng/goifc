#!/usr/bin/env bash
# Fails if goifc.org's news line is not the line announce-release.sh writes
# for the newest release in the changelog: a hand edit of the line, a
# headline reworded after the release, or an announce that failed. The page
# is left as it is.
#
# Usage: news-guard.sh [changelog] [page]
#   defaults: CHANGELOG.md, www/index.html
set -euo pipefail

root="$(dirname "$0")/../.."
changelog="${1:-$root/CHANGELOG.md}"
page="${2:-$root/www/index.html}"

version="$(awk '/^## v[0-9]/ { sub(/^## v/, ""); print $1; exit }' "$changelog")"
if [ -z "$version" ]; then
  echo "news-guard: no released version (a \"## vX.Y.Z\" heading) in $changelog" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cp "$page" "$tmp"
if ! "$(dirname "$0")/announce-release.sh" "$version" "$changelog" "$tmp" >&2; then
  echo "news-guard: cannot write the news line of v$version (above says why)" >&2
  exit 1
fi
if ! cmp -s "$page" "$tmp"; then
  echo "news-guard: the news line in $page does not name v$version as $changelog does." >&2
  echo "  has:  $(grep 'class="news"' "$page" | sed 's/^[ \t]*//')" >&2
  echo "  want: $(grep 'class="news"' "$tmp" | sed 's/^[ \t]*//')" >&2
  echo "Run .github/scripts/announce-release.sh $version, then commit www/index.html." >&2
  exit 1
fi
echo "OK: the news line names v$version"
