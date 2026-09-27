#!/usr/bin/env bash
# Verifies every link and asset reference in the landing site resolves.
#
# The documentation site has `mkdocs build --strict`, which turns a broken
# internal link into a failed build rather than a 404 someone finds later. The
# landing page is hand-written HTML with no build step, so it would have no
# equivalent — this is the equivalent. It is the Cloudflare Pages build command
# and a CI job, so a bad reference fails before it is served.
#
# Checked: relative hrefs and srcs resolve to a file, in-page fragments resolve
# to an id on that page, and url() references in the stylesheets resolve.
# External URLs are not fetched: a link checker that reaches the network fails
# when someone else's site has a bad afternoon, which trains people to ignore it.
#
# Usage: check-links.sh <site-dir>
set -euo pipefail

ROOT=${1:?usage: check-links.sh <site-dir>}
[ -d "$ROOT" ] || { echo "check-links: $ROOT is not a directory" >&2; exit 2; }

fail=0
problem() { echo "  BROKEN  $1" >&2; fail=1; }

# Documentation lives on the docs subdomain. A link to one of these paths on the
# apex would be caught by the redirects and still work, which is exactly why it
# has to fail here instead: a redirect that is never exercised rots unnoticed.
DOC_PREFIXES='docs|latest|dev'

for page in "$ROOT"/*.html; do
  [ -e "$page" ] || continue
  echo "$page"

  # Every id on the page, for resolving fragments.
  #
  # `|| true` on both greps: under `set -e` a grep that matches nothing exits 1
  # and takes the script with it. A page carrying no id at all is perfectly
  # valid — 404.html has none — and a page with no href or src would be too,
  # so neither is an error. Without this the checker dies on such a page having
  # printed only its name, which reads as a broken reference it cannot name.
  ids=$(grep -oE 'id="[^"]+"' "$page" | sed 's/id="//; s/"$//' | sort -u || true)

  # srcset carries the dark-mode mark, and a <picture> silently falls back to
  # its <img> when the source 404s — exactly the kind of break nobody reports.
  refs=$(grep -oE '(href|src|srcset)="[^"]*"' "$page" |
    sed 's/^[a-z]*="//; s/"$//' | tr ',' '\n' |
    sed -E 's/^[[:space:]]+//; s/[[:space:]]+[0-9.]+[xw]$//' | sort -u || true)

  while IFS= read -r ref; do
    [ -n "$ref" ] || continue

    case $ref in
      mailto:*|tel:*|data:*) continue ;;
      "https://goifc.org/"*)
        if [[ ${ref#https://goifc.org/} =~ ^($DOC_PREFIXES)(/|$) ]]; then
          problem "$ref — documentation lives on docs.goifc.org"
        fi
        continue ;;
      http://*|https://*|//*) continue ;;
      "#"*)
        frag=${ref#\#}
        grep -qxF "$frag" <<<"$ids" || problem "$ref — no element with that id"
        continue ;;
      "/")
        [ -f "$ROOT/index.html" ] || problem "/ — no index.html"
        continue ;;
      /*)
        target="$ROOT$ref" ;;
      *)
        target="$ROOT/$ref" ;;
    esac

    frag=""
    case $ref in *\#*) frag=${ref#*\#} ;; esac
    target=${target%%#*}
    target=${target%%\?*}

    if [ -d "$target" ]; then
      target="$target/index.html"
      [ -f "$target" ] || { problem "$ref — directory has no index.html"; continue; }
    elif [ ! -f "$target" ]; then
      problem "$ref — no such file"
      continue
    fi

    # A fragment on another page in this site is as checkable as one on this
    # page, so it is checked the same way. Today the site is one page and every
    # such reference takes the "#..." arm above; this is what keeps the
    # guarantee true on the day a second page is added.
    if [ -n "$frag" ] && [ "${target%.html}" != "$target" ]; then
      grep -oE 'id="[^"]+"' "$target" | sed 's/id="//; s/"$//' |
        grep -qxF "$frag" ||
        problem "$ref — $(basename "$target") has no element with that id"
    fi
  done <<<"$refs"
done

# Stylesheet references: fonts and images loaded by CSS 404 silently in a way a
# page never reports, which is the failure this script exists to prevent.
for sheet in "$ROOT"/*.css; do
  [ -e "$sheet" ] || continue
  echo "$sheet"
  urls=$(grep -oE "url\('[^']*'\)|url\(\"[^\"]*\"\)|url\([^'\")][^)]*\)" "$sheet" |
    sed -E "s/^url\(['\"]?//; s/['\"]?\)$//" | sort -u)
  while IFS= read -r url; do
    [ -n "$url" ] || continue
    case $url in
      http://*|https://*|//*|data:*) continue ;;
    esac
    [ -f "$ROOT/${url#/}" ] || problem "$url (in $(basename "$sheet")) — no such file"
  done <<<"$urls"
done

if [ "$fail" -ne 0 ]; then
  echo "check-links: broken references, see above" >&2
  exit 1
fi
echo "check-links: all references resolve"
