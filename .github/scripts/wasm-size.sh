#!/usr/bin/env bash
# wasm-size.sh FILE [BASE_FILE MAX_GROWTH_BYTES]
# Prints the raw and gzip -9 size of FILE (and BASE_FILE). With a base, also
# prints the gzip growth and fails when it is over MAX_GROWTH_BYTES, unless
# WASM_GROWTH_OK=1. Appends a table to GITHUB_STEP_SUMMARY when it is set.
set -euo pipefail

if (( $# != 1 && $# != 3 )); then
  echo "usage: wasm-size.sh FILE [BASE_FILE MAX_GROWTH_BYTES]" >&2
  exit 2
fi
file="$1" base="${2:-}" max="${3:-}"

gzsize() { gzip -9c "$1" | wc -c; }
line() { printf '%-28s raw %10d B  gzip -9 %10d B\n' "$(basename "$1")" "$(wc -c <"$1")" "$(gzsize "$1")"; }

size="$(gzsize "$file")"
line "$file"
growth=""
if [[ -n "$base" ]]; then
  line "$base"
  growth=$((size - $(gzsize "$base")))
  printf 'gzip growth over base: %+d B (budget %d B)\n' "$growth" "$max"
fi

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo '| file | raw (B) | gzip -9 (B) | growth (B) |'
    echo '| --- | ---: | ---: | ---: |'
    printf '| %s | %d | %d | %s |\n' "$(basename "$file")" "$(wc -c <"$file")" "$size" \
      "${growth:+$(printf '%+d' "$growth")}"
  } >>"$GITHUB_STEP_SUMMARY"
fi

if [[ -n "$growth" ]] && (( growth > max )); then
  if [[ "${WASM_GROWTH_OK:-0}" == 1 ]]; then
    echo "wasm growth ${growth} B is over the ${max} B budget; waived by WASM_GROWTH_OK=1"
    exit 0
  fi
  echo "::error::$(basename "$file") grew ${growth} bytes after gzip -9, over the ${max}-byte budget. Find what grew with: go tool nm -size -sort size $file | head (and the same for $base). If the growth is intended, add the PR label wasm-growth to waive this check." >&2
  exit 1
fi
