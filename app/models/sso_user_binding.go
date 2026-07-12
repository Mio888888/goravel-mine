package models

import "time"

type SSOUserBinding struct {
	ID             uint64    `gorm:"column:id;primaryKey" json:"id"`
	UserID         uint64    `gorm:"column:user_id" json:"user_id"`
	ProviderID     uint64    `gorm:"column:provider_id" json:"provider_id"`
	SSOUserID      string    `gorm:"column:sso_user_id" json:"sso_user_id"`
	SSOEmail       string    `gorm:"column:sso_email" json:"sso_email"`
	SSOUsername    string    `gorm:"column:sso_username" json:"sso_username"`
	SSOAvatar      string    `gorm:"column:sso_avatar" json:"sso_avatar"`
	AccessToken    string    `gorm:"column:access_token" json:"-"`
	RefreshToken   string    `gorm:"column:refresh_token" json:"-"`
	TokenExpiresAt time.Time `gorm:"column:token_expires_at" json:"token_expires_at"`
	FirstLoginAt   time.Time `gorm:"column:first_login_at" json:"first_login_at"`
	LastLoginAt    time.Time `gorm:"column:last_login_at" json:"last_login_at"`
	LoginCount     int       `gorm:"column:login_count" json:"login_count"`
	Timestamps
}

func (SSOUserBinding) TableName() string {
	return "sso_user_binding"
}
