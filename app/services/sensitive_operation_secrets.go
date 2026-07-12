package services

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"goravel/app/models"
)

func resolveSecretSensitivePlan(
	ctx context.Context,
	policyKey string,
	tenantID uint64,
	selector string,
) (string, []string, []string, error) {
	resourceType, action, err := secretSensitiveTarget(policyKey, selector)
	if err != nil {
		return "", nil, nil, err
	}
	before, after, err := secretSensitiveSnapshots(resourceType, action)
	if err != nil {
		return "", nil, nil, err
	}
	if action != "create" {
		ids := strings.Split(strings.TrimPrefix(selector, strings.ReplaceAll(resourceType, "_", "-")+":"+action+":"), ",")
		metadata, metadataErr := secretRotationMetadata(ctx, policyKey, tenantID, ids)
		if metadataErr != nil {
			return "", nil, nil, metadataErr
		}
		before = metadata
	}
	return selector, before, after, nil
}

func secretRotationMetadata(ctx context.Context, policyKey string, tenantID uint64, ids []string) ([]string, error) {
	connection, table := PlatformConnection(), "storage_config"
	columns := []string{"id", "secret_key_rotated_at"}
	if policyKey == "sso.provider.secret.change" {
		table = "sso_provider"
		columns = []string{"id", "jwt_secret_rotated_at", "client_secret_rotated_at", "updated_at"}
		if tenantID == 0 {
			return nil, ErrSensitiveOperationPolicy
		}
		connection = TenantConnectionFromContext(ctx)
	}
	rows := make([]map[string]any, 0)
	if err := OrmForConnectionWithContext(ctx, connection).Query().Table(table).Select(strings.Join(columns, ",")).WhereIn("id", stringAny(ids)).OrderBy("id").Get(&rows); err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, ErrSensitiveOperationPolicy
	}
	return sensitiveSnapshots(rows)
}

func secretSensitiveTarget(policyKey, selector string) (string, string, error) {
	policyKey = strings.TrimSpace(policyKey)
	parts := strings.Split(strings.TrimSpace(selector), ":")
	if len(parts) < 2 {
		return "", "", ErrSensitiveOperationPolicy
	}
	resourceType := parts[0]
	if (policyKey == "sso.provider.secret.change" && resourceType != "sso-provider") ||
		(policyKey == "storage.secret.change" && resourceType != "storage-config") {
		return "", "", ErrSensitiveOperationPolicy
	}
	action := parts[1]
	if action == "create" && len(parts) == 2 {
		return strings.ReplaceAll(resourceType, "-", "_"), action, nil
	}
	if (action != "update" && action != "delete") || len(parts) != 3 {
		return "", "", ErrSensitiveOperationPolicy
	}
	ids := strings.Split(parts[2], ",")
	if action == "update" && len(ids) != 1 {
		return "", "", ErrSensitiveOperationPolicy
	}
	for _, value := range ids {
		if id, err := strconv.ParseUint(value, 10, 64); err != nil || id == 0 {
			return "", "", ErrSensitiveOperationPolicy
		}
	}
	return strings.ReplaceAll(resourceType, "-", "_"), action, nil
}

func secretDeleteSelector(prefix string, ids []uint64) (string, error) {
	canonical := append([]uint64(nil), ids...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i] < canonical[j] })
	parts := make([]string, 0, len(canonical))
	var previous uint64
	for _, id := range canonical {
		if id == 0 {
			return "", ErrSensitiveOperationPolicy
		}
		if id != previous {
			parts = append(parts, strconv.FormatUint(id, 10))
			previous = id
		}
	}
	if len(parts) == 0 {
		return "", ErrSensitiveOperationPolicy
	}
	return prefix + ":delete:" + strings.Join(parts, ","), nil
}

func executeSecretSensitive(ctx context.Context, policyKey string, actorID, tenantID uint64, selector string, evidence SensitiveOperationEvidence, mutate func() error) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(ctx, policyKey, actorID, tenantID, SensitiveOperationPlanSelector{Resource: selector})
	if err != nil {
		return err
	}
	return guard.Execute(ctx, plan, evidence, mutate)
}

func (s *SSOProviderService) CreateSensitive(input SSOProviderPayload, operatorID, tenantID uint64, evidence SensitiveOperationEvidence) (SSOProvider, error) {
	if !ssoPayloadChangesProtectedConfiguration(input, SSOProvider{}) {
		return s.Create(input, operatorID)
	}
	var result SSOProvider
	err := executeSecretSensitive(s.ctx, "sso.provider.secret.change", operatorID, tenantID, "sso-provider:create", evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Create(input, operatorID)
		return mutationErr
	})
	return result, err
}

func (s *SSOProviderService) UpdateSensitive(id uint64, input SSOProviderPayload, operatorID, tenantID uint64, evidence SensitiveOperationEvidence) (SSOProvider, error) {
	var existing SSOProvider
	if err := s.orm().Query().Table("sso_provider").Where("id", id).First(&existing); err != nil {
		return SSOProvider{}, err
	}
	if !ssoPayloadChangesProtectedConfiguration(input, existing) {
		return s.Update(id, input, operatorID)
	}
	var result SSOProvider
	err := executeSecretSensitive(s.ctx, "sso.provider.secret.change", operatorID, tenantID, "sso-provider:update:"+strconv.FormatUint(id, 10), evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Update(id, input, operatorID)
		return mutationErr
	})
	return result, err
}

func (s *SSOProviderService) DeleteSensitive(ids []uint64, operatorID, tenantID uint64, evidence SensitiveOperationEvidence) error {
	selector, err := secretDeleteSelector("sso-provider", ids)
	if err != nil {
		return err
	}
	return executeSecretSensitive(s.ctx, "sso.provider.secret.change", operatorID, tenantID, selector, evidence, func() error {
		return s.Delete(ids)
	})
}

