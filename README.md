# SnapHop Maps CLI

SnapHop Maps from the command line, built for AI agents. The SnapHop Maps CLI, `snaphop-maps`, publishes interactive
web maps with plain-text markers at a stable link and as embeddable HTML, through
[SnapHop Maps](https://maps.snaphop.ai).

It is one static executable with no dependencies. Each command calls one tool of the service's MCP server, so the
CLI offers exactly what the MCP server offers:

| Command          | MCP tool         | What it does                                                        |
| ---------------- | ---------------- | ------------------------------------------------------------------- |
| `register-agent` | `register_agent` | Open an account; keeps its API key in the credentials file          |
| `get-account`    | `get_account`    | The account the key belongs to: workspace, role, permissions       |
| `get-installation` | `get_installation` | Styles, basemap and delivery that apply to every map here       |
| `create-map`     | `create_map`     | Create a map and, unless `--publish=false`, publish it              |
| `list-maps`      | `list_maps`      | List the workspace's maps                                           |
| `get-map`        | `get_map`        | Read a map: its whole draft, version, page link and embed codes     |
| `update-map`     | `update_map`     | Replace the fields given, keep the rest, and publish                |
| `publish-map`    | `publish_map`    | Publish the current draft as the next release                       |
| `list-releases`  | `list_releases`  | List a map's releases, newest first                                 |
| `rollback-map`   | `rollback_map`   | Make an earlier release live again; the draft is left as it is      |
| `withdraw-map`   | `withdraw_map`   | Take a map down for good (needs `--yes`)                            |
| `list-activity`  | `list_activity`  | The workspace's 50 most recent events                               |
| `replace-key`    | `replace_key`    | Replace the API key before it expires; keeps the new one            |
| `call`           | any              | Call any tool by name with `--args` JSON                            |
| `tools`          | `tools/list`     | The service's tools and the JSON Schema of their arguments, live    |
| `guide`          | `initialize`     | The service's own instructions for agents, with this installation's limits |
| `ping`           | `ping`           | Check that the service answers                                      |

Local commands: `credentials` (which key would be sent and from where, never the key itself), `skill` (the Agent
Skill for this CLI: print, install or pack it), `schema` (this program described as JSON), `version` and `help`.

## Install

```sh
go install github.com/snaphop/snaphop-maps-cli/cmd/snaphop-maps@latest
```

Or download a binary from [Releases](https://github.com/snaphop/snaphop-maps-cli/releases). The Release
workflow builds each one from the tagged commit and attests its provenance, so you can verify one before running
it:

```sh
sha256sum -c SHA256SUMS --ignore-missing
gh attestation verify snaphop-maps-linux-amd64 --repo snaphop/snaphop-maps-cli
```

To build every platform yourself, run `make -j dist`. The binaries go in `dist/`, next to their `SHA256SUMS`.

## Use

```sh
# Once: open an account. The key is shown this once and kept in the credentials file for every later command.
snaphop-maps register-agent --name "Trip planner"

# Create a map and publish it. published.page in the answer is the link to give a person.
snaphop-maps create-map --name "Coffee in Lisbon" --style positron \
  --markers '[{"position": [-9.1427, 38.7107], "title": "A Brasileira", "description": "Open since 1905"},
              {"position": [-9.1365, 38.7139], "title": "Fabrica Coffee Roasters", "color": "#b5452b"}]'

snaphop-maps list-maps
snaphop-maps get-map MAP_ID
snaphop-maps update-map MAP_ID --name "Coffee in Lisbon, 2026"   # publishes unless --publish=false
snaphop-maps list-releases MAP_ID
snaphop-maps rollback-map MAP_ID --release 1                    # make release 1 live again
snaphop-maps withdraw-map MAP_ID --yes
snaphop-maps replace-key                                        # before the key's expiresAt
```

Positions are `[longitude, latitude]`: longitude first. Text is plain and is never read as HTML or Markdown.
Without a view, a map opens on all of its markers. The account is deleted, and its maps taken down, after
`limits.inactivityDays` days without a request, and `get-map` returns a map's whole draft if you want to keep it.

A JSON flag (`--markers`, `--view`, `--controls`, `--args`) takes JSON inline, `@file` or `-` for standard input.
`--args` takes a tool's whole argument object, and flags override its fields, so a draft from `get-map` goes back
as it is:

```sh
snaphop-maps get-map MAP_ID | jq '.draft' > draft.json
snaphop-maps create-map --args @draft.json
```

## Skills for AI assistants

[`skills/snaphop-maps/SKILL.md`](skills/snaphop-maps/SKILL.md) is an [Agent Skill](https://agentskills.io). It
teaches an assistant when to reach for SnapHop Maps and how to use this CLI well: coordinates longitude first,
confirming before a withdrawal, keeping the key secret, and what each error means. Claude, ChatGPT and Codex, Gemini,
Grok, Cursor and the other clients that follow the standard all read the same file. The binary carries it too, so
the skill always matches the version you run:

| Assistant                     | Install                                                   | Where it goes                   |
| ----------------------------- | --------------------------------------------------------- | ------------------------------- |
| Claude Code                   | `snaphop-maps skill install --client claude`              | `~/.claude/skills/snaphop-maps` |
| OpenAI Codex                  | `snaphop-maps skill install --client codex`               | `~/.agents/skills/snaphop-maps` |
| Gemini CLI                    | `snaphop-maps skill install --client gemini`              | `~/.gemini/skills/snaphop-maps` |
| Grok Build                    | `snaphop-maps skill install --client grok`                | `~/.grok/skills/snaphop-maps`   |
| Cursor                        | `snaphop-maps skill install --client cursor`              | `~/.cursor/skills/snaphop-maps` |
| Any client reading `.agents/skills` | `snaphop-maps skill install --client agents`        | `~/.agents/skills/snaphop-maps` |
| claude.ai, ChatGPT, model APIs | Upload the zip from `snaphop-maps skill pack` (attached to each release from the next one on) | — |

Add `--project DIR` to install the skill into one project instead of your home directory. `snaphop-maps skill`
prints it, which an assistant can read to learn the CLI in one step. A skill needs a client that can run the binary.
An assistant without a shell can use the same service as an MCP server at `https://maps.snaphop.ai/mcp`.

## Output and exit statuses

On success, standard output holds one JSON document: the tool's structured result, exactly as the service returned
it. `--pretty` indents it. On failure, standard error holds `{"error": {"code", "message", "hint", ...}}`. `code` is
the service's own code when the service refused, and `hint` is the next step to take. A warning, such as a key
about to expire, is `{"warning": {...}}` on standard error and does not change the exit status.

| Status | Meaning                                                                                                 |
| ------ | ------------------------------------------------------------------------------------------------------- |
| 0      | Done.                                                                                                   |
| 1      | This program could not do its own part, such as reading the credentials file. Nothing was sent.         |
| 2      | The command line is wrong or incomplete. Nothing was sent.                                              |
| 3      | The service refused the request and changed nothing.                                                    |
| 4      | The exchange failed. Unless `error.outcomeKnown` is true, the request may still have been carried out, and `error.hint` says what to check before repeating it. |
| 5      | The map was saved but its publication was refused. Standard output holds the map, with `publicationError`. |
| 6      | A new API key was issued but could not be kept. Standard output holds it, and it is the only copy.       |

`snaphop-maps schema` returns all of this, every command and every flag as JSON.

## Credentials

The key the service issues is the account's only credential, and the service shows it once. `register-agent` and
`replace-key` keep it in `snaphop-maps/credentials.json` under the user's configuration directory. The file is
created with mode `0600`, written whole and renamed into place. It holds one account per service address, and a
key is only ever sent to the service that issued it.

A command sends the first key it finds: `--api-key`, then `$SNAPHOP_MAPS_API_KEY`, then the file. The key goes in
an `Authorization` bearer header. `register-agent` will not replace an account already kept for the same service
unless given `--overwrite`. `replace-key` only replaces the key it was called with. A replaced key keeps working
until the new key is first used, so an answer lost on the way costs nothing: run `replace-key` again.

| Variable                   | Default                                     |
| -------------------------- | ------------------------------------------- |
| `SNAPHOP_MAPS_URL`         | `https://maps.snaphop.ai`                   |
| `SNAPHOP_MAPS_API_KEY`     | the key kept for the service                |
| `SNAPHOP_MAPS_CREDENTIALS` | `<config dir>/snaphop-maps/credentials.json` |

Plain `http` is refused except to this machine (`localhost`, `127.0.0.1`, `::1`), so a key is never sent in the
clear. The CLI follows no redirects.

## Develop

`make check` runs `gofmt`, `go vet`, the build, and every test with the race detector in random order. It fails
unless every statement in the module is covered. `./scripts/ci-local.sh` also builds every platform concurrently
and runs `govulncheck`. To release, add the version's section to `CHANGELOG.md` and push the tag `vX.Y.Z`; the
Release workflow does the rest (ADR 0002). See [CONTRIBUTING.md](CONTRIBUTING.md), [AGENTS.md](AGENTS.md) and [docs/decisions](docs/decisions).

## License

[MIT](LICENSE).
