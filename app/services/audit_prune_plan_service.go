package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/models"
)

const auditPruneDefaultRetentionDays = 180

var (
	ErrAuditPrunePlanNotFound      = errors.New("audit prune plan was not found")
	ErrAuditPruneScopeInvalid      = errors.New("audit prune scope is invalid")
	ErrAuditPruneExecutionRequired = errors.New("audit prune requires explicit --execute")
	ErrAuditPrunePlanNotExecutable = errors.New("audit prune plan cannot be executed")
)

type AuditPrunePlanOptions struct {
	Scope         string
	RetentionDays int
}

type AuditPrunePlan struct {
	RunID         uint64                            `json:"run_id"`
	PlanID        string                            `json:"plan_id"`
	Scope         string                            `json:"scope"`
	RetentionDays int                               `json:"retention_days"`
	Cutoff        time.Time                         `json:"cutoff"`
	TargetDigest  string                            `json:"target_digest"`
	TargetCount   int64                             `json:"target_count"`
	Status        string                            `json:"status"`
	ScopeCounts   map[string]int64                  `json:"scope_counts"`
	TableCounts   map[string]int64                  `json:"table_counts"`
	MinTimestamp  *time.Time                        `json:"min_timestamp,omitempty"`
	MaxTimestamp  *time.Time                        `json:"max_timestamp,omitempty"`
	Targets       []models.SecurityAuditPruneTarget `json:"targets"`
}

func (p AuditPrunePlan) MinTimestampOrCutoff() time.Time {
	if p.MinTimestamp == nil {
		return p.Cutoff
	}
	return *p.MinTimestamp
}

func (p AuditPrunePlan) MaxTimestampOrCutoff() time.Time {
	if p.MaxTimestamp == nil {
		return p.Cutoff
	}
	return *p.MaxTimestamp
}

type auditPruneScope struct {
	Name           string
	Connection     string
	TenantID       uint64
	TenantCode     string
	DatabaseDigest string
	RetentionDays  int
}

type auditPruneCandidate struct {
	ID         uint64    `gorm:"column:id"`
	OccurredAt time.Time `gorm:"column:occurred_at"`
	RecordJSON string    `gorm:"column:record_json"`
}

type AuditPrunePlanService struct {
	ctx       context.Context
	now       func() time.Time
	id        func() (string, error)
	retention func(Tenant, int) (int, error)
}

func NewAuditPrunePlanService() *AuditPrunePlanService {
	return &AuditPrunePlanService{
		now:       time.Now,
		id:        newAuditPrunePlanID,
		retention: tenantAuditRetentionDays,
	}
}

