# 0002 — Release from a tag, with build provenance

Accepted 2026-09-30. Amends "Publishing releases" under "Left out" in
[0001](0001-call-the-mcp-server-for-parity.md).

## Context

v0.1.0 was built on a maintainer's machine and uploaded by hand. Nothing tied its binaries to the tagged source,
so someone who downloaded one could check it against `SHA256SUMS`, but not where it came from. snaphop-api-console
already publishes its releases from a tag, with a workflow (its ADR 0003).

## Decision

**Pushing a tag `vMAJOR.MINOR.PATCH` on `main` publishes the release, from CI alone.**

- **Agreement first.** `scripts/release-check.sh` refuses a tag that is not `vMAJOR.MINOR.PATCH`, has no
  `## MAJOR.MINOR.PATCH — date` section in `CHANGELOG.md`, or whose commit is not on `main`. `make check` and
  `govulncheck` then run as they do in Verify.
- **Concurrent builds.** Each of the six platforms builds in its own job with `make dist/…`, the same target
  as a local `make -j dist`, stamped with the tag's version.
- **Provenance.** The publishing job attests every binary with `actions/attest-build-provenance`, a Sigstore-signed
  statement of the workflow, commit and runner that built it. `gh attestation verify` checks it.
- **Notes.** `scripts/release-notes.sh` prints the version's changelog section. The notes open by saying whether it
  is a security release and close with how to install and verify the binaries.
- **Least privilege.** Only the publishing job holds `contents: write`, `id-token: write` and
  `attestations: write`, and it runs only for a pushed tag. Every job runs on a GitHub-hosted runner.
- **Rehearsal.** Running the workflow by hand with a tag runs the checks and builds from `main`, and skips the
  attestation and the release.

## Consequences

- Cutting a release is: add the version's changelog section on `main`, then push the tag. The workflow refuses
  anything else.
- The umbrella stack keeps pinning whichever commit it has reviewed. A release does not move that pointer.
