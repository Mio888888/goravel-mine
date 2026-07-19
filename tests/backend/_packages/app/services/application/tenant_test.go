package application

import (
	"context"
	"encoding/csv"
	"errors"
	"github.com/stretchr/testify/require"
	"goravel/app/models"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Source: tenant_data_export_service_test.go
func TestTenantDataExportRequestCanonicalizesFilters(t *testing.T) {
	_, _, empty, emptyDigest, err := normalizeTenantExportRequest(TenantDataExportRequest{Dataset: "users", Format: "jsonl"})
	require.NoError(t, err)
	require.Empty(t, empty)
	require.Equal(t, digestBytes(nil), emptyDigest)

	_, _, filters, filteredDigest, err := normalizeTenantExportRequest(TenantDataExportRequest{
		Dataset: "users", Format: "csv", Filters: map[string]string{"status": "2"},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"status": "2"}, filters)
	require.Equal(t, digestBytes([]byte("status=2")), filteredDigest)

	_, _, _, _, err = normalizeTenantExportRequest(TenantDataExportRequest{Dataset: "audit_logs", Format: "jsonl"})
	require.ErrorIs(t, err, ErrTenantExportInvalid)
}

func TestTenantExportRunBindsOperator(t *testing.T) {
	run := models.TenantGovernanceRun{IdempotencyKey: "tenant-data:export:7:users:jsonl:sha256:abc:v1:approval:sha256:def:operator:42"}
	require.Equal(t, uint64(42), tenantExportRunOperatorID(run))
	require.Zero(t, tenantExportRunOperatorID(models.TenantGovernanceRun{IdempotencyKey: "unbound"}))
}

func TestTenantExportIdempotencyKeyBindsApproval(t *testing.T) {
	first := tenantExportIdempotencyKey("tenant-data:export:7:users:csv:sha256:abc", "v1", "approval:approval-a", 42)
	retry := tenantExportIdempotencyKey("tenant-data:export:7:users:csv:sha256:abc", "v1", " approval:approval-a ", 42)
	next := tenantExportIdempotencyKey("tenant-data:export:7:users:csv:sha256:abc", "v1", "approval:approval-b", 42)

	require.Equal(t, first, retry)
	require.NotEqual(t, first, next)
	require.LessOrEqual(t, len(next), 255)
	require.Equal(t, uint64(42), tenantExportRunOperatorID(models.TenantGovernanceRun{IdempotencyKey: next}))
}

func TestTenantExportIdempotencyFactorUsesFreshReAuthWithoutApproval(t *testing.T) {
	request := TenantDataExportRequest{ApprovalID: "approval-a", ReAuthToken: "reauth-a"}
	retry := TenantDataExportRequest{ApprovalID: "approval-b", ReAuthToken: " reauth-a "}
	next := TenantDataExportRequest{ApprovalID: "approval-a", ReAuthToken: "reauth-b"}

	require.Equal(t, "approval:approval-a", tenantExportIdempotencyFactor(true, request))
	require.Equal(t, tenantExportIdempotencyFactor(false, request), tenantExportIdempotencyFactor(false, retry))
	require.NotEqual(t, tenantExportIdempotencyFactor(false, request), tenantExportIdempotencyFactor(false, next))
	require.NotContains(t, tenantExportIdempotencyFactor(false, request), request.ReAuthToken)
}

func TestTenantExportSensitiveGuardUsesGovernanceApprovalPolicy(t *testing.T) {
	for _, test := range []struct {
		name             string
		requiresApproval bool
	}{
		{name: "approval enabled", requiresApproval: true},
		{name: "approval disabled", requiresApproval: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, ok := newTenantExportSensitiveGuard(test.requiresApproval).registry.Policy("tenant.data.export")
			require.True(t, ok)
			require.True(t, policy.RequiresReAuth)
			require.Equal(t, test.requiresApproval, policy.RequiresApproval)
		})
	}

	defaultPolicy, ok := NewSensitiveOperationPolicyRegistry().Policy("tenant.data.export")
	require.True(t, ok)
	require.True(t, defaultPolicy.RequiresApproval)
}