func (s *AuditPrunePlanService) WithContext(ctx context.Context) *AuditPrunePlanService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *AuditPrunePlanService) Create(options AuditPrunePlanOptions) (AuditPrunePlan, error) {
	if s == nil {
		return AuditPrunePlan{}, ErrAuditPruneScopeInvalid
	}
	scopes, scope, err := s.resolveScopes(options)
	if err != nil {
		return AuditPrunePlan{}, err
	}
	planID, err := s.id()
	if err != nil {
		return AuditPrunePlan{}, err
	}
	cutoff := s.now().UTC()
	plan := AuditPrunePlan{PlanID: planID, Scope: scope, Cutoff: cutoff, Status: models.SecurityAuditPruneStatusPlanned, ScopeCounts: map[string]int64{}, TableCounts: map[string]int64{}}
	targets, err := s.collectTargets(scopes, cutoff)
	if err != nil {
		return AuditPrunePlan{}, err
	}
	plan.Targets = targets
	if len(scopes) == 1 {
		plan.RetentionDays = scopes[0].RetentionDays
	} else {
		plan.RetentionDays = normalizedAuditRetentionDays(options.RetentionDays)
	}
	plan.TargetCount = int64(len(targets))
	plan.TargetDigest = auditPruneTargetDigest(targets)
	plan.ScopeCounts, plan.TableCounts, plan.MinTimestamp, plan.MaxTimestamp = auditPruneSummary(targets)

	run := models.SecurityAuditPruneRun{
		PlanID: plan.PlanID, Scope: plan.Scope, RetentionDays: plan.RetentionDays, Cutoff: plan.Cutoff,
		TargetDigest: plan.TargetDigest, TargetCount: plan.TargetCount,
		ScopeCounts: int64MapJSON(plan.ScopeCounts), TableCounts: int64MapJSON(plan.TableCounts),
		MinTimestamp: plan.MinTimestamp, MaxTimestamp: plan.MaxTimestamp,
		Status:     models.SecurityAuditPruneStatusPlanned,
		Timestamps: models.Timestamps{CreatedAt: cutoff, UpdatedAt: cutoff},
	}
	orm := OrmForConnectionWithContext(s.ctx, PlatformConnection())
	if err := orm.Transaction(func(query contractsorm.Query) error {
		if err := query.Table(run.TableName()).Create(&run); err != nil {
			return err
		}
		plan.RunID = run.ID
		for index := range plan.Targets {
			plan.Targets[index].RunID = run.ID
			plan.Targets[index].Timestamps = models.Timestamps{CreatedAt: cutoff, UpdatedAt: cutoff}
			if err := query.Table(plan.Targets[index].TableName()).Create(&plan.Targets[index]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return AuditPrunePlan{}, err
	}
	return plan, nil
}

func (s *AuditPrunePlanService) Load(planID string) (AuditPrunePlan, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return AuditPrunePlan{}, ErrAuditPrunePlanNotFound
	}
	query := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query()
	var run models.SecurityAuditPruneRun
	if err := query.Table(run.TableName()).Where("plan_id", planID).First(&run); err != nil {
		return AuditPrunePlan{}, err
	}
	if run.ID == 0 {
		return AuditPrunePlan{}, ErrAuditPrunePlanNotFound
	}
	targets := make([]models.SecurityAuditPruneTarget, 0)
	if err := query.Table((models.SecurityAuditPruneTarget{}).TableName()).Where("run_id", run.ID).OrderBy("id").Get(&targets); err != nil {
		return AuditPrunePlan{}, err
	}
	return AuditPrunePlan{
		RunID: run.ID, PlanID: run.PlanID, Scope: run.Scope, RetentionDays: run.RetentionDays, Cutoff: run.Cutoff,
		TargetDigest: run.TargetDigest, TargetCount: run.TargetCount, ScopeCounts: parseInt64MapJSON(run.ScopeCounts),
		Status:      run.Status,
		TableCounts: parseInt64MapJSON(run.TableCounts), MinTimestamp: run.MinTimestamp, MaxTimestamp: run.MaxTimestamp,
		Targets: targets,
	}, nil
}

func (s *AuditPrunePlanService) resolveScopes(options AuditPrunePlanOptions) ([]auditPruneScope, string, error) {
	scope := strings.TrimSpace(options.Scope)
	if scope == "" {
		scope = "all"
	}
	retentionDays := normalizedAuditRetentionDays(options.RetentionDays)
	platform := auditPruneScope{Name: "platform", Connection: PlatformConnection(), RetentionDays: retentionDays}
	switch {
	case scope == "platform":
		return []auditPruneScope{platform}, scope, nil
	case scope == "all":
		scopes := []auditPruneScope{platform}
		tenants := make([]Tenant, 0)
		if err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Table("tenant").Get(&tenants); err != nil {
			return nil, "", err
		}
		for _, tenant := range tenants {
			days, err := s.retention(tenant, retentionDays)
			if err != nil {
				return nil, "", err
			}
			RegisterTenantConnection(tenant)
			scopes = append(scopes, auditPruneScope{Name: "tenant:" + tenant.Code, Connection: TenantConnectionName(tenant), TenantID: tenant.ID, TenantCode: tenant.Code, DatabaseDigest: tenantDatabaseDigest(tenant), RetentionDays: days})
		}
		return scopes, scope, nil
	case strings.HasPrefix(scope, "tenant:"):
		code := strings.TrimSpace(strings.TrimPrefix(scope, "tenant:"))
		if code == "" {
			return nil, "", ErrAuditPruneScopeInvalid
		}
		var tenant Tenant
		if err := OrmForConnectionWithContext(s.ctx, PlatformConnection()).Query().Table("tenant").Where("code", code).First(&tenant); err != nil {
			return nil, "", err
		}
		if tenant.ID == 0 {
			return nil, "", ErrAuditPruneScopeInvalid
		}
		days, err := s.retention(tenant, retentionDays)
		if err != nil {
			return nil, "", err
		}
		RegisterTenantConnection(tenant)
		return []auditPruneScope{{Name: "tenant:" + tenant.Code, Connection: TenantConnectionName(tenant), TenantID: tenant.ID, TenantCode: tenant.Code, DatabaseDigest: tenantDatabaseDigest(tenant), RetentionDays: days}}, "tenant:" + tenant.Code, nil
	default:
		return nil, "", ErrAuditPruneScopeInvalid
	}
}

func (s *AuditPrunePlanService) collectTargets(scopes []auditPruneScope, plannedAt time.Time) ([]models.SecurityAuditPruneTarget, error) {
	targets := make([]models.SecurityAuditPruneTarget, 0)
	for _, scope := range scopes {
		cutoff := plannedAt.AddDate(0, 0, -scope.RetentionDays)
		retention := NewAuditRetentionService(scope.Connection).WithContext(s.ctx)
		for _, definition := range retention.Targets() {
			rows := make([]auditPruneCandidate, 0)
			err := OrmForConnectionWithContext(s.ctx, scope.Connection).Query().Table(definition.Table).
				Select("id", definition.Column+" AS occurred_at", auditPruneRecordSelect(definition.Table)).
				Where(definition.Column+" < ?", cutoff).
				OrderBy("id").
				Get(&rows)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				record, err := canonicalAuditPruneRecord([]byte(row.RecordJSON))
				if err != nil {
					return nil, err
				}
				targets = append(targets, models.SecurityAuditPruneTarget{
					Scope: scope.Name, Connection: scope.Connection, TenantID: scope.TenantID, TenantCode: scope.TenantCode, DatabaseDigest: scope.DatabaseDigest,
					AuditTable: definition.Table, TimestampColumn: definition.Column, TargetID: row.ID, OccurredAt: row.OccurredAt,
					RecordDigest: digestBytes(record), Record: record,
					Cutoff: cutoff, RetentionDays: scope.RetentionDays, Status: models.SecurityAuditPruneStatusPlanned,
				})
			}
		}
	}
	sortAuditPruneTargets(targets)
	return targets, nil
}

func tenantAuditRetentionDays(tenant Tenant, fallback int) (int, error) {
	governance := NewTenantGovernanceService()
	policy, found, err := governance.loadPolicy(tenant.ID)
	if err != nil {
		return 0, err
	}
	if found && policy.Retention.AuditDays > 0 {
		return policy.Retention.AuditDays, nil
	}
	return normalizedAuditRetentionDays(fallback), nil
}

func normalizedAuditRetentionDays(days int) int {
	if days > 0 {
		return days
	}
	configured := facades.Config().GetInt("security.audit.retention_days", auditPruneDefaultRetentionDays)
	if configured > 0 {
		return configured
	}
	return auditPruneDefaultRetentionDays
}

func newAuditPrunePlanID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func auditPruneTargetDigest(targets []models.SecurityAuditPruneTarget) string {
	items := append([]models.SecurityAuditPruneTarget(nil), targets...)
	sortAuditPruneTargets(items)
	payload := make([]string, 0, len(items))
	for _, target := range items {
		payload = append(payload, strings.Join([]string{
			target.Scope, target.Connection, strconv.FormatUint(target.TenantID, 10), target.TenantCode, target.DatabaseDigest,
			target.AuditTable, target.TimestampColumn, strconv.FormatUint(target.TargetID, 10), target.OccurredAt.UTC().Format(time.RFC3339Nano), target.RecordDigest,
			target.Cutoff.UTC().Format(time.RFC3339Nano), strconv.Itoa(target.RetentionDays),
		}, "\x00"))
	}
	return digestBytes([]byte(strings.Join(payload, "\n")))
}

func tenantDatabaseDigest(tenant Tenant) string {
	payload := strings.Join([]string{
		strings.TrimSpace(tenant.DBHost), strconv.Itoa(tenant.DBPort), strings.TrimSpace(tenant.DBDatabase),
		strings.TrimSpace(tenant.DBUsername), strings.TrimSpace(tenant.DBSchema),
	}, "\x00")
	return digestBytes([]byte(payload))
}

func sortAuditPruneTargets(items []models.SecurityAuditPruneTarget) {
	sort.Slice(items, func(left, right int) bool {
		leftKey := fmt.Sprintf("%s\x00%s\x00%020d", items[left].Scope, items[left].AuditTable, items[left].TargetID)
		rightKey := fmt.Sprintf("%s\x00%s\x00%020d", items[right].Scope, items[right].AuditTable, items[right].TargetID)
		return leftKey < rightKey
	})
}

func auditPruneSummary(targets []models.SecurityAuditPruneTarget) (map[string]int64, map[string]int64, *time.Time, *time.Time) {
	scopes, tables := map[string]int64{}, map[string]int64{}
	var minimum, maximum *time.Time
	for _, target := range targets {
		scopes[target.Scope]++
		tables[target.Scope+":"+target.AuditTable]++
		occurredAt := target.OccurredAt.UTC()
		if minimum == nil || occurredAt.Before(*minimum) {
			value := occurredAt
			minimum = &value
		}
		if maximum == nil || occurredAt.After(*maximum) {
			value := occurredAt
			maximum = &value
		}
	}
	return scopes, tables, minimum, maximum
}

func int64MapJSON(items map[string]int64) string {
	payload, _ := json.Marshal(items)
	return string(payload)
}

func parseInt64MapJSON(value string) map[string]int64 {
	result := map[string]int64{}
	_ = json.Unmarshal([]byte(value), &result)
	return result
}
