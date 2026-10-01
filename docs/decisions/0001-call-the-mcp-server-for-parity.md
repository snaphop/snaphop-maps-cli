# 0001 — Call the MCP server for parity, and speak JSON to agents

Accepted 2026-09-30.

## Context

SnapHop Maps offers AI agents three ways in: the HTTP API, the MCP server at `/mcp`, and a browser console
(snaphop-maps ADRs 0021, 0024 and 0025). Many agents can run a command but cannot speak MCP. The CLI needs to offer
exactly what the MCP server offers, and to keep doing so as the server changes.

Several MCP tools do more than one route. `create_map` creates a map and publishes it. `update_map` reads the
draft, merges the fields given, fits a view to new markers (`MapDefinitions.withFittedView`), saves the draft
against the version it read, and publishes. A refused publication comes back as `publicationError`, and does not
fail the call.

## Decision

**Each command is a `tools/call` to the service's own MCP server.**

- **Transport.** One JSON-RPC request per command, in one `POST` to `<url>/mcp`, with MCP's Streamable HTTP
  headers, protocol version `2025-06-18` and no session. This is how the server is built to be spoken to (its ADR
  0021). `tools`, `guide` and `ping` send `tools/list`, `initialize` and `ping` as they are.
- **Parity by construction.** The server runs the same controller methods for the tools as for its routes, so
  every permission, limit, audit record and refusal code applies unchanged. None of it is re-implemented here:
  the CLI would otherwise hold a second copy of the draft merge and view fitting, and would drift. `call` reaches
  a tool this build has no command for yet.
- **Parity is tested.** `internal/cli/testdata/tools.json` is the server's `tools/list`, generated from
  `McpTools.java` at the snaphop-maps commit the stack pins. A test holds the command table to it, tool by tool
  and argument by argument.
- **Agent-first.** On success, stdout holds one JSON document. On failure, stderr holds one JSON error with the
  service's code and a hint for the next step. Exit statuses tell a refusal (nothing changed) apart from a failed
  exchange (maybe changed), and a failed exchange carries the service guide's advice on repeating that tool.
  `schema` describes the program as JSON. The key is kept, because the service shows it once.
- **One static binary from the standard library.** No dependency to review or pin, cross-compiled for Linux, macOS
  and Windows on amd64 and arm64.

## Left out

- **The HTTP API's other routes**, such as releases, rollback, activity and the account, which no MCP tool offered
  then. Adding them would go beyond parity with the MCP server. `call` will reach them once the server adds tools
  for them. The server has since added tools for these four (its ADR 0026), and 0.2.0 gave each a command.
- **Client-side validation** beyond what makes a request: the service decides, and its `MAP_INVALID` names every
  refused field.
- **Automatic retries.** Repeating `create_map` can make two maps, and repeating `publish_map` adds a release, so
  the CLI never retries by itself. Instead, each error says whether a repeat is safe.
- **Publishing releases.** Superseded by [0002](0002-release-from-a-tag-with-provenance.md): a pushed tag publishes
  the release from CI.

## Consequences

- A change to the service's tools reaches every CLI user at once, with no new build. A new argument can be sent
  through `--args` straight away, and it becomes a flag once `tools.json` is refreshed.
- The CLI depends on `/mcp` staying reachable and exempt from the edge's bot challenge. An `EDGE_CHALLENGE` error
  names that failure.
