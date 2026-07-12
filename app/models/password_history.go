package models

type UserPasswordHistory struct {
	ID       uint64 `gorm:"column:id;primaryKey" json:"id"`
	UserID   uint64 `gorm:"column:user_id" json:"user_id"`
	Password string `gorm:"column:password" json:"-"`
	Timestamps
}

func (UserPasswordHistory) TableName() string {
	return "user_password_history"
}

type PlatformUserPasswordHistory struct {
	ID       uint64 `gorm:"column:id;primaryKey" json:"id"`
	UserID   uint64 `gorm:"column:user_id" json:"user_id"`
	Password string `gorm:"column:password" json:"-"`
	Timestamps
}

func (PlatformUserPasswordHistory) TableName() string {
	return "platform_user_password_history"
}
