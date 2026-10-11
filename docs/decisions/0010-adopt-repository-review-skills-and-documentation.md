# 0010 — Adopt repository review skills and documentation

Accepted 2026-10-11.

## Context

SnapHop Maps has a shared project-documentation structure and repository-local workflows for whole bug,
enhancement, PR and documentation queues. The CLI had its usage skill and core policies, but lacked those
review workflows and the supporting current-state documents.

## Decision

Carry over the four review skills as CLI-specific workflows under `.agents/skills/`, with Claude discovery links
under `.claude/skills/` and interface metadata. Use the CLI's GitHub repository, Go packages, fake-service tests,
100.0% statement floor, public CI and centrally owned release workflow. Review alone does not authorize tracker
mutation, merging, branch deletion, publication or production work.

Adopt `REQUIREMENTS.md`, `DESIGN.md`, `CODE_REVIEW.md`, architecture, configuration, operations, verification,
embedding guidance, requirements traceability and an ADR index. Link them from the existing entry points.
`DESIGN.md` describes the command-line interface. The service owns OpenAPI, browser design and rendering;
keep its contracts there rather than copying them into this client. Do not add `update-design-system`.

## Consequences

Developers and agents can discover the same categories of guidance in Maps and its CLI, with each repository's
actual boundaries. The new development skills are separate from `skills/snaphop-maps/SKILL.md`, which remains the
embedded, installed and released usage skill. Keep the documents and test references current with future changes.
Existing credential, coverage, CI and release policies remain binding.
