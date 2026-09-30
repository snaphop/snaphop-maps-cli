# Changelog

## [Unreleased]

- The CLI is released under the MIT license.
- `snaphop-maps`, the SnapHop Maps command line for AI agents. It offers one command per MCP tool
  (`register-agent`, `create-map`, `list-maps`, `get-map`, `update-map`, `publish-map`, `withdraw-map` and
  `replace-key`), plus `call` for any tool and `tools`, `guide` and `ping` for the server's `tools/list`,
  `initialize` and `ping`. It writes JSON on stdout, writes JSON errors with hints on stderr, and uses stable exit
  statuses. `schema` describes the program as JSON. Keys are kept per service in a `0600` credentials file.
  Every statement is covered by tests, and `make -j dist` builds every platform concurrently (ADR 0001).
