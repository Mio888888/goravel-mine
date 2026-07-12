import json
import re
from datetime import date, timedelta


def scalar(text, name):
    match = re.search(rf"^{re.escape(name)}:\s*(.+)$", text, re.M)
    return match.group(1).strip() if match else ""


def list_section(text, name):
    match = re.search(rf"^{re.escape(name)}:\n((?:  - .+\n)+)", text, re.M)
    return [line.strip()[2:].strip() for line in match.group(1).splitlines()] if match else []


def nested_scalar(text, section, name):
    pattern = rf"^{re.escape(section)}:\n(?:  .+\n)*?  {re.escape(name)}:\s*(.+)$"
    match = re.search(pattern, text, re.M)
    return match.group(1).strip() if match else ""


def nested_list(text, section, name):
    pattern = rf"^{re.escape(section)}:\n(?:  .+\n)*?  {re.escape(name)}:\n((?:    - .+\n)+)"
    match = re.search(pattern, text, re.M)
    return [line.strip()[2:].strip() for line in match.group(1).splitlines()] if match else []


def load_json(path, key, errors):
    if not path or not path.exists():
        return {key: []}
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        errors.append(f"{path} is invalid JSON: {exc}")
        return {key: []}
    if not isinstance(payload.get(key, []), list):
        errors.append(f"{path} field {key} must be an array")
        return {key: []}
    return payload


def evidence_valid(value):
    return str(value).startswith((
        "https://",
        "s3://",
        "gs://",
        "az://",
        "azblob://",
        "worm://",
        "artifact://",
        "oci://",
    ))


def tokenize(expression):
    pattern = r"\(|\)|\bOR\b|\bAND\b|\bWITH\b|[^\s()]+"
    return [token for token in re.findall(pattern, expression) if token.strip()]


def split_top_level(tokens, operator):
    parts, current, depth = [], [], 0
    for token in tokens:
        if token == "(":
            depth += 1
        elif token == ")":
            depth = max(depth - 1, 0)
        if depth == 0 and token == operator:
            parts.append(current)
            current = []
        else:
            current.append(token)
    parts.append(current)
    return parts


def strip_parentheses(tokens):
    while len(tokens) >= 2 and tokens[0] == "(" and tokens[-1] == ")":
        depth, wraps = 0, True
        for index, token in enumerate(tokens):
            depth += token == "("
            depth -= token == ")"
            if depth == 0 and index != len(tokens) - 1:
                wraps = False
                break
        if not wraps:
            break
        tokens = tokens[1:-1]
    return tokens


def evaluate_tokens(tokens, resolver):
    tokens = strip_parentheses([token for token in tokens if token])
    if not tokens:
        return False, [("", "missing")]
    for operator, any_branch in (("OR", True), ("AND", False)):
        parts = split_top_level(tokens, operator)
        if len(parts) == 1:
            continue
        results = [evaluate_tokens(part, resolver) for part in parts]
        if any_branch and any(result[0] for result in results):
            return True, []
        findings = [finding for ok, items in results if not ok for finding in items]
        return (not findings), findings
    license_id = " ".join(split_top_level(tokens, "WITH")[0]).strip()
    reason = resolver(license_id)
    return (reason == "allowed"), ([] if reason == "allowed" else [(license_id, reason)])


def expressions(component):
    values = []
    for entry in component.get("licenses") or []:
        license_value = entry.get("license") or {}
        value = license_value.get("id") or license_value.get("name") or entry.get("expression") or ""
        if str(value).strip():
            values.append(str(value).strip())
    return values


def component_purl(component):
    purl = str(component.get("purl", "")).strip()
    ref = str(component.get("bom-ref", "")).strip()
    return purl or (ref if ref.startswith("pkg:") else "")


def validate_overrides(payload, allow, errors):
    exact, patterns, seen = {}, [], set()
    for item in payload.get("overrides", []):
        selector = str(item.get("purl") or "").strip()
        component_selector = str(item.get("component") or "").strip()
        pattern = str(item.get("purl_pattern") or "").strip()
        version = str(item.get("version", "")).strip()
        license_id = str(item.get("license", "")).strip()
        key = ("pattern" if pattern else "selector", pattern or selector, version)
        if component_selector:
            errors.append(f"license metadata override {key} component selector is forbidden; use purl or purl_pattern")
        validate_override_fields(item, selector, pattern, version, license_id, key, allow, errors)
        if pattern:
            compiled = compile_pattern(pattern, key, errors)
            if compiled:
                patterns.append((compiled, version, item))
        elif selector:
            exact[(selector, version)] = item
        if key in seen:
            errors.append(f"duplicate license metadata override {key}")
        seen.add(key)
    return exact, patterns


