package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
	"goravel/tests/backend/testcase"
)

type TenantGovernanceRunTestSuite struct {
	suite.Suite
	tests.TestCase
}

type isolationProbeStub struct {
	result services.TenantIsolationProbeResult
	err    error
}

func (s isolationProbeStub) Probe(context.Context, services.Tenant) (services.TenantIsolationProbeResult, error) {
	return s.result, s.err
}

type isolationStoreStub struct {
	artifact services.TenantIsolationArtifact
	err      error
}

func (s isolationStoreStub) WriteImmutable(context.Context, services.Tenant, []byte) (services.TenantIsolationArtifact, error) {
	return s.artifact, s.err
}

func TestTenantGovernanceRunTestSuite(t *testing.T) {
	suite.Run(t, new(TenantGovernanceRunTestSuite))
}

func (s *TenantGovernanceRunTestSuite) SetupTest() {
	s.RefreshDatabase()
}

func (s *TenantGovernanceRunTestSuite) TestRunIdempotencyEvidenceExpiryAndAppendOnly() {
	now := time.Now().UTC()
	repository := services.NewTenantGovernanceRunRepository().WithContext(s.T().Context())
	input := services.TenantGovernanceRunCreate{
		TenantID: 7, TenantCode: "acme", Kind: models.TenantGovernanceRunKindIsolationVerify,
		IdempotencyKey: "7:v1:2026-07-11:isolation_verify", PolicyVersion: "v1",
	}
	run, created, err := repository.CreateOrGetRun(input)
	require.NoError(s.T(), err)
	require.True(s.T(), created)
	duplicate, created, err := repository.CreateOrGetRun(input)
	require.NoError(s.T(), err)
	require.False(s.T(), created)
	require.Equal(s.T(), run.ID, duplicate.ID)
	require.NoError(s.T(), repository.Transition(run.ID, models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, ""))

	evidence, err := repository.AttachEvidence(run, services.TenantGovernanceEvidenceInput{
		URI: "s3://audit/isolation.json", ObjectVersion: "v1", SHA256: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		VerifiedAt: now, ExpiresAt: now.Add(time.Hour), Metadata: map[string]any{"schema": "tenant-isolation/v1"},
	})
	require.NoError(s.T(), err)
	_, err = repository.AttachEvidence(run, services.TenantGovernanceEvidenceInput{
		URI: "s3://audit/replacement.json", ObjectVersion: "v2", SHA256: evidence.SHA256,
		VerifiedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	require.ErrorIs(s.T(), err, services.ErrTenantGovernanceEvidenceExists)
	require.NoError(s.T(), repository.Transition(run.ID, models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted, ""))

	current, err := repository.CurrentEvidence(7, models.TenantGovernanceRunKindIsolationVerify)
	require.NoError(s.T(), err)
	require.Equal(s.T(), evidence.ID, current.ID)
	staleCount, err := repository.MarkStale(now.Add(2 * time.Hour))
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), staleCount)
	_, err = repository.CurrentEvidence(7, models.TenantGovernanceRunKindIsolationVerify)
	require.ErrorIs(s.T(), err, services.ErrTenantGovernanceEvidenceExpired)

	var count int64
	count, err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("tenant_governance_evidence").Where("run_id", run.ID).Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *TenantGovernanceRunTestSuite) TestTenantRetentionPlansEachTenantAndAwaitsEvidence() {
	now := time.Now().UTC()
	connection := "database.connections." + services.PlatformConnection()
	for _, tenant := range []struct {
		id   uint64
		code string
		days int
	}{
		{id: 101, code: "retention_short", days: 30},
		{id: 102, code: "retention_long", days: 90},
	} {
		err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("tenant").Create(map[string]any{
			"id": tenant.id, "code": tenant.code, "name": tenant.code, "status": services.TenantStatusActive,
			"plan": "standard", "db_host": facades.Config().GetString(connection + ".host"),
			"db_port": facades.Config().GetInt(connection+".port", 5432), "db_database": facades.Config().GetString(connection + ".database"),
			"db_username": facades.Config().GetString(connection + ".username"), "db_password": facades.Config().GetString(connection + ".password"),
			"db_schema": facades.Config().GetString(connection+".schema", "public"), "features": `{}`, "quotas": `{}`, "billing": `{}`, "branding": `{}`,
			"created_at": now, "updated_at": now,
		})
		require.NoError(s.T(), err)
		require.NoError(s.T(), services.NewTenantGovernanceService().SavePolicy(services.TenantGovernancePolicy{
			TenantID: tenant.id, TenantCode: tenant.code,
			Retention:    services.TenantRetentionPolicy{AuditDays: tenant.days, DataDays: 365},
			DataExport:   services.TenantDataActionPolicy{Enabled: true, RequiresApproval: true},
			DataDeletion: services.TenantDataActionPolicy{Enabled: true, RequiresApproval: true},
		}))
	}

	results, err := services.NewTenantRetentionService().WithContext(s.T().Context()).Run()
	require.NoError(s.T(), err)
	require.Len(s.T(), results, 2)
	require.Equal(s.T(), 30, results[0].AuditDays)
	require.Equal(s.T(), 90, results[1].AuditDays)
	for _, result := range results {
		require.Equal(s.T(), models.TenantGovernanceRunStatusAwaitingEvidence, result.Status)
		require.Empty(s.T(), result.Error)
		require.NotEmpty(s.T(), result.PlanID)
	}

	duplicate, err := services.NewTenantRetentionService().WithContext(s.T().Context()).Run()
	require.NoError(s.T(), err)
	require.Len(s.T(), duplicate, 2)
	require.Equal(s.T(), results[0].RunID, duplicate[0].RunID)
	require.Equal(s.T(), results[0].PlanID, duplicate[0].PlanID)
	require.Equal(s.T(), results[1].RunID, duplicate[1].RunID)
	require.Equal(s.T(), results[1].PlanID, duplicate[1].PlanID)
}

