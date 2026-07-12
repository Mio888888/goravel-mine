package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"goravel/app/models"
)

func TestTenantGovernanceDefaultMergesTenantQuotasAndFeatures(t *testing.T) {
	tenant := Tenant{
		ID:   7,
		Code: "acme",
		Quotas: models.JSONMap{
			"api_rate_per_minute": float64(120),
			"max_users":           float64(30),
		},
		Features: models.JSONMap{
			"modules": map[string]any{
				"scheduled-task": true,
			},
		},
	}

	policy := NewTenantGovernanceService().DefaultPolicy(tenant)

	if policy.TenantID != 7 || policy.TenantCode != "acme" {
		t.Fatalf("policy tenant = %#v", policy)
	}
	if policy.RateLimit.PerMinute != 120 {
		t.Fatalf("rate limit = %d", policy.RateLimit.PerMinute)
	}
	if policy.Quotas["max_users"] != float64(30) {
		t.Fatalf("quotas = %#v", policy.Quotas)
	}
	if enabled := policy.Modules["scheduled-task"]; !enabled {
		t.Fatalf("modules = %#v", policy.Modules)
	}
	if policy.Retention.AuditDays != 180 || !policy.DataExport.Enabled || !policy.DataDeletion.Enabled {
		t.Fatalf("defaults = %#v", policy)
	}
}

func TestTenantGovernanceSaveAndLoadPolicy(t *testing.T) {
	service := NewTenantGovernanceService()
	now := time.Now().UTC()
	service.evidenceHook = func(uint64) (models.TenantGovernanceEvidence, error) {
		return models.TenantGovernanceEvidence{
			TenantID: 7, URI: "s3://worm/acme/isolation.json", SHA256: "sha256:abc",
			VerifiedAt: now, ExpiresAt: now.Add(time.Hour),
		}, nil
	}
	policy := TenantGovernancePolicy{
		TenantID:   7,
		TenantCode: "acme",
		Modules: map[string]bool{
			"scheduled-task": false,
			"security":       true,
		},
		Quotas: models.JSONMap{
			"max_users": float64(50),
		},
		RateLimit:    TenantRateLimitPolicy{PerMinute: 240},
		Retention:    TenantRetentionPolicy{AuditDays: 365, DataDays: 730},
		DataExport:   TenantDataActionPolicy{Enabled: true, RequiresApproval: true},
		DataDeletion: TenantDataActionPolicy{Enabled: false, RequiresApproval: true},
		IsolationProof: TenantIsolationProof{
			Verified: true,
			Evidence: "s3://worm/acme/isolation.json",
			Digest:   "sha256:abc",
		},
	}

	if err := service.SavePolicy(policy); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	loaded, err := service.Policy(Tenant{ID: 7, Code: "acme"})
	if err != nil {
		t.Fatalf("Policy() error = %v", err)
	}

	if loaded.Modules["scheduled-task"] || !loaded.Modules["security"] {
		t.Fatalf("modules = %#v", loaded.Modules)
	}
	if loaded.RateLimit.PerMinute != 240 || loaded.Retention.DataDays != 730 {
		t.Fatalf("loaded = %#v", loaded)
	}
	if !loaded.IsolationProof.Verified || loaded.IsolationProof.Digest != "sha256:abc" {
		t.Fatalf("isolation proof = %#v", loaded.IsolationProof)
	}
}

