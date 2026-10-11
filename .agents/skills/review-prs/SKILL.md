---
name: review-prs
description: Review and process the complete SnapHop Maps CLI pull request queue when asked to review, test or resolve open PRs. Apply CODE_REVIEW.md, resolve authorized conflicts, verify locally, and merge or clean up only when authorized.
---

# Review Pull Requests

Review each open PR independently with evidence and the repository's CLI, security and release rules.

## Establish scope and inventory

Read [AGENTS.md](../../../AGENTS.md), [SECURITY.md](../../../SECURITY.md),
[REQUIREMENTS.md](../../../REQUIREMENTS.md), [CODE_REVIEW.md](../../../CODE_REVIEW.md),
[CONTRIBUTING.md](../../../CONTRIBUTING.md), [DESIGN.md](../../../DESIGN.md),
[architecture](../../../docs/architecture.md), [operations](../../../docs/operations.md),
[traceability](../../../docs/requirements-traceability.md) and the [decision index](../../../docs/decisions/README.md).
The GitHub repository is `snaphop/snaphop-maps-cli`; use an available connector or `gh`.
Suspected vulnerabilities follow the private reporting policy, with no public exploit details or secrets.

List all open PRs, including Dependabot, with pagination (`gh pr list --state open --limit 1000`, or the paginated
API for a larger queue). Inspect `.github/dependabot.yml` for the dependency ecosystems actually configured here.
Record the initial inventory, read each item's full description, comments and linked work, and reconcile the
queue again before finishing. Paginate to avoid silently skipping items. Preserve existing dirty work.
Review or triage alone does not authorize posting comments, relabeling, closing issues, merging, deleting branches
or rewriting history. Perform only externally mutating actions the user authorized; otherwise report the proposed
action and leave tracker state unchanged. Never cut a release or test against production during a review.

## Review and resolve

For each PR, read the description, diff, linked issues, acceptance criteria and rationale. Verify findings against
[CODE_REVIEW.md](../../../CODE_REVIEW.md), in its order: keys and transport, credential integrity, MCP parity,
agent recovery, hostile input, package structure, tests and documentation, public CI and releases, diff hygiene.
No Maven, database or browser checks belong in this Go client's handoff gate.

Check mergeability against current `main`. For authorized conflict resolution, use an isolated worktree or suitable
local branch, fetch current `main`, merge it into the working branch and preserve both changes' intent. Rebase only
with explicit history-rewrite authority. Preserve existing user work and combine compatible changelog and
traceability entries. Check the resolved diff as a new state needing review and validation.

## Verify

Run focused checks first, then `./scripts/ci-local.sh` on the resolved branch or candidate merged state. It must
pass formatting, vet, the static build, randomized race tests, every compiled statement covered, all six platform
builds, the usage skill zip, checksums, govulncheck and diff whitespace. Check separate scanner policy and CI results
as [verification](../../../docs/verification.md) describes. A local Linux run does not execute native macOS or
Windows lock tests. Use the local plane for necessary actual MCP checks, never production.

Fix failures within authorized scope, with regression coverage; never skip or weaken a check. Update the embedded
usage skill when an interface changes, and affected requirements, traceability, design, configuration, operations,
ADR index and changelog. Avoid unrequested implementation changes during a review-only task.

## Merge and clean up

With merge authority and passing review and checks, use `gh pr merge NUMBER --squash`, with a result-focused title
and issue references in the body. Verify the fetched remote `main` contains the change and GitHub Verify passes.
Without merge authority, report readiness and leave the PR open. Do not merge past failed required checks.

Delete merged remote and local branches only with explicit cleanup authority. Remove temporary worktrees created
for the review without discarding user changes. Merging does not cut a release, deploy or update the umbrella
repository's gitlink; each remains separate work with its own authorization.

## Finish

Re-list open PRs and reconcile the initial set. Report each PR's findings, resolution, conflicts, checks, platform
or scanner evidence gaps and final state. State why any PR remains open and confirm the final working tree state.

This repository-local workflow implements [ADR 0010](../../../docs/decisions/0010-adopt-repository-review-skills-and-documentation.md).
