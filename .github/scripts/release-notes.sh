#!/usr/bin/env bash
# Prints one CHANGELOG.md section's body, for use as a GitHub Release's notes:
# the lines under its "## " heading up to the next one, without leading or
# trailing blank lines. Prints nothing when the section is empty.
#
# Usage: release-notes.sh <section>
#   section - "Unreleased", or a released version without a leading "v"
#             (e.g. "0.15.3"), matching the "## v0.15.3 — <date>" heading
#
# Run from a repo checkout.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <section>" >&2
  exit 1
fi

file="CHANGELOG.md"
if [[ "$1" == "Unreleased" ]]; then
  heading="## Unreleased"
else
  heading="## v$1 —"
fi

awk -v heading="$heading" -v file="$file" '
  in_section && /^## / { exit }
  in_section {
    if ($0 == "") { blanks++; next }
    if (printed) { for (; blanks > 0; blanks--) print "" }
    blanks = 0
    print
    printed = 1
    next
  }
  $0 == heading || index($0, heading " ") == 1 { in_section = found = 1 }
  END {
    if (!found) {
      print "release-notes: no \"" heading "\" heading found in " file > "/dev/stderr"
      exit 1
    }
  }
' "$file"