func TestTenantGovernanceIsolationProofRequiresCurrentEvidence(t *testing.T) {
	now := time.Now().UTC()
	service := NewTenantGovernanceService()
	service.loadHook = func(uint64) (TenantGovernancePolicy, bool, error) {
		return TenantGovernancePolicy{
			TenantID: 7, TenantCode: "acme",
			IsolationProof: TenantIsolationProof{Verified: true, Evidence: "client://untrusted", Digest: "sha256:untrusted"},
		}, true, nil
	}
	service.evidenceHook = func(uint64) (models.TenantGovernanceEvidence, error) {
		return models.TenantGovernanceEvidence{
			TenantID: 7, URI: "s3://audit/isolation.json", SHA256: auditPruneTestDigest("isolation"),
			VerifiedAt: now, ExpiresAt: now.Add(time.Hour),
		}, nil
	}

	policy, err := service.Policy(Tenant{ID: 7, Code: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.RequireIsolationProof(); err != nil {
		t.Fatalf("RequireIsolationProof() error = %v", err)
	}
	if policy.IsolationProof.Evidence != "s3://audit/isolation.json" {
		t.Fatalf("proof = %#v", policy.IsolationProof)
	}

	service.evidenceHook = func(uint64) (models.TenantGovernanceEvidence, error) {
		return models.TenantGovernanceEvidence{}, ErrTenantGovernanceEvidenceExpired
	}
	policy, err = service.Policy(Tenant{ID: 7, Code: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.RequireIsolationProof(); !errors.Is(err, ErrTenantIsolationMissing) {
		t.Fatalf("RequireIsolationProof() error = %v", err)
	}
}

func TestTenantRuntimeAppliesTenantGovernancePolicy(t *testing.T) {
	tenant := Tenant{
		ID:   7001,
		Code: "runtime-governance",
		Quotas: models.JSONMap{
			"api_rate_per_minute": float64(30),
			"max_users":           float64(10),
		},
		Features: models.JSONMap{
			"modules": map[string]any{
				"scheduled-task": true,
				"security":       false,
			},
		},
	}
	cleanupTenantGovernancePolicy(t, tenant.ID)
	t.Cleanup(func() {
		cleanupTenantGovernancePolicy(t, tenant.ID)
	})

	err := NewTenantGovernanceService().SavePolicy(TenantGovernancePolicy{
		TenantID:   tenant.ID,
		TenantCode: tenant.Code,
		Modules: map[string]bool{
			"scheduled-task": false,
			"security":       true,
		},
		Quotas: models.JSONMap{
			"max_users": float64(24),
		},
		RateLimit:    TenantRateLimitPolicy{PerMinute: 180},
		Retention:    TenantRetentionPolicy{AuditDays: 365, DataDays: 730},
		DataExport:   TenantDataActionPolicy{Enabled: true, RequiresApproval: true},
		DataDeletion: TenantDataActionPolicy{Enabled: false, RequiresApproval: true},
	})
	if err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	runtime := NewTenantRuntimeService()
	quotas := runtime.EffectiveQuotas(tenant)
	features := runtime.EffectiveFeatures(tenant)
	modules, ok := features["modules"].(map[string]any)
	if !ok {
		t.Fatalf("features modules = %#v", features["modules"])
	}

	if jsonInt64(quotas, "api_rate_per_minute") != 180 || jsonInt64(quotas, "max_users") != 24 {
		t.Fatalf("runtime quotas = %#v", quotas)
	}
	if modules["scheduled-task"] != false || modules["security"] != true {
		t.Fatalf("runtime modules = %#v", modules)
	}
}

func TestTenantGovernanceRuntimeGuards(t *testing.T) {
	service := NewTenantGovernanceService()
	now := time.Now().UTC()
	service.evidenceHook = func(uint64) (models.TenantGovernanceEvidence, error) {
		return models.TenantGovernanceEvidence{
			TenantID: 12, URI: "s3://worm/guarded/isolation.json", SHA256: auditPruneTestDigest("guarded"),
			VerifiedAt: now, ExpiresAt: now.Add(time.Hour),
		}, nil
	}
	policy := TenantGovernancePolicy{
		TenantID:   12,
		TenantCode: "guarded",
		Modules: map[string]bool{
			"data-center": false,
			"security":    true,
		},
		DataExport:   TenantDataActionPolicy{Enabled: true, RequiresApproval: true},
		DataDeletion: TenantDataActionPolicy{Enabled: false, RequiresApproval: true},
		Retention:    TenantRetentionPolicy{AuditDays: 180, DataDays: 365},
		IsolationProof: TenantIsolationProof{
			Verified: true,
			Evidence: "s3://worm/guarded/isolation.json",
			Digest:   "sha256:guarded",
		},
	}

	if err := service.SavePolicy(policy); err != nil {
		t.Fatalf("SavePolicy() error = %v", err)
	}

	tenant := Tenant{ID: 12, Code: "guarded"}
	loaded, err := service.Policy(tenant)
	if err != nil {
		t.Fatalf("Policy() error = %v", err)
	}
	if loaded.ModuleEnabled("data-center") {
		t.Fatalf("data-center should be disabled: %#v", loaded.Modules)
	}
	if err := loaded.RequireModule("data-center"); err == nil {
		t.Fatal("RequireModule(data-center) nil error, want denial")
	}
	if err := loaded.RequireDataExportApproval(context.Background(), TenantDataActionApprovalRequest{}); err == nil {
		t.Fatal("RequireDataExportApproval(empty) nil error, want approval required")
	}
	if err := loaded.RequireDataDeletionApproval(context.Background(), TenantDataActionApprovalRequest{}); err == nil {
		t.Fatal("RequireDataDeletionApproval() nil error, want disabled")
	}
	if err := loaded.RequireRetentionPolicy(); err != nil {
		t.Fatalf("RequireRetentionPolicy() error = %v", err)
	}
	if err := loaded.RequireIsolationProof(); err != nil {
		t.Fatalf("RequireIsolationProof() error = %v", err)
	}
}

func TestTenantGovernancePatchPreservesOmittedPolicyFields(t *testing.T) {
	service := NewTenantGovernanceService()
	enabled := true
	disabled := false

	policy, err := service.PatchPolicy(Tenant{ID: 8, Code: "patch"}, TenantGovernancePatch{
		Modules: &map[string]bool{"scheduled-task": false},
		DataDeletion: &TenantDataActionPatch{
			Enabled:          &disabled,
			RequiresApproval: &enabled,
		},
	})
	if err != nil {
		t.Fatalf("PatchPolicy() error = %v", err)
	}

	if policy.DataExport.Enabled != true || policy.DataExport.RequiresApproval != true {
		t.Fatalf("data export defaults were not preserved: %#v", policy.DataExport)
	}
	if policy.DataDeletion.Enabled != false || policy.DataDeletion.RequiresApproval != true {
		t.Fatalf("data deletion explicit patch not applied: %#v", policy.DataDeletion)
	}
	if policy.Modules["scheduled-task"] {
		t.Fatalf("module patch not applied: %#v", policy.Modules)
	}
}

func TestTenantGovernanceRowPolicyPreservesDataActionDefaultsWhenColumnsEmpty(t *testing.T) {
	base := TenantGovernancePolicy{
		DataExport:   defaultTenantDataActionPolicy(),
		DataDeletion: defaultTenantDataActionPolicy(),
	}
	override, err := tenantGovernanceRow{
		TenantID:   10,
		TenantCode: "legacy",
		Modules:    `{"security":true}`,
		Quotas:     `{"max_users":10}`,
	}.policy()
	if err != nil {
		t.Fatalf("policy() error = %v", err)
	}

	policy := mergeTenantGovernancePolicy(base, override)

	if policy.DataExport.Enabled != true || policy.DataExport.RequiresApproval != true {
		t.Fatalf("data export defaults were not preserved: %#v", policy.DataExport)
	}
	if policy.DataDeletion.Enabled != true || policy.DataDeletion.RequiresApproval != true {
		t.Fatalf("data deletion defaults were not preserved: %#v", policy.DataDeletion)
	}
}

func TestTenantGovernanceRowPolicyAppliesExplicitDataActionFalse(t *testing.T) {
	base := TenantGovernancePolicy{
		DataExport:   defaultTenantDataActionPolicy(),
		DataDeletion: defaultTenantDataActionPolicy(),
	}
	override, err := tenantGovernanceRow{
		TenantID:     11,
		TenantCode:   "explicit",
		Modules:      `{}`,
		Quotas:       `{}`,
		DataDeletion: `{"enabled":false,"requires_approval":true}`,
	}.policy()
	if err != nil {
		t.Fatalf("policy() error = %v", err)
	}

	policy := mergeTenantGovernancePolicy(base, override)

	if policy.DataDeletion.Enabled != false || policy.DataDeletion.RequiresApproval != true {
		t.Fatalf("explicit data deletion policy not applied: %#v", policy.DataDeletion)
	}
}

func TestTenantGovernancePolicyReturnsLoadErrors(t *testing.T) {
	expected := errors.New("database unavailable")
	service := NewTenantGovernanceService()
	service.loadHook = func(uint64) (TenantGovernancePolicy, bool, error) {
		return TenantGovernancePolicy{}, false, expected
	}

	_, err := service.Policy(Tenant{ID: 9, Code: "broken"})

	if !errors.Is(err, expected) {
		t.Fatalf("Policy() error = %v, want %v", err, expected)
	}
}

func cleanupTenantGovernancePolicy(t *testing.T, tenantID uint64) {
	t.Helper()
	tenantGovernanceMemory.Lock()
	delete(tenantGovernanceMemory.items, tenantID)
	tenantGovernanceMemory.Unlock()

	service := NewTenantGovernanceService()
	if !service.hasGovernanceTable() {
		return
	}
	_, _ = OrmForConnection(PlatformConnection()).
		Query().
		Table("tenant_governance").
		Where("tenant_id", tenantID).
		Delete()
}
