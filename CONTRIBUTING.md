# Contributing

Thank you for helping with the SnapHop Maps CLI. Read [AGENTS.md](AGENTS.md) for the invariants every change keeps,
[SECURITY.md](SECURITY.md) before touching how keys are found, kept or sent, and the decision records in
[docs/decisions](docs/decisions) before changing how the CLI reaches the service. AI coding agents follow the same
rules; `CLAUDE.md` and `GEMINI.md` point them to `AGENTS.md`. Read [REQUIREMENTS.md](REQUIREMENTS.md),
[DESIGN.md](DESIGN.md), [CODE_REVIEW.md](CODE_REVIEW.md) and the [documentation index](docs/README.md) for the
product contract, command-line design, review criteria and supporting runbooks. Whole-queue reviews use the
four repository skills under [`.agents/skills/`](.agents/skills/), also linked under `.claude/skills/`;
these are separate from the CLI's embedded usage skill.

**Never report a vulnerability in an issue or pull request.** This repository is public. Email
<security@snaphop.com> as [SECURITY.md](SECURITY.md) describes.

## Set up

You need Go at the version `go.mod` names, and `make`. Nothing else: the module depends on the standard library
alone.

```sh
git clone https://github.com/snaphop/snaphop-maps-cli.git
cd snaphop-maps-cli
make check
```

To try a change against a real service, run SnapHop Maps locally with its `scripts/dev.sh` (the plane is on
<http://localhost:8084>) and point the CLI at it:

```sh
make build
SNAPHOP_MAPS_URL=http://localhost:8084 SNAPHOP_MAPS_CREDENTIALS=/tmp/dev-credentials.json bin/snaphop-maps ping
```

Never register agents or publish maps on production to test a change.

## Make a change

1. **Keep the scope to one concern.** A bug fix, a new flag and a documentation rewrite are three pull requests.
2. **Change the command table, not the parser.** `internal/cli/commands.go` drives parsing, help and `schema`. A new
   command or flag is an entry there.
3. **Stay at parity with the MCP server.** The CLI never re-implements the service (ADR 0001). When snaphop-maps
   changes its tools, refresh `internal/cli/testdata/tools.json` with `snaphop-maps tools --url <a local plane>
   --pretty` and change the table until `TestEveryToolIsACommand` passes.
4. **Keep the skill in step.** When a command, flag, error or behaviour an agent relies on changes, update
   [`skills/snaphop-maps/SKILL.md`](skills/snaphop-maps/SKILL.md) in the same pull request. Its tests check it
   against the [Agent Skills specification](https://agentskills.io/specification), check that it shows every command,
   and run every example in its code blocks. Keep its examples runnable: no placeholders a shell would reject.
5. **Test everything.** `make check` fails unless every statement is covered. Write table-driven tests with
   `t.Parallel()` against the fake service in `internal/cli/harness_test.go`, and do not mutate package state outside `cmd/snaphop-maps/main_test.go`. A
   failure path you cannot reach from a test is a sign the code should be simpler.
6. **Keep project guidance current.** Update affected requirements, design, configuration, operations, verification
   and test traceability, and keep the decision index current.
7. **Record it.** Add a line under `## Unreleased` in [CHANGELOG.md](CHANGELOG.md) for anything a user or agent can
   observe, and a decision record in `docs/decisions/` for a material choice.

## Before you open a pull request

```sh
./scripts/ci-local.sh
```

This runs `gofmt`, `go vet`, the build, and every test under the race detector in random order with the 100%
coverage floor. It also builds every platform concurrently and runs `govulncheck`, as the Verify workflow does
on your pull request. Verify runs on Linux, macOS and Windows and must pass before review.

Write commit messages in the imperative, describing the result: "Refuse plain http to other hosts", not "Fixed
http". Explain why in the body when it is not obvious.

## Releases

No workflow in this repository releases. A maintainer presses **Maps CLI Release** in SnapHop's private
snaphop-build-deploy repository, which runs these steps from a clean checkout of `main` and publishes what it built (ADR
0008). Its jobs that run this module's Go code hold no credentials, and every job runs on a fresh GitHub-hosted VM. When
it cannot run, a maintainer, or their coding agent when asked, runs the same steps with the maintainer's own GitHub
credentials:

```sh
version=$(scripts/release-cut.sh auto)     # records Unreleased in CHANGELOG.md as the next version's section
./scripts/ci-local.sh                      # every check; stop here if anything fails
git commit -m "Release $version" -- CHANGELOG.md
git push origin HEAD:main
git tag -a "v$version" -m "snaphop-maps $version"   # append ": security release" when the notes open with one
git fetch origin main && scripts/release-check.sh "v$version"
git push origin "v$version"
make -j dist VERSION="v$version"           # every platform, the Agent Skill and SHA256SUMS, from the tagged commit
notes=$(mktemp) && scripts/release-notes.sh "v$version" > "$notes"
gh release create "v$version" --verify-tag --title "SnapHop Maps CLI $version" --notes-file "$notes" \
  dist/snaphop-maps-* dist/SHA256SUMS
```

The version is `auto` by default: the next minor version when Unreleased has an Added, Changed, Removed or
Deprecated section, otherwise the next patch. Give `patch`, `minor`, `major` or an exact version to override it.
`release-cut.sh` refuses when Unreleased lists nothing, and `release-check.sh` refuses a tag that disagrees with the
changelog or whose commit is not on `main`. Publish only what `make dist` built from the tagged commit.

## License

The CLI is released under the [MIT license](LICENSE). By contributing, you agree that your contribution is licensed
under it too.
