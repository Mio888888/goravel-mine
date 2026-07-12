package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

func TestResolveSecretSensitivePlanKeepsSecretValuesOutOfBindingSnapshots(t *testing.T) {
	resource, before, after, err := resolveSecretSensitivePlan(
		context.Background(),
		"storage.secret.change",
		0,
		"storage-config:create",
	)

	require.NoError(t, err)
	require.Equal(t, "storage-config:create", resource)
	require.JSONEq(t, `{"resource_type":"storage_config","state":"absent"}`, before[0])
	require.JSONEq(t, `{"resource_type":"storage_config","rotation_intent":"set","secret_presence":"changed"}`, after[0])
	for _, snapshot := range append(before, after...) {
		require.NotContains(t, snapshot, "top-secret")
		require.NotContains(t, snapshot, "options")
		require.NotContains(t, snapshot, "sha256")
	}
}

func TestResolveSecretSensitivePlanRejectsNonCanonicalTarget(t *testing.T) {
	_, _, _, err := resolveSecretSensitivePlan(
		context.Background(),
		"storage.secret.change",
		0,
		"storage-config:create:top-secret",
	)

	require.ErrorIs(t, err, ErrSensitiveOperationPolicy)
}

func TestSSOPayloadChangesProtectedConfiguration(t *testing.T) {
	enabled, autoCreate := true, false
	existing := SSOProvider{
		Type:         "oidc",
		Audience:     "mine-admin",
		DiscoveryURL: "https://idp.example/.well-known/openid-configuration",
		JWKSURI:      "https://idp.example/jwks",
		ClientID:     "mine-client",
		Scope:        "openid profile email",
		ClientSecret: "existing-secret",
		Enabled:      true, EnablePKCE: true, EnableNonce: true, AutoCreate: false,
		RoleMapping:           models.JSONMap{"claim": "groups", "mapping": models.JSONMap{"ops": []any{"Admin"}}},
		DataPermissionMapping: models.JSONMap{"claim": "department", "default": "self"},
	}

	tests := []struct {
		name    string
		input   SSOProviderPayload
		changes bool
	}{
		{name: "discovery endpoint", input: SSOProviderPayload{DiscoveryURL: "https://other.example/discovery", JWKSURI: existing.JWKSURI}, changes: true},
		{name: "jwks endpoint", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: "https://other.example/jwks"}, changes: true},
		{name: "client secret", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, ClientSecret: "rotated"}, changes: true},
		{name: "audience", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.Audience = "other" }), changes: true},
		{name: "client id", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.ClientID = "other" }), changes: true},
		{name: "scope", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.Scope = "openid" }), changes: true},
		{name: "disable pkce", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.EnablePKCE = boolPointer(false) }), changes: true},
		{name: "disable nonce", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.EnableNonce = boolPointer(false) }), changes: true},
		{name: "role mapping", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: &enabled, AutoCreate: &autoCreate, RoleMapping: models.JSONMap{"default": []any{"SuperAdmin"}}, DataPermissionMapping: existing.DataPermissionMapping}, changes: true},
		{name: "data permission mapping", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: &enabled, AutoCreate: &autoCreate, RoleMapping: existing.RoleMapping, DataPermissionMapping: models.JSONMap{"default": "all"}}, changes: true},
		{name: "auto create", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: &enabled, AutoCreate: boolPointer(true), RoleMapping: existing.RoleMapping, DataPermissionMapping: existing.DataPermissionMapping}, changes: true},
		{name: "disable provider", input: SSOProviderPayload{DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, Enabled: boolPointer(false), AutoCreate: &autoCreate, RoleMapping: existing.RoleMapping, DataPermissionMapping: existing.DataPermissionMapping}, changes: true},
		{name: "empty secret preserves existing", input: ssoSensitiveTestPayload(existing, func(*SSOProviderPayload) {}), changes: false},
		{name: "display metadata only", input: ssoSensitiveTestPayload(existing, func(input *SSOProviderPayload) { input.DisplayName = "Renamed" }), changes: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.changes, ssoPayloadChangesProtectedConfiguration(test.input, existing))
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func ssoSensitiveTestPayload(existing SSOProvider, change func(*SSOProviderPayload)) SSOProviderPayload {
	input := SSOProviderPayload{
		Type: existing.Type, Enabled: boolPointer(existing.Enabled), Audience: existing.Audience,
		DiscoveryURL: existing.DiscoveryURL, JWKSURI: existing.JWKSURI, ClientID: existing.ClientID,
		Scope: existing.Scope, EnablePKCE: boolPointer(existing.EnablePKCE), EnableNonce: boolPointer(existing.EnableNonce),
		AutoCreate: boolPointer(existing.AutoCreate), RoleMapping: existing.RoleMapping, DataPermissionMapping: existing.DataPermissionMapping,
	}
	change(&input)
	return input
}

func TestStoragePayloadChangesProtectedConfiguration(t *testing.T) {
	existing := StorageConfig{
		Provider: "minio", Driver: storageDriverS3Compatible,
		Bucket: "uploads", Endpoint: "https://storage.example", Region: "us-east-1",
		AccessKey: "access", SecretKey: "secret", BaseURL: "https://cdn.example",
		PathPrefix: "tenant", IsDefault: false, Status: StorageConfigStatusEnabled,
		Options: models.JSONMap{"force_path_style": true},
	}

	tests := []struct {
		name    string
		input   StorageConfigPayload
		changes bool
	}{
		{name: "endpoint", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Endpoint = "https://other.example" }), changes: true},
		{name: "bucket", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Bucket = "archive" }), changes: true},
		{name: "region", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Region = "eu-west-1" }), changes: true},
		{name: "access key", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.AccessKey = "other" }), changes: true},
		{name: "base url", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.BaseURL = "https://other.example" }), changes: true},
		{name: "path prefix", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.PathPrefix = "archive" }), changes: true},
		{name: "default backend", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.IsDefault = true }), changes: true},
		{name: "status", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Status = 2 }), changes: true},
		{name: "options", input: storageSensitiveTestPayload(existing, func(input *StorageConfigPayload) { input.Options = models.JSONMap{"force_path_style": false} }), changes: true},
		{name: "empty secret preserves existing", input: storageSensitiveTestPayload(existing, func(*StorageConfigPayload) {}), changes: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.changes, storagePayloadChangesProtectedConfiguration(test.input, existing))
		})
	}
}

func storageSensitiveTestPayload(existing StorageConfig, change func(*StorageConfigPayload)) StorageConfigPayload {
	input := StorageConfigPayload{
		Provider: existing.Provider, Driver: existing.Driver, Bucket: existing.Bucket,
		Endpoint: existing.Endpoint, Region: existing.Region, AccessKey: existing.AccessKey,
		BaseURL: existing.BaseURL, PathPrefix: existing.PathPrefix, IsDefault: existing.IsDefault,
		Status: existing.Status, Options: existing.Options,
	}
	change(&input)
	return input
}
