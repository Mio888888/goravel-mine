package models

type TenantPermissionAudit struct {
	ID             uint64   `gorm:"column:id;primaryKey" json:"id"`
	TenantID       uint64   `gorm:"column:tenant_id" json:"tenant_id"`
	TenantCode     string   `gorm:"column:tenant_code" json:"tenant_code"`
	Operation      string   `gorm:"column:operation" json:"operation"`
	Source         string   `gorm:"column:source" json:"source"`
	BeforeSnapshot JSONMap  `gorm:"column:before_snapshot;type:jsonb" json:"before_snapshot"`
	AfterSnapshot  JSONMap  `gorm:"column:after_snapshot;type:jsonb" json:"after_snapshot"`
	Added          []string `gorm:"-" json:"added"`
	Removed        []string `gorm:"-" json:"removed"`
	Unchanged      []string `gorm:"-" json:"unchanged"`
	Diff           JSONMap  `gorm:"column:diff;type:jsonb" json:"diff"`
	OperatorID     uint64   `gorm:"column:operator_id" json:"operator_id"`
	OperatorName   string   `gorm:"column:operator_name" json:"operator_name"`
	Remark         string   `gorm:"column:remark" json:"remark"`
	Timestamps
}

func (TenantPermissionAudit) TableName() string {
	return "tenant_permission_audit"
}
