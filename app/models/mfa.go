package models

import "time"

type UserMFA struct {
	ID            uint64    `gorm:"column:id;primaryKey" json:"id"`
	UserID        uint64    `gorm:"column:user_id" json:"user_id"`
	Secret        string    `gorm:"column:secret" json:"-"`
	Enabled       bool      `gorm:"column:enabled" json:"enabled"`
	RecoveryCodes JSONSlice `gorm:"column:recovery_codes;type:jsonb" json:"-"`
	ConfirmedAt   time.Time `gorm:"column:confirmed_at" json:"confirmed_at"`
	LastUsedAt    time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	Timestamps
}

func (UserMFA) TableName() string {
	return "user_mfa"
}

type PlatformUserMFA struct {
	ID            uint64    `gorm:"column:id;primaryKey" json:"id"`
	UserID        uint64    `gorm:"column:user_id" json:"user_id"`
	Secret        string    `gorm:"column:secret" json:"-"`
	Enabled       bool      `gorm:"column:enabled" json:"enabled"`
	RecoveryCodes JSONSlice `gorm:"column:recovery_codes;type:jsonb" json:"-"`
	ConfirmedAt   time.Time `gorm:"column:confirmed_at" json:"confirmed_at"`
	LastUsedAt    time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	Timestamps
}

func (PlatformUserMFA) TableName() string {
	return "platform_user_mfa"
}
