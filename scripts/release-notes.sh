#!/usr/bin/env bash
# Prints a version's CHANGELOG section as release notes, opening with whether it fixes a vulnerability,
# and closing with how to install the release and verify what was downloaded.
# Usage: scripts/release-notes.sh vMAJOR.MINOR.PATCH
set -euo pipefail
cd "$(dirname "$0")/.."
version=${1:?usage: scripts/release-notes.sh vMAJOR.MINOR.PATCH}
version=${version#v}
section=$(awk -v heading="## $version " 'index($0, heading) == 1 {found = 1; next} found && /^## / {exit} found {print}' "${CHANGELOG:-CHANGELOG.md}")
[[ -n ${section//[[:space:]]/} ]] || { echo "CHANGELOG.md has no section for $version" >&2; exit 1; }
if grep -q '^### Security' <<<"$section"; then
    echo "**Security release: update now.**"
else
    echo "No security fixes."
fi
echo
echo "$section" | sed '/./,$!d'
cat <<NOTES

### Install

\`\`\`sh
go install github.com/snaphop/snaphop-maps-cli/cmd/snaphop-maps@v$version
\`\`\`

Or download the binary for your platform below. Each was built from the tagged commit; check the download
before running it:

\`\`\`sh
sha256sum -c SHA256SUMS --ignore-missing
\`\`\`

\`snaphop-maps-skill.zip\` is the Agent Skill that teaches an AI assistant to use the CLI. Upload it to claude.ai,
ChatGPT or a model API. With the CLI installed, \`snaphop-maps skill install --client claude|codex|gemini|grok|cursor\`
puts it where a local assistant looks.
NOTES
