# SnapHop Maps CLI requirements

This is the implemented product contract for `snaphop-maps`. The CLI is an agent-first Go client of
SnapHop Maps' MCP server. [Traceability](docs/requirements-traceability.md) connects these requirements to tests.

## 1. Service parity

- **CLI-01:** Every MCP tool has a named command with the same arguments, except the separately resolved API key.
  The command table drives parsing, help and `schema`; the service's tool-list fixture checks parity.
- **CLI-02:** Each service operation makes one stateless MCP request. Print the structured result as received;
  validation of map definitions, draft merging, fitted views, permissions and publication belong to the service.
  `call` permits tools newer than the binary; `tools`, `guide` and `ping` expose MCP discovery and readiness.

## 2. Agent interface

- **CLI-03:** Service successes emit one JSON document on stdout. Errors and warnings emit one JSON document per
  line on stderr, with stable codes, actionable hints and the exit statuses documented in [README.md](README.md).
  An uncertain exchange must not imply that repeating a mutation is safe. Local `skill` output can be Markdown.
- **CLI-04:** JSON arguments accept inline JSON, `@file` and stdin (`-`). Explicit flags override `--args` fields.
  Ambiguous command lines and missing required arguments fail before sending anything. Withdrawal and member
  removal require `--yes`, including through `call`.
- **CLI-05:** Embed, print, install and pack the version-matched usage skill. Installation confines project writes
  to the chosen directory and does not write through hostile links. Offline commands need no service configuration.

## 3. Credentials and transport

- **CLI-06:** Resolve credentials from an explicit flag, the environment or the per-service credentials file.
  Bind environment and kept keys to their service addresses, redact keys from diagnostics, require HTTPS except
  loopback HTTP, and follow no redirects. Caller-supplied `apiKey` in `--args` stays an argument.
- **CLI-07:** Keep keys privately, atomically and under a bounded lock on supported systems. When saving is enabled, preflight storage before
  issuing a key and keep it before printing it. With `--no-save`, the answer is its only copy. Registration needs `--overwrite` to replace a kept account;
  replacement changes only the key used. Preserve other accounts and unknown fields from newer binaries.
  Platforms and file systems without locking have the limitations [SECURITY.md](SECURITY.md) documents.

## 4. Build and distribution

- **CLI-08:** Build a static executable using only the Go standard library. Any module dependency needs an ADR.
  Keep every statement covered, with race tests in randomized order, and build Linux, macOS and Windows on
  amd64 and arm64. Cross-compilation does not substitute for platform-specific tests.
- **CLI-09:** Run public CI on GitHub-hosted runners with read-only tokens, no secrets and pinned actions. Keep
  source security checks and vulnerability checks blocking; no workflow here releases. Release only on a
  maintainer's instruction through snaphop-build-deploy, with the documented fallback and tagged artifacts.

## 5. Scope

The CLI owns parsing, transport, diagnostics, credential storage and skill distribution. It does not own map
validation, storage, rendering, the browser widget, publication or delivery infrastructure, identity policies,
account retention or installation limits. It does not cache or reconstruct service results, host a viewer,
provision an installation, or test against production. Consult `guide` and `tools` on the selected local
installation for its current contract; change the service before extending named map operations here.
