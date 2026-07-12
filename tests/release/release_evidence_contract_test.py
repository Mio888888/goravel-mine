#!/usr/bin/env python3
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
HARD_GATE = ROOT / "scripts/release-hard-gate.sh"
MODULE_VERIFIER = ROOT / "scripts/release/verify-module-release-evidence.sh"
METADATA_VERIFIER = ROOT / "scripts/release/verify-evidence-metadata.sh"


class ReleaseEvidenceContractTest(unittest.TestCase):
    def setUp(self):
        self.workdir = Path(tempfile.mkdtemp())
        self.sha = subprocess.check_output(["git", "-C", ROOT, "rev-parse", "HEAD"], text=True).strip()

    def tearDown(self):
        subprocess.run(["rm", "-rf", self.workdir], check=True)

    def test_hard_gate_rejects_plan_only_rollback_evidence(self):
        rollback = self.write_evidence("rollback.json", self.rollback_evidence(executed=False))
        slo = self.write_evidence("slo.json", self.slo_evidence())

        result = self.run_hard_gate(rollback, slo)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("ROLLBACK_DRILL_ARTIFACT must record a completed real execution", result.stderr)

    def test_hard_gate_records_verified_module_release_evidence(self):
        rollback = self.write_evidence("rollback.json", self.rollback_evidence())
        slo = self.write_evidence("slo.json", self.slo_evidence())
        module_evidence = self.write_module_evidence()

        result = self.run_hard_gate(rollback, slo, MODULE_RELEASE_EVIDENCE_DIR=str(module_evidence))

        self.assertEqual(0, result.returncode, result.stderr)
        manifest = json.loads((self.workdir / "artifacts/release-hard-gate.json").read_text(encoding="utf-8"))
        self.assertIn("module_release", manifest["artifacts"])

    def test_hard_gate_rejects_mismatched_slo_image_digest(self):
        rollback = self.write_evidence("rollback.json", self.rollback_evidence())
        slo = self.write_evidence("slo.json", self.slo_evidence())

        result = self.run_hard_gate(rollback, slo, RELEASE_IMAGE_DIGESTS="backend=sha256:" + "b" * 64)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("SLO_OBSERVATION_ARTIFACT deployment image digests do not match current release", result.stderr)

    def test_hard_gate_rejects_changed_digest_bound_evidence(self):
        rollback = self.rollback_evidence()
        rollback["digest"] = self.digest(rollback)
        rollback["execution"]["rollback_run_key"] = "tampered-after-digest"
        rollback_path = self.write_evidence("rollback.json", rollback)
        slo = self.write_evidence("slo.json", self.slo_evidence())

        result = self.run_hard_gate(rollback_path, slo)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("ROLLBACK_DRILL_ARTIFACT digest does not bind its artifact content", result.stderr)

    def test_hard_gate_rejects_short_or_predeploy_slo_window(self):
        rollback = self.write_evidence("rollback.json", self.rollback_evidence())
        evidence = self.slo_evidence()
        evidence["observation"] = {
            "started_at": "2026-07-10T23:59:00Z",
            "finished_at": "2026-07-11T00:09:00Z",
            "window_seconds": 600,
        }
        evidence["digest"] = self.digest(evidence)
        slo = self.write_evidence("slo.json", evidence)

        result = self.run_hard_gate(rollback, slo)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("SLO_OBSERVATION_ARTIFACT observation window is shorter than required", result.stderr)

    def test_hard_gate_rejects_external_uri_without_metadata_verifier(self):
        result = self.run_hard_gate("artifact://change/CHG-123/rollback", "artifact://change/CHG-123/slo")

        self.assertNotEqual(0, result.returncode)
        self.assertIn("RELEASE_EVIDENCE_METADATA_VERIFIER is required", result.stderr)

    def test_module_manifest_verifier_rejects_tampered_artifact(self):
        target = self.workdir / "module-release"
        artifacts = target / "artifacts"
        artifacts.mkdir(parents=True)
        required = {
            "module-manifest.json": b"{}\n",
            "module-compatibility-matrix.json": b"{}\n",
            "module-admission.lock.json": b'{"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}\n',
            "admitted_modules_gen.go": b'const AdmittedRegistryLockDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"\n',
            "modules.openapi.json": b"{}\n",
            "admin-api.ts": b"export {}\n",
            "module-upgrade-plan.json": b"{}\n",
            "module-upgrade-dry-run.json": b"{}\n",
        }
        manifest_artifacts = []
        for name, content in required.items():
            path = artifacts / name
            path.write_bytes(content)
            manifest_artifacts.append({
                "path": f"artifacts/{name}",
                "size": len(content),
                "sha256": hashlib.sha256(content).hexdigest(),
            })
        (target / "evidence-manifest.json").write_text(json.dumps({
            "evidence_type": "module-release",
            "git_sha": self.sha,
            "action": "upgrade",
            "admission_lock_digest": "sha256:" + "a" * 64,
            "artifacts": manifest_artifacts,
        }), encoding="utf-8")
        (artifacts / "admin-api.ts").write_text("tampered\n", encoding="utf-8")

        result = subprocess.run([MODULE_VERIFIER, "--target", target, "--git-sha", self.sha], text=True, capture_output=True)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("artifact digest mismatch", result.stderr)

    def test_module_manifest_verifier_rejects_mismatched_admission_lock(self):
        target = self.workdir / "module-release"
        artifacts = target / "artifacts"
        artifacts.mkdir(parents=True)
        required = {
            "module-manifest.json": b"{}\n",
            "module-compatibility-matrix.json": b"{}\n",
            "module-admission.lock.json": b'{"digest":"sha256:' + b"b" * 64 + b'"}\n',
            "admitted_modules_gen.go": b'const AdmittedRegistryLockDigest = "sha256:' + b"a" * 64 + b'"\n',
            "modules.openapi.json": b"{}\n",
            "admin-api.ts": b"export {}\n",
            "module-upgrade-plan.json": b"{}\n",
            "module-upgrade-dry-run.json": b"{}\n",
        }
        manifest_artifacts = []
        for name, content in required.items():
            path = artifacts / name
            path.write_bytes(content)
            manifest_artifacts.append({"path": f"artifacts/{name}", "size": len(content), "sha256": hashlib.sha256(content).hexdigest()})
        (target / "evidence-manifest.json").write_text(json.dumps({
            "evidence_type": "module-release",
            "git_sha": self.sha,
            "action": "upgrade",
            "admission_lock_digest": "sha256:" + "b" * 64,
            "artifacts": manifest_artifacts,
        }), encoding="utf-8")

        result = subprocess.run([MODULE_VERIFIER, "--target", target, "--git-sha", self.sha], text=True, capture_output=True)

        self.assertNotEqual(0, result.returncode)
        self.assertIn("static registry binding does not match admission lock", result.stderr)

    def test_metadata_verifier_requires_object_version_and_immutability(self):
        metadata = self.workdir / "metadata.json"
        metadata.write_text(json.dumps({
            "uri": "artifact://change/CHG-123/slo",
            "sha256": "a" * 64,
            "git_sha": self.sha,
            "verified_at": "2026-07-11T00:00:00Z",
            "immutable_until": "2026-07-12T00:00:00Z",
        }), encoding="utf-8")

        result = subprocess.run(
            [METADATA_VERIFIER, "--uri", "artifact://change/CHG-123/slo", "--git-sha", self.sha],
            text=True,
            capture_output=True,
            env={**os.environ, "RELEASE_EVIDENCE_METADATA_FILE": str(metadata)},
        )

        self.assertNotEqual(0, result.returncode)
        self.assertIn("object_version", result.stderr)

    def rollback_evidence(self, executed=True):
        payload = {
            "schema_version": 1,
            "evidence_type": "rollback-drill",
            "git_sha": self.sha,
            "digest": "sha256:pending",
            "execution": {"executed": executed, "upgrade_run_key": "run-upgrade", "smoke": "passed", "rollback_run_key": "run-rollback"},
            "state_diff": {"after_rollback": "in_sync"},
        }
        payload["digest"] = self.digest(payload)
        return payload

    def slo_evidence(self):
        payload = {
            "schema_version": 1,
            "evidence_type": "slo-observation",
            "git_sha": self.sha,
            "digest": "sha256:pending",
            "deployment": {
                "uid": "deployment-1",
                "started_at": "2026-07-11T00:00:00Z",
                "images": [{"name": "backend", "digest": "sha256:" + "a" * 64}],
            },
            "observation": {
                "started_at": "2026-07-11T00:00:00Z",
                "finished_at": "2026-07-11T00:30:00Z",
                "window_seconds": 1800,
            },
        }
        payload["digest"] = self.digest(payload)
        return payload

    def write_evidence(self, name, payload):
        path = self.workdir / name
        path.write_text(json.dumps(payload), encoding="utf-8")
        return path

    def digest(self, payload):
        normalized = {**payload, "digest": "sha256:pending"}
        content = json.dumps(normalized, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
        return "sha256:" + hashlib.sha256(content).hexdigest()

    def run_hard_gate(self, rollback, slo, **overrides):
        dependency = self.workdir / "dependency-policy.json"
        compatibility = self.workdir / "compatibility.json"
        dependency.write_text('{"status":"passed"}\n', encoding="utf-8")
        compatibility.write_text(
            '{"status":"passed","framework_version":"1.17.2","modules":[{"id":"platform-rbac","enabled":true,"framework_compatible":true}]}\n',
            encoding="utf-8",
        )
        environment = {
            **os.environ,
            "GITHUB_ACTIONS": "",
            "RELEASE_GIT_SHA": self.sha,
            "CHANGE_TICKET": "CHG-123",
            "RELEASE_APPROVER": "platform-approver",
            "ROLLBACK_DRILL_ARTIFACT": str(rollback),
            "SLO_OBSERVATION_ARTIFACT": str(slo),
            "DEPENDENCY_POLICY_ARTIFACT": str(dependency),
            "COMPATIBILITY_MATRIX_ARTIFACT": str(compatibility),
            **overrides,
        }
        return subprocess.run([HARD_GATE], cwd=self.workdir, text=True, capture_output=True, env=environment)

    def write_module_evidence(self):
        target = self.workdir / "module-release"
        artifacts = target / "artifacts"
        artifacts.mkdir(parents=True)
        required = {
            "module-manifest.json": b"{}\n",
            "module-compatibility-matrix.json": b"{}\n",
            "module-admission.lock.json": b'{"digest":"sha256:' + b"a" * 64 + b'"}\n',
            "admitted_modules_gen.go": b'const AdmittedRegistryLockDigest = "sha256:' + b"a" * 64 + b'"\n',
            "modules.openapi.json": b"{}\n",
            "admin-api.ts": b"export {}\n",
            "module-upgrade-plan.json": b"{}\n",
            "module-upgrade-dry-run.json": b"{}\n",
        }
        manifest_artifacts = []
        for name, content in required.items():
            path = artifacts / name
            path.write_bytes(content)
            manifest_artifacts.append({"path": f"artifacts/{name}", "size": len(content), "sha256": hashlib.sha256(content).hexdigest()})
        (target / "evidence-manifest.json").write_text(json.dumps({
            "evidence_type": "module-release",
            "git_sha": self.sha,
            "action": "upgrade",
            "admission_lock_digest": "sha256:" + "a" * 64,
            "artifacts": manifest_artifacts,
        }), encoding="utf-8")
        return target


if __name__ == "__main__":
    unittest.main()
