package auth

import (
	"errors"
	"sync"
	"testing"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"

	"goravel/app/facades"
	"goravel/app/support/apperror"
)

var errCacheDownForSecurityTest = errors.New("cache down")

func TestValidatePasswordPolicyRejectsMissingRequiredCharacterClasses(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.password.min_length":        10,
		"security.password.require_uppercase": true,
		"security.password.require_lowercase": true,
		"security.password.require_number":    true,
		"security.password.require_symbol":    true,
	})
	defer restore()

	err := ValidatePasswordPolicy("weakpass1!")

	require.ErrorIs(t, err, apperror.ErrBusinessRule)
	require.Contains(t, err.Error(), "密码必须包含大写字母")
}

func TestValidatePasswordPolicyAcceptsConfiguredComplexPassword(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.password.min_length":        10,
		"security.password.require_uppercase": true,
		"security.password.require_lowercase": true,
		"security.password.require_number":    true,
		"security.password.require_symbol":    true,
	})
	defer restore()

	require.NoError(t, ValidatePasswordPolicy("StrongPass1!"))
}

func TestValidatePasswordPolicyAcceptsEnterpriseDefaults(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.password.min_length":        12,
		"security.password.require_uppercase": true,
		"security.password.require_lowercase": true,
		"security.password.require_number":    true,
		"security.password.require_symbol":    true,
	})
	defer restore()

	require.NoError(t, ValidatePasswordPolicy("Enterprise1!"))
	require.ErrorIs(t, ValidatePasswordPolicy("123456"), apperror.ErrBusinessRule)
}

func TestLoginLockoutBlocksAfterConfiguredFailures(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.account_lockout.enabled":        true,
		"security.account_lockout.max_failures":   2,
		"security.account_lockout.window_minutes": 15,
		"security.account_lockout.lock_minutes":   30,
	})
	defer restore()
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	originalNow := loginSecurityNow
	t.Cleanup(func() { loginSecurityNow = originalNow })
	loginSecurityNow = func() time.Time { return now }

	require.NoError(t, CheckLoginLockout("default", "admin"))
	require.NoError(t, RecordLoginFailure("default", "admin"))
	require.NoError(t, CheckLoginLockout("default", "admin"))
	require.NoError(t, RecordLoginFailure("default", "admin"))

	require.ErrorIs(t, CheckLoginLockout("default", "admin"), ErrAccountLocked)
}

func TestLoginLockoutSuccessClearsFailures(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.account_lockout.enabled":        true,
		"security.account_lockout.max_failures":   1,
		"security.account_lockout.window_minutes": 15,
		"security.account_lockout.lock_minutes":   30,
	})
	defer restore()

	require.NoError(t, RecordLoginFailure("default", "admin"))
	require.ErrorIs(t, CheckLoginLockout("default", "admin"), ErrAccountLocked)
	require.NoError(t, RecordLoginSuccess("default", "admin"))
	require.NoError(t, CheckLoginLockout("default", "admin"))
}

func TestRecordLoginFailureReturnsCacheIncrementError(t *testing.T) {
	cache := failingIncrementCache{Driver: newTestCache(), err: errCacheDownForSecurityTest}
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.account_lockout.enabled": true,
	})
	defer restore()

	err := RecordLoginFailure("default", "admin")

	require.ErrorIs(t, err, errCacheDownForSecurityTest)
}

func TestRecordLoginFailureUsesAtomicIncrement(t *testing.T) {
	cache := &incrementTrackingCache{Driver: newTestCache()}
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.account_lockout.enabled":        true,
		"security.account_lockout.max_failures":   2,
		"security.account_lockout.window_minutes": 15,
		"security.account_lockout.lock_minutes":   30,
	})
	defer restore()

	require.NoError(t, RecordLoginFailure("default", "admin"))

	require.Equal(t, 1, cache.incrementCalls)
}

func TestRecordLoginRiskFailureUsesAtomicIncrement(t *testing.T) {
	cache := &incrementTrackingCache{Driver: newTestCache()}
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.login_risk.enabled":           true,
		"security.login_risk.ip_window_minutes": 15,
	})
	defer restore()

	require.NoError(t, RecordLoginRiskFailure("tenant:demo", "alice", "10.0.0.1", "ua"))

	require.Equal(t, 1, cache.incrementCalls)
}

