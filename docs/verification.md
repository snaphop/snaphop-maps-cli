# Verification

This page describes checks and their limits. A command listed here is not evidence that it passed in a particular
checkout; report the actual run and failures at handoff.

| Check | What it proves |
| --- | --- |
| `make check` | Formatting, vet, static build, randomized race tests and the 100.0% statement floor on the current platform |
| `./scripts/ci-local.sh` | The above, all six concurrent cross-builds, skill zip and checksums, pinned govulncheck, security policy tests, full Trivy gate and diff whitespace |
| Verify macOS and Windows jobs | Native tests and the same coverage floor, including each platform's compiled locking implementation |
| Verify build matrix | Linux, macOS and Windows binaries for amd64 and arm64 compile |
| `python3 -m unittest discover -s security -p 'test_*.py' -v` | Security policy regression checks |
| `python3 security/check.py` | Source vulnerabilities, secrets and configuration, plus owned container scans where applicable |
| `make security` | Security policy regression checks followed by the full Trivy gate |

The scanner runs as part of `ci-local.sh` and requires Python 3.11+, Docker with Buildx and network access; read
[security/README.md](../security/README.md) for prerequisites, exceptions, reports and cleanup. Do not weaken a
failed coverage or security gate. Generated binaries, coverage and scan reports stay out of source control.

The Go harness tests parsing, exact tool arguments and structured results, errors, key selection and file behavior
against a fake service. The recorded tool schema proves parity with that fixture, not an arbitrary running
installation. Refresh it against a local plane when tools change. A local `ping`, `tools` or command exercise
provides service integration evidence, but cannot prove real Cloudflare delivery, staging acceptance or every
platform's lock behavior. Those service checks remain owned by snaphop-maps. Never use production as a test target.

For documentation and skills also check relative links, cited test symbols, discovery links and skill frontmatter.
The embedded usage skill has executable examples checked by the Go suite; development review skills are not packaged
by the binary. No local check authorizes issuing a release or changing an operator-owned gate.
