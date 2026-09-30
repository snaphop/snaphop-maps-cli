# Security Policy

## Reporting a vulnerability

This repository is public. **Never open a public issue, pull request or discussion for a suspected
vulnerability.** Report it by email to <security@snaphop.com>. Describe the issue and its impact, how to reproduce
it with synthetic values, and the affected version or commit. Never include a real API key, a credentials file,
workspace data or production configuration. You should receive an acknowledgement within three business days.

A vulnerability in the SnapHop Maps service itself is reported the same way. It is fixed in snaphop-maps, not here.

## Threat model

The CLI holds an agent's API key: the account's only credential, which the service shows once and can never
recover. The key can create, publish and withdraw every map in the agent's workspace. The CLI sends it to a
network service, keeps it on disk, and runs in environments where other processes, logs and transcripts may
observe its arguments and output.

## Boundaries

- A key MUST only be sent as an `Authorization` bearer header, over https, or over http to a loopback address. The
  CLI MUST NOT follow redirects, which could carry a request to another host.
- A key kept in the credentials file MUST only be sent to the service address it was kept under.
- The credentials file MUST be created readable only by its owner (`0600`, in a `0700` directory) and written whole
  and renamed into place, so that an interrupted write leaves the previous file intact.
- No command may silently lose a kept key. Registering over a kept account needs `--overwrite`, and replacing a key
  replaces only the key that was used.
- Only the answer that issued a key may print it. `credentials`, errors, warnings and hints MUST NOT contain a key.
- The CLI MUST NOT execute anything the service returns or render it as markup. Its output is JSON, and map text
  stays plain text.
- The module MUST depend on the Go standard library alone. A new dependency needs an ADR and review.
- CI runs untrusted pull requests from forks. It MUST use GitHub-hosted runners, a read-only token and no secrets,
  and MUST NOT use `pull_request_target`.
- Tests MUST NOT reach production. Registering agents or publishing maps on production is not a verification step.

`--api-key` is visible to other processes on the same machine. Prefer `$SNAPHOP_MAPS_API_KEY` or the credentials
file.

## Supported versions

Only the current `main` branch is supported.
