# Changelog

Each release is a section `## MAJOR.MINOR.PATCH — date`, which the Release workflow publishes as its notes. A
section with a `### Security` subsection is a security release, and its notes say so first.

## Unreleased

### Added

- **An Agent Skill for AI assistants** (ADR 0003). `skills/snaphop-maps/SKILL.md` teaches Claude, ChatGPT and
  Codex, Gemini, Grok, Cursor and every other client that reads Agent Skills when and how to use the CLI. The
  binary embeds it: `snaphop-maps skill` prints it, `skill install --client claude|codex|gemini|grok|cursor|agents`
  installs it where that client looks, and `skill pack` writes the zip that claude.ai, ChatGPT and the model APIs
  take. Each release attaches the zip.
- `CONTRIBUTING.md`.
- **Releases publish themselves** (ADR 0002). Pushing a tag `vMAJOR.MINOR.PATCH` on `main` checks that the tag and
  this file agree, runs every check, builds each platform in its own concurrent job, attests each binary's build
  provenance, and publishes a GitHub release with the binaries, `SHA256SUMS` and this file's section as its notes.
  Running the workflow by hand with a tag rehearses that release from `main` and publishes nothing.

### Fixed

- `--view`'s help gave the opening zoom as 0 to 20. The service takes 0 to 22, and 20 is only the default
  `maxZoom`.

### Changed

- The README calls the project the SnapHop Maps CLI.

## 0.1.0 — 2026-09-30

### Added

- **`snaphop-maps`**, the SnapHop Maps command line for AI agents (ADR 0001). It has one command per MCP tool:
  `register-agent`, `create-map`, `list-maps`, `get-map`, `update-map`, `publish-map`, `withdraw-map` and
  `replace-key`. `call` reaches any tool, and `tools`, `guide` and `ping` return the server's `tools/list`,
  `initialize` and `ping`. It writes JSON on stdout, JSON errors with hints on stderr, and uses stable exit
  statuses. `schema` describes the program as JSON. Keys are kept per service in a `0600` credentials file.
- The CLI is released under the MIT license.
