import json
import os
import sys
from pathlib import Path

from license_policy_rules import (
    component_exclusion,
    evaluate_tokens,
    expressions,
    find_override,
    load_json,
    load_policy,
    component_purl,
    tokenize,
    validate_exceptions,
    validate_overrides,
    validate_reviews,
)


def inspect_component(component, state):
    name = component.get("name") or component.get("bom-ref") or "unknown"
    version = component.get("version") or ""
    values = expressions(component)
    if not values:
        override = find_override(state["overrides"], state["override_patterns"], component, version)
        if not override:
            state["license_findings"].append({"component": name, "version": version, "license": "", "reason": "missing"})
            return
        values = [str(override["license"])]
        if override not in state["applied_overrides"]:
            state["applied_overrides"].append(override)
    for expression in values:
        ok, findings = evaluate_tokens(tokenize(expression), lambda value: resolve_license(value, component, version, state))
        if not ok:
            state["license_findings"].extend(
                {"component": name, "version": version, "license": item, "reason": reason}
                for item, reason in findings
            )


def resolve_license(license_id, component, version, state):
    policy = state["policy"]
    if license_id in policy["deny"]:
        return "denied"
    if license_id in policy["review_required"]:
        key = (component_purl(component), str(version), license_id)
        review = state["reviews"].get(key)
        if review:
            if review not in state["applied_reviews"]:
                state["applied_reviews"].append(review)
            return "allowed"
        return "review_required"
    return "allowed" if license_id in policy["allow"] else "unlisted"


def inspect_vulnerabilities(sbom, by_ref, state):
    for vulnerability in sbom.get("vulnerabilities") or []:
        scores = [float(item["score"]) for item in vulnerability.get("ratings") or [] if item.get("score") is not None]
        score = max(scores) if scores else 0.0
        if score < state["policy"]["max_cvss"]:
            continue
        cve = vulnerability.get("id") or vulnerability.get("bom-ref") or ""
        for affected in vulnerability.get("affects") or [{"ref": ""}]:
            ref = str(affected.get("ref", ""))
            component = by_ref.get(ref, {})
            name, version = component.get("name") or ref, component.get("version") or ""
            purl = component_purl(component) or (ref if ref.startswith("pkg:") else "")
            if (purl, version, cve) not in state["exceptions"]:
                state["vulnerability_findings"].append(
                    {"component": name, "version": version, "purl": purl, "cve": cve, "cvss": score}
                )


def inspect_sbom(path, state):
    sbom = load_sbom(path, state["errors"])
    if sbom is None:
        return
    root_ref = str(((sbom.get("metadata") or {}).get("component") or {}).get("bom-ref", ""))
    evaluated = []
    for component in sbom.get("components") or []:
        exclusion = component_exclusion(component, root_ref, state["policy"]["first_party_purls"])
        if exclusion:
            state[f"excluded_{exclusion}"] += 1
        else:
            evaluated.append(component)
    state["component_count"] += len(evaluated)
    by_ref = {str(item.get("bom-ref", "")): item for item in evaluated}
    for component in evaluated:
        inspect_component(component, state)
    inspect_vulnerabilities(sbom, by_ref, state)


def load_sbom(path, errors):
    if not path.is_file() or path.stat().st_size == 0:
        errors.append(f"SBOM artifact is required: {path}")
        return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        errors.append(f"SBOM artifact is invalid JSON: {path}: {exc}")
        return None


def build_state(policy_path, exceptions_path, overrides_path, reviews_path):
    errors = []
    policy = load_policy(policy_path, errors)
    overrides, patterns = validate_overrides(load_json(overrides_path, "overrides", errors), policy["allow"], errors)
    return {
        "errors": errors, "policy": policy, "overrides": overrides, "override_patterns": patterns,
        "reviews": validate_reviews(load_json(reviews_path, "reviews", errors), policy["review_required"], errors),
        "exceptions": validate_exceptions(load_json(exceptions_path, "exceptions", errors), policy["statuses"], policy["sla"], errors),
        "license_findings": [], "vulnerability_findings": [], "applied_overrides": [], "applied_reviews": [],
        "component_count": 0, "excluded_root": 0, "excluded_manifest": 0, "excluded_first_party": 0,
    }


def report_payload(policy_path, sbom_paths, state):
    if state["license_findings"]:
        state["errors"].append("license policy violations found")
    if state["vulnerability_findings"]:
        state["errors"].append("high severity vulnerabilities require approved exceptions")
    return {
        "policy": str(policy_path), "owner": state["policy"]["owner"],
        "status": "failed" if state["errors"] else "passed", "errors": state["errors"],
        "sbom_artifacts": [str(path) for path in sbom_paths], "component_count": state["component_count"],
        "excluded_root_component_count": state["excluded_root"],
        "excluded_manifest_component_count": state["excluded_manifest"],
        "excluded_first_party_component_count": state["excluded_first_party"],
        "license_findings": state["license_findings"], "applied_license_overrides": state["applied_overrides"],
        "applied_license_reviews": state["applied_reviews"], "vulnerability_findings": state["vulnerability_findings"],
        "max_cvss_without_exception": state["policy"]["max_cvss"], "exception_sla_days": state["policy"]["sla"],
        "checked_at": os.environ.get("CHECKED_AT", ""),
    }


def parse_sbom_paths(raw, errors):
    try:
        values = json.loads(raw)
    except json.JSONDecodeError as exc:
        errors.append(f"SBOM_ARTIFACTS_JSON is invalid JSON: {exc}")
        return []
    if not isinstance(values, list) or any(not isinstance(item, str) or not item.strip() for item in values):
        errors.append("SBOM_ARTIFACTS_JSON must be an array of non-empty paths")
        return []
    return [Path(item) for item in values]


def run(args):
    if len(args) != 6:
        print("usage: check_license_policy.py POLICY REPORT EXCEPTIONS OVERRIDES REVIEWS SBOM_JSON", file=sys.stderr)
        return 2
    policy_path = Path(args[0])
    report_path = Path(args[1])
    exceptions_path = Path(args[2]) if args[2] else None
    overrides_path = Path(args[3]) if args[3] else None
    reviews_path = Path(args[4]) if args[4] else None
    state = build_state(policy_path, exceptions_path, overrides_path, reviews_path)
    sbom_paths = parse_sbom_paths(args[5], state["errors"])
    for path in sbom_paths:
        inspect_sbom(path, state)
    if not sbom_paths:
        state["errors"].append("at least one SBOM artifact is required")
    if state["component_count"] == 0:
        state["errors"].append("SBOM artifacts contain no dependency components")
    payload = report_payload(policy_path, sbom_paths, state)
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if payload["errors"]:
        print("\n".join(payload["errors"]), file=sys.stderr)
        return 1
    return 0
