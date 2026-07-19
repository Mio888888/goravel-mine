package auth

import (
	"crypto/rand"
	"strconv"
	"strings"

	"goravel/app/support/apperror"
)

var ErrPasswordChangeTokenInvalid = apperror.BusinessError{Message: "改密 Token 无效或已过期"}

func IssuePasswordChangeChallenge(scope string, userID uint64) (string, error) {
	token := rand.Text()
	err := loginSecurityCache().Put(passwordChangeChallengeKey(scope, token), userID, minutesDuration("security.password.change_challenge_minutes", 5))
	return token, err
}

func ConsumePasswordChangeChallenge(scope, token string) (uint64, error) {
	userID, err := PasswordChangeChallengeUserID(scope, token)
	if err != nil {
		return 0, err
	}
	ForgetPasswordChangeChallenge(scope, token)
	return userID, nil
}

func PasswordChangeChallengeUserID(scope, token string) (uint64, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, ErrPasswordChangeTokenInvalid
	}
	key := passwordChangeChallengeKey(scope, token)
	value := loginSecurityCache().Get(key)
	userID, ok := cacheUint64(value)
	if !ok || userID == 0 {
		return 0, ErrPasswordChangeTokenInvalid
	}
	return userID, nil
}

func cacheUint64(value any) (uint64, bool) {
	switch typed := value.(type) {
	case uint64:
		return typed, true
	case uint:
		return uint64(typed), true
	case int:
		if typed > 0 {
			return uint64(typed), true
		}
	case int64:
		if typed > 0 {
			return uint64(typed), true
		}
	case float64:
		if typed > 0 {
			return uint64(typed), true
		}
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}

func ForgetPasswordChangeChallenge(scope, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	_ = loginSecurityCache().Forget(passwordChangeChallengeKey(scope, token))
}

func passwordChangeChallengeKey(scope, token string) string {
	return "security:password:change:" + loginIdentity(scope, token)
}
