package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var ErrSensitiveOperationPolicy = errors.New("sensitive operation policy is invalid")

type SensitiveOperationPolicy struct {
	PolicyKey        string
	Scope            string
	Permission       string
	TenantPermission string
	Action           string
	RequiresReAuth   bool
	RequiresApproval bool
	ResourceBuilder  func(SensitiveOperationPrepareInput) (string, error)
}

type SensitiveOperationPrepareInput struct {
	Resource string
	Before   []string
	After    []string
}

type SensitiveOperationPlan struct {
	PolicyKey     string   `json:"policy_key"`
	Scope         string   `json:"scope"`
	Resource      string   `json:"resource"`
	BindingDigest string   `json:"binding_digest"`
	ActorID       uint64   `json:"actor_id"`
	TenantID      uint64   `json:"tenant_id"`
	Before        []string `json:"before"`
	After         []string `json:"after"`
}

type SensitiveOperationPolicyRegistry struct {
	policies      map[string]SensitiveOperationPolicy
	bindingDigest func(string, string, string, uint64, []string, []string) (string, error)
}

type SensitiveOperationPolicyContract struct {
	PolicyKey        string `json:"policy_key"`
	Scope            string `json:"scope"`
	Permission       string `json:"permission"`
	TenantPermission string `json:"tenant_permission,omitempty"`
	Action           string `json:"action"`
	RequiresReAuth   bool   `json:"requires_reauth"`
	RequiresApproval bool   `json:"requires_approval"`
}