func ssoPayloadChangesProtectedConfiguration(input SSOProviderPayload, existing SSOProvider) bool {
	next := input.Provider()
	return secretChanged(input.JWTSecret, existing.JWTSecret) ||
		secretChanged(input.ClientSecret, existing.ClientSecret) ||
		trimmedChanged(input.Issuer, existing.Issuer) ||
		trimmedChanged(input.DiscoveryURL, existing.DiscoveryURL) ||
		trimmedChanged(input.AuthorizationEndpoint, existing.AuthorizationEndpoint) ||
		trimmedChanged(input.TokenEndpoint, existing.TokenEndpoint) ||
		trimmedChanged(input.UserinfoEndpoint, existing.UserinfoEndpoint) ||
		trimmedChanged(input.JWKSURI, existing.JWKSURI) ||
		trimmedChanged(input.JWKSJSON, existing.JWKSJSON) ||
		trimmedChanged(input.Type, existing.Type) ||
		trimmedChanged(input.Audience, existing.Audience) ||
		trimmedChanged(input.ClientID, existing.ClientID) ||
		trimmedChanged(input.Scope, existing.Scope) ||
		trimmedChanged(input.RedirectURI, existing.RedirectURI) ||
		trimmedChanged(input.SAMLEntrypoint, existing.SAMLEntrypoint) ||
		trimmedChanged(input.SAMLEntityID, existing.SAMLEntityID) ||
		trimmedChanged(input.SAMLCertificate, existing.SAMLCertificate) ||
		next.Enabled != existing.Enabled ||
		next.EnablePKCE != existing.EnablePKCE ||
		next.EnableNonce != existing.EnableNonce ||
		next.AutoCreate != existing.AutoCreate ||
		!jsonMapsEqual(next.RoleMapping, existing.RoleMapping) ||
		!jsonMapsEqual(next.DataPermissionMapping, existing.DataPermissionMapping)
}

func jsonMapsEqual(left, right models.JSONMap) bool {
	leftJSON, leftErr := json.Marshal(nullIfEmpty(left))
	rightJSON, rightErr := json.Marshal(nullIfEmpty(right))
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func (s *StorageConfigService) CreateSensitive(input StorageConfigPayload, operatorID uint64, evidence SensitiveOperationEvidence) (StorageConfig, error) {
	if !storagePayloadChangesProtectedConfiguration(input, StorageConfig{}) {
		return s.Create(input, operatorID)
	}
	var result StorageConfig
	err := executeSecretSensitive(s.ctx, "storage.secret.change", operatorID, 0, "storage-config:create", evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Create(input, operatorID)
		return mutationErr
	})
	return result, err
}

func (s *StorageConfigService) UpdateSensitive(id uint64, input StorageConfigPayload, operatorID uint64, evidence SensitiveOperationEvidence) (StorageConfig, error) {
	existing, err := s.find(id)
	if err != nil {
		return StorageConfig{}, err
	}
	if !storagePayloadChangesProtectedConfiguration(input, existing) {
		return s.Update(id, input, operatorID)
	}
	var result StorageConfig
	err = executeSecretSensitive(s.ctx, "storage.secret.change", operatorID, 0, "storage-config:update:"+strconv.FormatUint(id, 10), evidence, func() error {
		var mutationErr error
		result, mutationErr = s.Update(id, input, operatorID)
		return mutationErr
	})
	return result, err
}

func storagePayloadChangesProtectedConfiguration(input StorageConfigPayload, existing StorageConfig) bool {
	next := input.StorageConfig()
	return trimmedChanged(next.Provider, existing.Provider) ||
		trimmedChanged(next.Driver, existing.Driver) ||
		trimmedChanged(next.Bucket, existing.Bucket) ||
		trimmedChanged(next.Endpoint, existing.Endpoint) ||
		trimmedChanged(next.Region, existing.Region) ||
		trimmedChanged(next.AccessKey, existing.AccessKey) ||
		secretChanged(next.SecretKey, existing.SecretKey) ||
		trimmedChanged(next.BaseURL, existing.BaseURL) ||
		trimmedChanged(next.PathPrefix, existing.PathPrefix) ||
		next.IsDefault != existing.IsDefault ||
		next.Status != existing.Status ||
		!jsonMapsEqual(next.Options, existing.Options)
}

func secretChanged(input, existing string) bool {
	return strings.TrimSpace(input) != "" && input != existing
}

func trimmedChanged(input, existing string) bool {
	return strings.TrimSpace(input) != strings.TrimSpace(existing)
}

func (s *StorageConfigService) DeleteSensitive(ids []uint64, operatorID uint64, evidence SensitiveOperationEvidence) error {
	selector, err := secretDeleteSelector("storage-config", ids)
	if err != nil {
		return err
	}
	return executeSecretSensitive(s.ctx, "storage.secret.change", operatorID, 0, selector, evidence, func() error {
		return s.Delete(ids)
	})
}

func secretSensitiveSnapshots(resourceType, action string) ([]string, []string, error) {
	before := map[string]string{"resource_type": resourceType}
	after := map[string]string{"resource_type": resourceType}
	switch action {
	case "create":
		before["state"] = "absent"
		after["secret_presence"] = "changed"
		after["rotation_intent"] = "set"
	case "update":
		before["state"] = "existing"
		after["secret_presence"] = "changed"
		after["rotation_intent"] = "rotate"
	case "delete":
		before["state"] = "existing"
		after["secret_presence"] = "removed"
		after["rotation_intent"] = "remove"
	default:
		return nil, nil, ErrSensitiveOperationPolicy
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return nil, nil, err
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return nil, nil, err
	}
	return []string{string(beforeJSON)}, []string{string(afterJSON)}, nil
}
