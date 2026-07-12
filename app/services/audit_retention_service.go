package services

import (
	"context"
	"time"

	"goravel/app/facades"
)

type AuditRetentionService struct {
	ctx        context.Context
	connection string
}

type AuditRetentionResult struct {
	Scope string `json:"scope"`
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
}

type AuditRetentionTarget struct {
	Table  string
	Column string
}

type auditRetentionTarget = AuditRetentionTarget

var auditRetentionHasTable = auditRetentionSchemaHasTable

func NewAuditRetentionService(connection string) *AuditRetentionService {
	return &AuditRetentionService{connection: connection}
}

func (s *AuditRetentionService) WithContext(ctx context.Context) *AuditRetentionService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *AuditRetentionService) Targets() []AuditRetentionTarget {
	return availableAuditRetentionTargets(s.connection, defaultAuditRetentionTargets())
}

func (s *AuditRetentionService) Prune(retentionDays int, dryRun bool) ([]AuditRetentionResult, error) {
	if retentionDays <= 0 {
		return nil, BusinessError{Message: "审计留存天数必须大于 0"}
	}
	if !dryRun {
		return nil, ErrAuditPruneExecutionRequired
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	results := make([]AuditRetentionResult, 0)
	for _, target := range s.Targets() {
		query := OrmForConnectionWithContext(s.ctx, s.connection).Query().
			Table(target.Table).
			Where(target.Column+" < ?", cutoff)
		count, err := query.Count()
		if err != nil {
			return nil, err
		}
		results = append(results, AuditRetentionResult{Scope: s.connection, Table: target.Table, Rows: count})
	}
	return results, nil
}

func PruneAllAuditRetention(ctx context.Context, retentionDays int, dryRun bool) ([]AuditRetentionResult, error) {
	if !dryRun {
		return nil, ErrAuditPruneExecutionRequired
	}
	results := make([]AuditRetentionResult, 0)
	platformConnection := PlatformConnection()
	platformResults, err := NewAuditRetentionService(platformConnection).WithContext(ctx).Prune(retentionDays, true)
	if err != nil {
		return nil, err
	}
	results = append(results, namedAuditRetentionResults("platform", platformResults)...)

	tenants := make([]Tenant, 0)
	if err := OrmForConnectionWithContext(ctx, platformConnection).Query().Table("tenant").Get(&tenants); err != nil {
		return nil, err
	}
	for _, tenant := range tenants {
		RegisterTenantConnection(tenant)
		items, err := NewAuditRetentionService(TenantConnectionName(tenant)).WithContext(ctx).Prune(retentionDays, true)
		if err != nil {
			return nil, err
		}
		results = append(results, namedAuditRetentionResults("tenant:"+tenant.Code, items)...)
	}
	return results, nil
}

func namedAuditRetentionResults(scope string, items []AuditRetentionResult) []AuditRetentionResult {
	for index := range items {
		items[index].Scope = scope
	}
	return items
}

func defaultAuditRetentionTargets() []AuditRetentionTarget {
	return []AuditRetentionTarget{
		{Table: "user_login_log", Column: "login_time"},
		{Table: "user_operation_log", Column: "created_at"},
		{Table: "sso_login_log", Column: "login_at"},
		{Table: "tenant_permission_audit", Column: "created_at"},
	}
}

func availableAuditRetentionTargets(connection string, targets []AuditRetentionTarget) []AuditRetentionTarget {
	available := make([]AuditRetentionTarget, 0, len(targets))
	for _, target := range targets {
		if auditRetentionHasTable(connection, target.Table) {
			available = append(available, target)
		}
	}
	return available
}

func auditRetentionSchemaHasTable(connection, table string) bool {
	previous := facades.Schema().GetConnection()
	facades.Schema().SetConnection(connection)
	defer facades.Schema().SetConnection(previous)
	return facades.Schema().HasTable(table)
}
