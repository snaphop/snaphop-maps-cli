# 0011 — Build every platform in the check job

Accepted 2026-11-23.

## Context

The owner asked on 2026-11-23 that every SnapHop repository use the fewest CI runs necessary, because runners are a
limited resource; that request is the approval this CI change needs. Verify ran ten jobs per pull request and per
push to `main`: the check, two other-system test jobs, the security checks and six platform builds. Go
cross-compiles every platform from one Linux machine in seconds each, so the six build jobs each spent a hosted
runner mostly on checkout and toolchain setup.

## Decision

The `Format, vet, race tests and 100% coverage` job builds all six release binaries after its checks, with
`make -j` over the same `dist/` targets the build jobs ran, and uploads them as one `snaphop-maps-binaries`
artifact. The step runs even when an earlier check failed, as the separate jobs did, so one run reports both. The
check job's checkout fetches the whole history, so the binaries carry the version `git describe` gives, as before.

The required checks keep their names and jobs: the check, the macOS and Windows tests, and the security checks. The
macOS and Windows tests stay jobs of their own, because only those systems compile their own files.

## Consequences

Verify runs four jobs instead of ten. A platform build failure now shows in the check job's log rather than as its
own job. The binaries are one artifact instead of six; nothing reads them, and releases build their own in
snaphop-build-deploy (ADR 0008).
