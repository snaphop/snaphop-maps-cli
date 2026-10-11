---
name: review-documentations
description: Audit and correct the complete SnapHop Maps CLI documentation set when asked to review all project docs, skills, ADRs or runbooks against implemented code and configuration.
---

# Review Documentation

Reconcile the full documentation set against implemented behavior, preserving accurate historical records.

## Inventory

Read [AGENTS.md](../../../AGENTS.md), [SECURITY.md](../../../SECURITY.md),
[REQUIREMENTS.md](../../../REQUIREMENTS.md), [CODE_REVIEW.md](../../../CODE_REVIEW.md),
[CONTRIBUTING.md](../../../CONTRIBUTING.md), [DESIGN.md](../../../DESIGN.md),
[architecture](../../../docs/architecture.md), [operations](../../../docs/operations.md),
[traceability](../../../docs/requirements-traceability.md) and the [decision index](../../../docs/decisions/README.md).
The GitHub repository is `snaphop/snaphop-maps-cli`; use an available connector or `gh`.
Suspected vulnerabilities follow the private reporting policy, with no public exploit details or secrets.

Read [README.md](../../../README.md), [CHANGELOG.md](../../../CHANGELOG.md), every current-state page under `docs/`
and every ADR. Inventory tracked artifacts with `git ls-files`, plus relevant new files in the working tree.
Include `CLAUDE.md`, `GEMINI.md`, review skills and their metadata and Claude links, the embedded usage skill,
`go.mod`, `Makefile`, `.github/`, `security/` policy docs, and script usage and comments. The command table and
`internal/cli/testdata/tools.json` are contract evidence. Preserve upstream `LICENSE` text.
Record the initial inventory; do not silently limit the review to `docs/`.

## Establish facts and reconcile

Check claims against Go implementation, tests, command schemas, build and release scripts, workflow configuration
and accepted ADRs. Check particularly:

- commands, arguments, required flags, defaults and tool-list parity;
- JSON stdout, stderr line format, stable codes and exit statuses, and uncertain-outcome recovery;
- credential precedence, service binding, redaction, atomic writes, locking limitations and save-before-print;
- Go version, standard-library dependencies, 100.0% coverage and the six cross-build targets;
- the local handoff gate versus separate scanner jobs and native platform tests;
- public CI with hosted runners, no secrets or write tokens, and releases owned by snaphop-build-deploy;
- usage-skill packaging versus repository review-skill discovery; and
- every test symbol in traceability, ADR index entries, and current versus superseded decisions.

Fix stale claims, broken commands or links, contradictions, ambiguous status and missing guidance supported by
current implementation. Keep accepted historical ADR context; add a superseding record for a new material decision
and keep supersession lines and the index consistent. Update affected entry points together, including the usage
skill for user-facing behavior. The CLI has no OpenAPI server, deployment console or design-system update workflow.

A documentation review does not authorize implementation, dependency, CI, release, tracker or production changes,
or marking operator-owned gates passed. Do not claim an unrun check as verified or planned behavior as implemented.

## Validate and finish

Reconcile the final inventory. Check relative Markdown links and paths with case-sensitive resolution, cited test
symbols, Claude skill links and frontmatter. Check external sources only when their authority or freshness matters.
Search for unfinished placeholders, stale names and duplicate changelog headings. Run `git diff --check` and
`make check` before handoff; run `./scripts/ci-local.sh` when the embedded usage skill or packaged content changes.

Review the final diff for secrets, generated output, license edits, unintended behavior and unrelated work.
Report reviewed artifacts, corrections, validation and unresolved evidence. Do not claim complete correctness if
any material artifact was skipped or unverifiable.

This repository-local workflow implements [ADR 0010](../../../docs/decisions/0010-adopt-repository-review-skills-and-documentation.md).
