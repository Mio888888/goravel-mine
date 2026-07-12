package config

import "goravel/app/facades"

func init() {
	config := facades.Config()
	profile := securityProfile(config.EnvBool("SECURITY_ENTERPRISE", false))
	config.Add("security", buildSecurityConfig(config, profile))
}

type securityConfig interface {
	Env(string, ...any) any
	EnvString(string, ...string) string
}

func buildSecurityConfig(config securityConfig, profile securityDefaults) map[string]any {
	return map[string]any{
		"enterprise": envOrDefault(config, "SECURITY_ENTERPRISE", false),
		"mfa": map[string]any{
			"totp_enabled":      envOrDefault(config, "SECURITY_MFA_TOTP_ENABLED", profile.MFATOTPEnabled),
			"totp_issuer":       config.Env("SECURITY_MFA_TOTP_ISSUER", config.Env("APP_NAME", "Goravel")),
			"challenge_minutes": config.Env("SECURITY_MFA_CHALLENGE_MINUTES", 5),
			"remember_minutes":  config.Env("SECURITY_MFA_REMEMBER_MINUTES", 720),
			"recovery_codes":    config.Env("SECURITY_MFA_RECOVERY_CODES", 8),
		},
		"password": map[string]any{
			"min_length":               envOrDefault(config, "SECURITY_PASSWORD_MIN_LENGTH", profile.PasswordMinLength),
			"require_uppercase":        envOrDefault(config, "SECURITY_PASSWORD_REQUIRE_UPPERCASE", profile.PasswordRequireUppercase),
			"require_lowercase":        envOrDefault(config, "SECURITY_PASSWORD_REQUIRE_LOWERCASE", profile.PasswordRequireLowercase),
			"require_number":           envOrDefault(config, "SECURITY_PASSWORD_REQUIRE_NUMBER", profile.PasswordRequireNumber),
			"require_symbol":           envOrDefault(config, "SECURITY_PASSWORD_REQUIRE_SYMBOL", profile.PasswordRequireSymbol),
			"history_limit":            envOrDefault(config, "SECURITY_PASSWORD_HISTORY_LIMIT", profile.PasswordHistoryLimit),
			"max_age_days":             envOrDefault(config, "SECURITY_PASSWORD_MAX_AGE_DAYS", profile.PasswordMaxAgeDays),
			"change_challenge_minutes": config.Env("SECURITY_PASSWORD_CHANGE_CHALLENGE_MINUTES", 5),
		},
		"account_lockout": map[string]any{
			"enabled":        config.Env("SECURITY_ACCOUNT_LOCKOUT_ENABLED", true),
			"max_failures":   config.Env("SECURITY_ACCOUNT_LOCKOUT_MAX_FAILURES", 5),
			"window_minutes": config.Env("SECURITY_ACCOUNT_LOCKOUT_WINDOW_MINUTES", 15),
			"lock_minutes":   config.Env("SECURITY_ACCOUNT_LOCKOUT_LOCK_MINUTES", 15),
		},
		"login_risk": map[string]any{
			"enabled":            config.Env("SECURITY_LOGIN_RISK_ENABLED", true),
			"ip_window_minutes":  config.Env("SECURITY_LOGIN_RISK_IP_WINDOW_MINUTES", 15),
			"ip_max_failures":    config.Env("SECURITY_LOGIN_RISK_IP_MAX_FAILURES", 30),
			"user_agent_enabled": config.Env("SECURITY_LOGIN_RISK_USER_AGENT_ENABLED", true),
		},
		"csrf": map[string]any{
			"enabled":         envOrDefault(config, "SECURITY_CSRF_ENABLED", profile.CSRFEnabled),
			"trusted_origins": envStringListFrom(config, "SECURITY_CSRF_TRUSTED_ORIGINS", appURLFallback(config)...),
			"same_site":       config.Env("SECURITY_CSRF_SAME_SITE", "lax"),
			"cookie_secure":   config.Env("SECURITY_CSRF_COOKIE_SECURE", ""),
		},
		"sensitive_data": map[string]any{
			"fields": envStringListFrom(
				config,
				"SECURITY_SENSITIVE_FIELDS",
				"password",
				"old_password",
				"new_password",
				"new_password_confirmation",
				"access_token",
				"refresh_token",
				"password_change_token",
				"token",
				"id_token",
				"jwt_secret",
				"client_secret",
				"secret_key",
				"db_password",
			),
		},
		"key_rotation": map[string]any{
			"days": config.Env("SECURITY_KEY_ROTATION_DAYS", 90),
		},
		"audit": map[string]any{
			"retention_days":               config.Env("SECURITY_AUDIT_RETENTION_DAYS", 180),
			"immutable_evidence_max_bytes": config.Env("SECURITY_IMMUTABLE_EVIDENCE_MAX_BYTES", 64<<20),
		},
	}
}

func envOrDefault(config securityConfig, name string, fallback any) any {
	value := config.Env(name, fallback)
	if text, ok := value.(string); ok && text == "" {
		return fallback
	}
	return value
}

func envStringListFrom(config interface {
	EnvString(string, ...string) string
}, name string, fallback ...string) []string {
	return splitConfigList(config.EnvString(name), fallback...)
}

func appURLFallback(config interface {
	EnvString(string, ...string) string
}) []string {
	appURL := config.EnvString("APP_URL")
	if appURL == "" {
		return nil
	}
	return []string{appURL}
}

type securityDefaults struct {
	MFATOTPEnabled           bool
	PasswordMinLength        int
	PasswordRequireUppercase bool
	PasswordRequireLowercase bool
	PasswordRequireNumber    bool
	PasswordRequireSymbol    bool
	PasswordHistoryLimit     int
	PasswordMaxAgeDays       int
	CSRFEnabled              bool
}

func securityProfile(enterprise bool) securityDefaults {
	if enterprise {
		return securityDefaults{
			MFATOTPEnabled:           true,
			PasswordMinLength:        12,
			PasswordRequireUppercase: true,
			PasswordRequireLowercase: true,
			PasswordRequireNumber:    true,
			PasswordRequireSymbol:    true,
			PasswordHistoryLimit:     5,
			PasswordMaxAgeDays:       90,
			CSRFEnabled:              true,
		}
	}

	return securityDefaults{
		MFATOTPEnabled:    false,
		PasswordMinLength: 6,
	}
}