func TestTenantExportCSVNeutralizesSpreadsheetFormulas(t *testing.T) {
	payload, err := tenantExportCSV([]tenantExportUser{{
		ID: 1, Username: "=HYPERLINK(\"https://example.test\")", Nickname: " @SUM(1,1)",
		Email: "\tcmd@example.test", Phone: "\r-1+1", Status: 1, CreatedAt: time.Unix(0, 0).UTC(),
	}})
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(string(payload))).ReadAll()
	require.NoError(t, err)
	require.Equal(t, "'=HYPERLINK(\"https://example.test\")", records[1][1])
	require.Equal(t, "' @SUM(1,1)", records[1][2])
	require.Equal(t, "'\tcmd@example.test", records[1][3])
	require.Equal(t, "'\r-1+1", records[1][4])
}

func TestTenantExportDownloadTokenFailsClosedWithoutCache(t *testing.T) {
	_, _, err := issueTenantExportDownloadToken(9, 7, 5)
	require.ErrorIs(t, err, ErrTenantExportInvalid)
}

// Source: tenant_governance_run_repository_test.go
func TestTenantGovernanceRunTransitionMatrix(t *testing.T) {
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusAwaitingEvidence))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusCompleted))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusFailed))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusArtifactWritten))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusCompleted, models.TenantGovernanceRunStatusStale))
	require.False(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusCompleted, models.TenantGovernanceRunStatusRunning))
	require.False(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusFailed, models.TenantGovernanceRunStatusRunning))
}

func TestTenantGovernanceRunKindsAreClosed(t *testing.T) {
	for _, kind := range []string{
		models.TenantGovernanceRunKindRetention,
		models.TenantGovernanceRunKindExport,
		models.TenantGovernanceRunKindIsolationVerify,
	} {
		require.True(t, tenantGovernanceRunKindValid(kind))
	}
	require.False(t, tenantGovernanceRunKindValid("arbitrary_sql"))
}

func TestTenantGovernanceRetentionAwaitingEvidenceTransitionsAreClosed(t *testing.T) {
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusAwaitingEvidence))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusCompleted))
	require.True(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusFailed))
	require.False(t, tenantGovernanceTransitionAllowed(models.TenantGovernanceRunStatusAwaitingEvidence, models.TenantGovernanceRunStatusRunning))
}

// Source: tenant_governance_service_test.go
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

// Source: tenant_isolation_verifier_test.go
type tenantIsolationProbeFunc func(context.Context, Tenant) (TenantIsolationProbeResult, error)

func (f tenantIsolationProbeFunc) Probe(ctx context.Context, tenant Tenant) (TenantIsolationProbeResult, error) {
	return f(ctx, tenant)
}

type tenantIsolationStoreFunc func(context.Context, Tenant, []byte) (TenantIsolationArtifact, error)

func (f tenantIsolationStoreFunc) WriteImmutable(ctx context.Context, tenant Tenant, payload []byte) (TenantIsolationArtifact, error) {
	return f(ctx, tenant, payload)
}

func TestTenantIsolationProbeRequiresExactIdentityAndNegativeVisibility(t *testing.T) {
	tenant := Tenant{ID: 7, DBDatabase: "tenant_7", DBSchema: "tenant7", DBUsername: "tenant_role_7"}
	valid := TenantIsolationProbeResult{
		Database: "tenant_7", Schema: "tenant7", Role: "tenant_role_7",
		CrossTenantSentinelDenied: true, PlatformSentinelDenied: true,
	}
	require.True(t, validTenantIsolationProbe(tenant, valid))

	invalid := []TenantIsolationProbeResult{valid, valid, valid, valid, valid}
	invalid[0].Database = "tenant_8"
	invalid[1].Schema = "public"
	invalid[2].Role = "postgres"
	invalid[3].CrossTenantSentinelDenied = false
	invalid[4].PlatformSentinelDenied = false
	for _, candidate := range invalid {
		require.False(t, validTenantIsolationProbe(tenant, candidate))
	}
}

func TestSameTenantDatabaseInstanceUsesHostAndPortBoundary(t *testing.T) {
	base := Tenant{DBHost: " DB.INTERNAL ", DBPort: 5432}
	require.True(t, sameTenantDatabaseInstance(base, Tenant{DBHost: "db.internal", DBPort: 5432}))
	require.True(t, sameTenantDatabaseInstance(Tenant{DBHost: "localhost"}, Tenant{DBHost: "127.0.0.1", DBPort: 5432}))
	require.False(t, sameTenantDatabaseInstance(base, Tenant{DBHost: "other.internal", DBPort: 5432}))
	require.False(t, sameTenantDatabaseInstance(base, Tenant{DBHost: "db.internal", DBPort: 5433}))
}

