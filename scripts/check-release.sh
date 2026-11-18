#!/usr/bin/env bash
# Checks that VERSION is new and has a CHANGELOG entry.
# Usage: scripts/check-release.sh               check against the latest tag
#        scripts/check-release.sh --base <ref>  also require VERSION > VERSION on <ref> (PR target)
#        scripts/check-release.sh --notes       print the CHANGELOG sections since the latest tag
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

version=$(tr -d '[:space:]' < VERSION)
headings=$(sed -n 's/^## \[\([0-9.]*\)\].*/\1/p' CHANGELOG.md)

# True if $1 > $2 (semver).
newer() { [[ "$1" != "$2" && "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n1)" == "$1" ]]; }

latest=$(git tag --list 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n1 || true)
latest=${latest#v}

# Prints the changelog from "## [$1]" up to (not including) the next heading in $2,
# or to the end if $2 is empty.
between() {
  awk -v h="## [$1]" -v s="${2:+## [$2]}" '
    index($0, h) == 1 { on = 1; next }
    on && s != "" && index($0, s) == 1 { exit }
    on { print }
  ' CHANGELOG.md
}

if [[ "${1:-}" == "--notes" ]]; then
  # Sections are in descending order (checked below), so stop at the first
  # one that is already released.
  stop=""
  for h in $headings; do
    if [[ -n "$latest" ]] && ! newer "$h" "$latest"; then stop=$h; break; fi
  done
  between "$version" "$stop"
  exit 0
fi

fail() { echo "release check: $*" >&2; exit 1; }

[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "VERSION '$version' is not X.Y.Z"

# Already released from this commit (e.g. CI re-run on main): nothing to check.
if git tag --points-at HEAD | grep -qx "v$version"; then
  echo "release check: ok (v$version is this commit)"
  exit 0
fi

if [[ -n "$latest" ]]; then
  newer "$version" "$latest" || fail "VERSION $version must be greater than latest tag v$latest"
fi

if [[ "${1:-}" == "--base" ]]; then
  base=${2:?--base needs a ref}
  git rev-parse -q --verify "$base^{commit}" >/dev/null || fail "unknown ref '$base'"
  # A base without VERSION (first change in a new repo) counts as 0.0.0.
  if git cat-file -e "$base:VERSION" 2>/dev/null; then
    base_version=$(git show "$base:VERSION" | tr -d '[:space:]')
  else
    base_version=0.0.0
  fi
  newer "$version" "$base_version" ||
    fail "VERSION $version must be greater than $base ($base_version); rebase and renumber"
fi

[[ "$(head -n1 <<<"$headings")" == "$version" ]] ||
  fail "'## [$version]' must be the first CHANGELOG.md section"
sort -rVCu <<<"$headings" ||
  fail "CHANGELOG.md sections must be unique and in descending order"

next=$(sed -n 2p <<<"$headings")
[[ -n "$(between "$version" "$next" | tr -d '[:space:]')" ]] ||
  fail "CHANGELOG.md section '## [$version]' is empty"

echo "release check: ok ($version)"
