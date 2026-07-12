package services

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goravel/app/models"
)

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
