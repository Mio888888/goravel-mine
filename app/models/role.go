package models

type Role struct {
	ID     uint64 `gorm:"column:id;primaryKey" json:"id"`
	Name   string `gorm:"column:name" json:"name"`
	Code   string `gorm:"column:code" json:"code"`
	Status int8   `gorm:"column:status" json:"status"`
	Sort   int16  `gorm:"column:sort" json:"sort"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (Role) TableName() string {
	return "role"
}

type RoleBelongsMenu struct {
	ID     uint64 `gorm:"column:id;primaryKey" json:"id"`
	RoleID uint64 `gorm:"column:role_id" json:"role_id"`
	MenuID uint64 `gorm:"column:menu_id" json:"menu_id"`
	Timestamps
}

func (RoleBelongsMenu) TableName() string {
	return "role_belongs_menu"
}
