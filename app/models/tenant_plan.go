package models

type TenantPlan struct {
	ID       uint64  `gorm:"column:id;primaryKey" json:"id"`
	Code     string  `gorm:"column:code" json:"code"`
	Name     string  `gorm:"column:name" json:"name"`
	Status   int8    `gorm:"column:status" json:"status"`
	Sort     int     `gorm:"column:sort" json:"sort"`
	Billing  JSONMap `gorm:"column:billing;type:jsonb" json:"billing"`
	Quotas   JSONMap `gorm:"column:quotas;type:jsonb" json:"quotas"`
	Features JSONMap `gorm:"column:features;type:jsonb" json:"features"`
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (TenantPlan) TableName() string {
	return "tenant_plan"
}
