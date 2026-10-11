---
name: review-bugs
description: Review and resolve the complete SnapHop Maps CLI bug queue when asked to review, triage, validate or fix all bug reports. Investigate reports, fix valid defects when authorized, and recommend resolutions with evidence.
---

# Review Bugs

Resolve the complete open bug queue with evidence, preserving CLI contracts and security boundaries.

## Establish scope and inventory

Read [AGENTS.md](../../../AGENTS.md), [SECURITY.md](../../../SECURITY.md),
[REQUIREMENTS.md](../../../REQUIREMENTS.md), [CODE_REVIEW.md](../../../CODE_REVIEW.md),
[CONTRIBUTING.md](../../../CONTRIBUTING.md), [DESIGN.md](../../../DESIGN.md),
[architecture](../../../docs/architecture.md), [operations](../../../docs/operations.md),
[traceability](../../../docs/requirements-traceability.md) and the [decision index](../../../docs/decisions/README.md).
The GitHub repository is `snaphop/snaphop-maps-cli`; use an available connector or `gh`.
Suspected vulnerabilities follow the private reporting policy, with no public exploit details or secrets.

List every open issue with the organization's Bug type or the `bug` label, excluding pull requests:

```bash
gh api --paginate 'repos/snaphop/snaphop-maps-cli/issues?state=open&per_page=100' \
  --jq '.[] | select(.pull_request == null) | select(.type.name == "Bug" or any(.labels[]; .name == "bug")) | [.number, .title] | @tsv'
```

Inspect the repository's actual taxonomy. Do not infer issue type from titles or treat a missing label as proof
there are no reports. State when access or pagination prevents a complete inventory.

Record the initial inventory, read each item's full description, comments and linked work, and reconcile the
queue again before finishing. Paginate to avoid silently skipping items. Preserve existing dirty work.
Review or triage alone does not authorize posting comments, relabeling, closing issues, merging, deleting branches
or rewriting history. Perform only externally mutating actions the user authorized; otherwise report the proposed
action and leave tracker state unchanged. Never cut a release or test against production during a review.

## Validate each report

Trace current code and tests, compare the reported version with current `main` and released behavior, and reproduce
with the smallest faithful fake-service test. Use a local plane only when actual service evidence is needed.
Check the requirements, configuration defaults and accepted ADRs before deciding behavior is defective.

Separate CLI parsing, transport, key selection, storage and output defects from service validation, publication,
viewer, account or delivery defects. The CLI must not implement a service workaround that breaks MCP parity.
A fix on `main` may still need a CLI release; a viewer defect belongs in snaphop-maps and may need republication.

Classify each report as **valid**, **invalid** (intended, unsupported, already resolved or duplicate), or **blocked**
(missing evidence or authority). Configuration can reveal a real usability defect. Cite concrete tests, code,
configuration or ADRs; missing information is not evidence of invalidity.

## Fix and resolve

When fixing is authorized, implement the smallest complete change in the owning package. Add parallel regression
coverage against `internal/cli/harness_test.go`, including refusal paths. Do not mutate package state except in
`cmd/snaphop-maps/main_test.go`. Update the command table and local tool fixture for changed service tools;
keep the embedded usage skill in step with interface changes. Update changelog, affected docs and traceability,
and record material decisions. The standard-library rule and 100.0% statement floor remain binding.

Run focused checks, then `./scripts/ci-local.sh`; inspect the separate security checks described in
[verification](../../../docs/verification.md). Never bypass a failed gate. Review the final diff for secrets,
unrelated changes and accidental semantic changes. Submit or land fixes only within authorized scope.

For an invalid report, explain the decision, evidence, relevant contract and useful next step. Confirm a duplicate
matches the canonical issue's trigger and impact. Post explanations and close only when authorized. Do not call a
valid issue fixed until its fix is on `main`; report a local or unlanded fix as such.

## Finish

Re-list both bug taxonomies. Report every issue's number, validity, evidence, action, checks and final tracker state.
Name unresolved issues and missing evidence. Do not claim all bugs resolved when a fix is only local, a check failed
or any report was skipped.

This repository-local workflow implements [ADR 0010](../../../docs/decisions/0010-adopt-repository-review-skills-and-documentation.md).
