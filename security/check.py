#!/usr/bin/env python3
"""Local and CI security gate. Uses only Python's standard library and Docker."""

import argparse
import datetime
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import uuid


TRIVY = "aquasec/trivy:0.74.0@sha256:62b1e65e8869bc4b4c6aa4fa2b21595256c7c2f6018a9d9ad61caf87187c1969"
ROOT = Path(__file__).resolve().parent.parent
REPORTS = ROOT / "security-reports"
REQUIRED = {"id", "expired_at", "owner", "rationale", "mitigation", "statement"}


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def validate_exceptions(file, today=None):
    """Strict JSON is also YAML; reject accidental broad or expired exemptions."""
    today = today or datetime.datetime.now(datetime.timezone.utc).date()
    if file.stat().st_size > 256 * 1024:
        raise ValueError("exception file too large")
    document = json.loads(file.read_text(), object_pairs_hook=unique_object)
    if not isinstance(document, dict) or set(document) != {"vulnerabilities", "misconfigurations", "secrets"}:
        raise ValueError("invalid exception sections")
    for section, entries in document.items():
        if not isinstance(entries, list):
            raise ValueError("exception section must be a list")
        for entry in entries:
            if not isinstance(entry, dict) or not REQUIRED <= entry.keys() or entry.keys() - REQUIRED - {"paths", "purls"}:
                raise ValueError("invalid exception fields")
            for field in REQUIRED:
                value = entry[field]
                if not isinstance(value, str) or not value or value != value.strip() or len(value) > 2048:
                    raise ValueError("invalid exception metadata")
            if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:-]{1,127}", entry["id"]):
                raise ValueError("invalid exception identifier")
            if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", entry["expired_at"]) or datetime.date.fromisoformat(entry["expired_at"]) <= today:
                raise ValueError("exception expired or invalid date")
            for field in ("paths", "purls"):
                if field in entry:
                    values = entry[field]
                    if not isinstance(values, list) or not values or any(not isinstance(v, str) or not v or any(c in v for c in "*?[]{}") for v in values):
                        raise ValueError("exception scope must contain exact paths or purls")
                    if len(set(values)) != len(values):
                        raise ValueError("duplicate exception scope")
            if section != "vulnerabilities" and ("paths" not in entry or "purls" in entry):
                raise ValueError("secret and configuration exceptions require exact paths")
    return document


def redact(value):
    """Keep finding metadata without matched secrets or configuration source."""
    if isinstance(value, list):
        return [redact(item) for item in value]
    if isinstance(value, dict):
        return {key: ("[REDACTED]" if key in {"Match", "Code", "CauseMetadata"} else redact(item)) for key, item in value.items()}
    return value