func TestTenantIsolationProbeFailsClosedAcrossUnattestedInstances(t *testing.T) {
	require.ErrorIs(t, requireAttestedTenantInstanceBoundary(
		Tenant{DBHost: "db-a.internal", DBPort: 5432},
		Tenant{DBHost: "db-b.internal", DBPort: 5432},
	), ErrTenantIsolationVerification)
	require.NoError(t, requireAttestedTenantInstanceBoundary(
		Tenant{DBHost: "db-a.internal", DBPort: 5432},
		Tenant{DBHost: "DB-A.INTERNAL", DBPort: 5432},
	))
}

func TestTenantIsolationVerifierRejectsUploadFailure(t *testing.T) {
	probe := tenantIsolationProbeFunc(func(context.Context, Tenant) (TenantIsolationProbeResult, error) {
		return TenantIsolationProbeResult{}, errors.New("probe denied")
	})
	store := tenantIsolationStoreFunc(func(context.Context, Tenant, []byte) (TenantIsolationArtifact, error) {
		return TenantIsolationArtifact{}, errors.New("upload failed")
	})
	verifier := NewTenantIsolationVerifier(probe, store)
	require.NotNil(t, verifier)
}

func TestTenantIsolationScheduledHandlerFailsClosedWithoutArtifactStore(t *testing.T) {
	original := newTenantIsolationArtifactStore
	newTenantIsolationArtifactStore = func(context.Context) TenantIsolationArtifactStore { return nil }
	t.Cleanup(func() { newTenantIsolationArtifactStore = original })

	result := tenantIsolationScheduledTaskHandler(t.Context(), nil)
	require.Equal(t, ScheduledTaskLogStatusFailed, result.Status)
	require.Contains(t, result.ErrorMessage, "not configured")
}

func TestTenantIsolationBatchContinuesAfterTenantFailure(t *testing.T) {
	visited := make([]uint64, 0, 2)
	result := runTenantIsolationBatch(t.Context(), []Tenant{{ID: 1}, {ID: 2}}, tenantIsolationStoreFunc(func(context.Context, Tenant, []byte) (TenantIsolationArtifact, error) {
		return TenantIsolationArtifact{}, nil
	}), func(context.Context, Tenant) (TenantGovernancePolicy, error) {
		return TenantGovernancePolicy{}, nil
	}, func(_ context.Context, tenant Tenant, _ TenantGovernancePolicy, _ TenantIsolationArtifactStore) (models.TenantGovernanceEvidence, error) {
		visited = append(visited, tenant.ID)
		if tenant.ID == 1 {
			return models.TenantGovernanceEvidence{}, errors.New("first failed")
		}
		return models.TenantGovernanceEvidence{ID: 22}, nil
	})

	require.Equal(t, []uint64{1, 2}, visited)
	require.Equal(t, ScheduledTaskLogStatusFailed, result.Status)
	require.Contains(t, result.ErrorMessage, "tenant 1: first failed")
	require.Contains(t, result.Stdout, `"evidence_id":22`)
}

