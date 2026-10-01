# 0007 — Release from a maintainer's coding agent

Accepted 2026-10-01. Supersedes [0002](0002-release-from-a-tag-with-provenance.md) and
[0006](0006-cut-a-release-with-one-click.md).

## Context

Under 0002 and 0006 the Release workflow cut, built, attested and published every release. To do that in this
public repository, two of its jobs held write permissions: one could push to `main` and create `v*` tags, the
other could publish releases and mint attestations. The maintainer does not want any workflow in a public
repository to hold write access, and releases are now cut by the coding agent (such as Claude Code) the
maintainer already works with.

## Decision

**No workflow releases. A coding agent cuts and publishes a release on a maintainer's machine, when the
maintainer asks for one.** Every workflow in the repository runs with a read-only token.

- **The version.** `scripts/release-cut.sh` still records what `CHANGELOG.md` lists under Unreleased as the next
  version's section, `auto` by default, and refuses when Unreleased lists nothing.
- **Checks first.** `./scripts/ci-local.sh` runs on the release commit before anything is pushed.
- **From `main`.** The release commit is pushed to `main`, then tagged `vX.Y.Z`. `scripts/release-check.sh` refuses
  a tag that disagrees with the changelog or whose commit is not on `origin/main`, before the tag is pushed.
- **Build from the tag.** `make -j dist VERSION=vX.Y.Z` builds every platform and the Agent Skill from the tagged
  commit, in a clean checkout, with `SHA256SUMS` beside them.
- **Notes.** `scripts/release-notes.sh` prints the version's changelog section as the notes, as before.
- **Publish.** `gh release create vX.Y.Z --verify-tag` publishes the files in `dist/`, with the maintainer's own
  credentials. The agent never stores them in the repository or prints them.

## Consequences

- Releases no longer carry a build provenance attestation: attestations are minted from a workflow's identity, and
  no workflow builds the binaries now. `SHA256SUMS` shows a download is intact, not where it was built. Anyone who
  needs to know that the binary matches the source can build the tag with `go install …@vX.Y.Z` or `make dist`.
- Builds are reproducible from the tag with the Go version in `go.mod` (`-trimpath`, no cgo, the version stamped
  from the tag), so anyone can rebuild a published binary and compare its checksum.
- The release is only as trustworthy as the maintainer's machine and GitHub credentials. The ruleset on `v*` tags
  stays: only maintainers may create, move or delete them.
- Whether to release stays a maintainer's decision. An agent cuts a release only when asked to, never as part of a
  routine change.
- v0.2.0 to v0.2.2, which the workflow built, keep their attestations, and `gh attestation verify` still checks them.