def scan(target, name, image=False, sbom=False):
    command = ["docker", "run", "--rm", "--volume", f"{ROOT}:/project:ro",
               "--volume", f"{ROOT.name}-trivy-cache:/root/.cache/trivy"]
    maven_cache = Path.home() / ".m2/repository"
    if maven_cache.is_dir():
        # Reuse only downloaded artifacts; do not expose Maven credential settings.
        command += ["--volume", f"{maven_cache}:/root/.m2/repository:ro"]
    if image:
        command += ["--volume", "/var/run/docker.sock:/var/run/docker.sock"]
    command += [TRIVY, "image" if image else "fs", "--quiet", "--no-progress",
                "--config", "/project/security/trivy.yaml", "--ignorefile", "/project/security/exceptions.yaml",
                "--timeout", "15m"]
    if not image:
        command += ["--include-dev-deps"]
    if sbom:
        command += ["--scanners", "vuln", "--format", "cyclonedx"]
    else:
        command += ["--scanners", "vuln" if image else "vuln,misconfig,secret",
                    "--severity", "HIGH,CRITICAL", "--show-suppressed",
                    "--format", "json", "--exit-code", "1"]
    if image:
        # Never fall back to a registry image if the local build is missing.
        command += ["--image-src", "docker"]
    command.append(target)
    # Do not relay raw scanner output: even an error can include source values.
    result = subprocess.run(command, cwd=ROOT, capture_output=True, timeout=1000, check=False)
    report = REPORTS / f"{name}{'-sbom' if sbom else ''}.json"
    try:
        document = json.loads(result.stdout)
        valid = isinstance(document, dict) and (
            document.get("bomFormat") == "CycloneDX" and isinstance(document.get("specVersion"), str)
            if sbom else document.get("SchemaVersion") == 2 and isinstance(document.get("Results"), list)
        )
        if not valid:
            raise ValueError("invalid scanner report")
        report.write_text(json.dumps(redact(document), indent=2) + "\n")
    except (ValueError, UnicodeError):
        report.write_text(json.dumps({"error": "Scanner did not produce a valid report", "exit_code": result.returncode}) + "\n")
        return False
    return result.returncode == 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-only", action="store_true", help="Scan source without building container images")
    parser.add_argument("--cleanup", action="store_true", help="Remove only this run's verification images")
    args = parser.parse_args()
    if args.cleanup:
        cleanup()
        return 0
    validate_exceptions(ROOT / "security/exceptions.yaml")
    images = json.loads((ROOT / "security/images.json").read_text())
    REPORTS.mkdir(exist_ok=True)
    failed = False
    owned = []
    run_tag = f"security-{os.environ.get('GITHUB_RUN_ID', 'local')}-{os.environ.get('GITHUB_RUN_ATTEMPT', '1')}-{uuid.uuid4().hex[:12]}"
    try:
        for sbom in (False, True):
            failed = not scan("/project", "source", sbom=sbom) or failed
        if not args.source_only:
            for name, dockerfile in images.items():
                if not re.fullmatch(r"[a-z0-9-]+", name) or not (ROOT / dockerfile).resolve().is_relative_to(ROOT):
                    raise ValueError("invalid container manifest")
                image = f"{ROOT.name}-{name}:{run_tag}"
                owned.append(image)
                (REPORTS / "images.json").write_text(json.dumps(owned) + "\n")
                built = subprocess.run(["docker", "buildx", "build", "--platform", "linux/amd64", "--load", "--sbom=true", "--metadata-file",
                                        str(REPORTS / f"{name}-build.json"), "--file", dockerfile,
                                        "--tag", image, "."], cwd=ROOT, check=False).returncode == 0
                if not built:
                    failed = True
                    continue
                for sbom in (False, True):
                    failed = not scan(image, name, image=True, sbom=sbom) or failed
    finally:
        if owned:
            subprocess.run(["docker", "image", "rm", "--force", *owned], cwd=ROOT, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    print("Security gate failed; inspect security-reports/" if failed else "Security gate passed")
    return int(failed)


def cleanup():
    inventory = REPORTS / "images.json"
    if not inventory.exists():
        return
    images = json.loads(inventory.read_text())
    prefix = f"{ROOT.name}-"
    tag = f"security-{os.environ.get('GITHUB_RUN_ID', 'local')}-{os.environ.get('GITHUB_RUN_ATTEMPT', '1')}-"
    if not isinstance(images, list) or any(not isinstance(i, str) or not re.fullmatch(re.escape(prefix) + r"[a-z0-9-]+:" + re.escape(tag) + r"[a-f0-9]{12}", i) for i in images):
        raise ValueError("invalid cleanup inventory")
    for image in images:
        present = subprocess.run(["docker", "image", "inspect", image], cwd=ROOT,
                                 stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
        if present.returncode == 0:
            subprocess.run(["docker", "image", "rm", "--force", image], cwd=ROOT, check=True)


def interrupted(*_):
    raise KeyboardInterrupt


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, interrupted)
    try:
        raise SystemExit(main())
    except (OSError, ValueError, TypeError, subprocess.SubprocessError):
        # Report no untrusted exception text, which might contain secrets.
        raise SystemExit("Security gate failed: invalid policy, unavailable tool, or scanner error")