func TestLoginRiskBlocksNoisyIPAcrossUsernames(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.login_risk.enabled":           true,
		"security.login_risk.ip_max_failures":   2,
		"security.login_risk.ip_window_minutes": 15,
	})
	defer restore()

	require.NoError(t, CheckLoginRisk("tenant:demo", "alice", "10.0.0.1", "ua"))
	require.NoError(t, RecordLoginRiskFailure("tenant:demo", "alice", "10.0.0.1", "ua"))
	require.NoError(t, RecordLoginRiskFailure("tenant:demo", "bob", "10.0.0.1", "ua"))

	require.ErrorIs(t, CheckLoginRisk("tenant:demo", "carol", "10.0.0.1", "ua"), ErrLoginRiskBlocked)
}

func TestLoginRiskSuccessDoesNotClearNoisyIPFailures(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.login_risk.enabled":            true,
		"security.login_risk.ip_max_failures":    2,
		"security.login_risk.ip_window_minutes":  15,
		"security.login_risk.user_agent_enabled": true,
	})
	defer restore()

	require.NoError(t, RecordLoginRiskFailure("tenant:demo", "alice", "10.0.0.1", "ua"))
	require.NoError(t, RecordLoginRiskSuccess("tenant:demo", "known", "10.0.0.1", "known-ua"))
	require.NoError(t, RecordLoginRiskFailure("tenant:demo", "bob", "10.0.0.1", "ua"))

	require.ErrorIs(t, CheckLoginRisk("tenant:demo", "carol", "10.0.0.1", "ua"), ErrLoginRiskBlocked)
	changed, err := LoginRiskUserAgentChanged("tenant:demo", "known", "new-ua")
	require.NoError(t, err)
	require.True(t, changed)
}

type incrementTrackingCache struct {
	contractscache.Driver
	mu             sync.Mutex
	incrementCalls int
}

func (c *incrementTrackingCache) Increment(key string, value ...int64) (int64, error) {
	c.mu.Lock()
	c.incrementCalls++
	c.mu.Unlock()
	return c.Driver.Increment(key, value...)
}

func (c *incrementTrackingCache) UnwrapCacheDriver() contractscache.Driver {
	return c.Driver
}

type failingIncrementCache struct {
	contractscache.Driver
	err error
}

func (c failingIncrementCache) Increment(string, ...int64) (int64, error) {
	return 0, c.err
}

func TestLoginRiskRecordsUserAgentChangeWithoutBlocking(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.login_risk.enabled":            true,
		"security.login_risk.user_agent_enabled": true,
	})
	defer restore()

	require.NoError(t, RecordLoginRiskSuccess("tenant:demo", "alice", "10.0.0.1", "old-ua"))

	changed, err := LoginRiskUserAgentChanged("tenant:demo", "alice", "new-ua")

	require.NoError(t, err)
	require.True(t, changed)
}

func TestLoginRiskAllowsChangedUserAgentForStepUp(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.login_risk.enabled":            true,
		"security.login_risk.user_agent_enabled": true,
	})
	defer restore()

	require.NoError(t, RecordLoginRiskSuccess("tenant:demo", "alice", "10.0.0.1", "known-ua"))

	err := CheckLoginRisk("tenant:demo", "alice", "10.0.0.1", "new-ua")

	require.NoError(t, err)
}

func TestValidateInitialPasswordAppliesPolicyToDefaultPassword(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.password.min_length": 10,
	})
	defer restore()

	_, err := InitialPassword("")

	require.ErrorIs(t, err, apperror.ErrBusinessRule)
}

func TestPasswordChangeChallengeConsumesCachedUserID(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.password.change_challenge_minutes": 5,
	})
	defer restore()

	token, err := IssuePasswordChangeChallenge("tenant:demo", 42)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userID, err := ConsumePasswordChangeChallenge("tenant:demo", token)
	require.NoError(t, err)
	require.Equal(t, uint64(42), userID)

	_, err = ConsumePasswordChangeChallenge("tenant:demo", token)
	require.ErrorIs(t, err, ErrPasswordChangeTokenInvalid)
}

func setSecurityPolicyConfig(t *testing.T, values map[string]any) func() {
	t.Helper()
	original := make(map[string]any, len(values))
	for key := range values {
		original[key] = facades.Config().Get(key)
	}
	for key, value := range values {
		facades.Config().Add(key, value)
	}
	return func() {
		for key, value := range original {
			facades.Config().Add(key, value)
		}
	}
}
