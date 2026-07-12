package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"goravel/app/facades"
	"goravel/app/models"
)

var ErrTenantIsolationVerification = errors.New("tenant isolation verification failed")

type TenantIsolationProbeResult struct {
	Database                  string `json:"database"`
	Schema                    string `json:"schema"`
	Role                      string `json:"role"`
	CrossTenantSentinelDenied bool   `json:"cross_tenant_sentinel_denied"`
	PlatformSentinelDenied    bool   `json:"platform_sentinel_denied"`
}

type TenantIsolationProofV1 struct {
	Schema     string                     `json:"schema"`
	TenantID   uint64                     `json:"tenant_id"`
	TenantCode string                     `json:"tenant_code"`
	VerifiedAt time.Time                  `json:"verified_at"`
	Probe      TenantIsolationProbeResult `json:"probe"`
}

type TenantIsolationArtifact struct {
	URI           string
	ObjectVersion string
	SHA256        string
}

type TenantIsolationProbe interface {
	Probe(context.Context, Tenant) (TenantIsolationProbeResult, error)
}

type TenantIsolationArtifactStore interface {
	WriteImmutable(context.Context, Tenant, []byte) (TenantIsolationArtifact, error)
}

var newTenantIsolationArtifactStore = configuredTenantIsolationArtifactStore
var verifyTenantIsolation = verifyTenantIsolationEvidence

type TenantIsolationVerifier struct {
	ctx   context.Context
	now   func() time.Time
	probe TenantIsolationProbe
	store TenantIsolationArtifactStore
	runs  *TenantGovernanceRunRepository
}

func NewTenantIsolationVerifier(probe TenantIsolationProbe, store TenantIsolationArtifactStore) *TenantIsolationVerifier {
	return &TenantIsolationVerifier{now: time.Now, probe: probe, store: store, runs: NewTenantGovernanceRunRepository()}
}

func (v *TenantIsolationVerifier) WithContext(ctx context.Context) *TenantIsolationVerifier {
	clone := *v
	clone.ctx = contextOrBackground(ctx)
	clone.runs = clone.runs.WithContext(ctx)
	return &clone
}

func (v *TenantIsolationVerifier) Verify(tenant Tenant, policy TenantGovernancePolicy) (models.TenantGovernanceEvidence, error) {
	if v == nil || v.probe == nil || v.store == nil || tenant.ID == 0 {
		return models.TenantGovernanceEvidence{}, ErrTenantIsolationVerification
	}
	verifiedAt := v.now().UTC()
	key := fmt.Sprintf("%d:%s:%s:isolation_verify", tenant.ID, tenantGovernancePolicyVersion(policy), verifiedAt.Format("2006-01-02T15"))
	run, created, err := v.runs.CreateOrGetRun(TenantGovernanceRunCreate{
		TenantID: tenant.ID, TenantCode: tenant.Code, Kind: models.TenantGovernanceRunKindIsolationVerify,
		IdempotencyKey: key, PolicyVersion: tenantGovernancePolicyVersion(policy),
	})
	if err != nil || !created {
		return models.TenantGovernanceEvidence{}, err
	}
	if err := v.runs.Transition(run.ID, models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, ""); err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	probe, err := v.probe.Probe(contextOrBackground(v.ctx), tenant)
	if err != nil || !validTenantIsolationProbe(tenant, probe) {
		message := ErrTenantIsolationVerification.Error()
		if err != nil {
			message = err.Error()
		}
		_ = v.runs.Transition(run.ID, models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusFailed, message)
		return models.TenantGovernanceEvidence{}, ErrTenantIsolationVerification
	}
	payload, err := json.Marshal(TenantIsolationProofV1{Schema: "tenant-isolation/v1", TenantID: tenant.ID, TenantCode: tenant.Code, VerifiedAt: verifiedAt, Probe: probe})
	if err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	artifact, err := v.store.WriteImmutable(contextOrBackground(v.ctx), tenant, payload)
	if err != nil || strings.TrimSpace(artifact.URI) == "" || strings.TrimSpace(artifact.ObjectVersion) == "" || !isSHA256(artifact.SHA256) {
		_ = v.runs.Transition(run.ID, models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusFailed, "immutable evidence upload failed")
		return models.TenantGovernanceEvidence{}, ErrTenantIsolationVerification
	}
	ttl := policy.IsolationProof.EvidenceTTLHours
	if ttl <= 0 {
		ttl = 24
	}
	evidence, err := v.runs.AttachEvidence(run, TenantGovernanceEvidenceInput{
		URI: artifact.URI, ObjectVersion: artifact.ObjectVersion, SHA256: artifact.SHA256,
		VerifiedAt: verifiedAt, ExpiresAt: verifiedAt.Add(time.Duration(ttl) * time.Hour),
		Metadata: map[string]any{"schema": "tenant-isolation/v1", "tenant_code": tenant.Code},
	})
	if err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	if err := v.runs.Transition(run.ID, models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted, ""); err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	return evidence, nil
}

func validTenantIsolationProbe(tenant Tenant, probe TenantIsolationProbeResult) bool {
	wantSchema := strings.TrimSpace(tenant.DBSchema)
	if wantSchema == "" {
		wantSchema = "public"
	}
	return probe.Database == strings.TrimSpace(tenant.DBDatabase) && probe.Schema == wantSchema &&
		probe.Role == strings.TrimSpace(tenant.DBUsername) && probe.CrossTenantSentinelDenied && probe.PlatformSentinelDenied
}

type DatabaseTenantIsolationProbe struct{}

