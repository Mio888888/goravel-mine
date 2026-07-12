package services

import (
	"errors"
	"math"
	"strings"
	"time"
)

func formatSSOBindingRows(rows []SSOUserBindingRow) []SSOUserBindingRow {
	out := make([]SSOUserBindingRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, formatSSOBindingRow(row))
	}
	return out
}

func formatSSOBindingRow(row SSOUserBindingRow) SSOUserBindingRow {
	row.FirstLoginAt = formatMaybeLogTime(row.FirstLoginAt)
	row.LastLoginAt = formatMaybeLogTime(row.LastLoginAt)
	row.TokenExpiresAt = formatMaybeLogTime(row.TokenExpiresAt)
	row.CreatedAt = formatMaybeLogTime(row.CreatedAt)
	row.UpdatedAt = formatMaybeLogTime(row.UpdatedAt)
	return row
}

func formatSSOLoginLogRows(rows []SSOLoginLogRow) []SSOLoginLogRow {
	out := make([]SSOLoginLogRow, 0, len(rows))
	for _, row := range rows {
		row.LoginAt = formatMaybeLogTime(row.LoginAt)
		out = append(out, row)
	}
	return out
}

func formatMaybeLogTime(value string) string {
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return formatLogTime(parsed)
	}
	if parsed, err := time.Parse("2006-01-02T15:04:05Z07:00", value); err == nil {
		return formatLogTime(parsed)
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05 -0700 MST", value); err == nil {
		return formatLogTime(parsed)
	}
	return strings.TrimSuffix(strings.ReplaceAll(value, "T", " "), "Z")
}

func detectDeviceType(userAgent string) string {
	lower := strings.ToLower(userAgent)
	switch {
	case strings.Contains(lower, "ipad") || strings.Contains(lower, "tablet"):
		return "tablet"
	case strings.Contains(lower, "mobile") || strings.Contains(lower, "iphone") || strings.Contains(lower, "android"):
		return "mobile"
	case strings.TrimSpace(lower) == "":
		return "unknown"
	default:
		return "desktop"
	}
}

func roundFloat(value float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}

func ssoFailureMessage(err error) string {
	switch {
	case errors.Is(err, ErrSSONotConfigured):
		return "SSO 未配置或已停用"
	case errors.Is(err, ErrSSOTokenInvalid):
		return "SSO Token 无效"
	case errors.Is(err, ErrUnauthorized):
		return "未登录或登录已过期"
	case errors.Is(err, ErrUserDisabled):
		return "用户已停用"
	case errors.Is(err, ErrQuotaExceeded):
		return "租户配额已用尽"
	case errors.Is(err, ErrSubscriptionInactive):
		return "租户订阅不可用"
	default:
		var businessErr BusinessError
		if errors.As(err, &businessErr) {
			return businessErr.Message
		}
		return "服务器错误"
	}
}
