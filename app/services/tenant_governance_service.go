package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"goravel/app/facades"
	"goravel/app/models"
)

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
