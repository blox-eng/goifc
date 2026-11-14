#!/usr/bin/env bash
# wasm-size.sh GATED_FILE MAX_GZIP_BYTES [OTHER_FILE...]
# Prints the raw and gzip -9 size of every file, and fails when GATED_FILE
# is over MAX_GZIP_BYTES after gzip -9.
set -euo pipefail
gated="$1" max="$2"
shift 2
for f in "$gated" "$@"; do
  printf '%-28s raw %10d B  gzip -9 %10d B\n' "$(basename "$f")" "$(wc -c <"$f")" "$(gzip -9c "$f" | wc -c)"
done
size="$(gzip -9c "$gated" | wc -c)"
if (( size > max )); then
  echo "::error::$(basename "$gated") is ${size} bytes after gzip -9, over the ${max}-byte limit. Find what grew with: go tool nm -size -sort size $gated | head" >&2
  exit 1
fi
