package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goravel/framework/contracts/database/driver"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"github.com/goravel/framework/contracts/database/schema"
	"github.com/goravel/framework/contracts/database/seeder"
	frameworkerrors "github.com/goravel/framework/errors"
	postgresfacades "github.com/goravel/postgres/facades"
	observabilitycontract "goravel/app/contracts/observability"
	tenantcontract "goravel/app/contracts/tenant"
	"goravel/app/facades"
	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"
	tenantservice "goravel/app/services/tenancy/tenant"
	"goravel/app/support/contextutil"
	"goravel/database/migrations"
	"goravel/database/seeders"
	"io"
	"net"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Source: tenant_connection_registry.go
var ErrTenantConnectionBudgetExceeded = tenantservice.ErrTenantConnectionBudgetExceeded

type TenantConnectionBudget = tenantservice.ConnectionBudget
type TenantConnectionBudgetReport = tenantservice.ConnectionBudgetReport
type TenantConnectionCapacityMetrics = tenantservice.ConnectionCapacityMetrics

var ValidateTenantConnectionBudget = tenantservice.ValidateConnectionBudget
var TenantConnectionBudgetFromConfig = tenantservice.ConnectionBudgetFromConfig
var RegisterTenantConnectionCapacity = tenantservice.RegisterConnectionCapacity
var TenantConnectionRegistryCount = tenantservice.ConnectionRegistryCount
var TenantConnectionRegistered = tenantservice.ConnectionRegistered
var ResetTenantConnectionRegistryForTest = tenantservice.ResetConnectionRegistryForTest
var TenantConnectionCapacitySnapshot = tenantservice.ConnectionCapacitySnapshot

// Source: tenant_context.go
var tenantORMConnectionMu sync.Mutex

func Orm() contractsorm.Orm {
	return facades.Orm()
}

func OrmForConnection(connection string) contractsorm.Orm {
	return OrmForConnectionWithContext(context.Background(), connection)
}

func OrmWithContext(ctx context.Context) contractsorm.Orm {
	return OrmForConnectionWithContext(ctx, "")
}

func OrmForConnectionWithContext(ctx context.Context, connection string) contractsorm.Orm {
	ctx = contextOrBackground(ctx)
	base := facades.Orm().WithContext(ctx)
	if connection == "" {
		return base
	}
	if TenantConnectionRegistered(connection) {
		tenantORMConnectionMu.Lock()
		defer tenantORMConnectionMu.Unlock()
	}
	instance := base.Connection(connection)
	configureTenantConnectionPool(connection, instance)
	return instance
}

func configureTenantConnectionPool(connection string, instance contractsorm.Orm) {
	if !TenantConnectionRegistered(connection) || instance.Query() == nil {
		return
	}
	database, err := instance.DB()
	if err == nil {
		database.SetMaxIdleConns(0)
	}
}

func contextOrBackground(ctx context.Context) context.Context {
	return contextutil.OrBackground(ctx)
}

func TenantConnectionFromContext(ctx context.Context) string {
	tenant, ok := CurrentTenant(ctx)
	if !ok {
		return ""
	}
	return TenantConnectionName(tenant)
}

// Source: tenant_data_export_job.go
type tenantDataExportPayload struct {
	RunID    uint64            `json:"run_id"`
	TenantID uint64            `json:"tenant_id"`
	Dataset  string            `json:"dataset"`
	Format   string            `json:"format"`
	Filters  map[string]string `json:"filters"`
}

