package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/facades"
	"goravel/app/models"
)

const (
	defaultTOTPStepSeconds   = 30
	defaultTOTPDigits        = 6
	defaultTOTPSkew          = 1
	encryptedMFASecretPrefix = "enc:"
)

var (
	ErrMFAAlreadyEnabled = BusinessError{Message: "MFA 已启用，请先验证后重置"}
	ErrMFAInvalidCode    = BusinessError{Message: "MFA 验证码无效"}
	ErrMFANotEnabled     = BusinessError{Message: "MFA 未启用"}
	ErrMFATokenInvalid   = BusinessError{Message: "MFA Token 无效或已过期"}
	mfaSecretEncrypt     = func(secret string) (string, error) {
		return facades.Crypt().EncryptString(secret)
	}
	mfaSecretDecrypt = func(payload string) (string, error) {
		return facades.Crypt().DecryptString(payload)
	}
)

type MFAService struct {
	ctx        context.Context
	connection string
	table      string
}

type MFASetupResult struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

func NewMFAServiceForTenant(tenant Tenant) *MFAService {
	return &MFAService{connection: TenantConnectionName(tenant), table: "user_mfa"}
}

func NewPlatformMFAService() *MFAService {
	return &MFAService{connection: PlatformConnection(), table: "platform_user_mfa"}
}

func (s *MFAService) WithContext(ctx context.Context) *MFAService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *MFAService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, s.connection)
}

func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "="), nil
}

func MFATOTPEnabled() bool {
	return facades.Config().GetBool("security.mfa.totp_enabled", false)
}

func DecodeTOTPSecret(secret string) ([]byte, error) {
	value := strings.ToUpper(strings.TrimSpace(secret))
	if value == "" {
		return nil, BusinessError{Message: "MFA 密钥不能为空"}
	}
	for len(value)%8 != 0 {
		value += "="
	}
	return base32.StdEncoding.DecodeString(value)
}

func GenerateTOTPCode(secret []byte, now time.Time, stepSeconds, digits int) (string, error) {
	if stepSeconds <= 0 {
		stepSeconds = defaultTOTPStepSeconds
	}
	if digits <= 0 {
		digits = defaultTOTPDigits
	}
	counter := uint64(now.Unix() / int64(stepSeconds))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	if _, err := mac.Write(buf[:]); err != nil {
		return "", err
	}
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)
	mod := uint32(math.Pow10(digits))
	return fmt.Sprintf("%0*d", digits, binCode%mod), nil
}

func VerifyTOTPCode(secret []byte, code string, now time.Time, stepSeconds, digits, skew int) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if stepSeconds <= 0 {
		stepSeconds = defaultTOTPStepSeconds
	}
	if digits <= 0 {
		digits = defaultTOTPDigits
	}
	if skew < 0 {
		skew = defaultTOTPSkew
	}
	for offset := -skew; offset <= skew; offset++ {
		candidate, err := GenerateTOTPCode(secret, now.Add(time.Duration(offset*stepSeconds)*time.Second), stepSeconds, digits)
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func (s *MFAService) Setup(userID uint64, username string) (MFASetupResult, error) {
	if !MFATOTPEnabled() {
		return MFASetupResult{}, BusinessError{Message: "MFA/TOTP 未启用"}
	}
	if err := ValidateMFASetupAllowed(s.Enabled(userID)); err != nil {
		return MFASetupResult{}, err
	}
	secret, err := GenerateTOTPSecret()
	if err != nil {
		return MFASetupResult{}, err
	}
	storedSecret, err := encryptMFASecret(secret)
	if err != nil {
		return MFASetupResult{}, err
	}
	now := time.Now()
	values := map[string]any{
		"user_id":        userID,
		"secret":         storedSecret,
		"enabled":        false,
		"recovery_codes": models.JSONSlice{},
		"updated_at":     now,
	}
	if s.hasUserMFA(userID) {
		if _, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(values); err != nil {
			return MFASetupResult{}, err
		}
	} else {
		row := models.UserMFA{
			UserID:        userID,
			Secret:        storedSecret,
			Enabled:       false,
			RecoveryCodes: models.JSONSlice{},
		}
		if err := s.orm().Query().Table(s.table).Create(&row); err != nil {
			return MFASetupResult{}, err
		}
	}
	return MFASetupResult{Secret: secret, URI: s.provisioningURI(username, secret)}, nil
}

func (s *MFAService) Confirm(userID uint64, code string) ([]string, error) {
	if !MFATOTPEnabled() {
		return nil, BusinessError{Message: "MFA/TOTP 未启用"}
	}
	row, err := s.find(userID)
	if err != nil {
		return nil, err
	}
	if !verifySecretCode(row.Secret, code) {
		return nil, ErrMFAInvalidCode
	}
	count := facades.Config().GetInt("security.mfa.recovery_codes", 8)
	codes, hashes, err := GenerateMFARecoveryCodes(count)
	if err != nil {
		return nil, err
	}
	_, err = s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"enabled":        true,
		"recovery_codes": models.JSONSlice(stringSliceAny(hashes)),
		"confirmed_at":   time.Now(),
		"updated_at":     time.Now(),
	})
	return codes, err
}

