#!/usr/bin/env bash
# Records what CHANGELOG.md lists under Unreleased as the next version's section, dated in UTC, and prints the
# version. "auto" is the next minor version when Unreleased has an Added, Changed, Removed or Deprecated section,
# and the next patch otherwise; patch, minor and major bump the newest released version; MAJOR.MINOR.PATCH is
# taken as given if it is newer. Refuses when Unreleased lists nothing.
# Usage: scripts/release-cut.sh [auto|patch|minor|major|MAJOR.MINOR.PATCH] [YYYY-MM-DD]
set -euo pipefail
cd "$(dirname "$0")/.."
changelog=${CHANGELOG:-CHANGELOG.md}
want=${1:-auto}
date=${2:-$(date -u +%F)}
fail() { echo "$*" >&2; exit 1; }

[[ $date =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || fail "$date is not YYYY-MM-DD"
grep -qx '## Unreleased' "$changelog" || fail "$changelog has no '## Unreleased' heading"
unreleased=$(awk '$0 == "## Unreleased" {found = 1; next} found && /^## / {exit} found {print}' "$changelog")
grep -qE '^[[:space:]]*- ' <<<"$unreleased" || fail "$changelog lists nothing under Unreleased, so there is nothing to release"

latest=$(grep -m1 -oE '^## [0-9]+\.[0-9]+\.[0-9]+ ' "$changelog" | cut -d' ' -f2 || true)
latest=${latest:-0.0.0}
IFS=. read -r major minor patch <<<"$latest"
if [[ $want == auto ]]; then
    if grep -qE '^### (Added|Changed|Removed|Deprecated)$' <<<"$unreleased"; then want=minor; else want=patch; fi
fi
case $want in
    major) version=$((major + 1)).0.0 ;;
    minor) version=$major.$((minor + 1)).0 ;;
    patch) version=$major.$minor.$((patch + 1)) ;;
    *)
        [[ $want =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] ||
            fail "$want is not auto, patch, minor, major or MAJOR.MINOR.PATCH"
        version=$want
        [[ $version != "$latest" && $(printf '%s\n%s\n' "$latest" "$version" | sort -V | tail -1) == "$version" ]] ||
            fail "$version is not newer than $latest, the newest version in $changelog"
        ;;
esac

tmp=$(mktemp "$changelog.XXXXXX")
trap 'rm -f "$tmp"' EXIT
awk -v heading="## $version — $date" '{print} $0 == "## Unreleased" {print ""; print heading}' "$changelog" >"$tmp"
cat "$tmp" >"$changelog"
echo "$version"
