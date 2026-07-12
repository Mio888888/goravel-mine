package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/facades"
	"goravel/app/models"
)

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