type tenantExportUser struct {
	ID        uint64    `gorm:"column:id" json:"id"`
	Username  string    `gorm:"column:username" json:"username"`
	Nickname  string    `gorm:"column:nickname" json:"nickname"`
	Email     string    `gorm:"column:email" json:"email"`
	Phone     string    `gorm:"column:phone" json:"phone"`
	Status    int8      `gorm:"column:status" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func init() {
	RegisterQueueOutboxHandler(tenantDataExportTopic, handleTenantDataExportEvent)
}

func handleTenantDataExportEvent(ctx context.Context, event QueueOutboxEvent) error {
	var payload tenantDataExportPayload
	if json.Unmarshal([]byte(event.Payload), &payload) != nil || payload.RunID == 0 || payload.TenantID == 0 {
		return ErrTenantExportInvalid
	}
	err := NewTenantDataExportWorker().WithContext(ctx).Handle(payload)
	if err != nil {
		markTenantExportFailed(ctx, payload.RunID, err)
	}
	return err
}

type TenantDataExportWorker struct {
	ctx   context.Context
	store TenantExportArtifactStore
}

func NewTenantDataExportWorker() *TenantDataExportWorker {
	return &TenantDataExportWorker{store: configuredTenantExportArtifactStore(context.Background())}
}

func (w *TenantDataExportWorker) WithContext(ctx context.Context) *TenantDataExportWorker {
	clone := *w
	clone.ctx = contextOrBackground(ctx)
	if clone.store == nil {
		clone.store = configuredTenantExportArtifactStore(clone.ctx)
	}
	return &clone
}

func (w *TenantDataExportWorker) Handle(payload tenantDataExportPayload) error {
	run, err := NewTenantDataExportService().WithContext(w.ctx).Status(payload.TenantID, payload.RunID)
	if err != nil || run.Status == models.TenantGovernanceRunStatusCompleted {
		return err
	}
	repository := NewTenantGovernanceRunRepository().WithContext(w.ctx)
	if run.Status == models.TenantGovernanceRunStatusPending {
		if err := repository.Transition(run.ID, models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, ""); err != nil {
			return err
		}
		run.Status = models.TenantGovernanceRunStatusRunning
	}
	if run.Status == models.TenantGovernanceRunStatusFailed {
		if err := resumeTenantExportRun(w.ctx, run.ID); err != nil {
			return err
		}
		run.Status = models.TenantGovernanceRunStatusRunning
	}
	if run.Status == models.TenantGovernanceRunStatusArtifactWritten {
		return repository.Transition(run.ID, models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted, "")
	}
	if run.Status != models.TenantGovernanceRunStatusRunning || w.store == nil {
		return ErrTenantExportInvalid
	}
	tenant, err := NewTenantService().WithContext(w.ctx).FindByID(payload.TenantID)
	if err != nil {
		return err
	}
	plain, err := exportTenantDataset(w.ctx, tenant, payload)
	if err != nil {
		return err
	}
	encrypted, err := facades.Crypt().EncryptString(string(plain))
	if err != nil {
		return err
	}
	artifact, err := w.store.WriteImmutable(w.ctx, run, []byte(encrypted))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = repository.AttachEvidence(run, TenantGovernanceEvidenceInput{
		URI: artifact.URI, ObjectVersion: artifact.ObjectVersion, SHA256: artifact.SHA256,
		VerifiedAt: now, ExpiresAt: now.Add(24 * time.Hour),
		Metadata: map[string]any{"dataset": payload.Dataset, "format": payload.Format, "encrypted": true},
	})
	if err != nil && !errors.Is(err, ErrTenantGovernanceEvidenceExists) {
		return err
	}
	return repository.Transition(run.ID, models.TenantGovernanceRunStatusArtifactWritten, models.TenantGovernanceRunStatusCompleted, "")
}

func exportTenantDataset(ctx context.Context, tenant Tenant, payload tenantDataExportPayload) ([]byte, error) {
	if payload.Dataset != "users" || !tenantExportFormatAllowed(payload.Format) {
		return nil, ErrTenantExportInvalid
	}
	rows := make([]tenantExportUser, 0)
	query := OrmForConnectionWithContext(ctx, TenantConnectionName(tenant)).Query().Table("user").
		Select("id", "username", "nickname", "email", "phone", "status", "created_at").OrderBy("id")
	if status := payload.Filters["status"]; status != "" {
		query = query.Where("status", status)
	}
	if err := query.Get(&rows); err != nil && !errors.Is(err, frameworkerrors.OrmRecordNotFound) {
		return nil, err
	}
	if payload.Format == "csv" {
		return tenantExportCSV(rows)
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func tenantExportCSV(rows []tenantExportUser) ([]byte, error) {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	_ = writer.Write([]string{"id", "username", "nickname", "email", "phone", "status", "created_at"})
	for _, row := range rows {
		_ = writer.Write([]string{
			strconv.FormatUint(row.ID, 10), safeTenantExportCSVCell(row.Username), safeTenantExportCSVCell(row.Nickname),
			safeTenantExportCSVCell(row.Email), safeTenantExportCSVCell(row.Phone), strconv.Itoa(int(row.Status)), row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writer.Flush()
	return output.Bytes(), writer.Error()
}

func safeTenantExportCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " ")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + value
	default:
		return value
	}
}

func markTenantExportFailed(ctx context.Context, runID uint64, runErr error) {
	if runID == 0 || runErr == nil {
		return
	}
	now := time.Now().UTC()
	_, _ = OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table((models.TenantGovernanceRun{}).TableName()).
		Where("id", runID).Where("kind", models.TenantGovernanceRunKindExport).
		WhereIn("status", []any{models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning}).
		Update(map[string]any{"status": models.TenantGovernanceRunStatusFailed, "error": runErr.Error(), "finished_at": now, "updated_at": now})
}

func resumeTenantExportRun(ctx context.Context, runID uint64) error {
	now := time.Now().UTC()
	result, err := OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table((models.TenantGovernanceRun{}).TableName()).
		Where("id", runID).Where("kind", models.TenantGovernanceRunKindExport).Where("status", models.TenantGovernanceRunStatusFailed).
		Update(map[string]any{"status": models.TenantGovernanceRunStatusRunning, "error": "", "finished_at": nil, "started_at": now, "updated_at": now})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrTenantGovernanceRunInvalidTransition
	}
	return nil
}

// Source: tenant_data_export_service.go
const tenantDataExportTopic = "tenant_data_export"

var (
	ErrTenantExportInvalid = errors.New("tenant export request is invalid")
	ErrTenantExportExpired = errors.New("tenant export download expired")
)

type TenantDataExportRequest struct {
	Dataset     string            `json:"dataset"`
	Format      string            `json:"format"`
	Filters     map[string]string `json:"filters"`
	ReAuthToken string            `json:"reauth_token"`
	ApprovalID  string            `json:"approval_id"`
	OperatorID  uint64            `json:"-"`
}

type TenantDataExportService struct{ ctx context.Context }

type TenantDataExportStatus struct {
	Run           models.TenantGovernanceRun `json:"run"`
	DownloadToken string                     `json:"download_token,omitempty"`
	ExpiresAt     *time.Time                 `json:"expires_at,omitempty"`
}

func NewTenantDataExportService() *TenantDataExportService { return &TenantDataExportService{} }

func (s *TenantDataExportService) WithContext(ctx context.Context) *TenantDataExportService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantDataExportService) Request(tenant Tenant, input TenantDataExportRequest) (models.TenantGovernanceRun, error) {
	dataset, format, filters, filterDigest, err := normalizeTenantExportRequest(input)
	if err != nil || tenant.ID == 0 || input.OperatorID == 0 {
		return models.TenantGovernanceRun{}, ErrTenantExportInvalid
	}
	policy, err := NewTenantGovernanceService().WithContext(s.ctx).Policy(tenant)
	if err != nil {
		return models.TenantGovernanceRun{}, err
	}
	if !policy.DataExport.Enabled {
		return models.TenantGovernanceRun{}, ErrTenantDataActionDenied
	}
	resource := tenantExportResource(tenant.ID, dataset, format, filterDigest)
	idempotencyFactor := tenantExportIdempotencyFactor(policy.DataExport.RequiresApproval, input)
	idempotencyKey := tenantExportIdempotencyKey(resource, tenantGovernancePolicyVersion(policy), idempotencyFactor, input.OperatorID)
	if existing, ok, findErr := s.findRun(tenant.ID, idempotencyKey); findErr != nil {
		return models.TenantGovernanceRun{}, findErr
	} else if ok {
		return existing, nil
	}
	guard := newTenantExportSensitiveGuard(policy.DataExport.RequiresApproval)
	plan, err := guard.PrepareCanonical(s.ctx, "tenant.data.export", input.OperatorID, 0, SensitiveOperationPlanSelector{Resource: resource})
	if err != nil {
		return models.TenantGovernanceRun{}, err
	}
	var run models.TenantGovernanceRun
	err = guard.ExecutePlatformTransaction(s.ctx, plan, SensitiveOperationEvidence{ReAuthToken: input.ReAuthToken, ApprovalID: input.ApprovalID}, func(query contractsorm.Query) error {
		created, createErr := createTenantExportRun(query, tenant, idempotencyKey, tenantGovernancePolicyVersion(policy))
		if createErr != nil {
			return createErr
		}
		run = created
		payload, marshalErr := json.Marshal(tenantDataExportPayload{RunID: run.ID, TenantID: tenant.ID, Dataset: dataset, Format: format, Filters: filters})
		if marshalErr != nil {
			return marshalErr
		}
		return EnqueueQueueOutboxEventWithQuery(query, QueueOutboxEvent{Topic: tenantDataExportTopic, Payload: string(payload)})
	})
	return run, err
}

func newTenantExportSensitiveGuard(requiresApproval bool) *SensitiveOperationGuard {
	registry := NewSensitiveOperationPolicyRegistry()
	policy := registry.policies["tenant.data.export"]
	policy.RequiresApproval = requiresApproval
	registry.policies[policy.PolicyKey] = policy
	return NewSensitiveOperationGuard(registry)
}

func tenantExportIdempotencyFactor(requiresApproval bool, input TenantDataExportRequest) string {
	if requiresApproval {
		return "approval:" + strings.TrimSpace(input.ApprovalID)
	}
	return "reauth:" + digestBytes([]byte(strings.TrimSpace(input.ReAuthToken)))
}

func tenantExportIdempotencyKey(resource, policyVersion, factor string, operatorID uint64) string {
	payload := strings.Join([]string{resource, policyVersion, strings.TrimSpace(factor)}, "\x00")
	return "tenant-export:" + digestBytes([]byte(payload)) + ":operator:" + strconv.FormatUint(operatorID, 10)
}

func (s *TenantDataExportService) Status(tenantID, runID uint64) (models.TenantGovernanceRun, error) {
	var run models.TenantGovernanceRun
	err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Table(run.TableName()).
		Where("id", runID).Where("tenant_id", tenantID).Where("kind", models.TenantGovernanceRunKindExport).First(&run)
	return run, err
}

func (s *TenantDataExportService) StatusForOperator(operatorID, tenantID, runID uint64) (TenantDataExportStatus, error) {
	run, err := s.Status(tenantID, runID)
	if err != nil || tenantExportRunOperatorID(run) != operatorID {
		return TenantDataExportStatus{}, ErrTenantExportExpired
	}
	if run.Status != models.TenantGovernanceRunStatusCompleted {
		return TenantDataExportStatus{Run: run}, err
	}
	token, expiresAt, err := issueTenantExportDownloadToken(operatorID, tenantID, runID)
	if err != nil {
		return TenantDataExportStatus{}, err
	}
	return TenantDataExportStatus{Run: run, DownloadToken: token, ExpiresAt: &expiresAt}, nil
}

func (s *TenantDataExportService) Download(operatorID, tenantID, runID uint64, token string) ([]byte, string, error) {
	if !consumeTenantExportDownloadToken(operatorID, tenantID, runID, token) {
		return nil, "", ErrTenantExportExpired
	}
	run, err := s.Status(tenantID, runID)
	if err != nil || tenantExportRunOperatorID(run) != operatorID || run.Status != models.TenantGovernanceRunStatusCompleted {
		return nil, "", ErrTenantExportExpired
	}
	var evidence models.TenantGovernanceEvidence
	err = OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Table(evidence.TableName()).
		Where("run_id", run.ID).Where("tenant_id", tenantID).Where("kind", models.TenantGovernanceRunKindExport).
		WhereNull("stale_at").Where("expires_at > ?", time.Now().UTC()).First(&evidence)
	if err != nil {
		return nil, "", ErrTenantExportExpired
	}
	config, err := NewStorageConfigService().WithContext(s.ctx).ActiveDefault()
	if err != nil || config.Driver != storageDriverS3Compatible {
		return nil, "", ErrTenantExportExpired
	}
	bucket, objectPath, err := immutableS3Object(evidence.URI)
	if err != nil || bucket != config.Bucket {
		return nil, "", ErrTenantExportExpired
	}
	response, err := immutableS3Request(s.ctx, newObjectStorageClient(config), "GET", objectPath, evidence.ObjectVersion)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = response.Body.Close() }()
	encrypted, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil || !sameDigest(digestBytes(encrypted), evidence.SHA256) {
		return nil, "", ErrTenantExportExpired
	}
	plain, err := facades.Crypt().DecryptString(string(encrypted))
	if err != nil {
		return nil, "", ErrTenantExportExpired
	}
	format := tenantExportFormatFromEvidence(evidence.Metadata)
	return []byte(plain), format, nil
}

func issueTenantExportDownloadToken(operatorID, tenantID, runID uint64) (string, time.Time, error) {
	if operatorID == 0 || tenantID == 0 || runID == 0 {
		return "", time.Time{}, ErrTenantExportInvalid
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(nonce[:])
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	cache := facades.Cache()
	if cache == nil {
		return "", time.Time{}, ErrTenantExportInvalid
	}
	err := cache.Put(tenantExportDownloadTokenKey(token), fmt.Sprintf("%d:%d:%d", operatorID, tenantID, runID), time.Until(expiresAt))
	return token, expiresAt, err
}

func consumeTenantExportDownloadToken(operatorID, tenantID, runID uint64, token string) bool {
	cache := facades.Cache()
	if cache == nil {
		return false
	}
	key := tenantExportDownloadTokenKey(token)
	want := fmt.Sprintf("%d:%d:%d", operatorID, tenantID, runID)
	value := cache.Pull(key)
	return strings.TrimSpace(fmt.Sprint(value)) == want
}

func tenantExportDownloadTokenKey(token string) string {
	return "tenant-export:download:" + digestBytes([]byte(strings.TrimSpace(token)))
}

func tenantExportRunOperatorID(run models.TenantGovernanceRun) uint64 {
	const marker = ":operator:"
	index := strings.LastIndex(run.IdempotencyKey, marker)
	if index < 0 {
		return 0
	}
	operatorID, _ := strconv.ParseUint(run.IdempotencyKey[index+len(marker):], 10, 64)
	return operatorID
}

func tenantExportFormatFromEvidence(raw string) string {
	metadata := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &metadata)
	format, _ := metadata["format"].(string)
	if !tenantExportFormatAllowed(format) {
		return "jsonl"
	}
	return format
}

func (s *TenantDataExportService) findRun(tenantID uint64, idempotencyKey string) (models.TenantGovernanceRun, bool, error) {
	var run models.TenantGovernanceRun
	err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Table(run.TableName()).
		Where("tenant_id", tenantID).Where("kind", models.TenantGovernanceRunKindExport).Where("idempotency_key", idempotencyKey).First(&run)
	if errors.Is(err, frameworkerrors.OrmRecordNotFound) || run.ID == 0 {
		return models.TenantGovernanceRun{}, false, nil
	}
	return run, err == nil, err
}

func createTenantExportRun(query contractsorm.Query, tenant Tenant, idempotencyKey, policyVersion string) (models.TenantGovernanceRun, error) {
	now := time.Now().UTC()
	run := models.TenantGovernanceRun{
		TenantID: tenant.ID, TenantCode: tenant.Code, Kind: models.TenantGovernanceRunKindExport,
		IdempotencyKey: idempotencyKey, PolicyVersion: policyVersion, Status: models.TenantGovernanceRunStatusPending,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}
	err := query.Table(run.TableName()).Create(&run)
	return run, err
}

func normalizeTenantExportRequest(input TenantDataExportRequest) (string, string, map[string]string, string, error) {
	dataset, format := strings.TrimSpace(input.Dataset), strings.TrimSpace(input.Format)
	if !tenantExportDatasetAllowed(dataset) || !tenantExportFormatAllowed(format) {
		return "", "", nil, "", ErrTenantExportInvalid
	}
	filters := map[string]string{}
	for key, value := range input.Filters {
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key != "status" || (value != "" && value != "1" && value != "2") {
			return "", "", nil, "", ErrTenantExportInvalid
		}
		if value != "" {
			filters[key] = value
		}
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	canonical := make([]string, 0, len(keys))
	for _, key := range keys {
		canonical = append(canonical, key+"="+filters[key])
	}
	return dataset, format, filters, digestBytes([]byte(strings.Join(canonical, "&"))), nil
}

// Source: tenant_export_artifact_store.go
type TenantExportArtifactStore interface {
	WriteImmutable(context.Context, models.TenantGovernanceRun, []byte) (TenantIsolationArtifact, error)
}

type s3TenantExportArtifactStore struct{ config StorageConfig }

func configuredTenantExportArtifactStore(ctx context.Context) TenantExportArtifactStore {
	config, err := NewStorageConfigService().WithContext(ctx).ActiveDefault()
	if err != nil || config.Driver != storageDriverS3Compatible || strings.TrimSpace(config.Bucket) == "" {
		return nil
	}
	return &s3TenantExportArtifactStore{config: config}
}

func (s *s3TenantExportArtifactStore) WriteImmutable(ctx context.Context, run models.TenantGovernanceRun, payload []byte) (TenantIsolationArtifact, error) {
	if s == nil || run.ID == 0 || run.TenantID == 0 || len(payload) == 0 {
		return TenantIsolationArtifact{}, ErrTenantExportInvalid
	}
	digest := digestBytes(payload)
	objectPath := path.Join(s.config.PathPrefix, "tenant-governance/export", fmt.Sprint(run.TenantID), fmt.Sprintf("run-%d-%s.enc", run.ID, strings.TrimPrefix(digest, "sha256:")[:16]))
	client := newObjectStorageClient(s.config)
	writtenAt := time.Now().UTC()
	version, err := putImmutableS3Object(ctx, client, objectPath, payload, writtenAt.Add(48*time.Hour))
	if err != nil || version == "" || verifyImmutableS3Object(ctx, client, objectPath, version, payload, writtenAt) != nil {
		return TenantIsolationArtifact{}, ErrTenantExportInvalid
	}
	return TenantIsolationArtifact{URI: "s3://" + s.config.Bucket + "/" + objectPath, ObjectVersion: version, SHA256: digest}, nil
}

// Source: tenant_governance_observability.go
type TenantGovernanceObservability = observabilitycontract.TenantGovernanceMetrics

func TenantGovernanceObservabilitySnapshot(ctx context.Context, now time.Time) (TenantGovernanceObservability, error) {
	database := Orm()
	if database == nil {
		return TenantGovernanceObservability{}, errors.New("tenant governance observability requires ORM binding")
	}
	query := database.WithContext(contextOrBackground(ctx)).Connection(PlatformConnection()).Query()
	expired, err := query.Table((models.TenantGovernanceEvidence{}).TableName()).Where("expires_at <= ?", now.UTC()).Count()
	if err != nil {
		return TenantGovernanceObservability{}, err
	}
	failed, err := query.Table((models.TenantGovernanceRun{}).TableName()).Where("kind", models.TenantGovernanceRunKindIsolationVerify).
		Where("status", models.TenantGovernanceRunStatusFailed).Count()
	if err != nil {
		return TenantGovernanceObservability{}, err
	}
	var oldest *time.Time
	err = query.Table((models.TenantGovernanceRun{}).TableName()).WhereIn("status", []any{
		models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusAwaitingEvidence,
	}).OrderBy("created_at").Limit(1).Pluck("created_at", &oldest)
	age := time.Duration(0)
	if err == nil && oldest != nil {
		age = now.UTC().Sub(oldest.UTC())
	}
	return TenantGovernanceObservability{EvidenceExpired: expired, VerificationFailed: failed, OldestRunAge: age}, err
}

// Source: tenant_governance_run_repository.go
var (
	ErrTenantGovernanceRunInvalidTransition = errors.New("tenant governance run transition is invalid")
	ErrTenantGovernanceEvidenceExpired      = errors.New("tenant governance evidence is expired")
	ErrTenantGovernanceEvidenceExists       = errors.New("tenant governance evidence already exists")
)

type TenantGovernanceRunCreate struct {
	TenantID       uint64
	TenantCode     string
	Kind           string
	IdempotencyKey string
	PolicyVersion  string
}

type TenantGovernanceEvidenceInput struct {
	URI           string
	ObjectVersion string
	SHA256        string
	VerifiedAt    time.Time
	ExpiresAt     time.Time
	Metadata      map[string]any
}

type TenantGovernanceRunRepository struct {
	ctx context.Context
	now func() time.Time
}

func NewTenantGovernanceRunRepository() *TenantGovernanceRunRepository {
	return &TenantGovernanceRunRepository{now: time.Now}
}

func (r *TenantGovernanceRunRepository) WithContext(ctx context.Context) *TenantGovernanceRunRepository {
	clone := *r
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (r *TenantGovernanceRunRepository) CreateOrGetRun(input TenantGovernanceRunCreate) (models.TenantGovernanceRun, bool, error) {
	input.TenantCode = strings.TrimSpace(input.TenantCode)
	input.Kind = strings.TrimSpace(input.Kind)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.TenantID == 0 || input.TenantCode == "" || !tenantGovernanceRunKindValid(input.Kind) || input.IdempotencyKey == "" {
		return models.TenantGovernanceRun{}, false, ErrTenantGovernanceRunInvalidTransition
	}
	query := r.query()
	var existing models.TenantGovernanceRun
	err := query.Table(existing.TableName()).Where("tenant_id", input.TenantID).Where("kind", input.Kind).
		Where("idempotency_key", input.IdempotencyKey).First(&existing)
	if err == nil && existing.ID != 0 {
		return existing, false, nil
	}
	if err != nil && !errors.Is(err, frameworkerrors.OrmRecordNotFound) {
		return models.TenantGovernanceRun{}, false, err
	}
	now := r.now().UTC()
	run := models.TenantGovernanceRun{
		TenantID: input.TenantID, TenantCode: input.TenantCode, Kind: input.Kind,
		IdempotencyKey: input.IdempotencyKey, PolicyVersion: input.PolicyVersion,
		Status:     models.TenantGovernanceRunStatusPending,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}
	if err := query.Table(run.TableName()).Create(&run); err != nil {
		if loadErr := query.Table(run.TableName()).Where("tenant_id", input.TenantID).Where("kind", input.Kind).
			Where("idempotency_key", input.IdempotencyKey).First(&existing); loadErr == nil && existing.ID != 0 {
			return existing, false, nil
		}
		return models.TenantGovernanceRun{}, false, err
	}
	return run, true, nil
}

func (r *TenantGovernanceRunRepository) Transition(runID uint64, from, to, message string) error {
	if runID == 0 || !tenantGovernanceTransitionAllowed(from, to) {
		return ErrTenantGovernanceRunInvalidTransition
	}
	now := r.now().UTC()
	values := map[string]any{"status": to, "updated_at": now}
	if to == models.TenantGovernanceRunStatusRunning {
		values["started_at"] = now
	}
	if tenantGovernanceRunTerminal(to) {
		values["finished_at"] = now
	}
	if strings.TrimSpace(message) != "" {
		values["error"] = strings.TrimSpace(message)
	}
	result, err := r.query().Table((models.TenantGovernanceRun{}).TableName()).Where("id", runID).Where("status", from).Update(values)
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrTenantGovernanceRunInvalidTransition
	}
	return nil
}

func (r *TenantGovernanceRunRepository) AwaitRetentionEvidence(runID uint64, planID string) error {
	planID = strings.TrimSpace(planID)
	if runID == 0 || planID == "" {
		return ErrTenantGovernanceRunInvalidTransition
	}
	now := r.now().UTC()
	result, err := r.query().Table((models.TenantGovernanceRun{}).TableName()).Where("id", runID).
		Where("kind", models.TenantGovernanceRunKindRetention).Where("status", models.TenantGovernanceRunStatusRunning).
		Update(map[string]any{"plan_id": planID, "status": models.TenantGovernanceRunStatusAwaitingEvidence, "error": "", "updated_at": now})
	if err != nil {
		return err
	}
	if result.RowsAffected != 1 {
		return ErrTenantGovernanceRunInvalidTransition
	}
	return nil
}

func (r *TenantGovernanceRunRepository) FinishRetentionPlan(planID, status, message string) error {
	planID = strings.TrimSpace(planID)
	if planID == "" || (status != models.TenantGovernanceRunStatusCompleted && status != models.TenantGovernanceRunStatusFailed) {
		return ErrTenantGovernanceRunInvalidTransition
	}
	var run models.TenantGovernanceRun
	err := r.query().Table(run.TableName()).Where("kind", models.TenantGovernanceRunKindRetention).
		Where("plan_id", planID).First(&run)
	if errors.Is(err, frameworkerrors.OrmRecordNotFound) || run.ID == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	return r.Transition(run.ID, models.TenantGovernanceRunStatusAwaitingEvidence, status, message)
}

func (r *TenantGovernanceRunRepository) AttachEvidence(run models.TenantGovernanceRun, input TenantGovernanceEvidenceInput) (models.TenantGovernanceEvidence, error) {
	if run.ID == 0 || run.TenantID == 0 || strings.TrimSpace(input.URI) == "" || strings.TrimSpace(input.ObjectVersion) == "" ||
		!isSHA256(input.SHA256) || input.VerifiedAt.IsZero() || !input.ExpiresAt.After(input.VerifiedAt) {
		return models.TenantGovernanceEvidence{}, ErrImmutableEvidenceInvalid
	}
	metadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	evidence := models.TenantGovernanceEvidence{
		RunID: run.ID, TenantID: run.TenantID, Kind: run.Kind, URI: input.URI,
		ObjectVersion: input.ObjectVersion, SHA256: input.SHA256,
		VerifiedAt: input.VerifiedAt.UTC(), ExpiresAt: input.ExpiresAt.UTC(), Metadata: string(metadata),
		Timestamps: models.Timestamps{CreatedAt: r.now().UTC(), UpdatedAt: r.now().UTC()},
	}
	err = r.orm().Transaction(func(query contractsorm.Query) error {
		var count int64
		var countErr error
		count, countErr = query.Table(evidence.TableName()).Where("run_id", run.ID).Count()
		if countErr != nil {
			return countErr
		}
		if count != 0 {
			return ErrTenantGovernanceEvidenceExists
		}
		if err := query.Table(evidence.TableName()).Create(&evidence); err != nil {
			return err
		}
		result, err := query.Table(run.TableName()).Where("id", run.ID).Where("status", models.TenantGovernanceRunStatusRunning).
			Update(map[string]any{"status": models.TenantGovernanceRunStatusArtifactWritten, "updated_at": r.now().UTC()})
		if err != nil {
			return err
		}
		if result.RowsAffected != 1 {
			return ErrTenantGovernanceRunInvalidTransition
		}
		return nil
	})
	return evidence, err
}

func (r *TenantGovernanceRunRepository) CurrentEvidence(tenantID uint64, kind string) (models.TenantGovernanceEvidence, error) {
	var evidence models.TenantGovernanceEvidence
	err := r.query().Table(evidence.TableName()).Select("tenant_governance_evidence.*").
		Join("JOIN tenant_governance_run ON tenant_governance_run.id = tenant_governance_evidence.run_id").
		Where("tenant_governance_evidence.tenant_id", tenantID).Where("tenant_governance_evidence.kind", kind).
		Where("tenant_governance_run.status", models.TenantGovernanceRunStatusCompleted).
		WhereNull("tenant_governance_evidence.stale_at").Where("tenant_governance_evidence.expires_at > ?", r.now().UTC()).
		OrderByDesc("tenant_governance_evidence.verified_at").First(&evidence)
	if errors.Is(err, frameworkerrors.OrmRecordNotFound) || evidence.ID == 0 {
		return models.TenantGovernanceEvidence{}, ErrTenantGovernanceEvidenceExpired
	}
	if err != nil {
		return models.TenantGovernanceEvidence{}, err
	}
	return evidence, nil
}

func (r *TenantGovernanceRunRepository) MarkStale(now time.Time) (int64, error) {
	now = now.UTC()
	result, err := r.query().Table((models.TenantGovernanceEvidence{}).TableName()).WhereNull("stale_at").Where("expires_at <= ?", now).
		Update(map[string]any{"stale_at": now, "updated_at": now})
	if err != nil {
		return 0, err
	}
	runIDs, err := r.expiredRunIDs(now)
	if err != nil {
		return 0, err
	}
	if len(runIDs) > 0 {
		if _, err := r.query().Table((models.TenantGovernanceRun{}).TableName()).Where("status", models.TenantGovernanceRunStatusCompleted).
			WhereIn("id", runIDs).Update(map[string]any{"status": models.TenantGovernanceRunStatusStale, "updated_at": now}); err != nil {
			return 0, err
		}
	}
	return result.RowsAffected, nil
}

func (r *TenantGovernanceRunRepository) expiredRunIDs(now time.Time) ([]any, error) {
	rows := make([]models.TenantGovernanceEvidence, 0)
	if err := r.query().Table((models.TenantGovernanceEvidence{}).TableName()).Select("run_id").Where("expires_at <= ?", now).Get(&rows); err != nil {
		return nil, err
	}
	ids := make([]any, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RunID)
	}
	return ids, nil
}

func (r *TenantGovernanceRunRepository) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(r.ctx, PlatformConnection())
}

func (r *TenantGovernanceRunRepository) query() contractsorm.Query { return r.orm().Query() }

func tenantGovernanceRunKindValid(kind string) bool {
	return kind == models.TenantGovernanceRunKindRetention || kind == models.TenantGovernanceRunKindExport || kind == models.TenantGovernanceRunKindIsolationVerify
}

func tenantGovernanceTransitionAllowed(from, to string) bool {
	allowed := map[string]map[string]bool{
		models.TenantGovernanceRunStatusPending:          {models.TenantGovernanceRunStatusRunning: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusRunning:          {models.TenantGovernanceRunStatusAwaitingEvidence: true, models.TenantGovernanceRunStatusArtifactWritten: true, models.TenantGovernanceRunStatusCompleted: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusAwaitingEvidence: {models.TenantGovernanceRunStatusCompleted: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusArtifactWritten:  {models.TenantGovernanceRunStatusCompleted: true, models.TenantGovernanceRunStatusFailed: true},
		models.TenantGovernanceRunStatusCompleted:        {models.TenantGovernanceRunStatusStale: true},
	}
	return allowed[from][to]
}

func tenantGovernanceRunTerminal(status string) bool {
	return status == models.TenantGovernanceRunStatusCompleted || status == models.TenantGovernanceRunStatusFailed || status == models.TenantGovernanceRunStatusStale
}

// Source: tenant_governance_service.go
type TenantGovernanceService struct {
	ctx          context.Context
	loadHook     func(uint64) (TenantGovernancePolicy, bool, error)
	saveHook     func(TenantGovernancePolicy) error
	tableHook    func() bool
	evidenceHook func(uint64) (models.TenantGovernanceEvidence, error)
}

type TenantGovernancePolicy struct {
	TenantID        uint64                 `json:"tenant_id"`
	TenantCode      string                 `json:"tenant_code"`
	Modules         map[string]bool        `json:"modules"`
	Quotas          models.JSONMap         `json:"quotas"`
	RateLimit       TenantRateLimitPolicy  `json:"rate_limit"`
	Retention       TenantRetentionPolicy  `json:"retention"`
	DataExport      TenantDataActionPolicy `json:"data_export"`
	DataDeletion    TenantDataActionPolicy `json:"data_deletion"`
	IsolationProof  TenantIsolationProof   `json:"isolation_proof"`
	dataExportSet   bool
	dataDeletionSet bool
}

type TenantRateLimitPolicy struct {
	PerMinute int64 `json:"per_minute"`
}

type TenantRetentionPolicy struct {
	AuditDays int `json:"audit_days"`
	DataDays  int `json:"data_days"`
}

type TenantDataActionPolicy struct {
	Enabled          bool `json:"enabled"`
	RequiresApproval bool `json:"requires_approval"`
}

type TenantDataActionApprovalRequest struct {
	ApprovalID  string
	RequesterID uint64
	Resource    string
}

type TenantGovernancePatch struct {
	Modules        *map[string]bool           `json:"modules"`
	Quotas         *models.JSONMap            `json:"quotas"`
	RateLimit      *TenantRateLimitPatch      `json:"rate_limit"`
	Retention      *TenantRetentionPatch      `json:"retention"`
	DataExport     *TenantDataActionPatch     `json:"data_export"`
	DataDeletion   *TenantDataActionPatch     `json:"data_deletion"`
	IsolationProof *TenantIsolationProofPatch `json:"isolation_proof"`
}

type TenantRateLimitPatch struct {
	PerMinute *int64 `json:"per_minute"`
}

type TenantRetentionPatch struct {
	AuditDays *int `json:"audit_days"`
	DataDays  *int `json:"data_days"`
}

type TenantDataActionPatch struct {
	Enabled          *bool `json:"enabled"`
	RequiresApproval *bool `json:"requires_approval"`
}

type TenantIsolationProofPatch struct {
	Verified                 *bool   `json:"verified"`
	Evidence                 *string `json:"evidence"`
	Digest                   *string `json:"digest"`
	VerifiedAt               *string `json:"verified_at"`
	ExpiresAt                *string `json:"expires_at"`
	EvidenceTTLHours         *int    `json:"evidence_ttl_hours"`
	VerificationCadenceHours *int    `json:"verification_cadence_hours"`
}

type TenantIsolationProof struct {
	Verified                 bool   `json:"verified"`
	Evidence                 string `json:"evidence"`
	Digest                   string `json:"digest"`
	VerifiedAt               string `json:"verified_at,omitempty"`
	ExpiresAt                string `json:"expires_at,omitempty"`
	EvidenceTTLHours         int    `json:"evidence_ttl_hours"`
	VerificationCadenceHours int    `json:"verification_cadence_hours"`
}

var (
	ErrTenantModuleDisabled   = errors.New("tenant module is disabled")
	ErrTenantApprovalRequired = errors.New("tenant governance action requires approval")
	ErrTenantDataActionDenied = errors.New("tenant governance data action disabled")
	ErrTenantRetentionInvalid = errors.New("tenant retention policy invalid")
	ErrTenantIsolationMissing = errors.New("tenant isolation proof missing")
)

type tenantGovernanceRow struct {
	TenantID       uint64     `gorm:"column:tenant_id"`
	TenantCode     string     `gorm:"column:tenant_code"`
	Modules        string     `gorm:"column:modules"`
	Quotas         string     `gorm:"column:quotas"`
	RateLimit      string     `gorm:"column:rate_limit"`
	Retention      string     `gorm:"column:retention"`
	DataExport     string     `gorm:"column:data_export"`
	DataDeletion   string     `gorm:"column:data_deletion"`
	IsolationProof string     `gorm:"column:isolation_proof"`
	UpdatedAt      *time.Time `gorm:"column:updated_at"`
}

var tenantGovernanceMemory = struct {
	sync.Mutex
	items map[uint64]TenantGovernancePolicy
}{items: map[uint64]TenantGovernancePolicy{}}

func NewTenantGovernanceService() *TenantGovernanceService {
	return &TenantGovernanceService{}
}

func (s *TenantGovernanceService) WithContext(ctx context.Context) *TenantGovernanceService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantGovernanceService) DefaultPolicy(tenant Tenant) TenantGovernancePolicy {
	features := baseTenantFeaturesWithContext(s.ctx, tenant)
	modules := map[string]bool{}
	if raw, ok := features["modules"].(map[string]any); ok {
		for key, value := range raw {
			modules[key] = truthy(value)
		}
	}
	quotas := baseTenantQuotasWithContext(s.ctx, tenant)
	return TenantGovernancePolicy{
		TenantID:   tenant.ID,
		TenantCode: tenant.Code,
		Modules:    modules,
		Quotas:     quotas,
		RateLimit: TenantRateLimitPolicy{
			PerMinute: jsonInt64(quotas, "api_rate_per_minute"),
		},
		Retention:      TenantRetentionPolicy{AuditDays: 180, DataDays: 365},
		DataExport:     defaultTenantDataActionPolicy(),
		DataDeletion:   defaultTenantDataActionPolicy(),
		IsolationProof: TenantIsolationProof{EvidenceTTLHours: 24, VerificationCadenceHours: 24},
	}
}

func (s *TenantGovernanceService) Policy(tenant Tenant) (TenantGovernancePolicy, error) {
	base := s.DefaultPolicy(tenant)
	persisted, ok, err := s.loadPolicy(tenant.ID)
	if err != nil {
		return TenantGovernancePolicy{}, err
	}
	if !ok {
		return base, nil
	}
	policy := mergeTenantGovernancePolicy(base, persisted)
	s.applyCurrentIsolationEvidence(&policy)
	return policy, nil
}

func (s *TenantGovernanceService) SavePolicy(policy TenantGovernancePolicy) error {
	normalizeTenantGovernancePolicy(&policy)
	if !s.hasGovernanceTable() {
		tenantGovernanceMemory.Lock()
		tenantGovernanceMemory.items[policy.TenantID] = policy
		tenantGovernanceMemory.Unlock()
		return nil
	}
	return s.savePolicyDB(policy)
}

func (s *TenantGovernanceService) PatchPolicy(tenant Tenant, patch TenantGovernancePatch) (TenantGovernancePolicy, error) {
	policy, err := s.Policy(tenant)
	if err != nil {
		return TenantGovernancePolicy{}, err
	}
	applyTenantGovernancePatch(&policy, patch)
	if err := s.SavePolicy(policy); err != nil {
		return TenantGovernancePolicy{}, err
	}
	return s.Policy(tenant)
}

func (p TenantGovernancePolicy) ModuleEnabled(module string) bool {
	module = strings.TrimSpace(module)
	if module == "" {
		return true
	}
	enabled, ok := p.Modules[module]
	return !ok || enabled
}

func (p TenantGovernancePolicy) RequireModule(module string) error {
	if p.ModuleEnabled(module) {
		return nil
	}
	return ErrTenantModuleDisabled
}

func (p TenantGovernancePolicy) RequireDataExportApproval(ctx context.Context, req TenantDataActionApprovalRequest) error {
	return p.requireDataActionApproval(ctx, p.DataExport, req, "tenant.data.export")
}

func (p TenantGovernancePolicy) RequireDataDeletionApproval(ctx context.Context, req TenantDataActionApprovalRequest) error {
	return p.requireDataActionApproval(ctx, p.DataDeletion, req, "tenant.data.delete")
}

func (p TenantGovernancePolicy) RequireRetentionPolicy() error {
	if p.Retention.AuditDays <= 0 || p.Retention.DataDays <= 0 {
		return ErrTenantRetentionInvalid
	}
	return nil
}

func (p TenantGovernancePolicy) RequireIsolationProof() error {
	expiresAt, err := time.Parse(time.RFC3339, p.IsolationProof.ExpiresAt)
	if !p.IsolationProof.Verified || strings.TrimSpace(p.IsolationProof.Evidence) == "" || strings.TrimSpace(p.IsolationProof.Digest) == "" ||
		err != nil || !expiresAt.After(time.Now().UTC()) {
		return ErrTenantIsolationMissing
	}
	return nil
}

func (s *TenantGovernanceService) applyCurrentIsolationEvidence(policy *TenantGovernancePolicy) {
	if policy == nil || policy.TenantID == 0 {
		return
	}
	evidence, err := s.currentIsolationEvidence(policy.TenantID)
	if err != nil {
		policy.IsolationProof.Verified = false
		return
	}
	policy.IsolationProof.Verified = true
	policy.IsolationProof.Evidence = evidence.URI
	policy.IsolationProof.Digest = evidence.SHA256
	policy.IsolationProof.VerifiedAt = evidence.VerifiedAt.UTC().Format(time.RFC3339)
	policy.IsolationProof.ExpiresAt = evidence.ExpiresAt.UTC().Format(time.RFC3339)
}

func (s *TenantGovernanceService) currentIsolationEvidence(tenantID uint64) (models.TenantGovernanceEvidence, error) {
	if s.evidenceHook != nil {
		return s.evidenceHook(tenantID)
	}
	if !s.hasGovernanceEvidenceTable() {
		return models.TenantGovernanceEvidence{}, ErrTenantGovernanceEvidenceExpired
	}
	return NewTenantGovernanceRunRepository().WithContext(s.ctx).CurrentEvidence(tenantID, models.TenantGovernanceRunKindIsolationVerify)
}

func (s *TenantGovernanceService) hasGovernanceEvidenceTable() bool {
	schema := facades.Schema()
	return schema != nil && schema.Connection(PlatformConnection()).HasTable("tenant_governance_evidence")
}

func (p TenantGovernancePolicy) requireDataActionApproval(ctx context.Context, policy TenantDataActionPolicy, req TenantDataActionApprovalRequest, scope string) error {
	if !policy.Enabled {
		return ErrTenantDataActionDenied
	}
	if !policy.RequiresApproval {
		return nil
	}
	if strings.TrimSpace(req.ApprovalID) == "" || req.RequesterID == 0 || strings.TrimSpace(req.Resource) == "" {
		return ErrTenantApprovalRequired
	}
	err := NewEnterpriseSecurityControlService().RequireRegisteredPermissionApproval(
		contextOrBackground(ctx), req.ApprovalID, req.RequesterID, scope, req.Resource,
	)
	if errors.Is(err, ErrApprovalRequired) || errors.Is(err, ErrApprovalSelfApproved) {
		return ErrTenantApprovalRequired
	}
	return err
}

func TenantDataActionResource(action string, tenantIDs []uint64, qualifiers ...string) string {
	ids := append([]uint64(nil), tenantIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(id, 10))
	}
	resource := fmt.Sprintf("tenant-data:%s:%s", strings.TrimSpace(action), strings.Join(parts, ","))
	for _, qualifier := range qualifiers {
		if qualifier = strings.TrimSpace(qualifier); qualifier != "" {
			resource += ":" + qualifier
		}
	}
	return resource
}

func (s *TenantGovernanceService) loadPolicy(tenantID uint64) (TenantGovernancePolicy, bool, error) {
	if s.loadHook != nil {
		return s.loadHook(tenantID)
	}
	if !s.hasGovernanceTable() {
		tenantGovernanceMemory.Lock()
		defer tenantGovernanceMemory.Unlock()
		item, ok := tenantGovernanceMemory.items[tenantID]
		return item, ok, nil
	}
	row := tenantGovernanceRow{}
	err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).
		Query().
		Table("tenant_governance").
		Where("tenant_id", tenantID).
		First(&row)
	if err != nil {
		return TenantGovernancePolicy{}, false, err
	}
	if row.TenantID == 0 {
		return TenantGovernancePolicy{}, false, nil
	}
	policy, err := row.policy()
	return policy, true, err
}

func (s *TenantGovernanceService) savePolicyDB(policy TenantGovernancePolicy) error {
	if s.saveHook != nil {
		return s.saveHook(policy)
	}
	row := tenantGovernanceRowFromPolicy(policy)
	now := time.Now()
	_, err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Exec(`
		INSERT INTO tenant_governance (
			tenant_id, tenant_code, modules, quotas, rate_limit, retention,
			data_export, data_deletion, isolation_proof, created_at, updated_at
		)
		VALUES (?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, ?::jsonb, ?, ?)
		ON CONFLICT (tenant_id) DO UPDATE SET
			tenant_code = EXCLUDED.tenant_code,
			modules = EXCLUDED.modules,
			quotas = EXCLUDED.quotas,
			rate_limit = EXCLUDED.rate_limit,
			retention = EXCLUDED.retention,
			data_export = EXCLUDED.data_export,
			data_deletion = EXCLUDED.data_deletion,
			isolation_proof = EXCLUDED.isolation_proof,
			updated_at = EXCLUDED.updated_at
	`, row.TenantID, row.TenantCode, row.Modules, row.Quotas, row.RateLimit, row.Retention,
		row.DataExport, row.DataDeletion, row.IsolationProof, now, now)
	return err
}

func (s *TenantGovernanceService) hasGovernanceTable() bool {
	if s.tableHook != nil {
		return s.tableHook()
	}
	schema := facades.Schema()
	if schema == nil {
		return false
	}
	return schema.Connection(PlatformConnection()).HasTable("tenant_governance")
}

func mergeTenantGovernancePolicy(base, override TenantGovernancePolicy) TenantGovernancePolicy {
	base.Modules = mergeBoolMaps(base.Modules, override.Modules)
	base.Quotas = mergeJSONMaps(base.Quotas, override.Quotas)
	if override.RateLimit.PerMinute > 0 {
		base.RateLimit = override.RateLimit
	}
	if override.Retention.AuditDays > 0 {
		base.Retention.AuditDays = override.Retention.AuditDays
	}
	if override.Retention.DataDays > 0 {
		base.Retention.DataDays = override.Retention.DataDays
	}
	if override.dataExportSet {
		base.DataExport = override.DataExport
	}
	if override.dataDeletionSet {
		base.DataDeletion = override.DataDeletion
	}
	base.IsolationProof = override.IsolationProof
	return base
}

func normalizeTenantGovernancePolicy(policy *TenantGovernancePolicy) {
	if policy.Modules == nil {
		policy.Modules = map[string]bool{}
	}
	if policy.Quotas == nil {
		policy.Quotas = models.JSONMap{}
	}
	policy.dataExportSet = true
	policy.dataDeletionSet = true
}

func defaultTenantDataActionPolicy() TenantDataActionPolicy {
	return TenantDataActionPolicy{Enabled: true, RequiresApproval: true}
}

func applyTenantGovernancePatch(policy *TenantGovernancePolicy, patch TenantGovernancePatch) {
	if patch.Modules != nil {
		policy.Modules = mergeBoolMaps(policy.Modules, *patch.Modules)
	}
	if patch.Quotas != nil {
		policy.Quotas = mergeJSONMaps(policy.Quotas, *patch.Quotas)
	}
	if patch.RateLimit != nil && patch.RateLimit.PerMinute != nil {
		policy.RateLimit.PerMinute = *patch.RateLimit.PerMinute
	}
	if patch.Retention != nil {
		if patch.Retention.AuditDays != nil {
			policy.Retention.AuditDays = *patch.Retention.AuditDays
		}
		if patch.Retention.DataDays != nil {
			policy.Retention.DataDays = *patch.Retention.DataDays
		}
	}
	applyTenantDataActionPatch(&policy.DataExport, patch.DataExport)
	applyTenantDataActionPatch(&policy.DataDeletion, patch.DataDeletion)
	applyTenantIsolationProofPatch(&policy.IsolationProof, patch.IsolationProof)
}

func applyTenantDataActionPatch(policy *TenantDataActionPolicy, patch *TenantDataActionPatch) {
	if patch == nil {
		return
	}
	if patch.Enabled != nil {
		policy.Enabled = *patch.Enabled
	}
	if patch.RequiresApproval != nil {
		policy.RequiresApproval = *patch.RequiresApproval
	}
}

func applyTenantIsolationProofPatch(policy *TenantIsolationProof, patch *TenantIsolationProofPatch) {
	if patch == nil {
		return
	}
	if patch.Verified != nil {
		policy.Verified = *patch.Verified
	}
	if patch.Evidence != nil {
		policy.Evidence = *patch.Evidence
	}
	if patch.Digest != nil {
		policy.Digest = *patch.Digest
	}
	if patch.VerifiedAt != nil {
		policy.VerifiedAt = *patch.VerifiedAt
	}
	if patch.ExpiresAt != nil {
		policy.ExpiresAt = *patch.ExpiresAt
	}
	if patch.EvidenceTTLHours != nil {
		policy.EvidenceTTLHours = *patch.EvidenceTTLHours
	}
	if patch.VerificationCadenceHours != nil {
		policy.VerificationCadenceHours = *patch.VerificationCadenceHours
	}
}

func mergeBoolMaps(base, override map[string]bool) map[string]bool {
	merged := map[string]bool{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true" || typed == "1" || typed == "enabled"
	default:
		return false
	}
}

func tenantGovernanceRowFromPolicy(policy TenantGovernancePolicy) tenantGovernanceRow {
	return tenantGovernanceRow{
		TenantID:       policy.TenantID,
		TenantCode:     policy.TenantCode,
		Modules:        jsonStringMust(policy.Modules),
		Quotas:         jsonStringMust(policy.Quotas),
		RateLimit:      jsonStringMust(policy.RateLimit),
		Retention:      jsonStringMust(policy.Retention),
		DataExport:     jsonStringMust(policy.DataExport),
		DataDeletion:   jsonStringMust(policy.DataDeletion),
		IsolationProof: jsonStringMust(policy.IsolationProof),
	}
}

func (r tenantGovernanceRow) policy() (TenantGovernancePolicy, error) {
	policy := TenantGovernancePolicy{
		TenantID:     r.TenantID,
		TenantCode:   r.TenantCode,
		DataExport:   defaultTenantDataActionPolicy(),
		DataDeletion: defaultTenantDataActionPolicy(),
	}
	if err := json.Unmarshal([]byte(emptyJSON(r.Modules)), &policy.Modules); err != nil {
		return TenantGovernancePolicy{}, err
	}
	if err := json.Unmarshal([]byte(emptyJSON(r.Quotas)), &policy.Quotas); err != nil {
		return TenantGovernancePolicy{}, err
	}
	_ = json.Unmarshal([]byte(emptyJSON(r.RateLimit)), &policy.RateLimit)
	_ = json.Unmarshal([]byte(emptyJSON(r.Retention)), &policy.Retention)
	if hasJSONPayload(r.DataExport) {
		_ = json.Unmarshal([]byte(r.DataExport), &policy.DataExport)
		policy.dataExportSet = true
	}
	if hasJSONPayload(r.DataDeletion) {
		_ = json.Unmarshal([]byte(r.DataDeletion), &policy.DataDeletion)
		policy.dataDeletionSet = true
	}
	_ = json.Unmarshal([]byte(emptyJSON(r.IsolationProof)), &policy.IsolationProof)
	return policy, nil
}

func jsonStringMust(value any) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}

func emptyJSON(value string) string {
	if value == "" {
		return "{}"
	}
	return value
}

func hasJSONPayload(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "null" && value != "{}"
}

// Source: tenant_isolation_artifact_store_s3.go
const tenantIsolationObjectRetention = 48 * time.Hour

type s3TenantIsolationArtifactStore struct {
	config StorageConfig
	now    func() time.Time
}

func configuredTenantIsolationArtifactStore(ctx context.Context) TenantIsolationArtifactStore {
	config, err := NewStorageConfigService().WithContext(ctx).ActiveDefault()
	if err != nil || config.Driver != storageDriverS3Compatible || strings.TrimSpace(config.Bucket) == "" {
		return nil
	}
	return &s3TenantIsolationArtifactStore{config: config, now: time.Now}
}

func (s *s3TenantIsolationArtifactStore) WriteImmutable(ctx context.Context, tenant Tenant, payload []byte) (TenantIsolationArtifact, error) {
	if s == nil || s.config.Driver != storageDriverS3Compatible || tenant.ID == 0 || len(payload) == 0 {
		return TenantIsolationArtifact{}, ErrTenantIsolationVerification
	}
	now := s.now
	if now == nil {
		now = time.Now
	}
	writtenAt := now().UTC()
	digest := digestBytes(payload)
	objectPath := path.Join(
		s.config.PathPrefix,
		"tenant-governance/isolation",
		fmt.Sprint(tenant.ID),
		fmt.Sprintf("%s-%s.json", writtenAt.Format("20060102T150405.000000000Z"), strings.TrimPrefix(digest, "sha256:")[:16]),
	)
	client := newObjectStorageClient(s.config)
	version, err := putImmutableS3Object(ctx, client, objectPath, payload, writtenAt.Add(tenantIsolationObjectRetention))
	if err != nil || strings.TrimSpace(version) == "" {
		return TenantIsolationArtifact{}, ErrTenantIsolationVerification
	}
	if err := verifyImmutableS3Object(ctx, client, objectPath, version, payload, writtenAt); err != nil {
		return TenantIsolationArtifact{}, ErrTenantIsolationVerification
	}
	return TenantIsolationArtifact{
		URI: "s3://" + s.config.Bucket + "/" + objectPath, ObjectVersion: version, SHA256: digest,
	}, nil
}

func putImmutableS3Object(ctx context.Context, client *objectStorageClient, objectPath string, payload []byte, retainUntil time.Time) (string, error) {
	endpoint, err := client.ObjectURL(objectPath)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Amz-Object-Lock-Mode", "COMPLIANCE")
	request.Header.Set("X-Amz-Object-Lock-Retain-Until-Date", retainUntil.UTC().Format(time.RFC3339))
	client.Sign(request, payload)
	response, err := client.HTTPClient().Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return "", fmt.Errorf("immutable object write failed: %s %s", response.Status, strings.TrimSpace(string(body)))
	}
	return strings.TrimSpace(response.Header.Get("X-Amz-Version-Id")), nil
}

func verifyImmutableS3Object(ctx context.Context, client *objectStorageClient, objectPath, version string, payload []byte, writtenAt time.Time) error {
	metadata, err := immutableS3Request(ctx, client, http.MethodHead, objectPath, version)
	if err != nil {
		return err
	}
	defer func() { _ = metadata.Body.Close() }()
	retainUntil, err := time.Parse(time.RFC3339, metadata.Header.Get("X-Amz-Object-Lock-Retain-Until-Date"))
	if err != nil || !strings.EqualFold(metadata.Header.Get("X-Amz-Object-Lock-Mode"), "COMPLIANCE") ||
		metadata.Header.Get("X-Amz-Version-Id") != version || !retainUntil.After(writtenAt) {
		return ErrTenantIsolationVerification
	}
	object, err := immutableS3Request(ctx, client, http.MethodGet, objectPath, version)
	if err != nil {
		return err
	}
	defer func() { _ = object.Body.Close() }()
	stored, err := io.ReadAll(io.LimitReader(object.Body, int64(len(payload)+1)))
	if err != nil || len(stored) != len(payload) || !sameDigest(digestBytes(stored), digestBytes(payload)) {
		return ErrTenantIsolationVerification
	}
	return nil
}

// Source: tenant_isolation_verifier.go
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

// Source: tenant_permission_audit_service.go
const (
	TenantPermissionAuditOperationUpdate         = "update"
	TenantPermissionAuditOperationPlanChange     = "plan_change"
	TenantPermissionAuditOperationLegacySnapshot = "legacy_snapshot"
	TenantPermissionAuditSourcePlatform          = "platform"
	TenantPermissionAuditSourceLegacyMigration   = "legacy_migration"
	TenantPermissionAuditSourcePlanChange        = "plan_change"
	TenantPermissionAuditSystemOperatorName      = "system"
)

type TenantPermissionAuditInput struct {
	TenantID     uint64
	TenantCode   string
	Operation    string
	Source       string
	Before       TenantPermissionPayload
	After        TenantPermissionPayload
	OperatorID   uint64
	OperatorName string
	Remark       string
}

type TenantPermissionAuditService struct{}

func NewTenantPermissionAuditService() *TenantPermissionAuditService {
	return &TenantPermissionAuditService{}
}

func (s *TenantPermissionAuditService) Log(input TenantPermissionAuditInput) error {
	return s.LogWithQuery(OrmForConnection(PlatformConnection()).Query(), input)
}

func (s *TenantPermissionAuditService) LogWithQuery(query contractsorm.Query, input TenantPermissionAuditInput) error {
	record := BuildTenantPermissionAudit(input)
	before, err := json.Marshal(nullIfEmpty(record.BeforeSnapshot))
	if err != nil {
		return err
	}
	after, err := json.Marshal(nullIfEmpty(record.AfterSnapshot))
	if err != nil {
		return err
	}
	diff, err := json.Marshal(nullIfEmpty(record.Diff))
	if err != nil {
		return err
	}
	_, err = query.Exec(`
			INSERT INTO tenant_permission_audit (
				tenant_id, tenant_code, operation, source,
				before_snapshot, after_snapshot, diff,
				operator_id, operator_name, created_at, updated_at, remark
			)
			VALUES (?, ?, ?, ?, ?::jsonb, ?::jsonb, ?::jsonb, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
		`,
		record.TenantID, record.TenantCode, record.Operation, record.Source,
		string(before), string(after), string(diff),
		record.OperatorID, record.OperatorName, record.Remark,
	)
	return err
}

func BuildTenantPermissionAudit(input TenantPermissionAuditInput) models.TenantPermissionAudit {
	before := normalizePermissionPayload(input.Before)
	after := normalizePermissionPayload(input.After)
	added := sortedSetDiff(after.Allowed, before.Allowed)
	removed := sortedSetDiff(before.Allowed, after.Allowed)
	unchanged := sortedSetIntersect(before.Allowed, after.Allowed)
	operatorName := input.OperatorName
	if operatorName == "" && input.OperatorID == 0 {
		operatorName = TenantPermissionAuditSystemOperatorName
	}

	return models.TenantPermissionAudit{
		TenantID:       input.TenantID,
		TenantCode:     input.TenantCode,
		Operation:      input.Operation,
		Source:         input.Source,
		BeforeSnapshot: permissionPayloadMap(before),
		AfterSnapshot:  permissionPayloadMap(after),
		Added:          added,
		Removed:        removed,
		Unchanged:      unchanged,
		Diff: models.JSONMap{
			"added":     added,
			"removed":   removed,
			"unchanged": unchanged,
		},
		OperatorID:   input.OperatorID,
		OperatorName: operatorName,
		Remark:       input.Remark,
	}
}

// Source: tenant_permission_service.go
const tenantPermissionsFeatureKey = "permissions"

type TenantPermissionPayload struct {
	Allowed []string `json:"allowed"`
}

type TenantPermissionPlanDiff struct {
	Plan       string   `json:"plan"`
	Allowed    []string `json:"allowed"`
	Added      []string `json:"added"`
	Removed    []string `json:"removed"`
	Unchanged  []string `json:"unchanged"`
	Permission []string `json:"permission"`
}

type TenantPermissionSnapshot struct {
	LegacyFullAccess bool
	Allowed          map[string]struct{}
}

func TenantAllowsRoute(tenant Tenant, method, path string) bool {
	permission := PermissionForRoute(method, path)
	if permission == "" {
		return true
	}
	return TenantAllowsPermission(tenant, permission)
}

func TenantAllowsPermission(tenant Tenant, permission string) bool {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return true
	}
	snapshot := TenantPermissionSnapshotFromFeatures(tenant.Features)
	if snapshot.LegacyFullAccess {
		return true
	}
	if _, allowed := snapshot.Allowed[permission]; !allowed {
		return false
	}
	return tenantPermissionAncestorsAllowed(snapshot.Allowed, permission)
}

func TenantAllowedPermissionNames(tenant Tenant) []string {
	snapshot := TenantPermissionSnapshotFromFeatures(tenant.Features)
	if snapshot.LegacyFullAccess {
		return nil
	}
	names := make([]string, 0, len(snapshot.Allowed))
	for name := range snapshot.Allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TenantFullPermissionNames() []string {
	names := make([]string, 0)
	for _, menu := range flattenAdminMenus(TenantPermissionCatalogMenus()) {
		names = append(names, menu.Name)
	}
	for _, permission := range routePermissionMap() {
		if strings.HasPrefix(permission, "platform:") {
			continue
		}
		names = append(names, permission)
	}
	return normalizeStrings(names)
}

func TenantPermissionPayloadFromTenant(tenant Tenant) TenantPermissionPayload {
	payload, _ := tenantPermissionPayloadFromFeatures(tenant.Features)
	return normalizePermissionPayload(payload)
}

func TenantEffectivePermissionPayload(tenant Tenant) TenantPermissionPayload {
	if TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess {
		return TenantPermissionPayload{
			Allowed: TenantFullPermissionNames(),
		}
	}
	return TenantPermissionPayloadFromTenant(tenant)
}

func TenantPermissionSnapshotFromFeatures(features models.JSONMap) TenantPermissionSnapshot {
	payload, ok := tenantPermissionPayloadFromFeatures(features)
	if !ok {
		return TenantPermissionSnapshot{
			LegacyFullAccess: true,
			Allowed:          map[string]struct{}{},
		}
	}
	return TenantPermissionSnapshot{
		Allowed: stringSet(payload.Allowed),
	}
}

func SnapshotFeaturesForPlan(planFeatures, input models.JSONMap) models.JSONMap {
	features := platformManagedFeatures(input)
	if planPayload, ok := tenantPermissionPayloadFromFeatures(planFeatures); ok {
		features[tenantPermissionsFeatureKey] = permissionPayloadMap(planPayload)
	}
	if inputPayload, ok := tenantPermissionPayloadFromFeatures(input); ok {
		features[tenantPermissionsFeatureKey] = permissionPayloadMap(inputPayload)
	}
	return features
}

func featuresWithoutTenantPermissions(input models.JSONMap) models.JSONMap {
	features := platformManagedFeatures(input)
	delete(features, tenantPermissionsFeatureKey)
	return features
}

func preserveTenantPermissionFeature(features, existing models.JSONMap) models.JSONMap {
	if features == nil {
		features = models.JSONMap{}
	}
	if existing == nil {
		delete(features, tenantPermissionsFeatureKey)
		return features
	}
	if raw, ok := existing[tenantPermissionsFeatureKey]; ok {
		features[tenantPermissionsFeatureKey] = raw
		return features
	}
	delete(features, tenantPermissionsFeatureKey)
	return features
}

func FilterAdminMenusByTenantPermissions(tenant Tenant, menus []AdminMenuItem) []AdminMenuItem {
	if TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess {
		return menus
	}
	allowedIDs := tenantAllowedMenuIDs(tenant, menus)
	filtered := make([]AdminMenuItem, 0, len(allowedIDs))
	for _, menu := range menus {
		if _, ok := allowedIDs[menu.ID]; ok {
			filtered = append(filtered, menu)
		}
	}
	return filtered
}

func TenantPermissionCatalogMenus() []AdminMenuItem {
	seeds := seeders.TenantMenuCatalogSeeds()
	menus := make([]AdminMenuItem, 0, len(seeds))
	for _, seed := range seeds {
		menus = append(menus, AdminMenuItem{
			ID:        seed.ID,
			ParentID:  seed.ParentID,
			Name:      seed.Name,
			Path:      seed.Path,
			Component: seed.Component,
			Redirect:  seed.Redirect,
			Status:    1,
			Sort:      int16(seed.Sort),
			Meta:      models.JSONMap(seed.Meta),
		})
	}
	return buildAdminMenuTree(menus, 0)
}

func BuildLegacyTenantPermissionSnapshot(tenant Tenant) (TenantPermissionPayload, bool) {
	if _, ok := tenantPermissionPayloadFromFeatures(tenant.Features); ok {
		return TenantPermissionPayload{}, false
	}
	return TenantPermissionPayload{
		Allowed: TenantFullPermissionNames(),
	}, true
}

func ValidateTenantRolePermissions(tenant Tenant, permissions []string) error {
	if TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess {
		return nil
	}
	for _, permission := range permissions {
		if !TenantAllowsPermission(tenant, permission) {
			return BusinessError{Message: "角色权限超出租户授权范围"}
		}
	}
	return nil
}

func flattenAdminMenus(menus []AdminMenuItem) []AdminMenuItem {
	flattened := make([]AdminMenuItem, 0, len(menus))
	for _, menu := range menus {
		flattened = append(flattened, menu)
		flattened = append(flattened, flattenAdminMenus(menu.Children)...)
	}
	return flattened
}

func adminMenuIDs(menus []AdminMenuItem) []uint64 {
	ids := make([]uint64, 0, len(menus))
	for _, menu := range menus {
		ids = append(ids, menu.ID)
	}
	return ids
}

func tenantAllowedMenuIDs(tenant Tenant, menus []AdminMenuItem) map[uint64]struct{} {
	byID := make(map[uint64]AdminMenuItem, len(menus))
	allowed := make(map[uint64]struct{})
	for _, menu := range menus {
		byID[menu.ID] = menu
		if TenantAllowsPermission(tenant, menu.Name) {
			allowed[menu.ID] = struct{}{}
		}
	}
	for id := range allowed {
		parentID := byID[id].ParentID
		for parentID != 0 {
			parent, ok := byID[parentID]
			if !ok {
				break
			}
			allowed[parent.ID] = struct{}{}
			parentID = parent.ParentID
		}
	}
	return allowed
}

func tenantPermissionAncestorsAllowed(allowed map[string]struct{}, permission string) bool {
	parentByName := tenantPermissionParentByName()
	parent, ok := parentByName[permission]
	for ok && parent != "" {
		if _, exists := allowed[parent]; !exists {
			return false
		}
		parent, ok = parentByName[parent]
	}
	return true
}

func tenantPermissionParentByName() map[string]string {
	menus := flattenAdminMenus(TenantPermissionCatalogMenus())
	byID := make(map[uint64]AdminMenuItem, len(menus))
	for _, menu := range menus {
		byID[menu.ID] = menu
	}

	parentByName := make(map[string]string, len(menus))
	for _, menu := range menus {
		if menu.Name == "" || menu.ParentID == 0 {
			continue
		}
		parent, ok := byID[menu.ParentID]
		if !ok || parent.Name == "" {
			continue
		}
		parentByName[menu.Name] = parent.Name
	}
	return parentByName
}

func tenantPermissionPayloadFromFeatures(features models.JSONMap) (TenantPermissionPayload, bool) {
	if features == nil {
		return TenantPermissionPayload{}, false
	}
	raw, ok := features[tenantPermissionsFeatureKey]
	if !ok || raw == nil {
		return TenantPermissionPayload{}, false
	}
	payload := TenantPermissionPayload{
		Allowed: stringListFromMap(raw, "allowed"),
	}
	return payload, true
}

func permissionPayloadMap(payload TenantPermissionPayload) models.JSONMap {
	payload = normalizePermissionPayload(payload)
	return models.JSONMap{
		"allowed": payload.Allowed,
	}
}

func stringListFromMap(raw any, key string) []string {
	values, ok := raw.(map[string]any)
	if !ok {
		if jsonValues, ok := raw.(models.JSONMap); ok {
			values = map[string]any(jsonValues)
		} else {
			return nil
		}
	}
	return normalizeStringSlice(values[key])
}

func normalizeStringSlice(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return normalizeStrings(values)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return normalizeStrings(out)
	default:
		return nil
	}
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizePermissionPayload(payload TenantPermissionPayload) TenantPermissionPayload {
	return TenantPermissionPayload{
		Allowed: normalizeStrings(payload.Allowed),
	}
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range normalizeStrings(values) {
		set[value] = struct{}{}
	}
	return set
}

func sortedSetDiff(left, right []string) []string {
	rightSet := stringSet(right)
	out := make([]string, 0)
	for _, value := range normalizeStrings(left) {
		if _, ok := rightSet[value]; !ok {
			out = append(out, value)
		}
	}
	return out
}

func sortedSetIntersect(left, right []string) []string {
	rightSet := stringSet(right)
	out := make([]string, 0)
	for _, value := range normalizeStrings(left) {
		if _, ok := rightSet[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

// Source: tenant_plan_service.go
const TenantPlanStatusEnabled int8 = 1

type TenantPlan = models.TenantPlan

type TenantPlanPayload struct {
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Status   int8           `json:"status"`
	Sort     int            `json:"sort"`
	Billing  models.JSONMap `json:"billing"`
	Quotas   models.JSONMap `json:"quotas"`
	Features models.JSONMap `json:"features"`
	Remark   string         `json:"remark"`
}

type TenantPlanService struct {
	ctx context.Context
}

func NewTenantPlanService() *TenantPlanService {
	return &TenantPlanService{}
}

func (s *TenantPlanService) WithContext(ctx context.Context) *TenantPlanService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantPlanService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (p TenantPlanPayload) TenantPlan() TenantPlan {
	status := p.Status
	if status == 0 {
		status = TenantPlanStatusEnabled
	}
	return TenantPlan{
		Code:     strings.TrimSpace(p.Code),
		Name:     strings.TrimSpace(p.Name),
		Status:   status,
		Sort:     p.Sort,
		Billing:  p.Billing,
		Quotas:   p.Quotas,
		Features: platformManagedFeatures(p.Features),
		Remark:   strings.TrimSpace(p.Remark),
	}
}

func (s *TenantPlanService) List(filters map[string]string, page, pageSize int) (request.PageResult[TenantPlan], error) {
	query := s.orm().Query().Table("tenant_plan")
	query = query.Scopes(scopes.Contains("code", filters["code"]))
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))
	return request.Paginate[TenantPlan](query.OrderBy("sort").OrderByDesc("id"), page, pageSize)
}

func (s *TenantPlanService) Options() ([]TenantPlan, error) {
	plans := make([]TenantPlan, 0)
	err := s.orm().
		Query().
		Table("tenant_plan").
		Where("status", TenantPlanStatusEnabled).
		OrderBy("sort").
		OrderBy("id").
		Get(&plans)
	return plans, err
}

func (s *TenantPlanService) Create(input TenantPlanPayload) (TenantPlan, error) {
	plan := input.TenantPlan()
	if err := validateTenantPlan(plan); err != nil {
		return TenantPlan{}, err
	}
	err := s.orm().Transaction(func(tx contractsorm.Query) error {
		row := TenantPlan{
			Code: plan.Code, Name: plan.Name, Status: plan.Status,
			Sort: plan.Sort, Remark: plan.Remark,
		}
		if err := tx.Table("tenant_plan").Create(&row); err != nil {
			return err
		}
		plan.ID = row.ID
		return updateTenantPlanJSONColumnsWithQuery(tx, plan.ID, plan)
	})
	if err != nil {
		return TenantPlan{}, err
	}
	return plan, nil
}

func (s *TenantPlanService) Update(id uint64, input TenantPlanPayload) (TenantPlan, error) {
	plan := input.TenantPlan()
	if err := validateTenantPlan(plan); err != nil {
		return TenantPlan{}, err
	}
	if err := s.ensureTenantPlanExists(id); err != nil {
		return TenantPlan{}, err
	}
	_, err := s.orm().
		Query().
		Table("tenant_plan").
		Where("id", id).
		Update(map[string]any{
			"code": plan.Code, "name": plan.Name, "status": plan.Status,
			"sort": plan.Sort, "remark": plan.Remark, "updated_at": time.Now(),
		})
	if err != nil {
		return TenantPlan{}, err
	}
	if err := s.updateTenantPlanJSONColumns(id, plan); err != nil {
		return TenantPlan{}, err
	}
	plan.ID = id
	return plan, nil
}

func (s *TenantPlanService) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	codes := make([]string, 0, len(ids))
	if err := s.orm().
		Query().
		Table("tenant_plan").
		WhereIn("id", uint64Any(ids)).
		Pluck("code", &codes); err != nil {
		return err
	}
	if len(codes) == 0 {
		return nil
	}
	count, err := s.orm().
		Query().
		Table("tenant").
		WhereIn("plan", stringAny(codes)).
		Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return BusinessError{Message: "套餐已被租户使用，不能删除"}
	}
	_, err = s.orm().
		Query().
		Table("tenant_plan").
		WhereIn("id", uint64Any(ids)).
		Delete()
	return err
}

func (s *TenantPlanService) ExistsActive(code string) (bool, error) {
	if strings.TrimSpace(code) == "" {
		return false, nil
	}
	count, err := s.orm().
		Query().
		Table("tenant_plan").
		Where("code", strings.TrimSpace(code)).
		Where("status", TenantPlanStatusEnabled).
		Count()
	return count > 0, err
}

func (s *TenantPlanService) ActiveByCode(code string) (TenantPlan, error) {
	var plan TenantPlan
	if strings.TrimSpace(code) == "" {
		return plan, nil
	}
	err := s.orm().
		Query().
		Table("tenant_plan").
		Where("code", strings.TrimSpace(code)).
		Where("status", TenantPlanStatusEnabled).
		First(&plan)
	return plan, err
}

func (s *TenantPlanService) ensureTenantPlanExists(id uint64) error {
	count, err := s.orm().
		Query().
		Table("tenant_plan").
		Where("id", id).
		Count()
	if err != nil {
		return err
	}
	if count == 0 {
		return BusinessError{Message: "套餐不存在"}
	}
	return nil
}

func validateTenantPlan(plan TenantPlan) error {
	if plan.Code == "" || plan.Name == "" {
		return BusinessError{Message: "套餐编码和名称不能为空"}
	}
	if plan.Status != TenantPlanStatusEnabled && plan.Status != 2 {
		return BusinessError{Message: "套餐状态无效"}
	}
	return nil
}

func (s *TenantPlanService) updateTenantPlanJSONColumns(id uint64, plan TenantPlan) error {
	return updateTenantPlanJSONColumnsWithQuery(s.orm().Query(), id, plan)
}

func updateTenantPlanJSONColumnsWithQuery(query contractsorm.Query, id uint64, plan TenantPlan) error {
	billing, err := json.Marshal(nullIfEmpty(plan.Billing))
	if err != nil {
		return err
	}
	quotas, err := json.Marshal(nullIfEmpty(plan.Quotas))
	if err != nil {
		return err
	}
	features, err := json.Marshal(nullIfEmpty(plan.Features))
	if err != nil {
		return err
	}
	_, err = query.Exec(
		"UPDATE tenant_plan SET billing = ?::jsonb, quotas = ?::jsonb, features = ?::jsonb WHERE id = ?",
		string(billing), string(quotas), string(features), id,
	)
	return err
}

// Source: tenant_rate_limiter.go
type TenantRateLimiter = tenantservice.RateLimiter

func NewTenantRateLimiter() *TenantRateLimiter {
	return tenantservice.NewRateLimiter()
}

// Source: tenant_retention_service.go
type TenantRetentionResult struct {
	TenantID    uint64 `json:"tenant_id"`
	TenantCode  string `json:"tenant_code"`
	RunID       uint64 `json:"run_id"`
	PlanID      string `json:"plan_id,omitempty"`
	AuditDays   int    `json:"audit_days"`
	TargetCount int64  `json:"target_count"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

type TenantRetentionService struct {
	ctx     context.Context
	now     func() time.Time
	tenants func(context.Context) ([]Tenant, error)
	plans   func(context.Context, Tenant, int) (AuditPrunePlan, error)
	runs    *TenantGovernanceRunRepository
}

func NewTenantRetentionService() *TenantRetentionService {
	return &TenantRetentionService{
		now: time.Now, tenants: activeRetentionTenants, plans: createTenantRetentionPlan,
		runs: NewTenantGovernanceRunRepository(),
	}
}

func (s *TenantRetentionService) WithContext(ctx context.Context) *TenantRetentionService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	clone.runs = clone.runs.WithContext(ctx)
	return &clone
}

func (s *TenantRetentionService) Run() ([]TenantRetentionResult, error) {
	tenants, err := s.tenants(contextOrBackground(s.ctx))
	if err != nil {
		return nil, err
	}
	results := make([]TenantRetentionResult, 0, len(tenants))
	for _, tenant := range tenants {
		results = append(results, s.runTenant(tenant))
	}
	return results, nil
}

func (s *TenantRetentionService) runTenant(tenant Tenant) TenantRetentionResult {
	policy, err := NewTenantGovernanceService().WithContext(s.ctx).Policy(tenant)
	if err != nil {
		return TenantRetentionResult{TenantID: tenant.ID, TenantCode: tenant.Code, Status: models.TenantGovernanceRunStatusFailed, Error: err.Error()}
	}
	idempotencyKey := fmt.Sprintf("%d:%s:%s:retention", tenant.ID, tenantGovernancePolicyVersion(policy), s.now().UTC().Format("2006-01-02"))
	run, created, err := s.runs.CreateOrGetRun(TenantGovernanceRunCreate{
		TenantID: tenant.ID, TenantCode: tenant.Code, Kind: models.TenantGovernanceRunKindRetention,
		IdempotencyKey: idempotencyKey, PolicyVersion: tenantGovernancePolicyVersion(policy),
	})
	result := TenantRetentionResult{TenantID: tenant.ID, TenantCode: tenant.Code, RunID: run.ID, PlanID: run.PlanID, AuditDays: policy.Retention.AuditDays, Status: run.Status}
	if err != nil || !created {
		if err != nil {
			result.Error = err.Error()
		}
		return result
	}
	if err := s.runs.Transition(run.ID, models.TenantGovernanceRunStatusPending, models.TenantGovernanceRunStatusRunning, ""); err != nil {
		result.Status, result.Error = models.TenantGovernanceRunStatusFailed, err.Error()
		return result
	}
	plan, err := s.plans(contextOrBackground(s.ctx), tenant, policy.Retention.AuditDays)
	if err != nil {
		_ = s.runs.Transition(run.ID, models.TenantGovernanceRunStatusRunning, models.TenantGovernanceRunStatusFailed, err.Error())
		result.Status, result.Error = models.TenantGovernanceRunStatusFailed, err.Error()
		return result
	}
	result.PlanID, result.TargetCount = plan.PlanID, plan.TargetCount
	if err := s.runs.AwaitRetentionEvidence(run.ID, plan.PlanID); err != nil {
		result.Status, result.Error = models.TenantGovernanceRunStatusFailed, err.Error()
		return result
	}
	result.Status = models.TenantGovernanceRunStatusAwaitingEvidence
	return result
}

func activeRetentionTenants(ctx context.Context) ([]Tenant, error) {
	tenants := make([]Tenant, 0)
	err := OrmForConnectionWithContext(ctx, PlatformConnection()).Query().Table("tenant").Where("status", TenantStatusActive).OrderBy("id").Get(&tenants)
	return tenants, err
}

func createTenantRetentionPlan(ctx context.Context, tenant Tenant, auditDays int) (AuditPrunePlan, error) {
	RegisterTenantConnection(tenant)
	return NewAuditPrunePlanService().WithContext(ctx).Create(AuditPrunePlanOptions{Scope: "tenant:" + tenant.Code, RetentionDays: auditDays})
}

func tenantGovernancePolicyVersion(policy TenantGovernancePolicy) string {
	payload, _ := json.Marshal(struct {
		Retention TenantRetentionPolicy `json:"retention"`
	}{policy.Retention})
	return digestBytes(payload)
}

func tenantRetentionScheduledTaskHandler(ctx context.Context, _ models.JSONMap) ScheduledTaskExecutionResult {
	results, err := NewTenantRetentionService().WithContext(ctx).Run()
	payload, _ := json.Marshal(results)
	if err != nil {
		return taskFailure(err.Error())
	}
	for _, result := range results {
		if result.Status == models.TenantGovernanceRunStatusFailed {
			return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusFailed, Stdout: string(payload), ErrorMessage: result.Error}
		}
	}
	return ScheduledTaskExecutionResult{Status: ScheduledTaskLogStatusSuccess, Stdout: string(payload)}
}

