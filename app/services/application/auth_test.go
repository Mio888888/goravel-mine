package application

import (
	"encoding/json"
	"github.com/goravel/framework/cache"
	contractscache "github.com/goravel/framework/contracts/cache"
	contractshash "github.com/goravel/framework/contracts/hash"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"goravel/app/facades"
	"goravel/app/models"
	authservice "goravel/app/services/access/auth"
	"strings"
	"testing"
	"time"
)

// Source: cache_test_helpers_test.go
type testConfig map[string]any

func newTestCache() *cache.Memory {
	driver, _ := cache.NewMemory(testConfig{"cache.prefix": "test"})
	return driver
}

type testPasswordHasher struct{}

func (testPasswordHasher) Make(value string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.MinCost)
	return string(hash), err
}

func (testPasswordHasher) Check(value, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(value)) == nil
}

func (testPasswordHasher) NeedsRehash(string) bool {
	return false
}

func useTestPasswordHasher(t *testing.T) {
	t.Helper()
	restore := authservice.SetHashProviderForTest(func() contractshash.Hash {
		return testPasswordHasher{}
	})
	t.Cleanup(restore)
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

func (c testConfig) Env(name string, defaultValue ...any) any {
	return c.Get(name, defaultValue...)
}

func (c testConfig) EnvString(name string, defaultValue ...string) string {
	return c.GetString(name, defaultValue...)
}

func (c testConfig) EnvBool(name string, defaultValue ...bool) bool {
	return c.GetBool(name, defaultValue...)
}

func (c testConfig) Add(name string, configuration any) {
	c[name] = configuration
}

func (c testConfig) Get(path string, defaultValue ...any) any {
	if value, ok := c[path]; ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return nil
}

func (c testConfig) GetString(path string, defaultValue ...string) string {
	if value, ok := c[path].(string); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

func (c testConfig) GetInt(path string, defaultValue ...int) int {
	if value, ok := c[path].(int); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

func (c testConfig) GetBool(path string, defaultValue ...bool) bool {
	if value, ok := c[path].(bool); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return false
}

func (c testConfig) GetDuration(path string, defaultValue ...time.Duration) time.Duration {
	if value, ok := c[path].(time.Duration); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

func (c testConfig) UnmarshalKey(string, any) error {
	return nil
}

// Source: csrf_service_test.go
func TestCSRFOriginAllowedRejectsWildcardTrustedOrigin(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.trusted_origins": []string{"*"},
	})
	defer restore()

	require.False(t, CSRFOriginAllowed("https://evil.example"))
}

func TestCSRFOriginAllowedAcceptsConfiguredOrigin(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.trusted_origins": []string{"https://admin.example"},
	})
	defer restore()

	require.True(t, CSRFOriginAllowed("https://admin.example/path"))
	require.False(t, CSRFOriginAllowed("https://evil.example"))
}

func TestCSRFCookieSecureDefaultsOnForSameSiteNone(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.same_site":     "none",
		"security.csrf.cookie_secure": "",
	})
	defer restore()

	require.True(t, CSRFCookieSecure())
}

func TestCSRFCookieSecureCanBeConfigured(t *testing.T) {
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.same_site":     "lax",
		"security.csrf.cookie_secure": "true",
	})
	defer restore()

	require.True(t, CSRFCookieSecure())

	restore = setSecurityPolicyConfig(t, map[string]any{
		"security.csrf.same_site":     "none",
		"security.csrf.cookie_secure": "false",
	})
	defer restore()

	require.False(t, CSRFCookieSecure())
}

