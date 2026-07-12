package services

import "time"

const (
	ssoLogStatusSuccess int16 = 1
	ssoLogStatusFailure int16 = 2
)

type SSOUserBindingRow struct {
	ID                  uint64 `json:"id"`
	UserID              uint64 `json:"user_id"`
	Username            string `json:"username"`
	Nickname            string `json:"nickname"`
	ProviderID          uint64 `json:"provider_id"`
	ProviderName        string `json:"provider_name"`
	ProviderDisplayName string `json:"provider_display_name"`
	ProviderType        string `json:"provider_type"`
	ProviderScene       string `json:"provider_scene"`
	SSOUserID           string `json:"sso_user_id"`
	SSOEmail            string `json:"sso_email"`
	SSOUsername         string `json:"sso_username"`
	SSOAvatar           string `json:"sso_avatar"`
	LoginCount          int    `json:"login_count"`
	FirstLoginAt        string `json:"first_login_at"`
	LastLoginAt         string `json:"last_login_at"`
	TokenExpiresAt      string `json:"token_expires_at"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type SSOLoginLogRow struct {
	ID                  uint64 `json:"id"`
	UserID              uint64 `json:"user_id"`
	Username            string `json:"username"`
	ProviderID          uint64 `json:"provider_id"`
	ProviderName        string `json:"provider_name"`
	ProviderDisplayName string `json:"provider_display_name"`
	ProviderType        string `json:"provider_type"`
	ProviderScene       string `json:"provider_scene"`
	BindingID           uint64 `json:"binding_id"`
	SSOUserID           string `json:"sso_user_id"`
	SSOEmail            string `json:"sso_email"`
	Status              int16  `json:"status"`
	FailureReason       string `json:"failure_reason"`
	IP                  string `json:"ip"`
	UserAgent           string `json:"user_agent"`
	DeviceType          string `json:"device_type"`
	LoginAt             string `json:"login_at"`
}

type SSOLoginStats struct {
	Total        int64                   `json:"total"`
	SuccessCount int64                   `json:"success_count"`
	FailCount    int64                   `json:"fail_count"`
	SuccessRate  float64                 `json:"success_rate"`
	Providers    []SSOProviderLogStatRow `json:"providers"`
}

type SSOProviderLogStatRow struct {
	ProviderID          uint64 `json:"provider_id"`
	ProviderName        string `json:"provider_name"`
	ProviderDisplayName string `json:"provider_display_name"`
	Total               int64  `json:"total"`
	SuccessCount        int64  `json:"success_count"`
	FailCount           int64  `json:"fail_count"`
}

type ssoBindingInput struct {
	UserID       uint64
	ProviderID   uint64
	SSOUserID    string
	SSOEmail     string
	SSOUsername  string
	SSOAvatar    string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type ssoLogInput struct {
	UserID        uint64
	ProviderID    uint64
	BindingID     uint64
	SSOUserID     string
	SSOEmail      string
	Status        int16
	FailureReason string
	IP            string
	UserAgent     string
}