// Source: tenant_runtime_service.go
const (
	BillingStatusActive     = "active"
	BillingStatusTrialing   = "trialing"
	BillingStatusPastDue    = "past_due"
	BillingStatusCanceled   = "canceled"
	BillingStatusExpired    = "expired"
	externalUserType        = "100"
	externalUserDefaultPass = "__sso_managed__"
)

var (
	ErrSubscriptionInactive = errors.New("tenant subscription is inactive")
	ErrQuotaExceeded        = tenantcontract.ErrQuotaExceeded
	ErrSSONotConfigured     = errors.New("sso provider is not configured")
	ErrSSOTokenInvalid      = errors.New("sso token is invalid")
)

type TenantRuntimeService struct {
	ctx context.Context
}

type TenantPublicConfig struct {
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Plan         string         `json:"plan"`
	CustomDomain *string        `json:"custom_domain"`
	Branding     models.JSONMap `json:"branding"`
	Features     models.JSONMap `json:"features"`
}

type TenantUsageReport struct {
	ID      uint64         `json:"id"`
	Code    string         `json:"code"`
	Name    string         `json:"name"`
	Plan    string         `json:"plan"`
	Billing models.JSONMap `json:"billing"`
	Quotas  models.JSONMap `json:"quotas"`
	Usage   models.JSONMap `json:"usage"`
}