// Source: current_user_service_test.go
func TestProfileUpdateValuesKeepsBackendSettingInUserUpdate(t *testing.T) {
	values, err := profileUpdateValues(ProfileUpdate{
		Nickname: "admin",
		BackendSetting: models.JSONMap{
			"app": map[string]any{"useLocale": "zh_CN"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "admin", values["nickname"])
	raw, ok := values["backend_setting"].(string)
	require.True(t, ok)
	require.JSONEq(t, `{"app":{"useLocale":"zh_CN"}}`, raw)
	require.True(t, json.Valid([]byte(raw)))
}

func TestProfileValidationMessageReturnsBusinessRuleMessage(t *testing.T) {
	err := BusinessError{Message: "密码必须包含特殊字符"}

	require.True(t, IsProfileValidationError(err))
	require.Equal(t, "密码必须包含特殊字符", ProfileValidationMessage(err))
}

// Source: mfa_service_test.go
func TestTOTPCodeMatchesRFC6238Vector(t *testing.T) {
	secret := []byte("12345678901234567890")

	code, err := GenerateTOTPCode(secret, time.Unix(59, 0), 30, 8)

	require.NoError(t, err)
	require.Equal(t, "94287082", code)
}

func TestVerifyTOTPAllowsAdjacentWindow(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(59, 0)

	require.True(t, VerifyTOTPCode(secret, "94287082", now.Add(30*time.Second), 30, 8, 1))
	require.False(t, VerifyTOTPCode(secret, "94287082", now.Add(90*time.Second), 30, 8, 1))
}

func TestGenerateRecoveryCodesReturnsUniqueHashes(t *testing.T) {
	useTestPasswordHasher(t)

	codes, hashes, err := GenerateMFARecoveryCodes(4)

	require.NoError(t, err)
	require.Len(t, codes, 4)
	require.Len(t, hashes, 4)
	require.NotEqual(t, codes[0], hashes[0])
	require.NotContains(t, hashes, codes[0])
}

func TestValidateMFASetupAllowedRejectsEnabledRecord(t *testing.T) {
	require.ErrorIs(t, ValidateMFASetupAllowed(true), ErrMFAAlreadyEnabled)
	require.NoError(t, ValidateMFASetupAllowed(false))
}

func TestValidateMFADisableCodeRequiresCode(t *testing.T) {
	require.ErrorIs(t, ValidateMFADisableCode(""), ErrMFAInvalidCode)
	require.NoError(t, ValidateMFADisableCode("123456"))
}

func TestPrepareMFAVerificationDefersRecoveryCodeMutation(t *testing.T) {
	useTestPasswordHasher(t)

	hash, err := bcrypt.GenerateFromPassword([]byte("abcd-efgh"), bcrypt.MinCost)
	require.NoError(t, err)
	row := models.UserMFA{
		Enabled:       true,
		RecoveryCodes: models.JSONSlice{string(hash), "kept-hash"},
	}
	markedUsed := false
	var updatedHashes []string

	commit, err := prepareMFAVerification(row, "abcd-efgh", func() error {
		markedUsed = true
		return nil
	}, func(hashes []string) error {
		updatedHashes = append([]string(nil), hashes...)
		return nil
	})

	require.NoError(t, err)
	require.NotNil(t, commit)
	require.False(t, markedUsed)
	require.Nil(t, updatedHashes)

	require.NoError(t, commit())
	require.False(t, markedUsed)
	require.Equal(t, []string{"kept-hash"}, updatedHashes)
}

func TestPrepareMFAVerificationRejectsInvalidCodeBeforeSensitiveEvidence(t *testing.T) {
	evidenceConsumed := false
	flow := sensitiveMFADisableFlow{
		validateEvidence:    func() error { return nil },
		prepareVerification: func() (func() error, error) { return nil, ErrMFAInvalidCode },
		executeSensitive:    func(func() error) error { evidenceConsumed = true; return nil },
		disable:             func() error { return nil },
	}
	err := flow.Execute()
	require.ErrorIs(t, err, ErrMFAInvalidCode)
	require.False(t, evidenceConsumed)
}

func TestMFASecretStorageEncryptsAndReadsPlainLegacySecret(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	originalEncrypt := mfaSecretEncrypt
	originalDecrypt := mfaSecretDecrypt
	t.Cleanup(func() {
		mfaSecretEncrypt = originalEncrypt
		mfaSecretDecrypt = originalDecrypt
	})
	mfaSecretEncrypt = func(value string) (string, error) {
		return "cipher:" + value, nil
	}
	mfaSecretDecrypt = func(value string) (string, error) {
		return strings.TrimPrefix(value, "cipher:"), nil
	}

	stored, err := encryptMFASecret(secret)
	require.NoError(t, err)
	require.NotEqual(t, secret, stored)

	decoded, err := decryptMFASecret(stored)
	require.NoError(t, err)
	require.Equal(t, secret, decoded)

	legacy, err := decryptMFASecret(secret)
	require.NoError(t, err)
	require.Equal(t, secret, legacy)
}

func TestEncryptedMFASecretCanExceedOldStringLimit(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	originalEncrypt := mfaSecretEncrypt
	t.Cleanup(func() { mfaSecretEncrypt = originalEncrypt })
	mfaSecretEncrypt = func(value string) (string, error) {
		return strings.Repeat("x", 140), nil
	}

	stored, err := encryptMFASecret(secret)

	require.NoError(t, err)
	require.Greater(t, len(stored), 128)
}

func TestProvisioningURIEscapesLabelAndIssuer(t *testing.T) {
	uri := (&MFAService{}).provisioningURI("alice+ops@example.com", "JBSWY3DPEHPK3PXP")

	require.Contains(t, uri, "otpauth://totp/")
	require.Contains(t, uri, "alice+ops@example.com")
	require.Contains(t, uri, "issuer=")
	require.NotContains(t, uri, " ")
}

func TestCacheUint64AcceptsRedisStringValue(t *testing.T) {
	userID, ok := cacheUint64("42")

	require.True(t, ok)
	require.Equal(t, uint64(42), userID)

	_, ok = cacheUint64("")
	require.False(t, ok)

	_, ok = cacheUint64("not-a-number")
	require.False(t, ok)
}

func TestMFAChallengeUserIDDoesNotConsumeToken(t *testing.T) {
	cache := newTestCache()
	originalCache := loginSecurityCache
	t.Cleanup(func() { loginSecurityCache = originalCache })
	loginSecurityCache = func() contractscache.Driver { return cache }
	restore := setSecurityPolicyConfig(t, map[string]any{
		"security.mfa.challenge_minutes": 5,
	})
	defer restore()
	mfa := &MFAService{}

	token, err := mfa.IssueChallenge("tenant:demo", 42)
	require.NoError(t, err)

	userID, err := mfa.ChallengeUserID("tenant:demo", token)
	require.NoError(t, err)
	require.Equal(t, uint64(42), userID)

	userID, err = mfa.ConsumeChallenge("tenant:demo", token)
	require.NoError(t, err)
	require.Equal(t, uint64(42), userID)
	_, err = mfa.ChallengeUserID("tenant:demo", token)
	require.ErrorIs(t, err, ErrMFATokenInvalid)
}
