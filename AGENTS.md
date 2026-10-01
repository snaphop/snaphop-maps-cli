# AGENTS.md

This repository builds the SnapHop Maps CLI, `snaphop-maps`. It is designed for AI agents first, and
it is written in Go as a single static executable that depends on nothing outside the standard library. Read
`README.md` for what it does, `CONTRIBUTING.md` for how to change it, and `SECURITY.md` before changing how keys are found, kept or sent. Record material
decisions in `docs/decisions/` and observable changes in `CHANGELOG.md`.

Core invariants:

- **Parity with the MCP server, by construction.** Every command that acts on a map calls one tool of SnapHop
  Maps' MCP server (`/mcp`, stateless Streamable HTTP, JSON responses) with the same arguments, and prints the
  tool's structured result exactly as it came. Never re-implement the service's logic here: validation, draft
  merging, fitted views, limits and refusal codes all belong to the service (ADR 0001). The command table in
  `internal/cli/commands.go` drives parsing, help and `schema`, so keep all three in step by editing only the table.
- **`internal/cli/testdata/tools.json` is the service's `tools/list`.** `TestEveryToolIsACommand` holds the table
  to it: every tool is a command, every argument but `apiKey` is a flag, and every flag that is sent is an argument
  the tool takes. When snaphop-maps changes `McpTools.java`, refresh the file with
  `snaphop-maps tools --url <a plane running that commit> --pretty` and change the table until the test passes.
- **Agent-first output.** On success, stdout holds one JSON document. On failure, stderr holds one JSON error with a
  stable `code` and a `hint`, and the exit status comes from the table in `commands.go`. Never print prose to
  stdout, never change what an existing exit status means, and never put a key in any output except the answer
  that issued it.
- **Keys.** A key the CLI finds is sent only as a bearer header, and a kept key only to the service address it was
  kept under (a key the caller writes into `--args` goes as that argument). Plain http is refused except to
  loopback, and redirects are never followed. The credentials file is `0600`, written whole
  and renamed into place. No command may lose a kept key: `register-agent` needs `--overwrite` to replace an
  account, and `replace-key` replaces only the key it was called with.
- **The skill is part of the interface.** `skills/snaphop-maps/SKILL.md` is the Agent Skill every assistant reads
  (ADR 0003), and the binary embeds it. Change it in the same change as any command, flag, error or behaviour an
  agent relies on. Its tests check the specification's frontmatter, check that it shows every command, and run every
  example in its code blocks.
- **Nothing outside the standard library.** Adding a module dependency needs an ADR.

Run `make check` before handoff. It runs `gofmt`, `go vet`, the build, and every test under the race detector in
random order, and it fails unless every statement in the module is covered: the floor is 100.0% and never drops.
Write tests with `t.Parallel()` against the fake service in `internal/cli/harness_test.go`. Mutate no package
state except in `cmd/snaphop-maps/main_test.go`. `./scripts/ci-local.sh` is the full handoff check: it also runs
`make -j dist`, which builds every platform concurrently, and `govulncheck`.

This repository is public. CI runs only on GitHub-hosted runners with a read-only token and no secrets, because
pull requests from forks run there. Never add a self-hosted runner, a secret or `pull_request_target`. Releases come only
from the Release workflow (ADR 0002): add the version's `## X.Y.Z — date` section to `CHANGELOG.md` on `main`, then
push the tag `vX.Y.Z`. Cutting a release is a maintainer's decision that no routine change implies, and a binary is
never built or uploaded by hand. Never commit a real API key, a
credentials file or production data. Test against a local plane (`snaphop-maps/scripts/dev.sh`), never by
registering agents on production.
