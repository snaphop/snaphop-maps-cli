# AGENTS.md

This repository builds the SnapHop Maps CLI, `snaphop-maps`. It is designed for AI agents first, and
it is written in Go as a single static executable that depends on nothing outside the standard library. Read
`README.md` and `REQUIREMENTS.md` for what it does, `CONTRIBUTING.md` for how to change it,
`docs/architecture.md` for its boundaries, and `SECURITY.md` before changing how keys are found, kept or sent.
Read `DESIGN.md` before changing commands or output, `CODE_REVIEW.md` when reviewing changes, and
`docs/operations.md` before release work. Record material
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
`make -j dist`, which builds every platform concurrently, `govulncheck`, and `make security` for policy tests
and the full pinned Trivy gate. The full check needs Python 3.11+, Docker with Buildx and network access.

This repository is public. CI runs only on GitHub-hosted runners with a read-only token and no secrets, because
pull requests from forks run there. Never add a self-hosted runner, a secret, a write permission or `pull_request_target`. No workflow here releases
(ADR 0008): a maintainer presses Maps CLI Release in snaphop-build-deploy, which runs this repository's release
scripts. Keep `scripts/release-*.sh`, `scripts/ci-local.sh` and `make dist` working as `CONTRIBUTING.md` shows,
because that workflow calls them. Only when it cannot run and a maintainer asks, you cut the release on their
machine by following those steps exactly. Cutting a release is a maintainer's decision that no routine change
implies, and only what `make dist` built from the tag is ever published. Never commit
a real API key, a credentials file or production data. Test against a local plane (`snaphop-maps/scripts/dev.sh`), never by
registering agents on production.

## Repository review skills and documentation

Asked to review a whole queue, follow its repository-local skill ([ADR 0010](docs/decisions/0010-adopt-repository-review-skills-and-documentation.md)):

- [`review-bugs`](.agents/skills/review-bugs/SKILL.md) for all open bug reports.
- [`review-enhancements`](.agents/skills/review-enhancements/SKILL.md) for all enhancement requests.
- [`review-prs`](.agents/skills/review-prs/SKILL.md) for open pull requests.
- [`review-documentations`](.agents/skills/review-documentations/SKILL.md) for the complete documentation set.

Canonical skills live under `.agents/skills/`; `.claude/skills/` links to them. These development workflows are
separate from the embedded usage skill in `skills/snaphop-maps/`. Review alone does not authorize external tracker
changes, merging or branch deletion. Follow the scope the user authorized. The CLI has no design-system update skill.

The [documentation index](docs/README.md) links configuration, operations, verification, embedding guidance,
requirements traceability and the [ADR index](docs/decisions/README.md). Keep them current with related changes.