func (DatabaseTenantIsolationProbe) Probe(ctx context.Context, tenant Tenant) (TenantIsolationProbeResult, error) {
	connection := RegisterTenantConnection(tenant)
	query := OrmForConnectionWithContext(ctx, connection).Query()
	var result TenantIsolationProbeResult
	if err := query.Raw("SELECT current_database() AS database, current_schema() AS schema, current_user AS role").Scan(&result); err != nil {
		return result, err
	}
	if !tenantUsesPlatformInstance(tenant) {
		return result, ErrTenantIsolationVerification
	}
	platformConnect, err := databaseConnectPrivilege(ctx, PlatformConnection(), tenant.DBUsername)
	if err != nil {
		return result, err
	}
	var tenants []Tenant
	err = OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table("tenant").
		Where("id <> ?", tenant.ID).Where("db_database <> ''").Where("db_username <> ''").Get(&tenants)
	if err != nil {
		return result, err
	}
	otherConnect := false
	for _, other := range tenants {
		if err := requireAttestedTenantInstanceBoundary(tenant, other); err != nil {
			return result, err
		}
		connect, probeErr := databaseConnectPrivilege(ctx, RegisterTenantConnection(other), tenant.DBUsername)
		if probeErr != nil {
			return result, probeErr
		}
		otherConnect = otherConnect || connect
	}
	result.CrossTenantSentinelDenied = !otherConnect
	result.PlatformSentinelDenied = !platformConnect
	return result, nil
}

func tenantUsesPlatformInstance(tenant Tenant) bool {
	prefix := "database.connections." + PlatformConnection()
	platform := Tenant{
		DBHost: facades.Config().GetString(prefix + ".host"),
		DBPort: facades.Config().GetInt(prefix+".port", 5432),
	}
	return sameTenantDatabaseInstance(tenant, platform)
}

func sameTenantDatabaseInstance(first, second Tenant) bool {
	return normalizeDatabaseHost(first.DBHost) == normalizeDatabaseHost(second.DBHost) &&
		normalizeDatabasePort(first.DBPort) == normalizeDatabasePort(second.DBPort)
}

func requireAttestedTenantInstanceBoundary(first, second Tenant) error {
	if !sameTenantDatabaseInstance(first, second) {
		return ErrTenantIsolationVerification
	}
	return nil
}

func normalizeDatabaseHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return "127.0.0.1"
	}
	return host
}

func normalizeDatabasePort(port int) string {
	if port == 0 {
		port = 5432
	}
	return strconv.Itoa(port)
}

func databaseConnectPrivilege(ctx context.Context, connection, username string) (bool, error) {
	var access struct {
		Connect bool `gorm:"column:connect"`
	}
	err := OrmForConnectionWithContext(ctx, connection).Query().
		Raw(`
			SELECT CASE
			  WHEN EXISTS (SELECT 1 FROM pg_roles WHERE rolname = ?)
			  THEN has_database_privilege(?, current_database(), 'CONNECT')
			  ELSE FALSE
			END AS connect
		`, username, username).
		Scan(&access)
	return access.Connect, err
}

func tenantIsolationScheduledTaskHandler(ctx context.Context, _ models.JSONMap) ScheduledTaskExecutionResult {
	store := newTenantIsolationArtifactStore(ctx)
	if store == nil {
		return taskFailure("immutable tenant isolation artifact store is not configured")
	}
	tenants, err := activeRetentionTenants(ctx)
	if err != nil {
		return taskFailure(err.Error())
	}
	return runTenantIsolationBatch(ctx, tenants, store, func(ctx context.Context, tenant Tenant) (TenantGovernancePolicy, error) {
		return NewTenantGovernanceService().WithContext(ctx).Policy(tenant)
	}, verifyTenantIsolation)
}

func runTenantIsolationBatch(
	ctx context.Context,
	tenants []Tenant,
	store TenantIsolationArtifactStore,
	policyFor func(context.Context, Tenant) (TenantGovernancePolicy, error),
	verify func(context.Context, Tenant, TenantGovernancePolicy, TenantIsolationArtifactStore) (models.TenantGovernanceEvidence, error),
) ScheduledTaskExecutionResult {
	results := make([]map[string]any, 0, len(tenants))
	failures := make([]string, 0)
	for _, tenant := range tenants {
		policy, err := policyFor(ctx, tenant)
		if err != nil {
			results = append(results, map[string]any{"tenant_id": tenant.ID, "evidence_id": uint64(0), "error": err.Error()})
			failures = append(failures, fmt.Sprintf("tenant %d: %s", tenant.ID, err.Error()))
			continue
		}
		evidence, err := verify(ctx, tenant, policy, store)
		results = append(results, map[string]any{"tenant_id": tenant.ID, "evidence_id": evidence.ID, "error": errorString(err)})
		if err != nil {
			failures = append(failures, fmt.Sprintf("tenant %d: %s", tenant.ID, err.Error()))
		}
	}
	payload, _ := json.Marshal(results)
	if len(failures) > 0 {
		return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusFailed, Stdout: string(payload), ErrorMessage: strings.Join(failures, "; ")}
	}
	return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusSuccess, Stdout: string(payload)}
}

func verifyTenantIsolationEvidence(ctx context.Context, tenant Tenant, policy TenantGovernancePolicy, store TenantIsolationArtifactStore) (models.TenantGovernanceEvidence, error) {
	return NewTenantIsolationVerifier(DatabaseTenantIsolationProbe{}, store).WithContext(ctx).Verify(tenant, policy)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
