#!/usr/bin/env sh
set -eu

OBS_DIR="${OBS_DIR:-deploy/observability}"

json_files="
$OBS_DIR/grafana-dashboard.json
$OBS_DIR/grafana-logs-dashboard.json
$OBS_DIR/grafana-audit-dashboard.json
"

yaml_files="
$OBS_DIR/alertmanager-route.yaml
$OBS_DIR/external-alert-rules.yaml
$OBS_DIR/grafana-dashboard-provider.yaml
$OBS_DIR/grafana-datasources.yaml
$OBS_DIR/kube-metrics-recording-rules.yaml
$OBS_DIR/loki-alert-rules.yaml
$OBS_DIR/otel-collector.yaml
$OBS_DIR/postgres-exporter.yaml
$OBS_DIR/prometheus-rules.yaml
$OBS_DIR/prometheus-scrape.yaml
$OBS_DIR/promtail.yaml
$OBS_DIR/redis-exporter.yaml
$OBS_DIR/servicemonitor.yaml
$OBS_DIR/synthetic-alert-rules.yaml
"

doc_files="
docs/observability/README.md
docs/observability/alert-drill.md
docs/observability/grafana-import.md
docs/observability/logql-queries.md
docs/observability/on-call-runbook.md
docs/observability/otel.md
docs/observability/production-acceptance.md
docs/observability/slo-review-template.md
docs/observability/slo.md
docs/observability/tenant-db-monitoring.md
docs/operations/tenant-governance-runbook.md
"

for file in $json_files; do
  python3 -m json.tool "$file" >/dev/null
done

ruby -e 'require "yaml"; ARGV.each { |file| YAML.load_stream(File.read(file)) { |_| } }' $yaml_files

python3 - $json_files <<'PY'
import json
import sys
from pathlib import Path

for file in sys.argv[1:]:
    data = json.loads(Path(file).read_text())
    assert data.get("title"), f"{file}: missing title"
    assert data.get("uid"), f"{file}: missing uid"
    assert data.get("panels"), f"{file}: missing panels"

for file in [
    "docs/observability/README.md",
    "docs/observability/production-acceptance.md",
    "docs/observability/slo.md",
    "docs/observability/on-call-runbook.md",
]:
    text = Path(file).read_text()
    assert "Grafana" in text or "grafana" in text, f"{file}: missing Grafana reference"
    assert "Prometheus" in text or "prometheus" in text, f"{file}: missing Prometheus reference"
PY

for file in $doc_files; do
  test -s "$file"
done

if grep -R -nE '\| json \| (request_id|trace_id|event|outcome|status|duration_ms)(=| >=)' "$OBS_DIR" docs/observability; then
  echo "top-level LogQL JSON field extraction found; use named extra.* extraction" >&2
  exit 1
fi

if grep -R -nE '^OTEL_(TRACES|METRICS|LOGS)_EXPORTER=otlp$' docs/observability .env.example .env.production.example; then
  echo "OTEL exporter env must reference configured exporter keys: otlptrace/otlpmetric/otlplog" >&2
  exit 1
fi

sh -n scripts/verify-observability-assets.sh
sh -n scripts/observability-runtime-smoke.sh

if command -v promtool >/dev/null 2>&1; then
  promtool check rules \
    "$OBS_DIR/prometheus-rules.yaml" \
    "$OBS_DIR/external-alert-rules.yaml" \
    "$OBS_DIR/kube-metrics-recording-rules.yaml" \
    "$OBS_DIR/synthetic-alert-rules.yaml"
else
  echo "promtool not found; skipped Prometheus rule syntax check" >&2
fi

if command -v amtool >/dev/null 2>&1; then
  amtool check-config "$OBS_DIR/alertmanager-route.yaml"
else
  echo "amtool not found; skipped Alertmanager config syntax check" >&2
fi

echo "observability assets static checks finished"
