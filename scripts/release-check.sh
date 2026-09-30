#!/usr/bin/env bash
# Refuses a release whose tag and changelog disagree, or whose commit is not on main.
# Usage: scripts/release-check.sh vMAJOR.MINOR.PATCH
set -euo pipefail
cd "$(dirname "$0")/.."
tag=${1:?usage: scripts/release-check.sh vMAJOR.MINOR.PATCH}
[[ $tag =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]] || { echo "$tag is not vMAJOR.MINOR.PATCH" >&2; exit 1; }
version=${BASH_REMATCH[1]}
grep -q "^## $version " "${CHANGELOG:-CHANGELOG.md}" || { echo "CHANGELOG.md has no '## $version …' section" >&2; exit 1; }
git rev-parse -q --verify origin/main >/dev/null || { echo "origin/main is not fetched, so the tagged commit cannot be checked against it" >&2; exit 1; }
git merge-base --is-ancestor HEAD origin/main || { echo "the tagged commit is not on main" >&2; exit 1; }
echo "release $tag is consistent"
