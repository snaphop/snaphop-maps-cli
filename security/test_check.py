"""Security policy failures, report privacy, and job-resource isolation."""

import importlib.util
import json
from pathlib import Path
import subprocess
import runpy
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("security_check", Path(__file__).with_name("check.py"))
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class SecurityGateTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "snaphop-test"
        (self.root / "security").mkdir(parents=True)
        self.reports = self.root / "security-reports"
        self.reports.mkdir()
        self.policy = self.root / "security/exceptions.yaml"
        self.empty = {"vulnerabilities": [], "misconfigurations": [], "secrets": []}
        self.policy.write_text(json.dumps(self.empty))
        (self.root / "security/images.json").write_text('{"app":"Dockerfile","postgres":"postgres/Dockerfile"}')
        self.addCleanup(patch.stopall)
        patch.object(check, "ROOT", self.root).start()
        patch.object(check, "REPORTS", self.reports).start()
        patch.dict(check.os.environ, {"GITHUB_RUN_ID": "local", "GITHUB_RUN_ATTEMPT": "1"}).start()

    def entry(self):
        return dict(id="CVE-2099-1234", expired_at="2099-12-31", owner="Security team",
                    rationale="Synthetic fixture", mitigation="Local-only data", statement="Reviewed for this test")

    def test_empty_and_reviewed_policy_pass(self):
        check.validate_exceptions(self.policy)
        self.empty["vulnerabilities"].append(self.entry())
        self.policy.write_text(json.dumps(self.empty))
        check.validate_exceptions(self.policy)

    def test_expired_malformed_broad_and_unowned_exceptions_fail(self):
        invalid = [dict(owner=""), dict(expired_at="2000-01-01"), dict(expired_at="2099-02-30"),
                   dict(expired_at="2099-1-1"), dict(id="*"), dict(paths=["**"]), dict(paths=["[ab].txt"]),
                   dict(paths=["same", "same"]), dict(purls=[]), dict(extra="unknown")]
        for changes in invalid:
            with self.subTest(changes=changes):
                doc = dict(self.empty, vulnerabilities=[dict(self.entry(), **changes)])
                self.policy.write_text(json.dumps(doc))
                with self.assertRaises(ValueError):
                    check.validate_exceptions(self.policy)
        for source in ['{"vulnerabilities":[],"vulnerabilities":[]}', '[]', '{"vulnerabilities":[]}']:
            self.policy.write_text(source)
            with self.assertRaises(ValueError):
                check.validate_exceptions(self.policy)

    def test_secret_and_configuration_exceptions_require_exact_file_scope(self):
        for section in ("secrets", "misconfigurations"):
            self.empty[section] = [self.entry()]
            self.policy.write_text(json.dumps(self.empty))
            with self.assertRaises(ValueError):
                check.validate_exceptions(self.policy)
            self.empty[section][0]["paths"] = ["test/synthetic.txt"]
            self.policy.write_text(json.dumps(self.empty))
            check.validate_exceptions(self.policy)

    def test_scanner_failure_retains_findings_without_secret_matches(self):
        report = {"SchemaVersion": 2, "Results": [{"Secrets": [{"RuleID": "fixture", "Match": "SENSITIVE", "Code": {"Lines": ["SENSITIVE"]}}]}]}
        with patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 1, json.dumps(report).encode(), b"SENSITIVE")) as run:
            self.assertFalse(check.scan("/project", "source"))
        stored = (self.reports / "source.json").read_text()
        self.assertNotIn("SENSITIVE", stored)
        self.assertIn("fixture", stored)
        command = run.call_args.args[0]
        self.assertIn("HIGH,CRITICAL", command)
        self.assertNotIn("--ignore-unfixed", command)
        self.assertNotIn("/var/run/docker.sock:/var/run/docker.sock", command)

    def test_complete_clean_report_and_sbom_pass(self):
        for sbom, report in [(False, {"SchemaVersion": 2, "Results": []}),
                             (True, {"bomFormat": "CycloneDX", "specVersion": "1.6"})]:
            with patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps(report).encode(), b"")):
                self.assertTrue(check.scan("/project", "source", sbom=sbom))

    def test_invalid_report_or_tool_error_never_passes(self):
        for output in (b"SENSITIVE", b"{}", b"null", b'{"SchemaVersion":2}'):
            with patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, output, b"SENSITIVE")):
                self.assertFalse(check.scan("/project", "source"))
                self.assertNotIn("SENSITIVE", (self.reports / "source.json").read_text())
        with patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 2, b'{"SchemaVersion":2}', b"")):
            self.assertFalse(check.scan("/project", "source"))

    def test_failed_build_still_scans_other_images_and_cleans_only_owned_tags(self):
        def run(command, **kwargs):
            code = 1 if "build" in command and "Dockerfile" in command else 0
            return subprocess.CompletedProcess(command, code)
        with patch.object(sys, "argv", ["check.py"]), patch.object(check, "scan", return_value=True) as scan, patch.object(check.subprocess, "run", side_effect=run) as commands:
            self.assertEqual(check.main(), 1)
        self.assertEqual(scan.call_count, 4)
        cleanup = commands.call_args.args[0]
        self.assertEqual(cleanup[:4], ["docker", "image", "rm", "--force"])
        self.assertEqual(len(cleanup[4:]), 2)
        self.assertTrue(all(":security-" in image for image in cleanup[4:]))

    def test_failure_does_not_skip_remaining_scans_or_sboms(self):
        with patch.object(sys, "argv", ["check.py", "--source-only"]), patch.object(check, "scan", side_effect=[False, True]) as scan:
            self.assertEqual(check.main(), 1)
            self.assertEqual(scan.call_count, 2)

    def test_cleanup_is_idempotent_after_finally_removed_images(self):
        image = "snaphop-test-app:security-local-1-0123456789ab"
        (self.reports / "images.json").write_text(json.dumps([image]))
        with patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 1)) as run:
            check.cleanup()
            self.assertEqual(run.call_count, 1)
            self.assertEqual(run.call_args.args[0][:3], ["docker", "image", "inspect"])

    def test_cleanup_uses_current_ci_run_and_attempt(self):
        image = "snaphop-test-app:security-12345-2-0123456789ab"
        with patch.dict(check.os.environ, {"GITHUB_RUN_ID": "12345", "GITHUB_RUN_ATTEMPT": "2"}), patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run:
            (self.reports / "images.json").write_text(json.dumps([image]))
            check.cleanup()
            self.assertEqual(run.call_args.args[0], ["docker", "image", "rm", "--force", image])
            for other in ("snaphop-test-app:security-12344-2-0123456789ab",
                          "snaphop-test-app:security-12345-1-0123456789ab"):
                run.reset_mock()
                (self.reports / "images.json").write_text(json.dumps([other]))
                with self.assertRaises(ValueError):
                    check.cleanup()
                run.assert_not_called()

    def test_cleanup_rejects_unrelated_images(self):
        (self.reports / "images.json").write_text('["production:latest"]')
        with patch.object(check.subprocess, "run") as run:
            with self.assertRaises(ValueError):
                check.cleanup()
            run.assert_not_called()

    def test_oversized_non_list_and_non_object_exemptions_fail(self):
        self.policy.write_text(" " * (256 * 1024 + 1))
        with self.assertRaises(ValueError):
            check.validate_exceptions(self.policy)
        for entries in ({}, [None], [{"id": "CVE-2099-1234"}]):
            self.policy.write_text(json.dumps(dict(self.empty, vulnerabilities=entries)))
            with self.assertRaises(ValueError):
                check.validate_exceptions(self.policy)

    def test_container_scan_reads_only_local_image_and_artifact_cache(self):
        report = {"SchemaVersion": 2, "Results": []}
        with patch.object(check.Path, "home", return_value=self.root), patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps(report).encode(), b"")) as run:
            self.assertTrue(check.scan("local:fixture", "app", image=True))
        command = run.call_args.args[0]
        self.assertIn("/var/run/docker.sock:/var/run/docker.sock", command)
        self.assertIn("--image-src", command)
        self.assertNotIn("--include-dev-deps", command)
        self.assertEqual(command[-3:], ["--image-src", "docker", "local:fixture"])
        self.assertNotIn(".m2/settings.xml", " ".join(command))

    def test_manifest_cannot_build_from_outside_checkout_or_invalid_tag(self):
        for manifest in ({"invalid*": "Dockerfile"}, {"app": "../Dockerfile"}):
            (self.root / "security/images.json").write_text(json.dumps(manifest))
            with patch.object(sys, "argv", ["check.py"]), patch.object(check, "scan", return_value=True), patch.object(check.subprocess, "run") as run:
                with self.assertRaises(ValueError):
                    check.main()
                run.assert_not_called()

    def test_empty_inventory_and_cleanup_are_safe(self):
        with patch.object(check.subprocess, "run") as run:
            check.cleanup()
            (self.reports / "images.json").write_text("[]")
            with patch.object(sys, "argv", ["check.py", "--cleanup"]):
                self.assertEqual(check.main(), 0)
            run.assert_not_called()
        (self.root / "security/images.json").write_text("{}")
        with patch.object(sys, "argv", ["check.py"]), patch.object(check, "scan", return_value=True):
            self.assertEqual(check.main(), 0)

    def test_cleanup_removes_present_images_and_interrupts_stop_work(self):
        image = "snaphop-test-app:security-local-1-0123456789ab"
        (self.reports / "images.json").write_text(json.dumps([image]))
        with patch.object(check.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run:
            check.cleanup()
            self.assertEqual(run.call_args.args[0], ["docker", "image", "rm", "--force", image])
        with self.assertRaises(KeyboardInterrupt):
            check.interrupted()

    def test_cli_tool_errors_do_not_disclose_error_values(self):
        script = Path(__file__).with_name("check.py")
        with patch.object(sys, "argv", [str(script), "--source-only"]), patch.object(check.Path, "resolve", return_value=self.root / "security/check.py"), patch("signal.signal"), patch("subprocess.run", side_effect=OSError("SENSITIVE")):
            with self.assertRaises(SystemExit) as exited:
                runpy.run_path(str(script), run_name="__main__")
        self.assertNotIn("SENSITIVE", str(exited.exception))


if __name__ == "__main__":
    unittest.main()
