package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"

	"goravel/app/facades"
)

const (
	ssoAuthorizationTransactionPrefix     = "sso:authorization:transaction:"
	ssoAuthorizationTransactionLockPrefix = "sso:authorization:transaction:lock:"
	ssoAuthorizationTransactionUsedPrefix = "sso:authorization:transaction:used:"
	ssoAuthorizationVerifiedPrefix        = "sso:authorization:verified:"
	ssoAuthorizationTransactionMaxTTL     = 5 * time.Minute
	ssoAuthorizationTransactionLockTTL    = 30 * time.Second
)

var (
	ErrSSOAuthorizationTransactionInvalid = errors.New("sso authorization transaction is invalid")
	ErrSSOAuthorizationTransactionExpired = errors.New("sso authorization transaction has expired")
	ErrSSOAuthorizationTransactionReused  = errors.New("sso authorization transaction has already been used")
)

var ssoAuthorizationTransactionCache = func() contractscache.Driver {
	return facades.Cache()
}

var ssoAuthorizationTransactionNow = time.Now

type SSOAuthorizationTransaction struct {
	ID           string    `json:"id"`
	TenantCode   string    `json:"tenant_code"`
	Provider     string    `json:"provider"`
	Scene        string    `json:"scene"`
	State        string    `json:"state"`
	Nonce        string    `json:"nonce"`
	CodeVerifier string    `json:"code_verifier"`
	RedirectURI  string    `json:"redirect_uri"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type SSOAuthorizationResult struct {
	TransactionID    string `json:"transaction_id"`
	State            string `json:"state"`
	AuthorizationURL string `json:"authorization_url"`
}

type ssoVerifiedAuthorization struct {
	TenantCode string    `json:"tenant_code"`
	ProviderID uint64    `json:"provider_id"`
	Provider   string    `json:"provider"`
	Scene      string    `json:"scene"`
	Claims     ssoClaims `json:"claims"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type SSOAuthorizationTransactionService struct{}

func NewSSOAuthorizationTransactionService() *SSOAuthorizationTransactionService {
	return &SSOAuthorizationTransactionService{}
}

func (s *SSOAuthorizationTransactionService) Create(tenant Tenant, provider SSOProvider) (SSOAuthorizationResult, error) {
	provider = withSSODiscoveryDefaults(provider)
	if tenant.ID == 0 || strings.TrimSpace(tenant.Code) == "" ||
		strings.TrimSpace(provider.Name) == "" || strings.TrimSpace(provider.AuthorizationEndpoint) == "" ||
		strings.TrimSpace(provider.ClientID) == "" || strings.TrimSpace(provider.RedirectURI) == "" {
		return SSOAuthorizationResult{}, ErrSSOAuthorizationTransactionInvalid
	}

	id, err := newSSOAuthorizationRandomValue(24)
	if err != nil {
		return SSOAuthorizationResult{}, err
	}
	state, err := newSSOAuthorizationRandomValue(32)
	if err != nil {
		return SSOAuthorizationResult{}, err
	}
	transaction := SSOAuthorizationTransaction{
		ID:          id,
		TenantCode:  tenant.Code,
		Provider:    provider.Name,
		Scene:       normalizeSSOScene(provider.Scene),
		State:       state,
		RedirectURI: provider.RedirectURI,
		ExpiresAt:   ssoAuthorizationTransactionNow().Add(ssoAuthorizationTransactionMaxTTL),
	}
	if provider.EnableNonce || provider.Type == "oidc" {
		transaction.Nonce, err = newSSOAuthorizationRandomValue(32)
		if err != nil {
			return SSOAuthorizationResult{}, err
		}
	}
	if provider.EnablePKCE || provider.Type == "oidc" {
		transaction.CodeVerifier, err = newSSOAuthorizationRandomValue(48)
		if err != nil {
			return SSOAuthorizationResult{}, err
		}
	}
	if err := s.Store(transaction); err != nil {
		return SSOAuthorizationResult{}, err
	}

	authorizationURL, err := s.authorizationURL(provider, transaction)
	if err != nil {
		_ = ssoAuthorizationTransactionCache().Forget(ssoAuthorizationTransactionKey(transaction.ID))
		return SSOAuthorizationResult{}, err
	}
	return SSOAuthorizationResult{
		TransactionID:    transaction.ID,
		State:            transaction.State,
		AuthorizationURL: authorizationURL,
	}, nil
}

func (s *SSOAuthorizationTransactionService) Store(transaction SSOAuthorizationTransaction) error {
	transaction = normalizeSSOAuthorizationTransaction(transaction)
	if !validSSOAuthorizationTransaction(transaction) {
		return ErrSSOAuthorizationTransactionInvalid
	}
	ttl := time.Until(transaction.ExpiresAt)
	if ttl <= 0 || ttl > ssoAuthorizationTransactionMaxTTL {
		return ErrSSOAuthorizationTransactionExpired
	}
	raw, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	return ssoAuthorizationTransactionCache().Put(ssoAuthorizationTransactionKey(transaction.ID), string(raw), ttl)
}

func (s *SSOAuthorizationTransactionService) Load(tenant Tenant, transactionID string) (SSOAuthorizationTransaction, error) {
	transactionID = strings.TrimSpace(transactionID)
	if tenant.ID == 0 || strings.TrimSpace(tenant.Code) == "" || transactionID == "" {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	raw := ssoAuthorizationTransactionCache().GetString(ssoAuthorizationTransactionKey(transactionID))
	transaction, err := parseSSOAuthorizationTransaction(raw)
	if err != nil {
		return SSOAuthorizationTransaction{}, err
	}
	if transaction.TenantCode != tenant.Code {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	return transaction, nil
}

func (s *SSOAuthorizationTransactionService) ValidateCallback(tenant Tenant, transactionID, state string) (SSOAuthorizationTransaction, error) {
	transaction, err := s.Load(tenant, transactionID)
	if err != nil {
		return SSOAuthorizationTransaction{}, err
	}
	if !secureSSOAuthorizationEqual(transaction.State, strings.TrimSpace(state)) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	return transaction, nil
}

func (s *SSOAuthorizationTransactionService) LoadVerified(tenant Tenant, transaction SSOAuthorizationTransaction) (ssoVerifiedAuthorization, bool) {
	raw := ssoAuthorizationTransactionCache().GetString(ssoAuthorizationVerifiedKey(transaction.ID))
	verified := ssoVerifiedAuthorization{}
	if json.Unmarshal([]byte(raw), &verified) != nil || verified.TenantCode != tenant.Code ||
		verified.Provider != transaction.Provider || verified.Scene != transaction.Scene ||
		verified.ProviderID == 0 || !ssoAuthorizationTransactionNow().Before(verified.ExpiresAt) {
		return ssoVerifiedAuthorization{}, false
	}
	return verified, true
}

func (s *SSOAuthorizationTransactionService) StoreVerified(transaction SSOAuthorizationTransaction, verified ssoVerifiedAuthorization) error {
	verified.ExpiresAt = transaction.ExpiresAt
	raw, err := json.Marshal(verified)
	if err != nil {
		return err
	}
	ttl := time.Until(transaction.ExpiresAt)
	if ttl <= 0 {
		return ErrSSOAuthorizationTransactionExpired
	}
	return ssoAuthorizationTransactionCache().Put(ssoAuthorizationVerifiedKey(transaction.ID), string(raw), ttl)
}

func (s *SSOAuthorizationTransactionService) ForgetVerified(transactionID string) {
	_ = ssoAuthorizationTransactionCache().Forget(ssoAuthorizationVerifiedKey(transactionID))
}

func (s *SSOAuthorizationTransactionService) VerifyAndConsumeCallback(
	tenant Tenant,
	transactionID, state string,
	verify func(SSOAuthorizationTransaction) error,
) (SSOAuthorizationTransaction, error) {
	transactionID = strings.TrimSpace(transactionID)
	if tenant.ID == 0 || strings.TrimSpace(tenant.Code) == "" || transactionID == "" || verify == nil {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	cache := ssoAuthorizationTransactionCache()
	lock := cache.Lock(ssoAuthorizationTransactionLockKey(transactionID), ssoAuthorizationTransactionLockTTL)
	if !lock.Get() {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionReused
	}
	defer lock.Release()
	transaction, err := s.Load(tenant, transactionID)
	if err != nil {
		if errors.Is(err, ErrSSOAuthorizationTransactionInvalid) {
			if cache.Has(ssoAuthorizationTransactionUsedKey(transactionID)) {
				return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionReused
			}
		}
		return SSOAuthorizationTransaction{}, err
	}
	if !secureSSOAuthorizationEqual(transaction.State, strings.TrimSpace(state)) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	if err := verify(transaction); err != nil {
		return SSOAuthorizationTransaction{}, err
	}
	if !cache.Forget(ssoAuthorizationTransactionKey(transaction.ID)) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionReused
	}
	usedTTL := time.Until(transaction.ExpiresAt)
	if usedTTL > 0 {
		_ = cache.Put(ssoAuthorizationTransactionUsedKey(transaction.ID), "1", usedTTL)
	}
	return transaction, nil
}

func (s *SSOAuthorizationTransactionService) authorizationURL(provider SSOProvider, transaction SSOAuthorizationTransaction) (string, error) {
	endpoint, err := ssoEndpointURL(provider.AuthorizationEndpoint)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", provider.ClientID)
	query.Set("redirect_uri", transaction.RedirectURI)
	query.Set("state", transaction.State)
	if scope := strings.TrimSpace(provider.Scope); scope != "" {
		query.Set("scope", scope)
	}
	if provider.EnableNonce || provider.Type == "oidc" {
		query.Set("nonce", transaction.Nonce)
	}
	if provider.EnablePKCE || provider.Type == "oidc" {
		query.Set("code_challenge", ssoPKCEChallenge(transaction.CodeVerifier))
		query.Set("code_challenge_method", "S256")
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func parseSSOAuthorizationTransaction(raw string) (SSOAuthorizationTransaction, error) {
	transaction := SSOAuthorizationTransaction{}
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &transaction) != nil {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	transaction = normalizeSSOAuthorizationTransaction(transaction)
	if !validSSOAuthorizationTransaction(transaction) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionInvalid
	}
	if !ssoAuthorizationTransactionNow().Before(transaction.ExpiresAt) {
		return SSOAuthorizationTransaction{}, ErrSSOAuthorizationTransactionExpired
	}
	return transaction, nil
}

func normalizeSSOAuthorizationTransaction(transaction SSOAuthorizationTransaction) SSOAuthorizationTransaction {
	transaction.ID = strings.TrimSpace(transaction.ID)
	transaction.TenantCode = strings.TrimSpace(transaction.TenantCode)
	transaction.Provider = strings.TrimSpace(transaction.Provider)
	transaction.Scene = normalizeSSOScene(transaction.Scene)
	transaction.State = strings.TrimSpace(transaction.State)
	transaction.Nonce = strings.TrimSpace(transaction.Nonce)
	transaction.CodeVerifier = strings.TrimSpace(transaction.CodeVerifier)
	transaction.RedirectURI = strings.TrimSpace(transaction.RedirectURI)
	return transaction
}

func validSSOAuthorizationTransaction(transaction SSOAuthorizationTransaction) bool {
	return transaction.ID != "" && transaction.TenantCode != "" && transaction.Provider != "" &&
		transaction.State != "" && transaction.RedirectURI != "" && !transaction.ExpiresAt.IsZero()
}

func newSSOAuthorizationRandomValue(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ssoPKCEChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func secureSSOAuthorizationEqual(expected, actual string) bool {
	if expected == "" || actual == "" || len(expected) != len(actual) {
		return false
	}
	var different byte
	for index := range expected {
		different |= expected[index] ^ actual[index]
	}
	return different == 0
}

func ssoAuthorizationTransactionKey(id string) string {
	return ssoAuthorizationTransactionPrefix + id
}

func ssoAuthorizationVerifiedKey(id string) string {
	return ssoAuthorizationVerifiedPrefix + id
}

func ssoAuthorizationTransactionLockKey(id string) string {
	return ssoAuthorizationTransactionLockPrefix + id
}

func ssoAuthorizationTransactionUsedKey(id string) string {
	return ssoAuthorizationTransactionUsedPrefix + id
}
