# Changelog

Each release is a section `## MAJOR.MINOR.PATCH — date`, which the Release workflow publishes as its notes. A
section with a `### Security` subsection is a security release, and its notes say so first.

## Unreleased

## 0.2.1 — 2026-09-30

### Security

Advisory [GHSA-4phv-xxcc-m8m6](https://github.com/snaphop/snaphop-maps-cli/security/advisories/GHSA-4phv-xxcc-m8m6)
covers the first two. Every version up to 0.2.0 is affected; upgrade to 0.2.1.

- **`skill install` no longer writes through links.** A project could plant `.claude/skills/snaphop-maps/SKILL.md`
  as a link to the credentials file, and `skill install --project` then replaced every kept key with the skill.
  The install now writes only inside the home or `--project` directory, refuses a skill directory that is a link
  or a link that leads outside, and replaces a link where a file goes. `skill pack --output` replaces a link too.
- **`$SNAPHOP_MAPS_API_KEY` is only sent to the environment's service** (ADR 0004): `$SNAPHOP_MAPS_URL`, or
  `https://maps.snaphop.ai` when that is not set. `--url` alone, such as a prompt could add to an agent's command,
  no longer sends it elsewhere; the command then uses the key kept for that service, or fails with
  `API_KEY_REQUIRED`.
- **Commands that keep keys at the same time no longer lose one.** Every writer of the credentials file now holds
  a lock, and `register-agent` and `replace-key` decide what to keep against the file as it is then; a key another
  command kept meanwhile is left in place, and the new one is printed with exit status 6. The file is also synced
  before and after it is renamed into place, so a crash cannot leave it empty.
- **Errors and warnings never show a key.** A key the command knows, from `--api-key`, `$SNAPHOP_MAPS_API_KEY` or
  the one it sent, is shown as `[REDACTED]` wherever a mistyped command line or an error body echoes it.
- Every GitHub Action is pinned to a full commit SHA, kept current by Dependabot, and `scripts/release-check.sh`
  refuses a release it cannot check against `main`.

### Fixed

- An answer that arrived with HTTP 200 but could not be read, such as one without structured content or one over
  8 MiB, was reported with `outcomeKnown: true` and "Nothing was carried out", although the tool may have run: an
  agent that repeated `create-map` could make two maps. It is now `outcomeKnown: false` with the tool's advice on
  repeating it, as is a JSON-RPC internal error.
- A JSON argument read from standard input was cut at 8 MiB and then refused as malformed. Standard input and
  `@file` are now both limited to 8 MiB and refused with `INPUT_TOO_LARGE`, and `@file` is read relative to the
  working directory a run is given.

## 0.2.0 — 2026-09-30

### Added

- **Parity with SnapHop Maps' 13 MCP tools** (its ADR 0026). New commands `get-account`, `get-installation`,
  `list-releases`, `rollback-map` (`--release N`, optionally `--expected-active N`) and `list-activity` call the
  server's new tools. The skill shows how to roll a map back, and `testdata/tools.json` is the server's tool list
  at snaphop-maps `05b98bb`.
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
