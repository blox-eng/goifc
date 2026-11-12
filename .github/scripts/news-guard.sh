#!/usr/bin/env bash
# Fails if goifc.org's news line is not the line announce-release.sh writes
# for the newest release in the changelog that has a headline: a hand edit of
# the line, a headline reworded after the release, or an announce that
# failed. A release without a headline (most patch releases) keeps the line
# of the last one with a headline. The page is left as it is.
#
# Usage: news-guard.sh [changelog] [page]
#   defaults: CHANGELOG.md, www/index.html
set -euo pipefail

here="$(dirname "$0")"
root="$here/../.."
changelog="${1:-$root/CHANGELOG.md}"
page="${2:-$root/www/index.html}"

if [ "$(grep -c 'class="news"' "$page" || true)" != 1 ]; then
  echo "news-guard: $page needs exactly one news line (class=\"news\")" >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
version=""
while read -r v; do
  cp "$page" "$tmp"
  if "$here/announce-release.sh" "$v" "$changelog" "$tmp" 2>/dev/null; then
    version="$v"
    break
  fi
done < <(awk '/^## v[0-9]/ { sub(/^## v/, ""); print $1 }' "$changelog")
if [ -z "$version" ]; then
  echo "news-guard: no release in $changelog has a headline (a \"- **...**\" line), so there is no news line to check" >&2
  exit 1
fi
if ! cmp -s "$page" "$tmp"; then
  echo "news-guard: the news line in $page does not name v$version, the newest release with a headline in $changelog." >&2
  echo "  has:  $(grep 'class="news"' "$page" | sed 's/^[[:space:]]*//')" >&2
  echo "  want: $(grep 'class="news"' "$tmp" | sed 's/^[[:space:]]*//')" >&2
  echo "Run .github/scripts/announce-release.sh $version, then commit www/index.html." >&2
  exit 1
fi
echo "OK: the news line names v$version"
