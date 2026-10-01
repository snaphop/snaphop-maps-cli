# 0006 — Cut a release with one click

Accepted 2026-10-01. Amends [0002](0002-release-from-a-tag-with-provenance.md). Superseded by
[0007](0007-release-from-a-maintainers-agent.md): the Release workflow is removed, and a maintainer's coding agent
cuts and publishes each release.

## Context

Under 0002 a maintainer released by hand: move `## Unreleased` into a dated version section, commit it to `main`,
create the tag and push it. Each step was a chance to get it wrong: a rehearsal of 0.2.2 failed because it
was given `0.2.2` rather than `v0.2.2`. The maintainer wants a release to be one button.

## Decision

**Running the Release workflow on `main` cuts the release.** "Run workflow" takes a `version`, `auto` by default,
and a `dry-run` box.

- **The version.** `scripts/release-cut.sh` records what `CHANGELOG.md` lists under Unreleased as the next
  version's section, dated in UTC. `auto` is the next minor version when Unreleased has an Added, Changed, Removed
  or Deprecated section, and the next patch otherwise. `patch`, `minor`, `major` or an exact newer version
  override it. It refuses when Unreleased lists nothing, so a second click releases nothing twice.
- **Checks first.** A read-only job records the section in its checkout, refuses a version already tagged, writes
  the release notes to the run's summary, and runs `make check` and `govulncheck`. Nothing is pushed unless they
  pass.
- **One atomic push.** A second job records the same section, commits it with the person who clicked as its
  author and `github-actions[bot]` as committer, tags it `vX.Y.Z`, and pushes the commit to `main` and the tag
  together with `git push --atomic`. If `main` moved since the click, the push is refused and nothing lands.
- **The release runs from the tag.** A tag pushed with the workflow's token starts no workflow, so the job
  dispatches the Release workflow on the tag. That run is the one 0002 describes: the tagged commit's workflow,
  `release-check.sh`, every check, the builds, the attestations and the release. Provenance records the tag and its
  commit, as for a tag pushed by hand.
- **Least privilege.** The job that pushes holds `contents: write` and `actions: write`, runs only on `main`, and
  runs the repository's shell scripts but none of the module's code. The publishing job is unchanged.
- **Dry run.** Ticking `dry-run` runs the first job from any branch and stops there. It replaces 0002's rehearsal,
  whose builds Verify already runs for every commit on `main`.

Pushing a tag by hand still releases, as 0002 describes.

## Consequences

- Releasing is one click on `main`; the changelog's Unreleased list is what decides that a release has something
  in it and, with `auto`, how large it is. Whether to release stays a maintainer's decision: only someone with
  write access can run the workflow.
- The workflow's token can now push to `main` and create `v*` tags. A ruleset that protects `main` or `v*` tags
  must let GitHub Actions through, or the push is refused and nothing is released.
- The release commit's committer is `github-actions[bot]`, and like the tag it is not signed.
