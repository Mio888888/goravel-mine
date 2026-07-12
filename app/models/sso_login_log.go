package models

import "time"

type SSOLoginLog struct {
	ID            uint64    `gorm:"column:id;primaryKey" json:"id"`
	UserID        uint64    `gorm:"column:user_id" json:"user_id"`
	ProviderID    uint64    `gorm:"column:provider_id" json:"provider_id"`
	BindingID     uint64    `gorm:"column:binding_id" json:"binding_id"`
	SSOUserID     string    `gorm:"column:sso_user_id" json:"sso_user_id"`
	SSOEmail      string    `gorm:"column:sso_email" json:"sso_email"`
	Status        int16     `gorm:"column:status" json:"status"`
	FailureReason string    `gorm:"column:failure_reason" json:"failure_reason"`
	IP            string    `gorm:"column:ip" json:"ip"`
	UserAgent     string    `gorm:"column:user_agent" json:"user_agent"`
	DeviceType    string    `gorm:"column:device_type" json:"device_type"`
	LoginAt       time.Time `gorm:"column:login_at" json:"login_at"`
}

func (SSOLoginLog) TableName() string {
	return "sso_login_log"
}
