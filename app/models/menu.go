package models

type Menu struct {
	ID        uint64  `gorm:"column:id;primaryKey" json:"id"`
	ParentID  uint64  `gorm:"column:parent_id" json:"parent_id"`
	Name      string  `gorm:"column:name" json:"name"`
	Meta      JSONMap `gorm:"column:meta;type:jsonb" json:"meta"`
	Path      string  `gorm:"column:path" json:"path"`
	Component string  `gorm:"column:component" json:"component"`
	Redirect  string  `gorm:"column:redirect" json:"redirect"`
	Status    int8    `gorm:"column:status" json:"status"`
	Sort      int16   `gorm:"column:sort" json:"sort"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (Menu) TableName() string {
	return "menu"
}
