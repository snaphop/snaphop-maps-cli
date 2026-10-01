# Changelog

Each release is a section `## MAJOR.MINOR.PATCH — date`, which `scripts/release-notes.sh` turns into its notes. A
section with a `### Security` subsection is a security release, and its notes say so first.

## Unreleased

## 0.3.2 — 2026-10-01

### Changed

- **A release is one press again** (ADR 0008). SnapHop's private snaphop-build-deploy repository cuts, tags, builds
  and publishes each release with this repository's own scripts and `make -j dist`. No workflow in this repository
  holds write permissions, and releases still carry `SHA256SUMS` and no build provenance attestation.

## 0.3.1 — 2026-10-01

### Fixed

- **Commands keeping keys at once on macOS no longer refuse for no reason.** macOS can answer that the credentials
  file's lock does not exist while other commands are creating it, and the command was refused with "the lock
  cannot be had here". The lock is now opened again until the usual 10-second wait runs out.

## 0.3.0 — 2026-10-01

### Removed

- **The Release workflow** (ADR 0007). No workflow in this public repository holds write permissions now: a
  maintainer's coding agent cuts each release, builds it with `make -j dist` from the tag and publishes it, as
  CONTRIBUTING.md shows. Release binaries no longer carry a build provenance attestation; check them against
  `SHA256SUMS`, or build the tag yourself. v0.2.0 to v0.2.2 keep theirs.

## 0.2.2 — 2026-10-01

### Security

- **A new key is kept before it is printed** (ADR 0005). `register-agent` printed the key first, so a reader of
  standard output that stopped early, such as `register-agent | head`, ended the process before it kept anything,
  and the account's only key was lost. SIGPIPE is now ignored, and an answer standard output cannot take is
  reported as `OUTPUT_FAILED` with the new exit status 7, whose hint says whether the key is kept.
- **A key is only issued when it can be kept.** `register-agent` and `replace-key` now rewrite the credentials file
  under its lock before sending anything, and refuse with `CREDENTIALS_UNWRITABLE` when they cannot, instead of
  registering and then failing to keep the key.
- **A replaced `$SNAPHOP_MAPS_API_KEY` is no longer sent.** After `replace-key`, the environment's old key went on
  being sent and the kept new one never was; when the old one expired, the advice to register again with
  `--overwrite` replaced the new one. The file now keeps the digests of the keys replaced, the last 64, so the
  variable's key is recognised however many replacements ago it was; a command sends the kept key in its place
  with the warning `ENVIRONMENT_KEY_REPLACED`, and a refused key never leads to that advice while another key is
  kept.
- **No part of a key reaches standard error.** An error body is redacted before it is cut to 200 characters, which
  could leave the start of a key; anything shaped like a SnapHop key (`sh_agent_…`) is redacted, including one
  typed where the command's name goes; and an unknown flag before the command is refused by its name alone.
- **The credentials lock is opened inside its directory, and waited for at most 10 seconds.** A link at
  `credentials.json.lock` that led elsewhere was followed, and a stuck lock hung a command for good.
- **The release is refused when its tag was moved while it built,** and runs for different tags no longer cancel
  each other. SECURITY.md and ADR 0002 now say that only a ruleset on `v*` tags makes "a tag on `main`" hold.

### Fixed

- A request that may have run is no longer reported as "Nothing was carried out": an HTTP 2xx other than 200, and
  a JSON-RPC error other than parse, invalid request, unknown method and invalid parameters, are now
  `outcomeKnown: false`. `call` with a tool this build does not know no longer says "It is safe to repeat."
- `withdraw-map --id keep-me --yes other` withdrew `other`: an id given both ways is now refused with
  `CONFLICTING_ARGUMENT`. `--publish false` sent `false` as the map's id: a switch followed by `true` or `false` is
  now refused, with a hint to write `--publish=false`.
- A JSON flag followed by a stray `]` or `}` was sent as valid; it is now `INVALID_JSON`. `--args '{"apiKey": null}'`
  sent no key at all; `apiKey` that is not a key is now refused.
- `$SNAPHOP_MAPS_API_KEY` was not sent with `--url https://maps.snaphop.ai:443`: a scheme's own port is now dropped
  from a service address. A key kept under an address with `:443` or `:80` is found under the address without it,
  and moves there with its agent, workspace and every other field when `replace-key` or `register-agent
  --overwrite` replaces it, so the old key is not left behind.
- `help`, `version`, `schema` and `skill` failed when `$SNAPHOP_MAPS_URL` or `--timeout` was invalid; they no
  longer read either. `INVALID_URL` names `$SNAPHOP_MAPS_URL` when that is where the address came from.
- `--pretty` spread errors over several lines; standard error is now always one JSON document per line.
- A 401 when no key was sent, from `ping`, `tools`, `guide` or `register-agent`, was reported as `API_KEY_INVALID`.
- `skill install` without `--project` failed when the home skills directory, such as `~/.claude`, is a link a
  dotfile manager made. Only a project's links are now confined.
- The skill files and `skill pack`'s zip were always `0644`; they now honour the umask.
- On Windows, a command replacing the credentials file could fail while another read it. Reads now take the lock,
  shared, so that commands reading at once do not queue, and each command reads the file once. On a file system
  that cannot lock, such as some network ones, reading still works; keeping a key is refused, as before.
- `--credentials`, `$SNAPHOP_MAPS_CREDENTIALS` and `--project` are read relative to the working directory a run
  is given, as `@file` and `--output` are. A global flag written `--url=…` before the command is now read.
- The credentials file keeps fields a newer build wrote. `schema` lists no flags as `[]`, not `null`, and says that
  `call` sends a key and `skill` writes files. `SKILL.md` covers exit statuses 6 and 7, `INPUT_TOO_LARGE`, where
  the environment's key is sent, and switches.
- Answers sent as server-sent events are read, and a deadline that passes while an answer arrives is `TIMEOUT`.
- A refusal now reaches standard error with every field the service gave, and the service's own `hint` when this
  program has none for its code. A refusal in plain text is `REFUSED` with the text as `detail`, exit status 3,
  not an unreadable answer that may have been carried out.
- An answer to `register-agent` or `replace-key` without an `apiKey` is `INVALID_RESPONSE`, exit status 4, not
  `CREDENTIALS_NOT_SAVED`, whose hint pointed to a key that was not there.
- A string flag's value that is a switch's name, as in `update-map --name publish true`, is no longer taken for that
  switch and refused. An unknown flag with no command, such as `snaphop-maps --verison`, is now `INVALID_FLAG`, exit
  status 2, not the help with exit status 0.
- An error body with a byte that is not UTF-8, such as a page in Latin-1, lost everything from that byte on; each
  such byte is now shown as U+FFFD.
- A key given as `--args '{"apiKey": ...}'` that the service refused got the advice to register again with
  `--overwrite`, which would replace the key kept for the service. Its hint now points to the kept key, as for a
  refused `--api-key` or `$SNAPHOP_MAPS_API_KEY`.
- The documentation and `schema` now say that a key written into `--args` goes as that argument rather than a
  header, that `$SNAPHOP_MAPS_API_KEY` is passed over once the kept key replaced it, that a new key is kept before
  it is printed, and that a refusal without a code of its own is `REFUSED`. `SKILL.md` covers `API_KEY_EXPIRED`.
- `make dist` rebuilt nothing once `dist/` held a file; it now rebuilds every file. The coverage floor counted a
  rounded percentage and passed at 99.95%; it now counts statements. macOS and Windows now run the coverage floor
  too, and `scripts/ci-local.sh` checks staged changes for whitespace errors.

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
