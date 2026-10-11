---
name: review-enhancements
description: Review and triage the complete SnapHop Maps CLI enhancement queue when asked to validate all feature requests or proposals. Classify requests and identify missing information without implementing them.
---

# Review Enhancements

Review the complete open enhancement queue with evidence; implementation is separate work.

## Establish scope and inventory

Read [AGENTS.md](../../../AGENTS.md), [SECURITY.md](../../../SECURITY.md),
[REQUIREMENTS.md](../../../REQUIREMENTS.md), [CODE_REVIEW.md](../../../CODE_REVIEW.md),
[CONTRIBUTING.md](../../../CONTRIBUTING.md), [DESIGN.md](../../../DESIGN.md),
[architecture](../../../docs/architecture.md), [operations](../../../docs/operations.md),
[traceability](../../../docs/requirements-traceability.md) and the [decision index](../../../docs/decisions/README.md).
The GitHub repository is `snaphop/snaphop-maps-cli`; use an available connector or `gh`.
Suspected vulnerabilities follow the private reporting policy, with no public exploit details or secrets.

List every open issue with the organization's Feature type or the `enhancement` label, excluding pull requests:

```bash
gh api --paginate 'repos/snaphop/snaphop-maps-cli/issues?state=open&per_page=100' \
  --jq '.[] | select(.pull_request == null) | select(.type.name == "Feature" or any(.labels[]; .name == "enhancement")) | [.number, .title] | @tsv'
```

Inspect the repository's actual taxonomy. Do not infer issue type from titles or treat a missing label as proof
there are no reports. State when access or pagination prevents a complete inventory.

Record the initial inventory, read each item's full description, comments and linked work, and reconcile the
queue again before finishing. Paginate to avoid silently skipping items. Preserve existing dirty work.
Review or triage alone does not authorize posting comments, relabeling, closing issues, merging, deleting branches
or rewriting history. Perform only externally mutating actions the user authorized; otherwise report the proposed
action and leave tracker state unchanged. Never cut a release or test against production during a review.

## Understand and classify

For each request, identify the underlying user problem separately from the proposed solution. Check current code,
tests, documented requirements and accepted decisions. Distinguish already released capability from changes only
on `main`. Identify duplicates and superseding requests without assuming their scopes match.

Determine ownership: this repository owns parsing, MCP transport, diagnostics, credential persistence and skill
distribution. SnapHop Maps owns tools, validation, authorization, publication, viewer and installation limits;
snaphop-build-deploy owns release automation. A new service operation needs its MCP tool before a named command
here. Assess compatibility of JSON output, exit statuses, flags, credential binding and embedded skill behavior.
A new runtime dependency requires an ADR. Preserve public CI and maintainer release controls.

Assign an evidence-backed outcome:

- **Valid:** a real unmet need within CLI ownership that can respect binding policies. State whether it is ready
  for prioritization or what scope and acceptance information is still missing.
- **Invalid:** already supported, duplicate, owned elsewhere, unsupported or incompatible with binding rules
  without a compatible formulation. State what could change the conclusion when applicable.
- **Needs information:** evidence cannot support either conclusion; name the specific missing facts.

Ask only material questions about the use case, affected version, example invocation, desired output, compatibility,
ownership or acceptance criteria. Do not ask for credentials, private maps, production data or a complete design.
Do not turn triage into speculative implementation.

## Record and finish

When issue updates are authorized, post a concise outcome with evidence, ownership, applicable ADRs, questions and
next step. Use existing issue types and labels; leave valid and needs-information requests open. Close invalid
requests only with closure authority, explaining the reason and linking a confirmed canonical duplicate.
Otherwise report proposed tracker changes without posting them.

Re-list both taxonomies and reconcile with the initial inventory. Report every request's number, outcome, missing
information, ownership, next action and final state. State explicitly what remains unreviewed or uncertain.
Do not promise priority, implementation or a release, or implement accepted requests during this workflow.

This repository-local workflow implements [ADR 0010](../../../docs/decisions/0010-adopt-repository-review-skills-and-documentation.md).