func (s *TenantGovernanceRunTestSuite) TestIsolationVerificationPersistsCurrentEvidenceAndUploadFailureDoesNotOverwrite() {
	now := time.Now().UTC()
	tenant := services.Tenant{ID: 88, Code: "isolated", DBDatabase: "tenant_88", DBSchema: "tenant88", DBUsername: "role_88"}
	policy := services.TenantGovernancePolicy{
		TenantID: 88, TenantCode: "isolated",
		IsolationProof: services.TenantIsolationProof{EvidenceTTLHours: 2, VerificationCadenceHours: 1},
	}
	probe := isolationProbeStub{result: services.TenantIsolationProbeResult{
		Database: "tenant_88", Schema: "tenant88", Role: "role_88",
		CrossTenantSentinelDenied: true, PlatformSentinelDenied: true,
	}}
	artifact := services.TenantIsolationArtifact{
		URI: "s3://audit/isolation/88.json", ObjectVersion: "v1",
		SHA256: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	verifier := services.NewTenantIsolationVerifier(probe, isolationStoreStub{artifact: artifact}).WithContext(s.T().Context())
	evidence, err := verifier.Verify(tenant, policy)
	require.NoError(s.T(), err)
	require.Equal(s.T(), artifact.URI, evidence.URI)
	require.WithinDuration(s.T(), now.Add(2*time.Hour), evidence.ExpiresAt, 5*time.Second)

	current, err := services.NewTenantGovernanceRunRepository().WithContext(s.T().Context()).CurrentEvidence(88, models.TenantGovernanceRunKindIsolationVerify)
	require.NoError(s.T(), err)
	require.Equal(s.T(), evidence.ID, current.ID)

	failing := services.NewTenantIsolationVerifier(probe, isolationStoreStub{err: errors.New("upload failed")}).WithContext(s.T().Context())
	_, err = failing.Verify(services.Tenant{ID: 89, Code: "upload-fail", DBDatabase: "tenant_89", DBSchema: "tenant89", DBUsername: "role_89"}, policy)
	require.ErrorIs(s.T(), err, services.ErrTenantIsolationVerification)
	var failedEvidence int64
	failedEvidence, err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("tenant_governance_evidence").Where("tenant_id", 89).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), failedEvidence)
}