type TenantQuotaSnapshot struct {
	Users     int64 `json:"users"`
	Roles     int64 `json:"roles"`
	StorageMB int64 `json:"storage_mb"`
}

type SSOLoginPayload struct {
	Provider      string `json:"provider"`
	Scene         string `json:"scene"`
	TransactionID string `json:"transaction_id"`
	State         string `json:"state"`
	IDToken       string `json:"id_token"`
	Code          string `json:"code"`
	CodeVerifier  string `json:"code_verifier"`
	Nonce         string `json:"nonce"`
	RedirectURI   string `json:"redirect_uri"`
	SAMLResponse  string `json:"saml_response"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	Subject       string `json:"subject"`
}

type ssoClaims struct {
	Subject string
	Email   string
	Name    string
	Issuer  string
	Raw     map[string]any
}

func NewTenantRuntimeService() *TenantRuntimeService {
	return &TenantRuntimeService{}
}

func (s *TenantRuntimeService) WithContext(ctx context.Context) *TenantRuntimeService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantRuntimeService) PublicConfig(tenant Tenant, scene ...string) TenantPublicConfig {
	features := s.EffectiveFeatures(tenant)
	features["sso"] = s.publicSSOFeatures(tenant, scene...)
	return TenantPublicConfig{
		Code:         tenant.Code,
		Name:         tenant.Name,
		Plan:         tenant.Plan,
		CustomDomain: tenant.CustomDomain,
		Branding:     mapOrEmpty(tenant.Branding),
		Features:     publicFeatures(features),
	}
}

func (s *TenantRuntimeService) Usage(tenant Tenant) (TenantUsageReport, error) {
	usage, err := s.QuotaSnapshot(tenant)
	if err != nil {
		return TenantUsageReport{}, err
	}

	return TenantUsageReport{
		ID: tenant.ID, Code: tenant.Code, Name: tenant.Name, Plan: tenant.Plan,
		Billing: s.EffectiveBilling(tenant),
		Quotas:  s.EffectiveQuotas(tenant),
		Usage: models.JSONMap{
			"users":      usage.Users,
			"roles":      usage.Roles,
			"storage_mb": usage.StorageMB,
		},
	}, nil
}

func (s *TenantRuntimeService) QuotaSnapshot(tenant Tenant) (TenantQuotaSnapshot, error) {
	connection := RegisterTenantConnection(tenant)
	query := OrmForConnectionWithContext(s.ctx, connection).Query()
	users, err := query.Table(`"user"`).Where("user_type", externalUserType).Count()
	if err != nil {
		return TenantQuotaSnapshot{}, err
	}
	roles, err := OrmForConnectionWithContext(s.ctx, connection).Query().Table("role").Count()
	if err != nil {
		return TenantQuotaSnapshot{}, err
	}
	var storageBytes int64
	err = OrmForConnectionWithContext(s.ctx, connection).Query().
		Raw("SELECT COALESCE(SUM(size_byte), 0) FROM attachment").
		Scan(&storageBytes)
	if err != nil {
		return TenantQuotaSnapshot{}, err
	}

	return TenantQuotaSnapshot{
		Users:     users,
		Roles:     roles,
		StorageMB: bytesToMB(storageBytes),
	}, nil
}

func (s *TenantRuntimeService) EnsureSubscription(tenant Tenant) error {
	billing := s.EffectiveBilling(tenant)
	status := strings.ToLower(strings.TrimSpace(jsonString(billing, "subscription_status")))
	if status == "" {
		status = BillingStatusActive
	}
	if status != BillingStatusActive && status != BillingStatusTrialing {
		return ErrSubscriptionInactive
	}
	if expiresAt := jsonString(billing, "expires_at"); expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return BusinessError{Message: "租户订阅到期时间格式错误"}
		}
		if time.Now().After(parsed) {
			return ErrSubscriptionInactive
		}
	}
	return nil
}

func (s *TenantRuntimeService) AllowRequest(tenant Tenant) error {
	limit := jsonInt64(s.EffectiveQuotas(tenant), "api_rate_per_minute")
	return NewTenantRateLimiter().Allow(tenant, limit)
}

func (s *TenantRuntimeService) EnsureResourceQuota(tenant Tenant, resource string, add int64) error {
	limit := jsonInt64(s.EffectiveQuotas(tenant), resourceQuotaKey(resource))
	if limit <= 0 {
		return nil
	}

	usage, err := s.QuotaSnapshot(tenant)
	if err != nil {
		return err
	}
	current := resourceUsage(usage, resource)
	if current+add > limit {
		return ErrQuotaExceeded
	}
	return nil
}

func (s *TenantRuntimeService) SSOLogin(tenant Tenant, payload SSOLoginPayload, ip, browser, os string) (LoginResult, error) {
	if strings.TrimSpace(payload.Code) != "" {
		return LoginResult{}, ErrSSOAuthorizationTransactionInvalid
	}
	provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).EnabledProviderForScene(payload.Provider, payload.Scene)
	if err != nil {
		return LoginResult{}, err
	}
	connection := RegisterTenantConnection(tenant)
	audit := NewSSOAuditServiceForConnection(connection).WithContext(s.ctx)
	claims, err := verifySSOClaims(provider, payload)
	if err != nil {
		_ = audit.Log(ssoLogInput{
			ProviderID: provider.ID, Status: ssoLogStatusFailure,
			FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	return s.completeSSOLogin(tenant, provider, claims, audit, ip, browser, os)
}

func (s *TenantRuntimeService) completeSSOLogin(
	tenant Tenant,
	provider SSOProvider,
	claims ssoClaims,
	audit *SSOAuditService,
	ip, browser, os string,
) (LoginResult, error) {
	connection := RegisterTenantConnection(tenant)
	passport := NewPassportServiceForTenant(tenant).WithContext(s.ctx)
	passport.connection = connection

	user, err := s.findOrCreateSSOUser(tenant, provider, claims, audit)
	if err != nil {
		_ = audit.Log(ssoLogInput{
			ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	if user.Status == 2 {
		_ = audit.Log(ssoLogInput{
			UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(ErrUserDisabled), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, ErrUserDisabled
	}
	if err := applySSOMappings(s.ctx, connection, user.ID, provider, claims); err != nil {
		_ = audit.Log(ssoLogInput{
			UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	InvalidateCurrentUserInfoForConnection(connection, user.ID)

	accessToken, err := passport.buildToken(user.ID, tenant.ID, "access", AccessTokenTTLSeconds())
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := passport.buildToken(user.ID, tenant.ID, "refresh", RefreshTokenTTLSeconds())
	if err != nil {
		return LoginResult{}, err
	}
	binding, err := audit.UpsertBinding(ssoBindingInput{
		UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject,
		SSOEmail: strings.TrimSpace(claims.Email), SSOUsername: ssoClaimPreferredUsername(claims),
		SSOAvatar: ssoClaimAvatar(claims),
	})
	if err != nil {
		_ = audit.Log(ssoLogInput{
			UserID: user.ID, ProviderID: provider.ID, SSOUserID: claims.Subject, SSOEmail: claims.Email,
			Status: ssoLogStatusFailure, FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return LoginResult{}, err
	}
	if err := audit.Log(ssoLogInput{
		UserID: user.ID, ProviderID: provider.ID, BindingID: binding.ID,
		SSOUserID: claims.Subject, SSOEmail: claims.Email, Status: ssoLogStatusSuccess,
		IP: ip, UserAgent: browser,
	}); err != nil {
		return LoginResult{}, err
	}
	_ = passport.writeLoginLog(user.Username, ip, browser, os, 1, "SSO 登录成功")

	return LoginResult{AccessToken: accessToken, RefreshToken: refreshToken, ExpireAt: AccessTokenTTLSeconds()}, nil
}

func (s *TenantRuntimeService) StartSSOAuthorization(tenant Tenant, providerName, scene string) (SSOAuthorizationResult, error) {
	provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).EnabledProviderForScene(providerName, scene)
	if err != nil {
		return SSOAuthorizationResult{}, err
	}
	if provider.Type != "oidc" && provider.Type != "oauth2" {
		return SSOAuthorizationResult{}, ErrSSOTokenInvalid
	}
	return NewSSOAuthorizationTransactionService().Create(tenant, provider)
}

func (s *TenantRuntimeService) CompleteSSOAuthorization(
	tenant Tenant,
	transactionID, code, state, ip, browser, os string,
) (LoginResult, error) {
	if strings.TrimSpace(code) == "" {
		return LoginResult{}, ErrSSOTokenInvalid
	}
	transactionService := NewSSOAuthorizationTransactionService()
	var result LoginResult
	_, err := transactionService.VerifyAndConsumeCallback(tenant, transactionID, state, func(transaction SSOAuthorizationTransaction) error {
		provider, claims, audit, err := s.verifiedSSOAuthorization(tenant, transactionService, transaction, code, ip, browser)
		if err != nil {
			return err
		}
		result, err = s.completeSSOLogin(tenant, provider, claims, audit, ip, browser, os)
		return err
	})
	if err != nil {
		return LoginResult{}, err
	}
	transactionService.ForgetVerified(transactionID)
	return result, nil
}

func (s *TenantRuntimeService) verifiedSSOAuthorization(
	tenant Tenant,
	transactionService *SSOAuthorizationTransactionService,
	transaction SSOAuthorizationTransaction,
	code, ip, browser string,
) (SSOProvider, ssoClaims, *SSOAuditService, error) {
	if verified, ok := transactionService.LoadVerified(tenant, transaction); ok {
		provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).EnabledProviderForScene(verified.Provider, verified.Scene)
		audit := NewSSOAuditServiceForConnection(RegisterTenantConnection(tenant)).WithContext(s.ctx)
		if err != nil || provider.ID != verified.ProviderID {
			return SSOProvider{}, ssoClaims{}, audit, ErrSSOAuthorizationTransactionInvalid
		}
		return provider, verified.Claims, audit, nil
	}
	provider, claims, audit, err := s.verifySSOAuthorizationCallback(tenant, transaction, code, ip, browser)
	if err != nil {
		return SSOProvider{}, ssoClaims{}, audit, err
	}
	err = transactionService.StoreVerified(transaction, ssoVerifiedAuthorization{
		TenantCode: tenant.Code, ProviderID: provider.ID, Provider: provider.Name, Scene: provider.Scene, Claims: claims,
	})
	return provider, claims, audit, err
}

func (s *TenantRuntimeService) verifySSOAuthorizationCallback(
	tenant Tenant,
	transaction SSOAuthorizationTransaction,
	code, ip, browser string,
) (SSOProvider, ssoClaims, *SSOAuditService, error) {
	connection := RegisterTenantConnection(tenant)
	audit := NewSSOAuditServiceForConnection(connection).WithContext(s.ctx)
	provider, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).
		EnabledProviderForScene(transaction.Provider, transaction.Scene)
	if err != nil {
		return SSOProvider{}, ssoClaims{}, audit, err
	}
	if strings.TrimSpace(provider.RedirectURI) == "" || provider.RedirectURI != transaction.RedirectURI {
		return SSOProvider{}, ssoClaims{}, audit, ErrSSOAuthorizationTransactionInvalid
	}
	claims, err := verifySSOClaims(provider, SSOLoginPayload{
		Code:         code,
		CodeVerifier: transaction.CodeVerifier,
		Nonce:        transaction.Nonce,
		RedirectURI:  transaction.RedirectURI,
	})
	if err != nil {
		_ = audit.Log(ssoLogInput{
			ProviderID: provider.ID, Status: ssoLogStatusFailure,
			FailureReason: ssoFailureMessage(err), IP: ip, UserAgent: browser,
		})
		return SSOProvider{}, ssoClaims{}, audit, err
	}
	return provider, claims, audit, nil
}

func (s *TenantRuntimeService) findOrCreateSSOUser(
	tenant Tenant,
	provider SSOProvider,
	claims ssoClaims,
	audit *SSOAuditService,
) (models.User, error) {
	connection := RegisterTenantConnection(tenant)
	if audit != nil {
		user, err := audit.BoundUser(provider.ID, claims.Subject)
		if err == nil && user.ID != 0 {
			return user, nil
		}
		if err != nil && !errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return models.User{}, err
		}
	}
	query := OrmForConnectionWithContext(s.ctx, connection).Query()
	username := strings.TrimSpace(claims.Subject)
	var user models.User
	if err := query.Table(`"user"`).Where("username", username).First(&user); err == nil && user.ID != 0 {
		return user, nil
	}
	if !provider.AutoCreate {
		return models.User{}, ErrUnauthorized
	}

	username, err := s.ssoAutoCreateUsername(connection, provider, claims)
	if err != nil {
		return models.User{}, err
	}
	password, err := randomTenantPassword()
	if err != nil {
		return models.User{}, err
	}
	hash, err := makePasswordHash(password)
	if err != nil {
		return models.User{}, err
	}
	nickname := strings.TrimSpace(claims.Name)
	if nickname == "" {
		nickname = username
	}
	user = models.User{
		Username: username, Password: hash, UserType: externalUserType,
		Nickname: nickname, Email: strings.TrimSpace(claims.Email), Status: 1,
		Dashboard: "dashboard:workbench", BackendSetting: nil,
	}
	if err := query.Create(&user); err != nil {
		return models.User{}, err
	}
	if _, err := OrmForConnectionWithContext(s.ctx, connection).Query().Exec(`UPDATE "user" SET backend_setting = '{}'::jsonb WHERE id = ?`, user.ID); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (s *TenantRuntimeService) ssoAutoCreateUsername(connection string, provider SSOProvider, claims ssoClaims) (string, error) {
	subject := strings.TrimSpace(claims.Subject)
	if subject != "" && len(subject) <= 20 {
		if available, err := s.ssoUsernameAvailable(connection, subject); err != nil || available {
			return subject, err
		}
	}

	base := normalizeSSOUsername(ssoUsernameSeed(claims))
	if base == "" {
		base = "sso_user"
	}
	if len(base) > 20 {
		base = strings.Trim(base[:20], "_")
	}
	if base == "" {
		base = "sso_user"
	}
	if available, err := s.ssoUsernameAvailable(connection, base); err != nil || available {
		return base, err
	}

	digest := ssoUsernameHash(provider, claims)
	for attempt := 0; attempt < 100; attempt++ {
		suffix := "_" + digest
		if attempt > 0 {
			suffix = fmt.Sprintf("_%s_%d", digest[:6], attempt+1)
		}
		limit := 20 - len(suffix)
		if limit < 1 {
			limit = 1
		}
		prefix := base
		if len(prefix) > limit {
			prefix = strings.Trim(prefix[:limit], "_")
		}
		if prefix == "" {
			prefix = "s"
		}
		candidate := prefix + suffix
		if available, err := s.ssoUsernameAvailable(connection, candidate); err != nil || available {
			return candidate, err
		}
	}

	return "", BusinessError{Message: "SSO 用户名生成失败"}
}

func ssoUsernameSeed(claims ssoClaims) string {
	for _, key := range []string{"preferred_username", "username"} {
		if value := strings.TrimSpace(jsonString(claims.Raw, key)); value != "" {
			return value
		}
	}
	if email := strings.TrimSpace(claims.Email); email != "" {
		if local, _, ok := strings.Cut(email, "@"); ok {
			return local
		}
		return email
	}
	if name := strings.TrimSpace(claims.Name); name != "" {
		return name
	}
	return claims.Subject
}

func normalizeSSOUsername(value string) string {
	var out strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && out.Len() > 0 {
			out.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(out.String(), "_")
}

func ssoUsernameHash(provider SSOProvider, claims ssoClaims) string {
	return sha256Hex([]byte(fmt.Sprintf("%d:%s:%s", provider.ID, provider.Name, claims.Subject)))[:8]
}

func (s *TenantRuntimeService) ssoUsernameAvailable(connection, username string) (bool, error) {
	count, err := OrmForConnectionWithContext(s.ctx, connection).
		Query().
		Table(`"user"`).
		Where("username", username).
		Count()
	return count == 0, err
}

func EffectiveTenantBilling(tenant Tenant) models.JSONMap {
	return effectiveTenantBillingWithContext(context.Background(), tenant)
}

func ssoClaimPreferredUsername(claims ssoClaims) string {
	for _, key := range []string{"preferred_username", "username", "name"} {
		if value := strings.TrimSpace(jsonString(claims.Raw, key)); value != "" {
			return value
		}
	}
	return claims.Subject
}

func ssoClaimAvatar(claims ssoClaims) string {
	for _, key := range []string{"picture", "avatar"} {
		if value := strings.TrimSpace(jsonString(claims.Raw, key)); value != "" {
			return value
		}
	}
	return ""
}

func EffectiveTenantQuotas(tenant Tenant) models.JSONMap {
	return effectiveTenantQuotasWithContext(context.Background(), tenant)
}

func EffectiveTenantFeatures(tenant Tenant) models.JSONMap {
	return effectiveTenantFeaturesWithContext(context.Background(), tenant)
}

func (s *TenantRuntimeService) EffectiveBilling(tenant Tenant) models.JSONMap {
	return effectiveTenantBillingWithContext(s.ctx, tenant)
}

func (s *TenantRuntimeService) EffectiveQuotas(tenant Tenant) models.JSONMap {
	return effectiveTenantQuotasWithContext(s.ctx, tenant)
}

func (s *TenantRuntimeService) EffectiveFeatures(tenant Tenant) models.JSONMap {
	return effectiveTenantFeaturesWithContext(s.ctx, tenant)
}

func effectiveTenantBillingWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	plan := tenantPlanForRuntime(ctx, tenant.Plan)
	return mergeJSONMaps(plan.Billing, tenant.Billing)
}

func effectiveTenantQuotasWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	quotas := baseTenantQuotasWithContext(ctx, tenant)
	policy, ok := tenantGovernanceRuntimePolicy(ctx, tenant.ID)
	if !ok {
		return quotas
	}
	quotas = mergeJSONMaps(quotas, policy.Quotas)
	if policy.RateLimit.PerMinute > 0 {
		quotas["api_rate_per_minute"] = policy.RateLimit.PerMinute
	}
	return quotas
}

func baseTenantQuotasWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	plan := tenantPlanForRuntime(ctx, tenant.Plan)
	return mergeJSONMaps(plan.Quotas, tenant.Quotas)
}

func effectiveTenantFeaturesWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	features := baseTenantFeaturesWithContext(ctx, tenant)
	policy, ok := tenantGovernanceRuntimePolicy(ctx, tenant.ID)
	if !ok || len(policy.Modules) == 0 {
		return features
	}
	features["modules"] = mergeModuleFlags(features["modules"], policy.Modules)
	return features
}

func baseTenantFeaturesWithContext(ctx context.Context, tenant Tenant) models.JSONMap {
	plan := tenantPlanForRuntime(ctx, tenant.Plan)
	return mergeJSONMaps(plan.Features, tenant.Features)
}

func tenantGovernanceRuntimePolicy(ctx context.Context, tenantID uint64) (TenantGovernancePolicy, bool) {
	if tenantID == 0 {
		return TenantGovernancePolicy{}, false
	}
	policy, ok, err := NewTenantGovernanceService().WithContext(ctx).loadPolicy(tenantID)
	if err != nil || !ok {
		return TenantGovernancePolicy{}, false
	}
	return policy, true
}

func tenantPlanForRuntime(ctx context.Context, code string) TenantPlan {
	plan, err := NewTenantPlanService().WithContext(ctx).ActiveByCode(code)
	if err != nil {
		return TenantPlan{}
	}
	return plan
}

func mergeJSONMaps(base, override models.JSONMap) models.JSONMap {
	merged := models.JSONMap{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		nestedBase, baseOK := asJSONMap(merged[key])
		nestedOverride, overrideOK := asJSONMap(value)
		if baseOK && overrideOK {
			merged[key] = mergeJSONMaps(nestedBase, nestedOverride)
			continue
		}
		merged[key] = value
	}
	return merged
}

func mergeModuleFlags(base any, override map[string]bool) map[string]any {
	merged := map[string]any{}
	switch typed := base.(type) {
	case map[string]any:
		for key, value := range typed {
			merged[key] = value
		}
	case models.JSONMap:
		for key, value := range typed {
			merged[key] = value
		}
	case map[string]bool:
		for key, value := range typed {
			merged[key] = value
		}
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func publicFeatures(features models.JSONMap) models.JSONMap {
	out := models.JSONMap{}
	sso, ok := jsonObject(features, "sso")
	if !ok {
		out["sso"] = map[string]any{"password_login": true, "providers": []any{}}
		return out
	}
	providers := make([]any, 0)
	if rawProviders, ok := sso["providers"].([]any); ok {
		for _, item := range rawProviders {
			raw, ok := item.(map[string]any)
			if !ok || !jsonBool(raw, "enabled", true) {
				continue
			}
			providers = append(providers, map[string]any{
				"name":                   jsonString(raw, "name"),
				"display_name":           jsonString(raw, "display_name"),
				"scene":                  jsonString(raw, "scene"),
				"type":                   jsonString(raw, "type"),
				"issuer":                 jsonString(raw, "issuer"),
				"discovery_url":          jsonString(raw, "discovery_url"),
				"authorization_endpoint": jsonString(raw, "authorization_endpoint"),
				"client_id":              jsonString(raw, "client_id"),
				"scope":                  jsonString(raw, "scope"),
				"redirect_uri":           jsonString(raw, "redirect_uri"),
				"enable_pkce":            jsonBool(raw, "enable_pkce", true),
				"enable_nonce":           jsonBool(raw, "enable_nonce", true),
				"saml_entrypoint":        jsonString(raw, "saml_entrypoint"),
				"saml_entity_id":         jsonString(raw, "saml_entity_id"),
				"icon":                   jsonString(raw, "icon"),
				"button_color":           jsonString(raw, "button_color"),
				"enabled":                true,
			})
		}
	}
	out["sso"] = map[string]any{
		"password_login": jsonBool(sso, "password_login", true),
		"providers":      providers,
	}
	return out
}

func (s *TenantRuntimeService) publicSSOFeatures(tenant Tenant, scene ...string) models.JSONMap {
	selectedScene := DefaultSSOScene
	if len(scene) > 0 {
		selectedScene = scene[0]
	}
	providers, err := NewSSOProviderServiceForTenant(tenant).WithContext(s.ctx).PublicProviders(selectedScene)
	if err != nil {
		providers = []PublicSSOProvider{}
	}
	rawProviders := make([]any, 0, len(providers))
	for _, provider := range providers {
		rawProviders = append(rawProviders, map[string]any{
			"name":                   provider.Name,
			"display_name":           provider.DisplayName,
			"scene":                  provider.Scene,
			"type":                   provider.Type,
			"issuer":                 provider.Issuer,
			"discovery_url":          provider.DiscoveryURL,
			"authorization_endpoint": provider.AuthorizationEndpoint,
			"client_id":              provider.ClientID,
			"scope":                  provider.Scope,
			"redirect_uri":           provider.RedirectURI,
			"enable_pkce":            provider.EnablePKCE,
			"enable_nonce":           provider.EnableNonce,
			"saml_entrypoint":        provider.SAMLEntrypoint,
			"saml_entity_id":         provider.SAMLEntityID,
			"icon":                   provider.Icon,
			"button_color":           provider.ButtonColor,
			"enabled":                provider.Enabled,
		})
	}
	return models.JSONMap{"password_login": true, "providers": rawProviders}
}

func jsonObject(source models.JSONMap, key string) (map[string]any, bool) {
	value, ok := source[key]
	if !ok {
		return nil, false
	}
	nested, ok := asJSONMap(value)
	if !ok {
		return nil, false
	}
	return map[string]any(nested), true
}

func asJSONMap(value any) (models.JSONMap, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return models.JSONMap(typed), true
	case models.JSONMap:
		return typed, true
	default:
		return nil, false
	}
}

func TenantHostCode(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if value, _, err := net.SplitHostPort(host); err == nil {
		return strings.ToLower(value)
	}
	return strings.ToLower(host)
}

func lastForwardedHost(value string) string {
	parts := strings.Split(value, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if host := strings.TrimSpace(parts[i]); host != "" {
			return host
		}
	}
	return ""
}

func TrustedForwardedHost(header func(string, ...string) string, remoteAddr string) string {
	name := strings.TrimSpace(facades.Config().GetString("tenant.trusted_forwarded_host_header"))
	if name == "" || !trustedForwardedHostProxy(remoteAddr) {
		return ""
	}
	return lastForwardedHost(header(name, ""))
}

func trustedForwardedHostProxy(remoteAddr string) bool {
	remoteIP := remoteAddrIP(remoteAddr)
	if remoteIP == nil {
		return false
	}
	for _, candidate := range strings.Split(facades.Config().GetString("tenant.trusted_forwarded_host_proxies"), ",") {
		if trustedProxyContains(strings.TrimSpace(candidate), remoteIP) {
			return true
		}
	}
	return false
}

func trustedProxyContains(candidate string, remoteIP net.IP) bool {
	if candidate == "" {
		return false
	}
	if _, network, err := net.ParseCIDR(candidate); err == nil {
		return network.Contains(remoteIP)
	}
	return net.ParseIP(candidate).Equal(remoteIP)
}

func remoteAddrIP(remoteAddr string) net.IP {
	value := strings.TrimSpace(remoteAddr)
	if value == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return net.ParseIP(strings.Trim(value, "[]"))
}

func resourceQuotaKey(resource string) string {
	switch resource {
	case "users":
		return "max_users"
	case "roles":
		return "max_roles"
	case "storage":
		return "max_storage_mb"
	default:
		return resource
	}
}

func resourceUsage(usage TenantQuotaSnapshot, resource string) int64 {
	switch resource {
	case "users":
		return usage.Users
	case "roles":
		return usage.Roles
	case "storage":
		return usage.StorageMB
	default:
		return 0
	}
}

func bytesToMB(bytes int64) int64 {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 1024*1024 - 1) / (1024 * 1024)
}

func audienceMatches(value any, expected string) bool {
	switch aud := value.(type) {
	case string:
		return aud == expected
	case []any:
		for _, item := range aud {
			if fmt.Sprint(item) == expected {
				return true
			}
		}
	}
	return false
}

func jsonString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func jsonBool(values map[string]any, key string, fallback bool) bool {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	enabled, ok := value.(bool)
	if !ok {
		return fallback
	}
	return enabled
}

func jsonInt64(values map[string]any, key string) int64 {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		var parsed int64
		_, _ = fmt.Sscan(v, &parsed)
		return parsed
	default:
		return 0
	}
}

// Source: tenant_service.go
const (
	TenantStatusActive    = tenantcontract.StatusActive
	TenantStatusSuspended = tenantcontract.StatusSuspended
	TenantStatusArchived  = tenantcontract.StatusArchived
)

var tenantModuleMigrationsProvider func() []schema.Migration

func SetTenantModuleMigrationsProvider(provider func() []schema.Migration) {
	tenantModuleMigrationsProvider = provider
}

var (
	ErrTenantRequired  = tenantcontract.ErrRequired
	ErrTenantNotFound  = tenantcontract.ErrNotFound
	ErrTenantSuspended = tenantcontract.ErrSuspended
)

type Tenant = tenantcontract.Tenant

type TenantService struct {
	ctx context.Context
}

type tenantContextKey struct{}

var (
	tenantInitializeMu         sync.Mutex
	tenantConnectionRegisterMu sync.Mutex
)

type TenantPayload struct {
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Status       int8           `json:"status"`
	Plan         string         `json:"plan"`
	DBHost       string         `json:"db_host"`
	DBPort       int            `json:"db_port"`
	DBDatabase   string         `json:"db_database"`
	DBUsername   string         `json:"db_username"`
	DBPassword   string         `json:"db_password"`
	DBSchema     string         `json:"db_schema"`
	CustomDomain string         `json:"custom_domain"`
	Branding     models.JSONMap `json:"branding"`
	Billing      models.JSONMap `json:"billing"`
	Quotas       models.JSONMap `json:"quotas"`
	Features     models.JSONMap `json:"features"`
	Remark       string         `json:"remark"`
	Initialize   bool           `json:"initialize"`
}

type TenantDestroyPayload struct {
	IDs          []uint64 `json:"ids"`
	ConfirmCode  string   `json:"confirm_code"`
	DropDatabase bool     `json:"drop_database"`
	ReAuthToken  string   `json:"reauth_token"`
	ApprovalID   string   `json:"approval_id"`
	OperatorID   uint64   `json:"-"`
}

type TenantPlanUpdatePayload struct {
	Plan     string         `json:"plan"`
	Features models.JSONMap `json:"features"`
}

type TenantPermissionOperator struct {
	ID   uint64
	Name string
}

type PostgresProvisionPlan struct {
	PlatformStatements []string
	TenantStatements   []string
}

func NewTenantService() *TenantService {
	return &TenantService{}
}

func (s *TenantService) WithContext(ctx context.Context) *TenantService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func TenantContextKey() any {
	return tenantContextKey{}
}

func (p TenantPayload) Tenant() Tenant {
	status := p.Status
	if status == 0 {
		status = TenantStatusActive
	}
	plan := strings.TrimSpace(p.Plan)
	if plan == "" {
		plan = "standard"
	}
	var customDomain *string
	if value := strings.TrimSpace(p.CustomDomain); value != "" {
		customDomain = &value
	}
	dbPort := p.DBPort
	if dbPort == 0 {
		dbPort = 5432
	}
	dbDatabase := strings.TrimSpace(p.DBDatabase)
	if dbDatabase == "" {
		dbDatabase = defaultTenantDatabaseName(p.Code)
	}
	dbSchema := strings.TrimSpace(p.DBSchema)
	if dbSchema == "" {
		dbSchema = "public"
	}

	return Tenant{
		Code:         strings.TrimSpace(p.Code),
		Name:         strings.TrimSpace(p.Name),
		Status:       status,
		Plan:         plan,
		DBHost:       strings.TrimSpace(p.DBHost),
		DBPort:       dbPort,
		DBDatabase:   dbDatabase,
		DBUsername:   strings.TrimSpace(p.DBUsername),
		DBPassword:   p.DBPassword,
		DBSchema:     dbSchema,
		CustomDomain: customDomain,
		Branding:     p.Branding,
		Billing:      p.Billing,
		Quotas:       p.Quotas,
		Features:     platformManagedFeatures(p.Features),
		Remark:       p.Remark,
	}
}

func (s *TenantService) List(filters map[string]string, page, pageSize int) (request.PageResult[Tenant], error) {
	query := s.orm().
		Query().
		Table("tenant")
	query = query.Scopes(scopes.Contains("code", filters["code"]))
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	query = query.Scopes(scopes.Contains("plan", filters["plan"]))
	query = query.Scopes(scopes.EqualIfPresent("status", filters["status"]))
	return request.Paginate[Tenant](query.OrderByDesc("id"), page, pageSize)
}

func (s *TenantService) Create(input TenantPayload) (Tenant, error) {
	tenant := input.Tenant()
	if err := s.applyPlanFeatureSnapshot(&tenant, featuresWithoutTenantPermissions(input.Features)); err != nil {
		return Tenant{}, err
	}
	if input.Initialize {
		var err error
		tenant, err = ApplyPostgresProvisionDefaults(tenant)
		if err != nil {
			return Tenant{}, err
		}
	}
	if err := s.validateTenant(tenant); err != nil {
		return Tenant{}, err
	}
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		row := Tenant{
			Code: tenant.Code, Name: tenant.Name, Status: tenant.Status,
			Plan: tenant.Plan, DBHost: tenant.DBHost, DBPort: tenant.DBPort,
			DBDatabase: tenant.DBDatabase, DBUsername: tenant.DBUsername,
			DBPassword: tenant.DBPassword, DBSchema: tenant.DBSchema,
			CustomDomain: tenant.CustomDomain, Remark: tenant.Remark,
		}
		if err := tx.Table("tenant").Create(&row); err != nil {
			return err
		}
		tenant.ID = row.ID
		return updateTenantJSONColumnsWithQuery(tx, tenant.ID, tenant)
	}); err != nil {
		return Tenant{}, err
	}
	if input.Initialize {
		if err := s.ProvisionPostgresTenant(tenant); err != nil {
			return Tenant{}, err
		}
		if err := s.InitializeDatabase(tenant); err != nil {
			return Tenant{}, err
		}
	}
	return tenant, nil
}

func (s *TenantService) Update(id uint64, input TenantPayload) (Tenant, error) {
	existing, err := s.FindByID(id)
	if err != nil {
		return Tenant{}, err
	}
	tenant := input.Tenant()
	if tenant.Plan != existing.Plan || tenant.Status != existing.Status {
		return Tenant{}, ErrSensitiveOperationPolicy
	}
	features := featuresWithoutTenantPermissions(input.Features)
	if tenant.Plan == existing.Plan {
		features = preserveTenantPermissionFeature(features, existing.Features)
	}
	if err := s.applyPlanFeatureSnapshot(&tenant, features); err != nil {
		return Tenant{}, err
	}
	if err := s.validateTenant(tenant); err != nil {
		return Tenant{}, err
	}

	values := map[string]any{
		"code": tenant.Code, "name": tenant.Name, "status": tenant.Status,
		"plan": tenant.Plan, "db_host": tenant.DBHost, "db_port": tenant.DBPort,
		"db_database": tenant.DBDatabase, "db_username": tenant.DBUsername,
		"db_schema": tenant.DBSchema, "custom_domain": tenant.CustomDomain,
		"remark":     tenant.Remark,
		"updated_at": time.Now(),
	}
	if tenant.DBPassword != "" {
		values["db_password"] = tenant.DBPassword
	}

	if _, err := s.orm().
		Query().
		Table("tenant").
		Where("id", id).
		Update(values); err != nil {
		return Tenant{}, err
	}
	if err := s.updateTenantJSONColumns(id, tenant); err != nil {
		return Tenant{}, err
	}
	tenant.ID = id
	return tenant, nil
}

func (s *TenantService) applyPlanFeatureSnapshot(tenant *Tenant, inputFeatures models.JSONMap) error {
	plan, err := NewTenantPlanService().WithContext(s.ctx).ActiveByCode(tenant.Plan)
	if err != nil {
		return err
	}
	if plan.ID == 0 {
		return BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	tenant.Features = SnapshotFeaturesForPlan(plan.Features, inputFeatures)
	return nil
}

func (s *TenantService) updateTenantJSONColumns(id uint64, tenant Tenant) error {
	return updateTenantJSONColumnsWithQuery(s.orm().Query(), id, tenant)
}

func updateTenantJSONColumnsWithQuery(query contractsorm.Query, id uint64, tenant Tenant) error {
	branding, err := json.Marshal(nullIfEmpty(tenant.Branding))
	if err != nil {
		return err
	}
	billing, err := json.Marshal(nullIfEmpty(tenant.Billing))
	if err != nil {
		return err
	}
	quotas, err := json.Marshal(nullIfEmpty(tenant.Quotas))
	if err != nil {
		return err
	}
	features, err := json.Marshal(nullIfEmpty(tenant.Features))
	if err != nil {
		return err
	}
	_, err = query.Exec(
		"UPDATE tenant SET branding = ?::jsonb, billing = ?::jsonb, quotas = ?::jsonb, features = ?::jsonb WHERE id = ?",
		string(branding), string(billing), string(quotas), string(features), id,
	)
	return err
}

func nullIfEmpty(value models.JSONMap) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func platformManagedFeatures(input models.JSONMap) models.JSONMap {
	features := models.JSONMap{}
	for key, value := range input {
		if key == "sso" {
			continue
		}
		features[key] = value
	}
	return features
}

func (s *TenantService) Suspend(id uint64) error {
	return s.UpdateStatus(id, TenantStatusSuspended)
}

func (s *TenantService) Resume(id uint64) error {
	return s.UpdateStatus(id, TenantStatusActive)
}

func (s *TenantService) Archive(id uint64) error {
	return s.UpdateStatus(id, TenantStatusArchived)
}

func (s *TenantService) UpdateStatus(id uint64, status int8) error {
	if !validTenantStatus(status) {
		return BusinessError{Message: "租户状态无效"}
	}
	if err := s.ensureTenantExists(id); err != nil {
		return err
	}
	_, err := s.orm().
		Query().
		Table("tenant").
		Where("id", id).
		Update(map[string]any{"status": status, "updated_at": time.Now()})
	return err
}

func (s *TenantService) Usage(id uint64) (TenantUsageReport, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantUsageReport{}, ErrTenantNotFound
	}
	return NewTenantRuntimeService().WithContext(s.ctx).Usage(tenant)
}

func (s *TenantService) Permissions(id uint64) (TenantPermissionPayload, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	return TenantEffectivePermissionPayload(tenant), nil
}

func (s *TenantService) UpdatePermissions(id uint64, payload TenantPermissionPayload, operator TenantPermissionOperator) (TenantPermissionPayload, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	before := TenantEffectivePermissionPayload(tenant)
	tenant.Features = platformManagedFeatures(tenant.Features)
	tenant.Features[tenantPermissionsFeatureKey] = permissionPayloadMap(payload)
	after := TenantPermissionPayloadFromTenant(tenant)

	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		if err := updateTenantJSONColumnsWithQuery(tx, id, tenant); err != nil {
			return err
		}
		return NewTenantPermissionAuditService().LogWithQuery(tx, TenantPermissionAuditInput{
			TenantID:     tenant.ID,
			TenantCode:   tenant.Code,
			Operation:    TenantPermissionAuditOperationUpdate,
			Source:       TenantPermissionAuditSourcePlatform,
			Before:       before,
			After:        after,
			OperatorID:   operator.ID,
			OperatorName: operator.Name,
		})
	})
	if err != nil {
		return TenantPermissionPayload{}, err
	}
	return TenantPermissionPayloadFromTenant(tenant), nil
}

func (s *TenantService) PermissionPlanDiff(id uint64, targetPlan string) (TenantPermissionPlanDiff, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return TenantPermissionPlanDiff{}, err
	}
	if strings.TrimSpace(targetPlan) == "" {
		targetPlan = tenant.Plan
	}
	plan, err := NewTenantPlanService().WithContext(s.ctx).ActiveByCode(targetPlan)
	if err != nil {
		return TenantPermissionPlanDiff{}, err
	}
	if plan.ID == 0 {
		return TenantPermissionPlanDiff{}, BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	current := TenantEffectivePermissionPayload(tenant)
	next, _ := tenantPermissionPayloadFromFeatures(SnapshotFeaturesForPlan(plan.Features, models.JSONMap{}))
	next = normalizePermissionPayload(next)
	return TenantPermissionPlanDiff{
		Plan:       plan.Code,
		Allowed:    next.Allowed,
		Added:      sortedSetDiff(next.Allowed, current.Allowed),
		Removed:    sortedSetDiff(current.Allowed, next.Allowed),
		Unchanged:  sortedSetIntersect(current.Allowed, next.Allowed),
		Permission: next.Allowed,
	}, nil
}

func (s *TenantService) UpdatePlan(id uint64, input TenantPlanUpdatePayload, operator TenantPermissionOperator) (Tenant, error) {
	tenant, err := s.FindByID(id)
	if err != nil {
		return Tenant{}, err
	}
	plan := strings.TrimSpace(input.Plan)
	if plan == "" {
		return Tenant{}, BusinessError{Message: "租户套餐不能为空"}
	}
	planModel, err := NewTenantPlanService().WithContext(s.ctx).ActiveByCode(plan)
	if err != nil {
		return Tenant{}, err
	}
	if planModel.ID == 0 {
		return Tenant{}, BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	before := TenantEffectivePermissionPayload(tenant)
	features := input.Features
	if features == nil {
		features = models.JSONMap{}
	}
	tenant.Plan = plan
	tenant.Features = SnapshotFeaturesForPlan(planModel.Features, features)
	after := TenantPermissionPayloadFromTenant(tenant)

	err = s.orm().Transaction(func(tx contractsorm.Query) error {
		if _, err := tx.
			Table("tenant").
			Where("id", id).
			Update(map[string]any{"plan": tenant.Plan, "updated_at": time.Now()}); err != nil {
			return err
		}
		if err := updateTenantJSONColumnsWithQuery(tx, id, tenant); err != nil {
			return err
		}
		return NewTenantPermissionAuditService().LogWithQuery(tx, TenantPermissionAuditInput{
			TenantID:     tenant.ID,
			TenantCode:   tenant.Code,
			Operation:    TenantPermissionAuditOperationPlanChange,
			Source:       TenantPermissionAuditSourcePlanChange,
			Before:       before,
			After:        after,
			OperatorID:   operator.ID,
			OperatorName: operator.Name,
			Remark:       "update tenant plan to " + tenant.Plan,
		})
	})
	if err != nil {
		return Tenant{}, err
	}
	return tenant, nil
}

func (s *TenantService) SnapshotLegacyPermissions(dryRun bool) (int, error) {
	tenants := make([]Tenant, 0)
	if err := s.orm().
		Query().
		Table("tenant").
		OrderBy("id").
		Get(&tenants); err != nil {
		return 0, err
	}

	count := 0
	for _, tenant := range tenants {
		snapshot, ok := BuildLegacyTenantPermissionSnapshot(tenant)
		if !ok {
			continue
		}
		count++
		if dryRun {
			continue
		}
		before := TenantEffectivePermissionPayload(tenant)
		tenant.Features = platformManagedFeatures(tenant.Features)
		tenant.Features[tenantPermissionsFeatureKey] = permissionPayloadMap(snapshot)
		if err := s.orm().Transaction(func(tx contractsorm.Query) error {
			if err := updateTenantJSONColumnsWithQuery(tx, tenant.ID, tenant); err != nil {
				return err
			}
			return NewTenantPermissionAuditService().LogWithQuery(tx, TenantPermissionAuditInput{
				TenantID:     tenant.ID,
				TenantCode:   tenant.Code,
				Operation:    TenantPermissionAuditOperationLegacySnapshot,
				Source:       TenantPermissionAuditSourceLegacyMigration,
				Before:       before,
				After:        snapshot,
				OperatorName: TenantPermissionAuditSystemOperatorName,
				Remark:       "snapshot legacy full permissions",
			})
		}); err != nil {
			return count - 1, err
		}
	}
	return count, nil
}

func (s *TenantService) FindByID(id uint64) (Tenant, error) {
	var tenant Tenant
	err := s.orm().
		Query().
		Where("id", id).
		First(&tenant)
	if err != nil || tenant.ID == 0 {
		return Tenant{}, ErrTenantNotFound
	}
	RegisterTenantConnection(tenant)
	return tenant, nil
}

func (s *TenantService) Destroy(input TenantDestroyPayload) error {
	if len(input.IDs) == 0 {
		return nil
	}
	if input.DropDatabase && len(input.IDs) != 1 {
		return BusinessError{Message: "物理库删除仅支持单租户"}
	}

	tenants := make([]Tenant, 0, len(input.IDs))
	if err := s.orm().
		Query().
		Table("tenant").
		WhereIn("id", uint64Any(input.IDs)).
		Get(&tenants); err != nil {
		return err
	}
	if len(tenants) != len(input.IDs) {
		return ErrTenantNotFound
	}
	if len(tenants) == 1 && strings.TrimSpace(input.ConfirmCode) != tenants[0].Code {
		return BusinessError{Message: "租户销毁确认码不匹配"}
	}
	deleteMode := "metadata"
	if input.DropDatabase {
		deleteMode = "database"
	}
	resource := TenantDataActionResource("delete", input.IDs, deleteMode)
	approvalRequired := false
	for _, tenant := range tenants {
		policy, err := NewTenantGovernanceService().WithContext(s.ctx).Policy(tenant)
		if err != nil {
			return err
		}
		if !policy.DataDeletion.Enabled {
			return BusinessError{Message: "租户治理策略禁止数据删除"}
		}
		approvalRequired = approvalRequired || policy.DataDeletion.RequiresApproval
	}
	security := NewEnterpriseSecurityControlService()
	request := SensitiveOperationRequest{
		UserID: input.OperatorID, Operation: "tenant.data.delete", Resource: resource, ReAuthToken: input.ReAuthToken,
	}
	destroy := func() error {
		if input.DropDatabase {
			for _, tenant := range tenants {
				if err := s.DropTenantDatabase(tenant, input.ConfirmCode); err != nil {
					return err
				}
			}
		}
		_, err := s.orm().Query().Table("tenant").WhereIn("id", uint64Any(input.IDs)).Delete()
		return err
	}
	if !approvalRequired {
		if err := security.ExecuteSensitiveOperation(request, destroy); err != nil {
			if !errors.Is(err, ErrReAuthRequired) {
				return err
			}
			return BusinessError{Message: "tenant deletion requires valid re-auth token"}
		}
	} else {
		err := security.ExecuteSensitiveOperationWithApproval(
			s.ctx, request, input.ApprovalID, input.OperatorID, "tenant.data.delete", resource, destroy,
		)
		if errors.Is(err, ErrReAuthRequired) {
			return BusinessError{Message: "tenant deletion requires valid re-auth token"}
		}
		if errors.Is(err, ErrApprovalRequired) || errors.Is(err, ErrApprovalSelfApproved) {
			return BusinessError{Message: "tenant deletion requires approved approval record"}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantService) DropTenantDatabase(tenant Tenant, confirmCode string) error {
	if strings.TrimSpace(confirmCode) != tenant.Code {
		return BusinessError{Message: "租户物理库删除确认码不匹配"}
	}
	platformDB := facades.Config().GetString("database.connections." + PlatformConnection() + ".database")
	if tenant.DBDatabase == "" || tenant.DBDatabase == platformDB {
		return BusinessError{Message: "拒绝删除平台数据库"}
	}
	_, err := s.orm().
		Query().
		Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdentifier(tenant.DBDatabase)))
	return err
}

func (s *TenantService) validateTenant(tenant Tenant) error {
	if tenant.Code == "" || tenant.Name == "" || tenant.DBDatabase == "" {
		return BusinessError{Message: "租户编码、名称和数据库名不能为空"}
	}
	if !validTenantStatus(tenant.Status) {
		return BusinessError{Message: "租户状态无效"}
	}
	exists, err := NewTenantPlanService().WithContext(s.ctx).ExistsActive(tenant.Plan)
	if err != nil {
		return err
	}
	if !exists {
		return BusinessError{Message: "租户套餐不存在或已禁用"}
	}
	return nil
}

func validTenantStatus(status int8) bool {
	return status == TenantStatusActive || status == TenantStatusSuspended || status == TenantStatusArchived
}

func (s *TenantService) ensureTenantExists(id uint64) error {
	count, err := s.orm().
		Query().
		Table("tenant").
		Where("id", id).
		Count()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrTenantNotFound
	}
	return nil
}

func WithTenant(ctx context.Context, tenant Tenant) context.Context {
	return context.WithValue(ctx, TenantContextKey(), tenant)
}

func CurrentTenant(ctx context.Context) (Tenant, bool) {
	tenant, ok := ctx.Value(TenantContextKey()).(Tenant)
	return tenant, ok
}

func TenantOrm(ctx context.Context) string {
	if tenant, ok := CurrentTenant(ctx); ok {
		return TenantConnectionName(tenant)
	}
	return facades.Config().GetString("database.default")
}

func (s *TenantService) Resolve(code string) (Tenant, error) {
	return s.ResolveByCodeOrHost(code, "")
}

func (s *TenantService) FindByCustomDomain(host string) (Tenant, error) {
	host = TenantHostCode(host)
	if host == "" {
		return Tenant{}, ErrTenantNotFound
	}

	var tenant Tenant
	err := s.orm().
		Query().
		Where("custom_domain", host).
		First(&tenant)
	if err != nil {
		if errors.Is(err, frameworkerrors.OrmRecordNotFound) {
			return Tenant{}, ErrTenantNotFound
		}
		return Tenant{}, err
	}
	if tenant.ID == 0 {
		return Tenant{}, ErrTenantNotFound
	}
	RegisterTenantConnection(tenant)
	return tenant, nil
}

func (s *TenantService) ResolveByCodeOrHost(code, host string) (Tenant, error) {
	code = strings.TrimSpace(code)
	host = TenantHostCode(host)
	if code == "" && host == "" {
		code = facades.Config().GetString("tenant.default")
	}
	if code == "" && host == "" {
		return Tenant{}, ErrTenantRequired
	}

	var tenant Tenant
	query := s.orm().
		Query()
	if code != "" {
		query = query.Where("code", code)
	} else {
		query = query.Where("custom_domain", host)
	}
	err := query.First(&tenant)
	if err != nil || tenant.ID == 0 {
		if code == "" && host != "" {
			if fallback := facades.Config().GetString("tenant.default"); fallback != "" {
				return s.Resolve(fallback)
			}
		}
		return Tenant{}, ErrTenantNotFound
	}
	if tenant.Status != TenantStatusActive {
		return Tenant{}, ErrTenantSuspended
	}

	RegisterTenantConnection(tenant)
	return tenant, nil
}

func PlatformConnection() string {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		return facades.Config().GetString("database.default")
	}
	return connection
}

func RegisterTenantConnection(tenant Tenant) string {
	tenantConnectionRegisterMu.Lock()
	defer tenantConnectionRegisterMu.Unlock()
	name := TenantConnectionName(tenant)
	configs := facades.Config().Get("database.connections", map[string]any{})
	merged := make(map[string]any)
	if existing, ok := configs.(map[string]any); ok {
		for key, value := range existing {
			merged[key] = value
		}
	}
	connectionConfig := TenantDatabaseConfig(tenant)
	if err := RegisterTenantConnectionCapacity(name); err != nil {
		connectionConfig["via"] = func() (driver.Driver, error) { return nil, err }
	}
	merged[name] = connectionConfig
	facades.Config().Add("database.connections", merged)
	return name
}

func TenantConnectionName(tenant Tenant) string {
	return tenantcontract.ConnectionName(tenant)
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}

func ApplyPostgresProvisionDefaults(tenant Tenant) (Tenant, error) {
	if strings.TrimSpace(tenant.DBUsername) == "" {
		tenant.DBUsername = defaultTenantDBUsername(tenant.Code)
	}
	if strings.TrimSpace(tenant.DBPassword) == "" {
		password, err := randomTenantPassword()
		if err != nil {
			return Tenant{}, err
		}
		tenant.DBPassword = password
	}
	return tenant, nil
}

func NewPostgresProvisionPlan(tenant Tenant) (PostgresProvisionPlan, error) {
	if strings.TrimSpace(tenant.DBDatabase) == "" || strings.TrimSpace(tenant.DBUsername) == "" || tenant.DBPassword == "" {
		return PostgresProvisionPlan{}, BusinessError{Message: "租户数据库名、用户和密码不能为空"}
	}
	schema := strings.TrimSpace(tenant.DBSchema)
	if schema == "" {
		schema = "public"
	}

	database := quoteIdentifier(tenant.DBDatabase)
	username := quoteIdentifier(tenant.DBUsername)
	schemaName := quoteIdentifier(schema)
	password := quoteLiteral(tenant.DBPassword)
	roleName := quoteLiteral(tenant.DBUsername)

	return PostgresProvisionPlan{
		PlatformStatements: []string{
			fmt.Sprintf("DO $$\nBEGIN\n\tIF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) THEN\n\t\tCREATE ROLE %s LOGIN PASSWORD %s;\n\tELSE\n\t\tALTER ROLE %s WITH LOGIN PASSWORD %s;\n\tEND IF;\nEND $$", roleName, username, password, username, password),
			fmt.Sprintf("CREATE DATABASE %s OWNER %s", database, username),
			fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC", database),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", database, username),
		},
		TenantStatements: []string{
			fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s AUTHORIZATION %s", schemaName, username),
			fmt.Sprintf("GRANT USAGE, CREATE ON SCHEMA %s TO %s", schemaName, username),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA %s TO %s", schemaName, username),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA %s TO %s", schemaName, username),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT ALL PRIVILEGES ON TABLES TO %s", schemaName, username),
			fmt.Sprintf("ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT ALL PRIVILEGES ON SEQUENCES TO %s", schemaName, username),
		},
	}, nil
}

func (s *TenantService) ProvisionPostgresTenant(tenant Tenant) error {
	plan, err := NewPostgresProvisionPlan(tenant)
	if err != nil {
		return err
	}
	if err := s.runPostgresPlatformProvision(tenant, plan.PlatformStatements); err != nil {
		return err
	}

	connection := RegisterTenantConnection(tenant)
	for _, statement := range plan.TenantStatements {
		if _, err := OrmForConnectionWithContext(s.ctx, connection).Query().Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantService) runPostgresPlatformProvision(tenant Tenant, statements []string) error {
	for index, statement := range statements {
		if index == 1 {
			exists, err := s.databaseExists(tenant.DBDatabase)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
		}
		if _, err := s.orm().Query().Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *TenantService) databaseExists(name string) (bool, error) {
	count, err := s.orm().
		Query().
		Table("pg_database").
		Where("datname", name).
		Count()
	return count > 0, err
}

func defaultTenantDBUsername(code string) string {
	return "tenant_" + normalizedTenantCode(code)
}

func defaultTenantDatabaseName(code string) string {
	return "tenant_" + normalizedTenantCode(code)
}

func normalizedTenantCode(code string) string {
	name := strings.ToLower(strings.TrimSpace(code))
	name = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = "default"
	}
	return name
}

func randomTenantPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func TenantDatabaseConfig(tenant Tenant) map[string]any {
	port := tenant.DBPort
	if port == 0 {
		port = 5432
	}
	schema := tenant.DBSchema
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}

	connection := TenantConnectionName(tenant)
	return map[string]any{
		"host":     tenant.DBHost,
		"port":     port,
		"database": tenant.DBDatabase,
		"username": tenant.DBUsername,
		"password": tenant.DBPassword,
		"sslmode":  "disable",
		"singular": false,
		"prefix":   "",
		"schema":   schema,
		"via": func() (driver.Driver, error) {
			return postgresfacades.Postgres(connection)
		},
	}
}

func TenantBusinessMigrations() []schema.Migration {
	items := []schema.Migration{
		&migrations.M202606290001CreateCasbinRuleTable{},
		&migrations.M202606290002CreateUserTable{},
		&migrations.M202606290003CreateRoleTable{},
		&migrations.M202606290004CreateMenuTable{},
		&migrations.M202606290005CreateRoleBelongsMenuTable{},
		&migrations.M202606290006CreateUserBelongsRoleTable{},
		&migrations.M202606290007CreateDepartmentTables{},
		&migrations.M202606290008CreateAttachmentTable{},
		&migrations.M202607030002AddStorageConfigIDToAttachmentTable{},
		&migrations.M202606290009CreateUserLoginLogTable{},
		&migrations.M202606290010CreateUserOperationLogTable{},
		&migrations.M202606290012CreateSSOProviderTable{},
		&migrations.M202606300003CreateSSOUserBindingTable{},
		&migrations.M202606300004CreateSSOLoginLogTable{},
		&migrations.M202606300005CreateDictionaryTables{},
		&migrations.M202607050001CreateUserMFATable{},
		&migrations.M202607050003CreateUserPasswordHistoryTable{},
		&migrations.M202607050005AddSecretRotationMetadata{},
	}
	if tenantModuleMigrationsProvider != nil {
		items = append(items, tenantModuleMigrationsProvider()...)
	}
	return items
}

func TenantInitialSeeders() []seeder.Seeder {
	return []seeder.Seeder{
		&seeders.AdminSeeder{},
		&seeders.MenuSeeder{},
		&seeders.DictionarySeeder{},
		&seeders.DepartmentSeeder{},
		&seeders.CasbinSeeder{},
	}
}

func (s *TenantService) InitializeDatabase(tenant Tenant) error {
	tenantInitializeMu.Lock()
	defer tenantInitializeMu.Unlock()

	if err := runTenantMigrations(tenant); err != nil {
		return err
	}

	connection := TenantConnectionName(tenant)
	restoreSeederConnection := seeders.SetConnection(connection)
	defer restoreSeederConnection()
	for _, item := range TenantInitialSeeders() {
		if err := item.Run(); err != nil {
			return err
		}
	}
	return NewTenantDictionaryServiceForTenant(tenant).SyncFromPlatform()
}

func (s *TenantService) MigrateAllTenants(dryRun bool) (int, error) {
	tenants := make([]Tenant, 0)
	if err := s.orm().Query().Table("tenant").Get(&tenants); err != nil {
		return 0, err
	}
	if dryRun {
		return len(tenants), nil
	}
	for _, tenant := range tenants {
		if err := runTenantMigrations(tenant); err != nil {
			return 0, err
		}
	}
	return len(tenants), nil
}

func runTenantMigrations(tenant Tenant) error {
	connection := RegisterTenantConnection(tenant)
	previous := facades.Schema().GetConnection()
	facades.Schema().SetConnection(connection)
	defer facades.Schema().SetConnection(previous)

	for _, migration := range TenantBusinessMigrations() {
		if err := migration.Up(); err != nil {
			return err
		}
	}
	return nil
}
