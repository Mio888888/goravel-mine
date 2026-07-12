#!/usr/bin/env bash
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
policy="${LICENSE_POLICY_FILE:-config/license_policy.yml}"
report="${LICENSE_REPORT_ARTIFACT:-artifacts/compliance/license-policy.json}"
exceptions="${VULNERABILITY_EXCEPTIONS_FILE:-}"
overrides="${LICENSE_METADATA_OVERRIDES_FILE:-config/license_metadata_overrides.json}"
reviews="${LICENSE_REVIEWS_FILE:-}"
sbom_files_json="${SBOM_ARTIFACTS_JSON:-[\"artifacts/sbom/backend.cdx.json\",\"artifacts/sbom/frontend.cdx.json\"]}"

PYTHONDONTWRITEBYTECODE=1 python3 "$script_dir/check_license_policy.py" \
  "$policy" "$report" "$exceptions" "$overrides" "$reviews" "$sbom_files_json"

echo "license policy check passed"
