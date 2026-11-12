#!/usr/bin/env bash
# Writes goifc.org's news line from a release's changelog section: its
# version and its headline (the first "- **...**" run), linking to the
# release. The page loads nothing from another origin, so the line is written
# here, at release time. A section without a headline, or a page without
# exactly one news line, leaves the page as it is and fails.
#
# Ported from matchblox's script of the same name, for goifc's headings
# ("## vX.Y.Z — date").
#
# Usage: announce-release.sh VERSION [changelog] [page]
#   VERSION without the v (0.4.0); defaults: CHANGELOG.md, www/index.html
set -euo pipefail

version="${1:?usage: announce-release.sh VERSION [changelog] [page]}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "announce-release: '$version' is not a version (like 0.4.0)" >&2
  exit 1
fi
root="$(dirname "$0")/../.."
changelog="${2:-$root/CHANGELOG.md}"
page="${3:-$root/www/index.html}"

headline="$(awk -v head="## v$version" '
  !on && ($0 == head || index($0, head " ") == 1) { on = 1; next }
  on && /^## / { exit }
  # The headline is the first bold run; it may wrap onto the next lines of
  # its bullet. A bullet that ends first has none.
  on && open && (/^[ \t]*$/ || /^- /) { exit }
  on && open { line = $0; sub(/^[ \t]+/, "", line); text = text " " line }
  on && !open && /^- \*\*/ { open = 1; text = substr($0, 5) }
  open && (end = index(text, "**")) { print substr(text, 1, end - 1); exit }
' "$changelog")"
headline="${headline//\`/}"
headline="${headline%.}"
if [ -z "$headline" ]; then
  echo "announce-release: no headline (a \"- **...**\" line) under v$version in $changelog; the news line is left as it is" >&2
  exit 1
fi

if [ "$(grep -c 'class="news"' "$page" || true)" != 1 ]; then
  echo "announce-release: $page needs exactly one news line (class=\"news\"); left as it is" >&2
  exit 1
fi

escaped="$(printf '%s' "$headline" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' -e "s/'/\&#39;/g")"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
NEWS="<a class=\"news\" href=\"https://github.com/blox-eng/goifc/releases/tag/v$version\"><span class=\"tag\">New</span> <span>v$version · $escaped</span> <span class=\"go\">Release notes →</span></a>" \
  awk '
    index($0, "class=\"news\"") {
      match($0, /^[ \t]*/)
      print substr($0, 1, RLENGTH) ENVIRON["NEWS"]
      next
    }
    { print }
  ' "$page" >"$tmp"
cat "$tmp" >"$page"
