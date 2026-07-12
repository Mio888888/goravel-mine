import pathlib
import unittest


class ReleaseWorkflowContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = pathlib.Path(".github/workflows/release.yml").read_text(encoding="utf-8")

    def test_runtime_evidence_is_not_a_manual_input(self):
        dispatch = self.workflow.split("permissions:", 1)[0]
        self.assertNotIn("rollback_drill_artifact", dispatch)
        self.assertNotIn("slo_observation_artifact", dispatch)

    def test_release_order_is_build_deploy_observe_gate_publish(self):
        required = [
            "Enforce pre-deployment release hard gate",
            "Build and push backend image",
            "Build and push frontend image",
            "Build and push backup image",
            "Deploy digest-bound release to production",
            "Collect fixed-window production SLO evidence",
            "Enforce final release hard gate",
            "Publish GitHub release",
        ]
        positions = [self.workflow.index(name) for name in required]
        self.assertEqual(positions, sorted(positions))

    def test_predeployment_gate_runs_before_any_production_mutation(self):
        predeploy = self.workflow.index("Enforce pre-deployment release hard gate")
        capture = self.workflow.index("Capture production rollback state")
        deploy = self.workflow.index("Deploy digest-bound release to production")
        first_push = self.workflow.index("Build and push backend image")
        self.assertLess(predeploy, first_push)
        self.assertLess(predeploy, capture)
        self.assertLess(predeploy, deploy)
        step = self.workflow.split("- name: Enforce pre-deployment release hard gate", 1)[1].split("- name:", 1)[0]
        self.assertIn("RELEASE_GATE_PHASE: predeploy", step)
        self.assertIn("DEPENDENCY_POLICY_ARTIFACT", step)
        self.assertIn("COMPATIBILITY_MATRIX_ARTIFACT", step)
        self.assertIn("MODULE_RELEASE_EVIDENCE_DIR", step)

    def test_deploy_and_slo_are_immutable_bound(self):
        for expected in (
            'image.digest="$BACKEND_DIGEST"',
            "PROD_FRONTEND_DEPLOYMENT is required",
            "RELEASE_DEPLOYMENT_UID: ${{ steps.deploy.outputs.uid }}",
            'RELEASE_METRICS_SELECTOR: release_git_sha="${{ github.sha }}"',
            'RELEASE_LOG_SELECTOR: namespace="goravel-mine",pod=~"${{ steps.deploy.outputs.backend_pod_regex }}"',
            'RELEASE_SLO_WINDOW_SECONDS: "1800"',
            "artifacts/golden-rollback/evidence.json",
            "artifacts/module-release/evidence-manifest.json",
            "artifacts/sbom/backup.cdx.json",
            "artifacts/cosign-backup-verify.txt",
            "artifacts/release-predeploy-gate.json",
        ):
            self.assertIn(expected, self.workflow)

    def test_failed_post_deploy_gate_rolls_back_both_workloads(self):
        rollback = self.workflow.split("- name: Roll back failed production release", 1)[1].split("- name:", 1)[0]
        self.assertIn("always() && (failure() || cancelled())", rollback)
        self.assertIn("steps.rollback_state.outcome == 'success'", rollback)
        self.assertIn("steps.deploy.outputs.mutation_started == 'true'", rollback)
        self.assertIn("steps.rollback_state.outputs.previous_backend_revision", rollback)
        self.assertIn("steps.rollback_state.outputs.previous_frontend_image", rollback)
        self.assertIn('helm rollback goravel-mine "$PREVIOUS_BACKEND_REVISION"', rollback)
        self.assertIn('$PROD_FRONTEND_CONTAINER=$PREVIOUS_FRONTEND_IMAGE', rollback)

    def test_rollback_state_is_captured_before_deployment(self):
        capture = self.workflow.index("Capture production rollback state")
        deploy = self.workflow.index("Deploy digest-bound release to production")
        self.assertLess(capture, deploy)
        deploy_step = self.workflow.split("- name: Deploy digest-bound release to production", 1)[1].split("- name:", 1)[0]
        self.assertLess(deploy_step.index("mutation_started=true"), deploy_step.index("helm upgrade --install"))

    def test_release_manifest_binds_backup_image_reference(self):
        manifest_step = self.workflow.split("- name: Write release manifest", 1)[1].split("- name:", 1)[0]
        self.assertIn("BACKUP_REF: ${{ steps.meta.outputs.backup_image }}@${{ steps.build_backup.outputs.digest }}", manifest_step)
        self.assertIn('"backup": "$BACKUP_REF"', manifest_step)


if __name__ == "__main__":
    unittest.main()
