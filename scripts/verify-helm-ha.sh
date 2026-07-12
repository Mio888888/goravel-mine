#!/usr/bin/env bash
set -euo pipefail

manifest_path="${1:?usage: scripts/verify-helm-ha.sh <rendered-manifest.yaml>}"

if [[ ! -s "$manifest_path" ]]; then
  echo "rendered manifest is missing or empty: $manifest_path" >&2
  exit 1
fi

ruby - "$manifest_path" <<'RUBY'
require "yaml"

manifest_path = ARGV.fetch(0)
resources = YAML.load_stream(File.read(manifest_path)).compact

def fail!(message)
  warn "HA verification failed: #{message}"
  exit 1
end

def resource(resources, kind, name)
  resources.find { |item| item["kind"] == kind && item.dig("metadata", "name") == name } || fail!("missing #{kind}/#{name}")
end

def assert_equal!(actual, expected, description)
  fail!("#{description}: expected #{expected.inspect}, got #{actual.inspect}") unless actual == expected
end

def assert!(condition, message)
  fail!(message) unless condition
end

def matching_label?(selector, key, value)
  selector.dig("matchLabels", key) == value
end

def pod_spec(resource)
  case resource["kind"]
  when "Deployment", "Job"
    resource.dig("spec", "template", "spec")
  when "CronJob"
    resource.dig("spec", "jobTemplate", "spec", "template", "spec")
  end
end

def assert_secure_containers!(resource)
  spec = pod_spec(resource)
  assert!(!spec.nil?, "#{resource["kind"]}/#{resource.dig("metadata", "name")} has no pod spec")

  containers = Array(spec["initContainers"]) + Array(spec["containers"])
  assert!(!containers.empty?, "#{resource["kind"]}/#{resource.dig("metadata", "name")} has no containers")

  containers.each do |container|
    security_context = container["securityContext"] || {}
    assert!(security_context["privileged"] == false, "#{resource.dig("metadata", "name")}/#{container["name"]} must set privileged: false")

    resources = container["resources"] || {}
    %w[requests limits].each do |resource_type|
      values = resources[resource_type] || {}
      %w[cpu memory].each do |resource_name|
        assert!(!values[resource_name].nil?, "#{resource.dig("metadata", "name")}/#{container["name"]} is missing #{resource_type}.#{resource_name}")
      end
    end
  end
end

hpa = resource(resources, "HorizontalPodAutoscaler", "goravel-mine")
assert_equal!(hpa.dig("spec", "scaleTargetRef", "kind"), "Deployment", "HPA target kind")
assert_equal!(hpa.dig("spec", "scaleTargetRef", "name"), "goravel-mine", "HPA target name")
assert_equal!(hpa.dig("spec", "minReplicas"), 3, "HPA minReplicas")
assert!(hpa.dig("spec", "maxReplicas").to_i >= hpa.dig("spec", "minReplicas").to_i, "HPA maxReplicas must not be below minReplicas")
assert!(hpa.dig("spec", "behavior", "scaleDown", "stabilizationWindowSeconds").to_i > 0, "HPA needs scale-down stabilization")

metric_resources = Array(hpa.dig("spec", "metrics")).map do |metric|
  metric.dig("resource", "name") if metric["type"] == "Resource"
end.compact
assert!(metric_resources.include?("cpu") && metric_resources.include?("memory"), "HPA must target CPU and memory")

pdb = resource(resources, "PodDisruptionBudget", "goravel-mine")
assert_equal!(pdb.dig("spec", "minAvailable"), 2, "PDB minAvailable")
assert!(matching_label?(pdb.dig("spec", "selector") || {}, "app.kubernetes.io/component", "web"), "PDB must select web pods")

