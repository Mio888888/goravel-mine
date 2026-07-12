package models

type Department struct {
	ID       uint64 `gorm:"column:id;primaryKey" json:"id"`
	Name     string `gorm:"column:name" json:"name"`
	ParentID uint64 `gorm:"column:parent_id" json:"parent_id"`
	Timestamps
	SoftDelete
}

func (Department) TableName() string {
	return "department"
}

type DeptLeader struct {
	DeptID uint64 `gorm:"column:dept_id;primaryKey" json:"dept_id"`
	UserID uint64 `gorm:"column:user_id;primaryKey" json:"user_id"`
	Timestamps
	SoftDelete
}

func (DeptLeader) TableName() string {
	return "dept_leader"
}
