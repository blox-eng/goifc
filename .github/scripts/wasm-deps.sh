#!/usr/bin/env bash
# wasm-deps.sh
# Fails when either wasm command pulls in a package that does not belong in a
# browser build: regexp, os/exec, text/template, html/template, net or any
# net/ subpackage (net/http, ...). That is the check that keeps the file size
# honest; wasm-size.sh only measures growth.
set -euo pipefail

# Pulled in by github.com/qmuntal/gltf; pure parsers, not the network stack.
allowed_net=(net/url net/netip)
cmds=(./cmd/goifc-build ./cmd/goifc-read)

deps="$(CGO_ENABLED=0 GOOS=js GOARCH=wasm go list -deps "${cmds[@]}")"

status=0
while IFS= read -r pkg; do
  case "$pkg" in
    regexp | os/exec | text/template | html/template | net | net/*) ;;
    *) continue ;;
  esac
  for ok in "${allowed_net[@]}"; do
    [[ "$pkg" == "$ok" ]] && continue 2
  done
  echo "::error::${pkg} is a dependency of the wasm build. Find who imports it with: CGO_ENABLED=0 GOOS=js GOARCH=wasm go list -deps -f '{{.ImportPath}}: {{join .Imports \" \"}}' ${cmds[*]} | grep -E ' ${pkg}( |\$)'" >&2
  status=1
done <<<"$deps"
exit "$status"