default_deny = resource(resources, "NetworkPolicy", "goravel-mine-default-deny")
default_deny_selector = default_deny.dig("spec", "podSelector") || {}
assert!(matching_label?(default_deny_selector, "app.kubernetes.io/name", "goravel-mine"), "default-deny must select this chart only")
component_expression = Array(default_deny_selector["matchExpressions"]).find { |expression| expression["key"] == "app.kubernetes.io/component" }
assert_equal!(component_expression && component_expression["operator"], "In", "default-deny component selector operator")
assert_equal!(Array(component_expression && component_expression["values"]).sort, %w[backup migration queue-worker web], "default-deny components")
policy_types = Array(default_deny.dig("spec", "policyTypes"))
assert!(policy_types.include?("Ingress") && policy_types.include?("Egress"), "default-deny must block ingress and egress")
assert_equal!(default_deny.dig("spec", "ingress"), [], "default-deny ingress")
assert_equal!(default_deny.dig("spec", "egress"), [], "default-deny egress")

resource(resources, "NetworkPolicy", "goravel-mine-allow-web-ingress")
resource(resources, "NetworkPolicy", "goravel-mine-allow-web-metrics")
%w[web queue-worker migration backup].each do |component|
  policy = resource(resources, "NetworkPolicy", "goravel-mine-allow-#{component}-egress")
  assert!(matching_label?(policy.dig("spec", "podSelector") || {}, "app.kubernetes.io/component", component), "#{component} egress policy selector")

  ports = Array(policy.dig("spec", "egress")).flat_map { |rule| Array(rule["ports"]) }
  port_numbers = ports.map { |port| port["port"].to_i }
  assert!(port_numbers.include?(53), "#{component} egress must allow DNS")
  assert!(port_numbers.include?(5432), "#{component} egress must allow PostgreSQL")
  assert!(port_numbers.include?(6379), "#{component} egress must allow Redis")
  assert!(port_numbers.include?(4318), "#{component} egress must allow OTEL")
  assert!(port_numbers.include?(443), "#{component} egress must allow object storage and SSO")
  cidrs = Array(policy.dig("spec", "egress")).flat_map do |rule|
    Array(rule["to"]).filter_map { |destination| destination.dig("ipBlock", "cidr") }
  end
  assert!(cidrs.include?("192.0.2.10/32"), "#{component} egress must allow external PostgreSQL CIDR")
  assert!(cidrs.include?("192.0.2.20/32"), "#{component} egress must allow external Redis CIDR")
end

web = resource(resources, "Deployment", "goravel-mine")
worker = resource(resources, "Deployment", "goravel-mine-queue-worker")

[[web, "web", "requiredDuringSchedulingIgnoredDuringExecution"], [worker, "queue-worker", "preferredDuringSchedulingIgnoredDuringExecution"]].each do |workload, component, affinity_type|
  spec = pod_spec(workload)
  assert_equal!(workload.dig("spec", "template", "metadata", "labels", "app.kubernetes.io/component"), component, "#{component} pod label")
  spreads = Array(spec["topologySpreadConstraints"])
  topology_keys = spreads.map { |spread| spread["topologyKey"] }
  assert!(topology_keys.include?("topology.kubernetes.io/zone"), "#{component} needs zone topology spread")
  assert!(topology_keys.include?("kubernetes.io/hostname"), "#{component} needs hostname topology spread")
  assert!(spreads.all? { |spread| spread["maxSkew"] == 1 }, "#{component} topology maxSkew must be 1")
  assert!(Array(spec.dig("affinity", "podAntiAffinity", affinity_type)).any?, "#{component} needs #{affinity_type}")
end

resource(resources, "Job", "goravel-mine-migrate").tap do |migration|
  assert_equal!(migration.dig("spec", "template", "metadata", "labels", "app.kubernetes.io/component"), "migration", "migration pod label")
end
resource(resources, "CronJob", "goravel-mine-postgres-backup").tap do |backup|
  assert_equal!(backup.dig("spec", "jobTemplate", "spec", "template", "metadata", "labels", "app.kubernetes.io/component"), "backup", "backup pod label")
end

resources.select { |item| %w[Deployment Job CronJob].include?(item["kind"]) }.each do |workload|
  assert_secure_containers!(workload)
end

puts "HA manifest verification passed: #{resources.length} resources checked"
RUBY
