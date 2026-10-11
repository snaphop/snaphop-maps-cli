# Architecture

`snaphop-maps` is a static Go executable with no runtime dependency outside the standard library. It is a client
of the service's stateless Streamable HTTP MCP endpoint, not a second map implementation ([ADR 0001](decisions/0001-call-the-mcp-server-for-parity.md)).

## Boundaries

- `cmd/snaphop-maps/`: entry point, version and process exit handling.
- `internal/cli/`: command table, argument parsing, help and schema, key selection, diagnostics and command orchestration.
- `internal/mcp/`: JSON-RPC requests to `/mcp`, bounded response reading, structured results and transport errors.
- `internal/credentials/`: per-service accounts, replacement history and platform-specific file locking.
- `internal/atomicfile/`: complete file replacement and confined writes used by credentials and skill installation.
- `skills/`: embedded usage skill; `.agents/skills/`: development review workflows, linked from `.claude/skills/`.
- `scripts/`: coverage, local verification and release tooling. `security/`: scanner policy and regression suite.

A command reads the table, parses its arguments, selects the intended service and credential, then calls one
MCP tool or method. Tool results go to stdout unchanged apart from JSON formatting. Service refusal and exchange
failure go to stderr with stable codes and hints. Unknown outcomes remain unknown; there is no automatic retry
that could duplicate a mutation. When saving is enabled, registration and key replacement preflight storage and save before printing.

Help, schema and parsing share a table. `TestEveryToolIsACommand` checks the table against the service's recorded
`tools/list`. The usage skill is embedded and tested against commands and executable examples. The service owns
validation, workspace authorization, draft merging, publication, viewer rendering and limits. Its changes may
require a fixture refresh and a new CLI release, but do not justify implementing its logic here.

[SECURITY.md](../SECURITY.md) defines credential, transport and public CI boundaries.
[Operations](operations.md) describes the release owner and [verification](verification.md) the evidence limits.
