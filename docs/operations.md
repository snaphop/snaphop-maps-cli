# Operations

## Local development

Install the Go version named by `go.mod` and `make`. Build with `make build`; run the executable in `bin/`.
Tests use the fake service in `internal/cli/harness_test.go`. For an actual MCP exchange, start the sibling
snaphop-maps project's local plane with its `scripts/dev.sh`, then run:

```sh
bin/snaphop-maps ping --url http://localhost:8084 --credentials /tmp/maps-cli-dev-credentials.json
bin/snaphop-maps tools --url http://localhost:8084 --pretty
```

When the service changes `McpTools.java`, refresh `internal/cli/testdata/tools.json` from that local plane's
`tools --pretty` output and update the command table until `TestEveryToolIsACommand` passes. Update the usage
skill with changed commands and behavior. Do not register or publish on production to test.

## Credentials and failed exchanges

Keep credentials out of source control, logs and transcripts. `credentials` describes which source would be used
without printing the key. `register-agent` and `replace-key` save before printing unless given `--no-save`; do not register again with
`--overwrite` as routine recovery. Exit 6 means the new key was returned but could not be kept; protect that
answer. Exit 7 means output failed; follow its hint about what was saved.

After a timeout or unreadable response, inspect `outcomeKnown` and follow the hint before repeating a mutation.
A publication refusal with exit 5 keeps the draft on stdout. The service decides how publication can recover.
[README.md](../README.md) defines every exit status; [SECURITY.md](../SECURITY.md) defines storage guarantees.

## Verification and releases

Run `make check`, then `./scripts/ci-local.sh` for handoff. The latter also builds all six platforms, packs the
usage skill, writes checksums and runs `govulncheck`. Security scanning is a separate Verify job; see
[verification](verification.md) and [security/README.md](../security/README.md).

A maintainer presses **Maps CLI Release** in snaphop-build-deploy ([ADR 0008](decisions/0008-release-from-snaphop-build-deploy.md)).
No workflow in this public repository publishes or holds a write token. Only if that workflow cannot run and a
maintainer asks, follow the exact fallback in [CONTRIBUTING.md](../CONTRIBUTING.md). Keep the release scripts and
`make -j dist` working. Publish only artifacts built from the accepted tag, with `SHA256SUMS`. Releases after
v0.2.2 have no build provenance attestation. A routine documentation or code change does not authorize a release.

The CLI owns no deployment, database, delivery bucket or host console. Service operations belong to snaphop-maps;
production release and deployment automation belongs to snaphop-build-deploy.
