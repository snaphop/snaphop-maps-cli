# GEMINI.md

`AGENTS.md` is the canonical source of truth for the SnapHop Maps CLI.

Before making changes:

1. Read `AGENTS.md` for the invariants, the build, and the coverage and testing rules.
2. Follow `SECURITY.md` for the binding boundaries on keys, the credentials file and CI.
3. Read `REQUIREMENTS.md`, `docs/architecture.md` and `DESIGN.md` for the CLI contract, and the ADRs indexed in
   `docs/decisions/README.md` before changing how the CLI reaches the service.
4. Follow `CODE_REVIEW.md` for reviews and `docs/operations.md` before release work.
5. Use the matching skill under `.agents/skills/` for a whole bug, enhancement, PR or documentation queue.

If this file conflicts with `AGENTS.md`, `AGENTS.md` wins. There are currently no Gemini-CLI-specific overrides.
