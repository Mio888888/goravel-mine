package models

type DataPermissionPolicy struct {
	ID         uint64    `gorm:"column:id;primaryKey" json:"id"`
	UserID     uint64    `gorm:"column:user_id" json:"user_id"`
	PositionID uint64    `gorm:"column:position_id" json:"position_id"`
	PolicyType string    `gorm:"column:policy_type" json:"policy_type"`
	IsDefault  bool      `gorm:"column:is_default" json:"is_default"`
	Value      JSONSlice `gorm:"column:value;type:jsonb" json:"value"`
	Timestamps
	SoftDelete
}

func (DataPermissionPolicy) TableName() string {
	return "data_permission_policy"
}
