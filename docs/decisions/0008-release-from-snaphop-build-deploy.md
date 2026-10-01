# 0008 — Release from snaphop-build-deploy, with one press

Accepted 2026-10-01. Supersedes [0007](0007-release-from-a-maintainers-agent.md).

## Context

Under 0007 no workflow released: a maintainer's coding agent ran each step on the maintainer's machine. That kept
write access out of this public repository's workflows, which still holds, but made every release a manual session
again. The maintainer wants a release to be one press, as under 0006, without giving a workflow here write access.

## Decision

**The Maps CLI Release workflow in SnapHop's private snaphop-build-deploy repository cuts and publishes each
release** (its ADR 27). A maintainer presses it on that repository's `main` when they want a release. It runs this
repository's own scripts, in the order [CONTRIBUTING.md](../../CONTRIBUTING.md) gives:

- `scripts/release-cut.sh` records what `CHANGELOG.md` lists under Unreleased as the next version, `auto` by default,
  and `./scripts/ci-local.sh` passes on the result. A dry run stops here.
- The release commit and the tag `vX.Y.Z` are pushed to `main` in one atomic push, from the commit that was checked.
- `scripts/release-check.sh` accepts the tag, and `make -j dist VERSION=vX.Y.Z` builds every platform, the Agent
  Skill and `SHA256SUMS` from it.
- `gh release create vX.Y.Z --verify-tag` publishes what was built, with `scripts/release-notes.sh` as the notes.

Every workflow in this repository still runs with a read-only token and no secret. The module's Go code runs only in
that workflow's jobs that hold no credentials; the jobs that push and publish run none of it. Every job runs on a
fresh GitHub-hosted VM, as this repository's own Verify does, so nothing one job runs outlives it.

## Consequences

- Releases still carry no build provenance attestation: one minted in a private repository's workflow names an
  identity a user of this public repository cannot inspect. `SHA256SUMS` shows a download is intact, and a
  reproducible `make dist` or `go install …@vX.Y.Z` shows a binary matches the source.
- Pushing the release commit and tag and publishing the release use a token of snaphop-build-deploy's that may
  write to this repository. Whoever can change that repository's `main` can change how a release is cut.
- Whether to release stays a maintainer's decision: the workflow only runs when pressed.
- The steps stay in CONTRIBUTING.md, so a maintainer can release by hand, as under 0007, when the workflow cannot
  run.
