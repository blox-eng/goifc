#!/usr/bin/env bash
# Promotes the CHANGELOG.md "## Unreleased" section to a released version
# heading, and inserts a fresh empty "## Unreleased" above it.
#
# Usage: promote-changelog.sh <version> <date>
#   version - released version, without a leading "v" (e.g. "0.7.0")
#   date    - release date as YYYY-MM-DD (e.g. "2026-08-17")
#
# Run from a repo checkout; rewrites CHANGELOG.md in place.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <version> <date>" >&2
  exit 1
fi

version="$1"
date="$2"
file="CHANGELOG.md"

if [[ ! -f "$file" ]]; then
  echo "promote-changelog: $file not found" >&2
  exit 1
fi

if ! grep -qx "## Unreleased" "$file"; then
  echo "promote-changelog: no '## Unreleased' heading found in $file — refusing to guess" >&2
  exit 1
fi

# Em dash (U+2014), matching the heading format already used in the file
# (e.g. "## v0.2.0 — 2026-08-13").
heading="## v${version} — ${date}"

awk -v heading="$heading" '
  found == 0 && $0 == "## Unreleased" {
    print
    print ""
    print heading
    found = 1
    next
  }
  { print }
' "$file" > "${file}.tmp"

mv "${file}.tmp" "$file"
