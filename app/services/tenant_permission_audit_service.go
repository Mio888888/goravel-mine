package services

import (
	"encoding/json"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/models"
)

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
	return s.LogWithQuery(facades.Orm().Connection(PlatformConnection()).Query(), input)
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
