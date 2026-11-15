#!/usr/bin/env bash
# Promotes CHANGELOG.md's "## Unreleased" section to the newest release and
# writes goifc.org's news line for it. Run it on a branch from main after a
# release, with no entries of its own: the Release workflow commits nothing
# (#140), and changelog-guard.sh fails every PR until this is done.
#
# Usage: promote-release.sh [version] [date]   (make promote)
#   version - without the v (0.19.0); default: the newest v* tag
#   date    - YYYY-MM-DD; default: the tag's commit date in UTC
# Run from the repository root; rewrites CHANGELOG.md and www/index.html.
set -euo pipefail

here="$(dirname "$0")"
mapfile -t tags < <("$here/release-tags.sh")
version="${1:-}"
if [[ -z "$version" ]]; then
  if [[ ${#tags[@]} -eq 0 ]]; then
    echo "promote-release: no release tag to promote; run git fetch --tags" >&2
    exit 1
  fi
  version="${tags[0]#v}"
fi
if ! git rev-parse -q --verify "refs/tags/v$version" >/dev/null; then
  echo "promote-release: no tag v$version here; run git fetch --tags" >&2
  exit 1
fi
date="${2:-$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%d "v$version")}"

if "$here/changelog-guard.sh" "v$version" CHANGELOG.md >/dev/null 2>&1; then
  echo "promote-release: CHANGELOG.md already has a heading for v$version, the newest tag here." >&2
  echo "If CI names a newer tag, run git fetch --tags and try again." >&2
  exit 1
fi

# Only the entries the release shipped move. An entry added since the tag,
# by this branch or by a PR that merged before the promotion, belongs to the
# next release, so refuse rather than credit it to this one.
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
git show "v$version:CHANGELOG.md" >"$tmp"
if [[ "$("$here/release-notes.sh" Unreleased "$tmp" 2>/dev/null)" != "$("$here/release-notes.sh" Unreleased CHANGELOG.md)" ]]; then
  echo "promote-release: ## Unreleased differs from v$version's, so it holds entries the release did not ship." >&2
  echo "Promote on a branch with no entries of its own: git switch -c docs/promote-v$version origin/main && make promote" >&2
  echo "If origin/main itself differs, an entry merged after the tag: add \"## v$version — $date\" by hand above the entries in git show v$version:CHANGELOG.md." >&2
  exit 1
fi

# Two releases cut before a promotion share one Unreleased; only a person can
# split them.
for i in "${!tags[@]}"; do
  if [[ "${tags[$i]}" == "v$version" && -n "${tags[$((i + 1))]:-}" ]]; then
    prev="${tags[$((i + 1))]}"
    if ! "$here/changelog-guard.sh" "$prev" CHANGELOG.md >/dev/null 2>&1; then
      echo "promote-release: CHANGELOG.md has no heading for $prev either, so ## Unreleased holds two releases." >&2
      echo "Split them by hand: add \"## v$version — $date\" above the entries git show v$version:CHANGELOG.md adds since $prev, and a $prev heading above the rest." >&2
      exit 1
    fi
  fi
done

"$here/promote-changelog.sh" "$version" "$date"
echo "CHANGELOG.md: ## Unreleased is now ## v$version — $date"

# announce-release.sh exits 2 when the release has no headline, which most
# patch releases do; any other failure is real.
status=0
"$here/announce-release.sh" "$version" CHANGELOG.md www/index.html || status=$?
if [[ $status -eq 2 ]]; then
  echo "www/index.html keeps the last headline. To announce v$version, add a \"- **...**\" line to its section and run .github/scripts/announce-release.sh $version." >&2
  exit 0
elif [[ $status -ne 0 ]]; then
  exit "$status"
fi
echo "www/index.html: the news line names v$version"