func (s *MFAService) Disable(userID uint64) error {
	return s.DisableWithCode(userID, "")
}

func (s *MFAService) DisableWithCode(userID uint64, code string) error {
	if err := ValidateMFADisableCode(code); err != nil {
		return err
	}
	if err := s.Verify(userID, code); err != nil {
		return err
	}
	_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"enabled":        false,
		"recovery_codes": models.JSONSlice{},
		"updated_at":     time.Now(),
	})
	return err
}

func (s *MFAService) DisableSensitive(userID, tenantID uint64, code string, evidence SensitiveOperationEvidence) error {
	guard := NewSensitiveOperationGuard(nil)
	plan, err := guard.PrepareCanonical(s.ctx, "mfa.disable", userID, tenantID, SensitiveOperationPlanSelector{
		Resource: fmt.Sprintf("mfa:user:%d:disable", userID),
	})
	if err != nil {
		return err
	}
	flow := sensitiveMFADisableFlow{
		validateEvidence:    func() error { return guard.Validate(s.ctx, plan, evidence) },
		prepareVerification: func() (func() error, error) { return s.PrepareVerify(userID, code) },
		executeSensitive:    func(mutate func() error) error { return guard.Execute(s.ctx, plan, evidence, mutate) },
		disable: func() error {
			_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Where("enabled", true).Update(map[string]any{
				"enabled": false, "recovery_codes": models.JSONSlice{}, "updated_at": time.Now(),
			})
			return err
		},
	}
	return flow.Execute()
}

type sensitiveMFADisableFlow struct {
	validateEvidence    func() error
	prepareVerification func() (func() error, error)
	executeSensitive    func(func() error) error
	disable             func() error
}

func (flow sensitiveMFADisableFlow) Execute() error {
	if err := flow.validateEvidence(); err != nil {
		return err
	}
	commitVerification, err := flow.prepareVerification()
	if err != nil {
		return err
	}
	return flow.executeSensitive(func() error {
		if err := commitVerification(); err != nil {
			return err
		}
		return flow.disable()
	})
}

func ValidateMFASetupAllowed(enabled bool) error {
	if enabled {
		return ErrMFAAlreadyEnabled
	}
	return nil
}

func ValidateMFADisableCode(code string) error {
	if strings.TrimSpace(code) == "" {
		return ErrMFAInvalidCode
	}
	return nil
}

func (s *MFAService) Enabled(userID uint64) bool {
	if !MFATOTPEnabled() {
		return false
	}
	row, err := s.find(userID)
	return err == nil && row.Enabled
}

func (s *MFAService) Verify(userID uint64, code string) error {
	commit, err := s.PrepareVerify(userID, code)
	if err != nil {
		return err
	}
	return commit()
}

// PrepareVerify validates an MFA code and returns a commit function for successful use.
func (s *MFAService) PrepareVerify(userID uint64, code string) (func() error, error) {
	if !MFATOTPEnabled() {
		return nil, BusinessError{Message: "MFA/TOTP 未启用"}
	}
	row, err := s.find(userID)
	if err != nil {
		return nil, err
	}
	return prepareMFAVerification(
		row,
		code,
		func() error { return s.markUsed(userID) },
		func(hashes []string) error { return s.updateRecoveryCodes(userID, hashes) },
	)
}

func prepareMFAVerification(row models.UserMFA, code string, markUsed func() error, updateRecoveryCodes func([]string) error) (func() error, error) {
	if !row.Enabled {
		return nil, ErrMFANotEnabled
	}
	if verifySecretCode(row.Secret, code) {
		return markUsed, nil
	}
	trimmedCode := strings.TrimSpace(code)
	hashes := jsonSliceStrings(row.RecoveryCodes)
	for i, hash := range hashes {
		if secretHashMatches(hash, trimmedCode) {
			remaining := make([]string, 0, len(hashes)-1)
			remaining = append(remaining, hashes[:i]...)
			remaining = append(remaining, hashes[i+1:]...)
			return func() error { return updateRecoveryCodes(remaining) }, nil
		}
	}
	return nil, ErrMFAInvalidCode
}