type SensitiveOperationRouteContract struct {
	RouteName   string `json:"route_name"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	PolicyKey   string `json:"policy_key"`
	Domain      string `json:"domain"`
	Resource    string `json:"resource"`
	Permission  string `json:"required_permission"`
	FeatureTest string `json:"feature_test"`
}

func NewSensitiveOperationPolicyRegistry() *SensitiveOperationPolicyRegistry {
	policies := []SensitiveOperationPolicy{
		newSensitiveOperationPolicy("audit.prune.execute", "platform:security:control", "", "DELETE"),
		newSensitiveOperationPolicy("mfa.disable", "platform:security:mfa", "security:mfa", "POST"),
		{
			PolicyKey:        "module.lifecycle.execute",
			Scope:            "module.lifecycle.execute",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "POST",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		{
			PolicyKey:        "module.admission.approve",
			Scope:            "module.admission.approve",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "POST",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		{
			PolicyKey:        "module.replacement.emergency-remove",
			Scope:            "module.replacement.emergency-remove",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "DELETE",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		newSensitiveOperationPolicy("role.permissions.sync", "platform:role:setMenu", "permission:role:setMenu", "PUT"),
		newSensitiveOperationPolicy("sso.provider.secret.change", "", "security:ssoProvider:update", "PUT"),
		newSensitiveOperationPolicy("storage.secret.change", "platform:storageConfig:update", "", "PUT"),
		{
			PolicyKey:        "module.lifecycle.release-lock",
			Scope:            "module.lifecycle.release-lock",
			Permission:       "platform:moduleLifecycle:execute",
			Action:           "POST",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
		newSensitiveOperationPolicy("tenant.governance.change", "platform:tenant:governance", "", "PUT"),
		newSensitiveOperationPolicy("tenant.data.export", "platform:tenant:export", "", "POST"),
		newSensitiveOperationPolicy("tenant.permissions.sync", "platform:tenant:permissions", "", "PUT"),
		newSensitiveOperationPolicy("tenant.plan.change", "platform:tenant:updatePlan", "", "PUT"),
		newSensitiveOperationPolicy("tenant.status.change", "platform:tenant:suspend", "", "PUT"),
		newSensitiveOperationPolicy("user.password.reset", "platform:user:password", "permission:user:password", "PUT"),
		newSensitiveOperationPolicy("user.roles.sync", "platform:user:setRole", "permission:user:setRole", "PUT"),
		{
			PolicyKey:        "tenant.data.delete",
			Scope:            "tenant.data.delete",
			Permission:       "platform:tenant:destroy",
			Action:           "DELETE",
			RequiresReAuth:   true,
			RequiresApproval: true,
			ResourceBuilder:  sensitiveOperationInputResource,
		},
	}
	registry := &SensitiveOperationPolicyRegistry{
		policies:      make(map[string]SensitiveOperationPolicy, len(policies)),
		bindingDigest: sensitiveOperationBindingDigest,
	}
	for _, policy := range policies {
		registry.policies[policy.PolicyKey] = policy
	}
	return registry
}

func (r *SensitiveOperationPolicyRegistry) Policy(policyKey string) (SensitiveOperationPolicy, bool) {
	if r == nil {
		return SensitiveOperationPolicy{}, false
	}
	policy, ok := r.policies[strings.TrimSpace(policyKey)]
	return policy, ok
}

func (r *SensitiveOperationPolicyRegistry) PermissionFor(policyKey string, tenantID uint64, resource string) (string, string, error) {
	policy, ok := r.Policy(policyKey)
	if !ok {
		return "", "", ErrSensitiveOperationPolicy
	}
	permission, action := policy.Permission, policy.Action
	if tenantID != 0 {
		permission = policy.TenantPermission
	}
	switch policy.PolicyKey {
	case "tenant.status.change":
		if strings.HasPrefix(resource, "tenant-change:status:") {
			var status int8
			if _, _, err := parseTenantSensitiveResource(resource, "status", &status); err != nil {
				return "", "", err
			}
			resource = ":status:" + tenantStatusName(status)
		}
		switch {
		case strings.HasSuffix(resource, ":status:suspended"):
			permission = "platform:tenant:suspend"
		case strings.HasSuffix(resource, ":status:active"):
			permission = "platform:tenant:resume"
		case strings.HasSuffix(resource, ":status:archived"):
			permission = "platform:tenant:archive"
		default:
			return "", "", ErrSensitiveOperationPolicy
		}
	case "sso.provider.secret.change":
		permission, action = mutationPermission(resource, "sso-provider", "security:ssoProvider:save", "security:ssoProvider:update", "security:ssoProvider:delete")
	case "storage.secret.change":
		permission, action = mutationPermission(resource, "storage-config", "platform:storageConfig:save", "platform:storageConfig:update", "platform:storageConfig:delete")
	}
	if strings.TrimSpace(permission) == "" || strings.TrimSpace(action) == "" {
		return "", "", ErrSensitiveOperationPolicy
	}
	return permission, action, nil
}

func mutationPermission(resource, prefix, createPermission, updatePermission, deletePermission string) (string, string) {
	switch {
	case resource == prefix+":create":
		return createPermission, "POST"
	case strings.HasPrefix(resource, prefix+":update:"):
		return updatePermission, "PUT"
	case strings.HasPrefix(resource, prefix+":delete:"):
		return deletePermission, "DELETE"
	default:
		return "", ""
	}
}

func (r *SensitiveOperationPolicyRegistry) Export() []SensitiveOperationPolicyContract {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.policies))
	for key := range r.policies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	contracts := make([]SensitiveOperationPolicyContract, 0, len(keys))
	for _, key := range keys {
		policy := r.policies[key]
		contracts = append(contracts, SensitiveOperationPolicyContract{
			PolicyKey: policy.PolicyKey, Scope: policy.Scope, Permission: policy.Permission,
			TenantPermission: policy.TenantPermission, Action: policy.Action,
			RequiresReAuth: policy.RequiresReAuth, RequiresApproval: policy.RequiresApproval,
		})
	}
	return contracts
}

func (r *SensitiveOperationPolicyRegistry) RouteContracts() []SensitiveOperationRouteContract {
	contracts := sensitiveOperationRouteContracts()
	sort.Slice(contracts, func(i, j int) bool {
		return contracts[i].RouteName < contracts[j].RouteName
	})
	return contracts
}

func sensitiveOperationRouteContracts() []SensitiveOperationRouteContract {
	return []SensitiveOperationRouteContract{
		newSensitiveRoute("platform.module-lifecycle.execute", "POST", "/admin/platform/module-lifecycle/execute", "module.lifecycle.execute", "platform", "module-lifecycle:alpha:upgrade", "platform:moduleLifecycle:execute", "TestPlatformAdminExecuteRequiresRegisteredApprovalAndBoundReAuth"),
		newSensitiveRoute("platform.module-lifecycle.release", "POST", "/admin/platform/module-lifecycle/locks/release-stale", "module.lifecycle.release-lock", "platform", "module-lifecycle:stale-locks:module-lifecycle:alpha", "platform:moduleLifecycle:execute", "TestStaleLockReleaseConsumesSecurityEvidenceOnce"),
		newSensitiveRoute("platform.role.set-menu", "PUT", "/admin/platform/role/{id}/permissions", "role.permissions.sync", "platform", "rbac:role:9:permissions:W10", "platform:role:setMenu", "TestPlatformRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.security.mfa.disable", "POST", "/admin/platform/security/mfa/disable", "mfa.disable", "platform", "mfa:user:9:disable", "platform:security:mfa", "TestPlatformMFADisableRequiresSensitiveEvidence"),
		newSensitiveRoute("platform.storage-config.create", "POST", "/admin/platform/storage-config", "storage.secret.change", "platform", "storage-config:create", "platform:storageConfig:save", "TestPlatformStorageConfigSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.storage-config.delete", "DELETE", "/admin/platform/storage-config", "storage.secret.change", "platform", "storage-config:delete:9", "platform:storageConfig:delete", "TestPlatformStorageConfigSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.storage-config.update", "PUT", "/admin/platform/storage-config/{id}", "storage.secret.change", "platform", "storage-config:update:9", "platform:storageConfig:update", "TestPlatformStorageConfigSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.archive", "PUT", "/admin/platform/tenant/{id}/archive", "tenant.status.change", "platform", "tenant:9:status:archived", "platform:tenant:archive", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.destroy", "DELETE", "/admin/platform/tenant", "tenant.data.delete", "platform", "tenant-data:delete:9:metadata", "platform:tenant:destroy", "TestPlatformTenantDestroyRequiresBoundApproval"),
		newSensitiveRoute("platform.tenant.export", "POST", "/admin/platform/tenant/{id}/exports", "tenant.data.export", "platform", "tenant-data:export:9:users:jsonl:sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "platform:tenant:export", "TestPlatformTenantExportRequiresBoundApproval"),
		newSensitiveRoute("platform.tenant.resume", "PUT", "/admin/platform/tenant/{id}/resume", "tenant.status.change", "platform", "tenant:9:status:active", "platform:tenant:resume", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.set-governance", "PUT", "/admin/platform/tenant/{id}/governance", "tenant.governance.change", "platform", "tenant-change:governance:e30", "platform:tenant:governance", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.set-permissions", "PUT", "/admin/platform/tenant/{id}/permissions", "tenant.permissions.sync", "platform", "tenant-change:permissions:e30", "platform:tenant:permissions", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.suspend", "PUT", "/admin/platform/tenant/{id}/suspend", "tenant.status.change", "platform", "tenant:9:status:suspended", "platform:tenant:suspend", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.tenant.update-plan", "PUT", "/admin/platform/tenant/{id}/plan", "tenant.plan.change", "platform", "tenant-change:plan:e30", "platform:tenant:updatePlan", "TestPlatformTenantSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.user.password", "PUT", "/admin/platform/user/password", "user.password.reset", "platform", "rbac:user:9:password:reset", "platform:user:password", "TestPlatformRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("platform.user.set-roles", "PUT", "/admin/platform/user/{id}/roles", "user.roles.sync", "platform", "rbac:user:9:roles:W10", "platform:user:setRole", "TestPlatformRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.role.set-permissions", "PUT", "/admin/role/{id}/permissions", "role.permissions.sync", "tenant", "rbac:role:9:permissions:W10", "permission:role:setMenu", "TestTenantRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.security.mfa.disable", "POST", "/admin/security/mfa/disable", "mfa.disable", "tenant", "mfa:user:9:disable", "security:mfa", "TestTenantMFADisableRequiresSensitiveEvidence"),
		newSensitiveRoute("tenant.sso-provider.create", "POST", "/admin/sso-provider", "sso.provider.secret.change", "tenant", "sso-provider:create", "security:ssoProvider:save", "TestTenantSSOProviderSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.sso-provider.delete", "DELETE", "/admin/sso-provider", "sso.provider.secret.change", "tenant", "sso-provider:delete:9", "security:ssoProvider:delete", "TestTenantSSOProviderSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.sso-provider.update", "PUT", "/admin/sso-provider/{id}", "sso.provider.secret.change", "tenant", "sso-provider:update:9", "security:ssoProvider:update", "TestTenantSSOProviderSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.user.password", "PUT", "/admin/user/password", "user.password.reset", "tenant", "rbac:user:9:password:reset", "permission:user:password", "TestTenantRBACSensitiveMutationsRequireEvidence"),
		newSensitiveRoute("tenant.user.set-roles", "PUT", "/admin/user/{id}/roles", "user.roles.sync", "tenant", "rbac:user:9:roles:W10", "permission:user:setRole", "TestTenantRBACSensitiveMutationsRequireEvidence"),
	}
}

func newSensitiveRoute(routeName, method, path, policyKey, domain, resource, permission, featureTest string) SensitiveOperationRouteContract {
	return SensitiveOperationRouteContract{
		RouteName: routeName, Method: method, Path: path, PolicyKey: policyKey,
		Domain: domain, Resource: resource, Permission: permission, FeatureTest: featureTest,
	}
}

func (r *SensitiveOperationPolicyRegistry) Prepare(
	_ context.Context,
	policyKey string,
	actorID uint64,
	tenantID uint64,
	input SensitiveOperationPrepareInput,
) (SensitiveOperationPlan, error) {
	policy, ok := r.Policy(policyKey)
	if !ok || actorID == 0 {
		return SensitiveOperationPlan{}, ErrSensitiveOperationPolicy
	}
	resource, err := policy.ResourceBuilder(input)
	if err != nil {
		return SensitiveOperationPlan{}, err
	}
	before := canonicalSnapshot(input.Before)
	after := canonicalSnapshot(input.After)
	digest, err := r.bindingDigestForPolicy()(policy.PolicyKey, policy.Scope, resource, tenantID, before, after)
	if err != nil {
		return SensitiveOperationPlan{}, err
	}
	return SensitiveOperationPlan{
		PolicyKey: policy.PolicyKey, Scope: policy.Scope, Resource: resource, BindingDigest: digest,
		ActorID: actorID, TenantID: tenantID, Before: before, After: after,
	}, nil
}

func (r *SensitiveOperationPolicyRegistry) bindingDigestForPolicy() func(string, string, string, uint64, []string, []string) (string, error) {
	if r != nil && r.bindingDigest != nil {
		return r.bindingDigest
	}
	return sensitiveOperationBindingDigest
}

func sensitiveOperationInputResource(input SensitiveOperationPrepareInput) (string, error) {
	resource := strings.TrimSpace(input.Resource)
	if resource == "" {
		return "", ErrSensitiveOperationPolicy
	}
	return resource, nil
}

func canonicalSnapshot(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[strings.TrimSpace(value)] = struct{}{}
	}
	delete(unique, "")
	canonical := make([]string, 0, len(unique))
	for value := range unique {
		canonical = append(canonical, value)
	}
	sort.Strings(canonical)
	return canonical
}

func sensitiveOperationBindingDigest(policyKey, scope, resource string, tenantID uint64, before, after []string) (string, error) {
	payload, err := json.Marshal(struct {
		PolicyKey string   `json:"policy_key"`
		Scope     string   `json:"scope"`
		Resource  string   `json:"resource"`
		TenantID  uint64   `json:"tenant_id"`
		Before    []string `json:"before"`
		After     []string `json:"after"`
	}{
		PolicyKey: strings.TrimSpace(policyKey), Scope: strings.TrimSpace(scope), Resource: strings.TrimSpace(resource),
		TenantID: tenantID,
		Before:   canonicalSnapshot(before), After: canonicalSnapshot(after),
	})
	if err != nil {
		return "", err
	}
	return sha256Hex(payload), nil
}

func newSensitiveOperationPolicy(policyKey, platformPermission, tenantPermission, action string) SensitiveOperationPolicy {
	return SensitiveOperationPolicy{
		PolicyKey: policyKey, Scope: policyKey, Permission: platformPermission, TenantPermission: tenantPermission,
		Action: action, RequiresReAuth: true, RequiresApproval: true, ResourceBuilder: sensitiveOperationInputResource,
	}
}
