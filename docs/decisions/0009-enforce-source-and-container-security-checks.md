# Enforce source and container security checks

Date: 2026-10-06
Status: Accepted

## Context

Security Services and Messaging Services already gate verification on pinned
vulnerability scanners, retained reports and reviewed expiring exceptions.
This repository needs that baseline before a release is requested, alongside
its existing build, static analysis and regression checks.

## Decision

Verify runs the local `security/check.py` command using the reviewed Trivy
0.74.0 image digest already used by Messaging Services. Check repository
vulnerabilities, secrets and misconfiguration, build every Dockerfile listed in
`security/images.json`, and scan those exact local images. HIGH/CRITICAL findings
are blocking even without a published fix. Fail on tool errors too. Generate
SBOMs and retain reports for 30 days, redacting secret matches and source snippets.
Validate identifier-specific, owned and expiring exceptions before scanning.

Each repository owns its copy, policy and container inventory. Use only the
Python standard library on the host; do not add runtime dependencies or fetch
an unpinned script from another project. Preserve the existing runner admission
policy, use a read-only job token with no secrets, and remove only uniquely tagged
verification images. Production release and deployment ownership is unchanged.

## Consequences

New findings or expired exceptions can make Verify fail on otherwise unchanged
code. Triage them under SECURITY.md instead of disabling the scanner. Downloads
of scanner images, databases and Maven metadata require network access. Container
builds add verification time; source scans cover Java dependencies that a native
executable's OS scan cannot reconstruct. See `security/README.md` for the local
commands, retained artifacts and exception contract.
