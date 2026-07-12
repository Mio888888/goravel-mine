#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
chart_dir="$repo_root/deploy/helm/goravel-mine"
output_dir="${CI_DEPLOYMENT_OUTPUT_DIR:-$repo_root/artifacts/ci-deployments}"
release_name="${HELM_RELEASE_NAME:-goravel-mine}"
namespace="${HELM_NAMESPACE:-goravel-mine}"

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "required command not found: $1" >&2
    exit 1
  }
}

render() {
  local output_path="$1"
  shift
  helm template "$release_name" "$chart_dir" \
    --namespace "$namespace" \
    --set image.repository=ghcr.io/example/goravel-mine \
    --set image.tag=ci \
    --set secret.existingSecret=goravel-mine-secret \
    "$@" > "$output_path"
}

require_command helm
require_command ruby

verify_profile() {
  ruby - "$1" "$2" <<'RUBY'
require "yaml"

manifest_path = ARGV.fetch(0)
profile = ARGV.fetch(1)
resources = YAML.load_stream(File.read(manifest_path)).compact

def fail!(message)
  warn "deployment verification failed: #{message}"
  exit 1
end

def resource(resources, kind, name)
  resources.find { |item| item["kind"] == kind && item.dig("metadata", "name") == name } || fail!("missing #{kind}/#{name}")
end

def assert!(condition, message)
  fail!(message) unless condition
end

def env_value(container, name)
  Array(container["env"]).find { |entry| entry["name"] == name }
end

case profile
when "hpa"
  worker_hpa = resource(resources, "HorizontalPodAutoscaler", "goravel-mine-queue-worker")
  assert!(worker_hpa.dig("spec", "scaleTargetRef", "kind") == "Deployment", "worker HPA target must be a Deployment")
  assert!(worker_hpa.dig("spec", "scaleTargetRef", "name") == "goravel-mine-queue-worker", "worker HPA targets the wrong deployment")
  assert!(worker_hpa.dig("spec", "minReplicas").to_i >= 1, "worker HPA minReplicas must be positive")
  assert!(worker_hpa.dig("spec", "maxReplicas").to_i >= worker_hpa.dig("spec", "minReplicas").to_i, "worker HPA maxReplicas must not be below minReplicas")

  metric_names = Array(worker_hpa.dig("spec", "metrics")).map { |metric| metric.dig("external", "metric", "name") }
  assert!(metric_names.include?("goravel_queue_pending_jobs"), "worker HPA is missing pending queue metric")
  assert!(metric_names.include?("goravel_queue_oldest_backlog_age_seconds"), "worker HPA is missing oldest queue age metric")
when "keda"
  scaled_object = resource(resources, "ScaledObject", "goravel-mine-queue-worker")
  assert!(scaled_object.dig("spec", "scaleTargetRef", "name") == "goravel-mine-queue-worker", "ScaledObject targets the wrong deployment")
  assert!(scaled_object.dig("spec", "minReplicaCount").to_i >= 1, "ScaledObject minReplicaCount must be positive")
  assert!(scaled_object.dig("spec", "maxReplicaCount").to_i >= scaled_object.dig("spec", "minReplicaCount").to_i, "ScaledObject maxReplicaCount must not be below minReplicaCount")

  metric_names = Array(scaled_object.dig("spec", "triggers")).map { |trigger| trigger.dig("metadata", "metricName") }
  assert!(metric_names.include?("goravel_queue_pending_jobs"), "ScaledObject is missing pending queue metric")
  assert!(metric_names.include?("goravel_queue_oldest_backlog_age_seconds"), "ScaledObject is missing oldest queue age metric")
when "backup-restore"
  backup = resource(resources, "CronJob", "goravel-mine-postgres-backup")
  backup_container = backup.dig("spec", "jobTemplate", "spec", "template", "spec", "containers", 0) || fail!("backup CronJob has no container")
  assert!(backup_container["command"] == ["/usr/local/bin/backup-to-object-storage"], "backup CronJob must invoke the immutable backup entrypoint")
  %w[BACKUP_S3_BUCKET BACKUP_KMS_KEY_ID BACKUP_RETENTION_DAYS RELEASE_GIT_SHA RELEASE_IMAGE_DIGEST].each do |name|
    assert!(!env_value(backup_container, name).nil?, "backup CronJob is missing #{name}")
  end
  assert!(backup.dig("spec", "concurrencyPolicy") == "Forbid", "backup CronJob must forbid concurrent runs")
  assert!(backup.dig("spec", "startingDeadlineSeconds").to_i > 0, "backup CronJob needs startingDeadlineSeconds")
  assert!(backup.dig("spec", "jobTemplate", "spec", "activeDeadlineSeconds").to_i > 0, "backup CronJob needs activeDeadlineSeconds")
  assert!(!backup.dig("spec", "timeZone").to_s.empty?, "backup CronJob needs a timeZone")

  restore = resource(resources, "Job", "goravel-mine-restore-drill")
  restore_container = restore.dig("spec", "template", "spec", "containers", 0) || fail!("restore drill Job has no container")
  restore_args = Array(restore_container["args"])
  assert!(restore_container["command"] == ["/usr/local/bin/restore-drill"], "restore drill Job must invoke the isolated restore entrypoint")
  assert!(restore_args.include?("--target-db") && restore_args.any? { |value| value.to_s.include?("_restore_") || value.to_s.include?("_drill_") }, "restore drill target must be isolated")
  assert!(env_value(restore_container, "RESTORE_PGPASSWORD")&.dig("valueFrom", "secretKeyRef", "key") == "DB_PASSWORD", "restore drill must bind RESTORE_PGPASSWORD from DB_PASSWORD")
  assert!(restore.dig("metadata", "namespace") == "goravel-mine-restore-ci", "restore drill must render into its isolated namespace")
else
  fail!("unsupported profile: #{profile}")
end

puts "deployment verification passed: #{profile}"
RUBY
}

mkdir -p "$output_dir"

helm lint "$chart_dir" \
  -f "$chart_dir/values-ha-test.yaml" \
  --set image.repository=ghcr.io/example/goravel-mine \
  --set image.tag=ci \
  --set secret.existingSecret=goravel-mine-secret

render "$output_dir/ha.yaml" \
  -f "$chart_dir/values-ha-test.yaml" \
  --set worker.autoscaling.enabled=true \
  --set worker.autoscaling.keda.enabled=false
bash "$repo_root/scripts/verify-helm-ha.sh" "$output_dir/ha.yaml"
verify_profile "$output_dir/ha.yaml" hpa

render "$output_dir/worker-keda.yaml" \
  -f "$chart_dir/values-ha-test.yaml" \
  --set worker.autoscaling.enabled=true \
  --set worker.autoscaling.keda.enabled=true
verify_profile "$output_dir/worker-keda.yaml" keda

render "$output_dir/backup-restore.yaml" \
  -f "$chart_dir/values-ha-test.yaml" \
  --set restoreDrill.enabled=true \
  --set restoreDrill.sourceManifestSecret=goravel-mine-backup-manifest \
  --set restoreDrill.targetDatabase=goravel_mine_restore_ci \
  --set restoreDrill.targetNamespace=goravel-mine-restore-ci
verify_profile "$output_dir/backup-restore.yaml" backup-restore

printf 'CI deployment verification passed: %s\n' "$output_dir"
