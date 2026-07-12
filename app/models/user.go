package models

import "time"

type User struct {
	ID             uint64    `gorm:"column:id;primaryKey" json:"id"`
	Username       string    `gorm:"column:username" json:"username"`
	Password       string    `gorm:"column:password" json:"-"`
	UserType       string    `gorm:"column:user_type" json:"user_type"`
	Nickname       string    `gorm:"column:nickname" json:"nickname"`
	Phone          string    `gorm:"column:phone" json:"phone"`
	Email          string    `gorm:"column:email" json:"email"`
	Avatar         string    `gorm:"column:avatar" json:"avatar"`
	Signed         string    `gorm:"column:signed" json:"signed"`
	Dashboard      string    `gorm:"column:dashboard" json:"dashboard"`
	Status         int8      `gorm:"column:status" json:"status"`
	LoginIP        string    `gorm:"column:login_ip" json:"login_ip"`
	LoginTime      time.Time `gorm:"column:login_time" json:"login_time"`
	BackendSetting JSONMap   `gorm:"column:backend_setting;type:jsonb" json:"backend_setting"`
	AuditColumns
	Timestamps
	Remark string `gorm:"column:remark" json:"remark"`
}

func (User) TableName() string {
	return "user"
}

type UserBelongsRole struct {
	ID     uint64 `gorm:"column:id;primaryKey" json:"id"`
	UserID uint64 `gorm:"column:user_id" json:"user_id"`
	RoleID uint64 `gorm:"column:role_id" json:"role_id"`
	Timestamps
}

func (UserBelongsRole) TableName() string {
	return "user_belongs_role"
}

type UserDept struct {
	UserID uint64 `gorm:"column:user_id;primaryKey" json:"user_id"`
	DeptID uint64 `gorm:"column:dept_id;primaryKey" json:"dept_id"`
	Timestamps
	SoftDelete
}

func (UserDept) TableName() string {
	return "user_dept"
}

type UserPosition struct {
	UserID     uint64 `gorm:"column:user_id;primaryKey" json:"user_id"`
	PositionID uint64 `gorm:"column:position_id;primaryKey" json:"position_id"`
	Timestamps
	SoftDelete
}

func (UserPosition) TableName() string {
	return "user_position"
}