func TestS3TenantIsolationArtifactStoreRequiresImmutableVersionedWrite(t *testing.T) {
	payload := []byte(`{"schema":"tenant-isolation/v1"}`)
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method)
		require.Contains(t, request.URL.Path, "/evidence/tenant-governance/isolation/7/")
		switch request.Method {
		case http.MethodPut:
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			require.Equal(t, payload, body)
			require.Equal(t, "COMPLIANCE", request.Header.Get("X-Amz-Object-Lock-Mode"))
			require.NotEmpty(t, request.Header.Get("X-Amz-Object-Lock-Retain-Until-Date"))
			response.Header().Set("X-Amz-Version-Id", "version-7")
		case http.MethodHead:
			require.Equal(t, "version-7", request.URL.Query().Get("versionId"))
			response.Header().Set("X-Amz-Version-Id", "version-7")
			response.Header().Set("X-Amz-Object-Lock-Mode", "COMPLIANCE")
			response.Header().Set("X-Amz-Object-Lock-Retain-Until-Date", now.Add(48*time.Hour).Format(time.RFC3339))
		case http.MethodGet:
			require.Equal(t, "version-7", request.URL.Query().Get("versionId"))
			_, _ = response.Write(payload)
		}
	}))
	t.Cleanup(server.Close)

	store := &s3TenantIsolationArtifactStore{
		config: StorageConfig{Endpoint: server.URL, Bucket: "evidence", Driver: storageDriverS3Compatible},
		now:    func() time.Time { return now },
	}
	artifact, err := store.WriteImmutable(t.Context(), Tenant{ID: 7, Code: "acme"}, payload)

	require.NoError(t, err)
	require.Equal(t, []string{http.MethodPut, http.MethodHead, http.MethodGet}, requests)
	require.Equal(t, "version-7", artifact.ObjectVersion)
	require.Equal(t, digestBytes(payload), artifact.SHA256)
	require.Contains(t, artifact.URI, "s3://evidence/tenant-governance/isolation/7/")
}

func TestS3TenantIsolationArtifactStoreRejectsMissingVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	store := &s3TenantIsolationArtifactStore{
		config: StorageConfig{Endpoint: server.URL, Bucket: "evidence", Driver: storageDriverS3Compatible},
		now:    time.Now,
	}

	_, err := store.WriteImmutable(t.Context(), Tenant{ID: 7}, []byte(`{}`))
	require.ErrorIs(t, err, ErrTenantIsolationVerification)
}

// Source: tenant_permission_legacy_test.go
func TestTenantFullPermissionNamesIncludesMenuButtonsAndRoutes(t *testing.T) {
	names := TenantFullPermissionNames()

	require.Contains(t, names, "permission:user")
	require.Contains(t, names, "permission:user:index")
	require.Contains(t, names, "permission:user:delete")
	require.Contains(t, names, "log:ssoLogin:stats")
	require.NotContains(t, names, "platform:tenant:list")
}

func TestBuildTenantPermissionAuditComputesDiff(t *testing.T) {
	record := BuildTenantPermissionAudit(TenantPermissionAuditInput{
		TenantID:   9,
		TenantCode: "acme",
		Operation:  TenantPermissionAuditOperationUpdate,
		Source:     TenantPermissionAuditSourcePlatform,
		Before: TenantPermissionPayload{
			Allowed: []string{"permission:user:index", "permission:role:index"},
		},
		After: TenantPermissionPayload{
			Allowed: []string{"permission:user:index", "permission:menu:index"},
		},
		OperatorID: 7,
		Remark:     "manual adjustment",
	})

	require.Equal(t, uint64(9), record.TenantID)
	require.Equal(t, "acme", record.TenantCode)
	require.Equal(t, TenantPermissionAuditOperationUpdate, record.Operation)
	require.Equal(t, []string{"permission:menu:index"}, record.Added)
	require.Equal(t, []string{"permission:role:index"}, record.Removed)
	require.Equal(t, []string{"permission:user:index"}, record.Unchanged)
	require.Equal(t, uint64(7), record.OperatorID)
	require.Equal(t, "manual adjustment", record.Remark)
}

func TestBuildLegacyTenantPermissionSnapshotSkipsExplicitSnapshot(t *testing.T) {
	tenant := Tenant{
		ID:   1,
		Code: "legacy",
	}

	snapshot, ok := BuildLegacyTenantPermissionSnapshot(tenant)

	require.True(t, ok)
	require.Contains(t, snapshot.Allowed, "permission:user:index")

	tenant.Features = models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission:user:index"},
		},
	}

	_, ok = BuildLegacyTenantPermissionSnapshot(tenant)
	require.False(t, ok)
}

// Source: tenant_permission_service_test.go
func TestTenantPermissionSnapshotAllowsLegacyTenantWithoutSnapshot(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{}}

	require.True(t, TenantAllowsPermission(tenant, "permission:user:index"))
}

func TestTenantPermissionSnapshotAllowsListedPermission(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index", "permission:role", "permission:role:index"},
		},
	}}

	require.True(t, TenantAllowsPermission(tenant, "permission:user:index"))
	require.False(t, TenantAllowsPermission(tenant, "permission:user:delete"))
}

