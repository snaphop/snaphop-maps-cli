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

- A key the CLI finds, from `--api-key`, `$SNAPHOP_MAPS_API_KEY` or the credentials file, MUST only be sent as an
  `Authorization` bearer header. A key the caller writes into a tool's arguments, as `--args '{"apiKey": ...}'`, is
  sent there, as the tool takes it, and no header goes with it. Either way a key MUST only travel over https, or
  over http to a loopback address. The CLI MUST NOT follow redirects, which could carry a request to another host.
- A key kept in the credentials file MUST only be sent to the service address it was kept under. A key from
  `$SNAPHOP_MAPS_API_KEY` MUST only be sent to the service the environment names, `$SNAPHOP_MAPS_URL` or else the
  default, never to one that `--url` alone names (ADR 0004).
- The credentials file MUST be created readable only by its owner (`0600`; a directory the CLI creates for it is
  `0700`), written whole, synced and renamed into place, so that an interrupted write or a crash leaves the previous
  file intact. Every writer MUST hold the file's lock from reading it to renaming its replacement, and decide what to
  keep against the file as it is under the lock. A reader takes the lock shared, and reads the file as it is only
  where the lock cannot be opened or taken at all, since no writer can then replace the file either. The lock MUST
  be opened inside the file's directory, never through a link that leads out of it, and a command MUST NOT wait for
  it without end.
- No command may silently lose a kept key. Registering over a kept account needs `--overwrite`, and replacing a key
  replaces only the key that was used. A command that would issue a key MUST first prove it can keep it, and MUST
  keep it before printing it (ADR 0005). A hint MUST NOT advise registering again while another key is kept for the
  service.
- Only the answer that issued a key may print it. `credentials`, errors, warnings and hints MUST NOT contain a key:
  a key the run knows, or anything shaped like a SnapHop key, that is echoed back, by the command line or by a
  response body, is redacted, and a body is redacted before it is shortened.
- An answer that arrived but could not be read MUST be reported with `outcomeKnown` false: the tool may have run.
- The CLI MUST NOT execute anything the service returns or render it as markup. Its output is JSON, and map text
  stays plain text.
- `skill install` MUST write only the skill's own files, in the skill's own directory under the chosen client's
  skills directory, inside the home or `--project` directory. A project may come from anyone: the install MUST NOT
  follow a link out of that directory, MUST refuse a skill directory that is a link, and MUST replace a link where
  a file goes rather than write through it. `skill pack` MUST likewise replace, not follow, a link at its output. The skill MUST NOT tell an assistant to reveal, log or pass on the API key, or to withdraw a
  map without the user's agreement.
- The module MUST depend on the Go standard library alone. A new dependency needs an ADR and review.
- CI runs untrusted pull requests from forks. It MUST use GitHub-hosted runners, a read-only token and no secrets,
  and MUST NOT use `pull_request_target`. Every action MUST be pinned to a full commit SHA.
- Release binaries MUST be built and published only by the Release workflow, from a tag on `main`, with a build
  provenance attestation for each. Only its publishing job may hold write permissions. The workflow that runs is the
  tagged commit's own, so a repository ruleset MUST restrict creating, moving and deleting `v*` tags to maintainers
  (ADR 0002).
- Tests MUST NOT reach production. Registering agents or publishing maps on production is not a verification step.

`--api-key` and `--args` are visible to other processes on the same machine. Prefer `$SNAPHOP_MAPS_API_KEY` or the
credentials file. On a system other than Linux, macOS, the BSDs and Windows the credentials file cannot be locked,
and commands that keep keys at the same time may lose one. On a file system that cannot lock, such as some network
ones, the file is read without the lock, and `register-agent` and `replace-key` refuse to keep a key there.

## Supported versions

Only the current `main` branch is supported.