func (s *MFAService) IssueChallenge(scope string, userID uint64) (string, error) {
	token := rand.Text()
	err := loginSecurityCache().Put(mfaChallengeKey(scope, token), userID, minutesDuration("security.mfa.challenge_minutes", 5))
	return token, err
}

func (s *MFAService) ConsumeChallenge(scope, token string) (uint64, error) {
	userID, err := s.ChallengeUserID(scope, token)
	if err != nil {
		return 0, err
	}
	s.ForgetChallenge(scope, token)
	return userID, nil
}

func (s *MFAService) ChallengeUserID(scope, token string) (uint64, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, ErrMFATokenInvalid
	}
	key := mfaChallengeKey(scope, token)
	value := loginSecurityCache().Get(key)
	userID, ok := cacheUint64(value)
	if !ok || userID == 0 {
		return 0, ErrMFATokenInvalid
	}
	return userID, nil
}

func (s *MFAService) ForgetChallenge(scope, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	_ = loginSecurityCache().Forget(mfaChallengeKey(scope, token))
}

func GenerateMFARecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 {
		count = 8
	}
	codes := make([]string, 0, count)
	hashes := make([]string, 0, count)
	seen := make(map[string]struct{}, count)
	for len(codes) < count {
		code, err := randomRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		if _, ok := seen[code]; ok {
			continue
		}
		hash, err := makeSecretHash(code)
		if err != nil {
			return nil, nil, err
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
		hashes = append(hashes, hash)
	}
	return codes, hashes, nil
}

func randomRecoveryCode() (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	if len(encoded) > 16 {
		encoded = encoded[:16]
	}
	return encoded[:8] + "-" + encoded[8:], nil
}

func (s *MFAService) hasUserMFA(userID uint64) bool {
	count, err := s.orm().Query().Table(s.table).Where("user_id", userID).Count()
	return err == nil && count > 0
}

func (s *MFAService) find(userID uint64) (models.UserMFA, error) {
	var row models.UserMFA
	err := s.orm().Query().Table(s.table).Where("user_id", userID).First(&row)
	if err != nil {
		return models.UserMFA{}, ErrMFANotEnabled
	}
	return row, nil
}

func (s *MFAService) markUsed(userID uint64) error {
	_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"last_used_at": time.Now(),
		"updated_at":   time.Now(),
	})
	return err
}

func (s *MFAService) updateRecoveryCodes(userID uint64, hashes []string) error {
	_, err := s.orm().Query().Table(s.table).Where("user_id", userID).Update(map[string]any{
		"recovery_codes": models.JSONSlice(stringSliceAny(hashes)),
		"last_used_at":   time.Now(),
		"updated_at":     time.Now(),
	})
	return err
}

func (s *MFAService) provisioningURI(username, secret string) string {
	issuer := facades.Config().GetString("security.mfa.totp_issuer", "Goravel")
	label := issuer + ":" + username
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s", url.PathEscape(label), url.QueryEscape(secret), url.QueryEscape(issuer))
}

func verifySecretCode(secret, code string) bool {
	plainSecret, err := decryptMFASecret(secret)
	if err != nil {
		return false
	}
	raw, err := DecodeTOTPSecret(plainSecret)
	if err != nil {
		return false
	}
	return VerifyTOTPCode(raw, code, time.Now(), defaultTOTPStepSeconds, defaultTOTPDigits, defaultTOTPSkew)
}

func encryptMFASecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", BusinessError{Message: "MFA 密钥不能为空"}
	}
	encrypted, err := mfaSecretEncrypt(secret)
	if err != nil {
		return "", err
	}
	return encryptedMFASecretPrefix + encrypted, nil
}

func decryptMFASecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, encryptedMFASecretPrefix) {
		return secret, nil
	}
	return mfaSecretDecrypt(strings.TrimPrefix(secret, encryptedMFASecretPrefix))
}

func jsonSliceStrings(items models.JSONSlice) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func stringSliceAny(items []string) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
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

func mfaChallengeKey(scope, token string) string {
	return "security:mfa:challenge:" + loginIdentity(scope, token)
}