def validate_override_fields(item, selector, pattern, version, license_id, key, allow, errors):
    if bool(selector) == bool(pattern) or not version or not license_id:
        errors.append(f"license metadata override {key} missing exactly one selector, version, or license")
    if not item.get("owner") or not evidence_valid(item.get("evidence", "")):
        errors.append(f"license metadata override {key} missing owner or evidence")
    ok, _ = evaluate_tokens(tokenize(license_id), lambda value: "allowed" if value in allow else "unlisted")
    if not ok:
        errors.append(f"license metadata override {key} uses non-allowed license {license_id}")


def compile_pattern(pattern, key, errors):
    if not pattern.startswith("^") or not pattern.endswith("$"):
        errors.append(f"license metadata override {key} purl_pattern must be anchored")
    try:
        return re.compile(pattern)
    except re.error as exc:
        errors.append(f"license metadata override {key} has invalid purl_pattern: {exc}")
        return None


def find_override(exact, patterns, component, version):
    purl = component_purl(component)
    exact_override = exact.get((purl, str(version))) if purl else None
    if exact_override:
        return exact_override
    for pattern, expected_version, item in patterns:
        if version == expected_version and purl and pattern.fullmatch(purl):
            return item
    return None


def validate_reviews(payload, review_required, errors):
    indexed = {}
    for item in payload.get("reviews", []):
        purl = str(item.get("purl", "")).strip()
        key = (purl, str(item.get("version", "")).strip(), str(item.get("license", "")).strip())
        if not all(key) or key[2] not in review_required or item.get("status") != "approved":
            errors.append(f"license review {key} has invalid purl, version, license, or status")
        if not item.get("owner") or not item.get("approval_id") or not evidence_valid(item.get("evidence", "")):
            errors.append(f"license review {key} missing owner, approval_id, or evidence")
        if key in indexed:
            errors.append(f"duplicate license review {key}")
        indexed[key] = item
    return indexed


def validate_exceptions(payload, statuses, sla, errors):
    indexed = {}
    for item in payload.get("exceptions", []):
        key = tuple(str(item.get(name, "")).strip() for name in ("purl", "version", "cve"))
        if not all(key) or not positive_number(item.get("cvss")):
            errors.append(f"vulnerability exception {key} missing purl, version, cve, or cvss")
        if item.get("status") not in statuses:
            errors.append(f"vulnerability exception {key} has unsupported status {item.get('status', '')}")
        if not item.get("owner") or not item.get("approval_id") or not item.get("mitigation"):
            errors.append(f"vulnerability exception {key} missing owner, approval_id, or mitigation")
        validate_expiry(item, key, sla, errors)
        indexed[key] = item
    return indexed


def positive_number(value):
    try:
        return float(value) > 0
    except (TypeError, ValueError):
        return False


def validate_expiry(item, key, sla, errors):
    expires_at = str(item.get("expires_at", "")).strip()
    try:
        expiry = date.fromisoformat(expires_at)
    except ValueError:
        errors.append(f"vulnerability exception {key} {'missing' if not expires_at else 'has invalid'} expires_at")
        return
    if expiry < date.today():
        errors.append(f"vulnerability exception {key} expired at {expires_at}")
    if expiry > date.today() + timedelta(days=sla):
        errors.append(f"vulnerability exception {key} expires_at {expires_at} exceeds exception SLA of {sla} days")


def load_policy(path, errors):
    text = path.read_text(encoding="utf-8") if path.is_file() else ""
    policy = {
        "owner": scalar(text, "owner"), "allow": list_section(text, "allow"),
        "deny": list_section(text, "deny"), "review_required": list_section(text, "review_required"),
        "first_party_purls": set(list_section(text, "first_party_purls")),
        "statuses": nested_list(text, "vulnerability", "allowed_exception_status"),
    }
    if not path.is_file():
        errors.append(f"license policy is required: {path}")
    for key in ("owner", "allow", "deny", "review_required", "statuses"):
        if not policy[key]:
            errors.append(f"{key} {'list ' if key != 'owner' else ''}is required")
    if set(policy["allow"]).intersection(policy["deny"]):
        errors.append("allow and deny lists overlap")
    policy["max_cvss"] = parse_number(nested_scalar(text, "vulnerability", "max_cvss_without_exception"), float, "max_cvss_without_exception", errors)
    policy["sla"] = parse_number(nested_scalar(text, "vulnerability", "exception_sla_days"), int, "exception_sla_days", errors)
    return policy


def parse_number(raw, converter, name, errors):
    try:
        value = converter(raw)
    except ValueError:
        value = converter(0)
        errors.append(f"{name} must be numeric")
    if value <= 0:
        errors.append(f"{name} must be positive")
    return value


def component_exclusion(component, root_ref, first_party_purls):
    if root_ref and str(component.get("bom-ref", "")) == root_ref:
        return "root"
    properties = {str(item.get("name", "")): str(item.get("value", "")) for item in component.get("properties") or []}
    if properties.get("aquasecurity:trivy:Class") == "lang-pkgs":
        return "manifest"
    if component_purl(component) in first_party_purls:
        return "first_party"
    return ""