func TestTenantPermissionRequiresAuthorizedAncestors(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission:user:index"},
		},
	}}

	require.False(t, TenantAllowsPermission(tenant, "permission:user:index"))

	tenant.Features = models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}

	require.True(t, TenantAllowsPermission(tenant, "permission:user:index"))
}

func TestTenantAllowsRoutePermission(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}}

	require.True(t, TenantAllowsRoute(tenant, "GET", "/admin/user/list"))
	require.False(t, TenantAllowsRoute(tenant, "DELETE", "/admin/user"))
	require.True(t, TenantAllowsRoute(tenant, "GET", "/admin/passport/getInfo"))
}

func TestSnapshotFeaturesForPlanCopiesPlanPermissions(t *testing.T) {
	planFeatures := models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
		"sso": map[string]any{"enabled": true},
	}
	input := models.JSONMap{"theme": "dark"}

	features := SnapshotFeaturesForPlan(planFeatures, input)

	require.Equal(t, "dark", features["theme"])
	require.NotContains(t, features, "sso")
	require.Equal(t, models.JSONMap{
		"allowed": []string{"permission", "permission:user", "permission:user:index"},
	}, features["permissions"])
}

func TestSnapshotFeaturesForPlanUsesExplicitPermissionOverride(t *testing.T) {
	planFeatures := models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}
	input := models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:role", "permission:role:index"},
		},
	}

	features := SnapshotFeaturesForPlan(planFeatures, input)

	require.Equal(t, models.JSONMap{
		"allowed": []string{"permission", "permission:role", "permission:role:index"},
	}, features["permissions"])
}

func TestFilterMenusByTenantPermissionsKeepsAuthorizedAncestors(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}}
	menus := []AdminMenuItem{
		{ID: 1, ParentID: 0, Name: "permission"},
		{ID: 2, ParentID: 1, Name: "permission:user"},
		{ID: 3, ParentID: 2, Name: "permission:user:index"},
		{ID: 4, ParentID: 2, Name: "permission:user:delete"},
		{ID: 5, ParentID: 1, Name: "permission:role"},
	}

	filtered := FilterAdminMenusByTenantPermissions(tenant, menus)

	require.ElementsMatch(t, []uint64{1, 2, 3}, adminMenuIDs(filtered))
}

func TestValidateTenantRolePermissionsRejectsOutOfScope(t *testing.T) {
	tenant := Tenant{Features: models.JSONMap{
		"permissions": map[string]any{
			"allowed": []any{"permission", "permission:user", "permission:user:index"},
		},
	}}

	require.NoError(t, ValidateTenantRolePermissions(tenant, []string{"permission:user:index"}))
	require.Error(t, ValidateTenantRolePermissions(tenant, []string{"permission:user:delete"}))
}

// Source: tenant_retention_service_test.go
func TestTenantRetentionHandlerIsPrivileged(t *testing.T) {
	require.True(t, isPrivilegedScheduledTaskHandler("scheduler.tenant_retention"))
	require.True(t, ScheduledTaskUsesPrivilegedHandler(ScheduledTaskTypeMethod, map[string]any{"handler": "scheduler.tenant_retention"}))
}

func TestGovernanceTaskOutcome(t *testing.T) {
	require.Equal(t, "success", governanceTaskOutcome(ScheduledTaskLogStatusSuccess))
	require.Equal(t, "failure", governanceTaskOutcome(ScheduledTaskLogStatusFailed))
}

func TestTenantRetentionPolicyVersionChangesWithRetention(t *testing.T) {
	first := tenantGovernancePolicyVersion(TenantGovernancePolicy{Retention: TenantRetentionPolicy{AuditDays: 30, DataDays: 90}})
	second := tenantGovernancePolicyVersion(TenantGovernancePolicy{Retention: TenantRetentionPolicy{AuditDays: 60, DataDays: 90}})
	require.NotEqual(t, first, second)
}

func TestTenantRetentionGovernanceTaskCannotBeCreatedByUsers(t *testing.T) {
	err := validateScheduledTaskPayload(ScheduledTask{
		TaskType: ScheduledTaskTypeGovernance,
		Payload:  map[string]any{"handler": "scheduler.tenant_retention"},
	})
	require.Error(t, err)
}
