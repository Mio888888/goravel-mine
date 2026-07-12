package models

type Position struct {
	ID     uint64 `gorm:"column:id;primaryKey" json:"id"`
	Name   string `gorm:"column:name" json:"name"`
	DeptID uint64 `gorm:"column:dept_id" json:"dept_id"`
	Timestamps
	SoftDelete
}

func (Position) TableName() string {
	return "position"
}
