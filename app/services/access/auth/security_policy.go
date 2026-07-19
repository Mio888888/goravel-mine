package auth

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	frameworkcache "github.com/goravel/framework/cache"
	contractscache "github.com/goravel/framework/contracts/cache"

	authcontract "goravel/app/contracts/auth"
	"goravel/app/facades"
	"goravel/app/support/apperror"
)

const DefaultPassword = "123456"

var ErrAccountLocked = authcontract.ErrAccountLocked
var ErrLoginRiskBlocked = authcontract.ErrLoginRiskBlocked

var loginSecurityNow = time.Now
var loginSecurityCache = func() contractscache.Driver {
	return facades.Cache()
}

func ConfigureSecurityCache(provider func() contractscache.Driver) {
	if provider != nil {
		loginSecurityCache = provider
	}
}

func SecurityCache() contractscache.Driver {
	return loginSecurityCache()
}

type cacheDriverUnwrapper interface {
	UnwrapCacheDriver() contractscache.Driver
}

type LoginSignal struct {
	IP        string
	UserAgent string
}

func firstLoginSignal(signals []LoginSignal) LoginSignal {
	if len(signals) == 0 {
		return LoginSignal{}
	}
	return signals[0]
}

func FirstLoginSignal(signals []LoginSignal) LoginSignal {
	return firstLoginSignal(signals)
}

func ValidatePasswordPolicy(password string) error {
	minLength := facades.Config().GetInt("security.password.min_length", 6)
	if len([]rune(password)) < minLength {
		return apperror.BusinessError{Message: fmt.Sprintf("密码长度不能少于 %d 位", minLength)}
	}
	if facades.Config().GetBool("security.password.require_uppercase", false) && !hasRune(password, unicode.IsUpper) {
		return apperror.BusinessError{Message: "密码必须包含大写字母"}
	}
	if facades.Config().GetBool("security.password.require_lowercase", false) && !hasRune(password, unicode.IsLower) {
		return apperror.BusinessError{Message: "密码必须包含小写字母"}
	}
	if facades.Config().GetBool("security.password.require_number", false) && !hasRune(password, unicode.IsDigit) {
		return apperror.BusinessError{Message: "密码必须包含数字"}
	}
	if facades.Config().GetBool("security.password.require_symbol", false) && !hasRune(password, isPasswordSymbol) {
		return apperror.BusinessError{Message: "密码必须包含特殊字符"}
	}
	return nil
}

func InitialPassword(password string) (string, error) {
	if password == "" {
		password = DefaultPassword
	}
	if err := ValidatePasswordPolicy(password); err != nil {
		return "", err
	}
	return password, nil
}

func CheckLoginLockout(scope, username string) error {
	if !accountLockoutEnabled() {
		return nil
	}
	cache := loginSecurityCache()
	if cache.Has(loginLockoutKey(scope, username)) {
		return ErrAccountLocked
	}
	return nil
}

func CheckLoginRisk(scope, username, ip, userAgent string) error {
	if !loginRiskEnabled() {
		return nil
	}
	if ip == "" {
		return nil
	}
	count := loginSecurityCache().GetInt64(loginRiskIPFailureKey(scope, ip))
	if count >= int64(facades.Config().GetInt("security.login_risk.ip_max_failures", 30)) {
		return ErrLoginRiskBlocked
	}
	return nil
}

func RecordLoginFailure(scope, username string) error {
	if !accountLockoutEnabled() {
		return nil
	}
	cache := loginSecurityCache()
	count, err := incrementCacheCounter(cache, loginFailureKey(scope, username), minutesDuration("security.account_lockout.window_minutes", 15))
	if err != nil {
		return err
	}
	if count >= int64(facades.Config().GetInt("security.account_lockout.max_failures", 5)) {
		return cache.Put(loginLockoutKey(scope, username), loginSecurityNow().Unix(), minutesDuration("security.account_lockout.lock_minutes", 15))
	}
	return nil
}

func RecordLoginRiskFailure(scope, username, ip, userAgent string) error {
	if !loginRiskEnabled() || ip == "" {
		return nil
	}
	cache := loginSecurityCache()
	_, err := incrementCacheCounter(cache, loginRiskIPFailureKey(scope, ip), minutesDuration("security.login_risk.ip_window_minutes", 15))
	return err
}

func RecordLoginSuccess(scope, username string) error {
	cache := loginSecurityCache()
	cache.Forget(loginFailureKey(scope, username))
	cache.Forget(loginLockoutKey(scope, username))
	return nil
}

func RecordLoginRiskSuccess(scope, username, ip, userAgent string) error {
	if !loginRiskEnabled() {
		return nil
	}
	cache := loginSecurityCache()
	if userAgent != "" && facades.Config().GetBool("security.login_risk.user_agent_enabled", true) {
		return cache.Put(loginRiskUserAgentKey(scope, username), userAgent, 30*24*time.Hour)
	}
	return nil
}

func LoginRiskUserAgentChanged(scope, username, userAgent string) (bool, error) {
	if !loginRiskEnabled() || !facades.Config().GetBool("security.login_risk.user_agent_enabled", true) || userAgent == "" {
		return false, nil
	}
	cached := loginSecurityCache().Get(loginRiskUserAgentKey(scope, username))
	last, ok := cached.(string)
	return ok && last != "" && last != userAgent, nil
}

func hasRune(value string, match func(rune) bool) bool {
	for _, r := range value {
		if match(r) {
			return true
		}
	}
	return false
}

func isPasswordSymbol(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func accountLockoutEnabled() bool {
	return facades.Config().GetBool("security.account_lockout.enabled", true)
}

func loginRiskEnabled() bool {
	return facades.Config().GetBool("security.login_risk.enabled", true)
}

func minutesDuration(configKey string, fallback int) time.Duration {
	minutes := facades.Config().GetInt(configKey, fallback)
	if minutes <= 0 {
		minutes = fallback
	}
	return time.Duration(minutes) * time.Minute
}

func MinutesDuration(configKey string, fallback int) time.Duration {
	return minutesDuration(configKey, fallback)
}

func incrementCacheCounter(cache contractscache.Driver, key string, ttl time.Duration) (int64, error) {
	cache.Add(key, initialCacheCounterValue(cache), ttl)
	return cache.Increment(key)
}

func initialCacheCounterValue(cache contractscache.Driver) any {
	if isMemoryCacheDriver(cache) {
		return new(int64)
	}
	return int64(0)
}

func isMemoryCacheDriver(cache contractscache.Driver) bool {
	if _, ok := cache.(*frameworkcache.Memory); ok {
		return true
	}
	if app, ok := cache.(*frameworkcache.Application); ok {
		return isMemoryCacheDriver(app.Driver)
	}
	if wrapped, ok := cache.(cacheDriverUnwrapper); ok {
		return isMemoryCacheDriver(wrapped.UnwrapCacheDriver())
	}
	return false
}

func loginFailureKey(scope, username string) string {
	return "security:login:fail:" + loginIdentity(scope, username)
}

func loginLockoutKey(scope, username string) string {
	return "security:login:lock:" + loginIdentity(scope, username)
}

func loginRiskIPFailureKey(scope, ip string) string {
	return "security:login:risk:ip:" + loginIdentity(scope, ip)
}

func loginRiskUserAgentKey(scope, username string) string {
	return "security:login:risk:ua:" + loginIdentity(scope, username)
}

func loginIdentity(scope, username string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	username = strings.ToLower(strings.TrimSpace(username))
	if scope == "" {
		scope = "default"
	}
	return scope + ":" + username
}

func LoginIdentity(scope, username string) string {
	return loginIdentity(scope, username)
}
