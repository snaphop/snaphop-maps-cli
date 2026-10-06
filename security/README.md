# Security checks

Run `python3 security/check.py` with Python 3.11+ and Docker with Buildx.
`python3 security/check.py --source-only` runs just the repository checks.
Run the policy regression suite with
`python3 -m unittest discover -s security -p 'test_*.py' -v`.

Verify runs this same gate with a read-only token and no secrets. Trivy 0.74.0
is pinned to the same image digest as Messaging Services. It refreshes its
vulnerability databases in a project-specific cache volume. HIGH and CRITICAL
source dependency, secret and configuration findings, and fixed **or unfixed**
container vulnerabilities fail the gate. Scanner/build errors fail too.
All scans are attempted even if a previous scan found vulnerabilities.

Source scanning reads lockfiles and Maven POMs, including nested projects and
supported development dependencies. Generated tool trees and build outputs are
excluded by `trivy.yaml`; Maven resolution requires the declared repositories
to be reachable. The scanner mounts only `~/.m2/repository` read-only when it
exists, never Maven credential settings, and uses Maven Central's alternate
`repo1.maven.org` endpoint. Native image scanning finds OS packages, but cannot reconstruct
Java dependencies compiled into native executables, so the source dependency
scan is required as well. This does not replace the project's existing static
analysis, tests, or language-specific vulnerability checks.

`images.json` enumerates the application-owned Dockerfiles, built from the
repository root. Each build targets `linux/amd64`, as production does, with a unique verification
tag and Buildx SBOM
attestation. Separate CycloneDX SBOMs and JSON reports are retained in
`security-reports/` and uploaded for 30 days, including when verification fails.
Only this run's image tags are removed; the cache volume is retained. No image
is published and no deployment command is run.

Secret matches and source-code snippets are redacted before reports are written;
raw scanner output is never printed. Finding identifiers and file locations
remain available for triage. Reports are local generated output and are ignored
by Git.

## Exceptions

`exceptions.yaml` is strict JSON (also valid YAML). It starts with no exemptions.
Every entry requires an exact `id`, `owner`, `rationale`, `mitigation`, `statement`
and a future `expired_at` date in `YYYY-MM-DD` form. Optional `paths` or `purls`
contain exact values, never wildcards; secret and configuration exceptions must
name exact paths. Duplicate keys, unknown fields and expired entries fail before
scanning. Suppressed vulnerability findings remain visible in the JSON report.
Review any exemption under this project's SECURITY.md; never blanket-ignore
unfixed vulnerabilities or bypass a failed gate.

The scanner and regression suite are reviewed copies shared across the SnapHop
stack. Each repository owns its policy, container inventory and future changes;
there is no checkout of another repository or unpinned remote script at runtime.
