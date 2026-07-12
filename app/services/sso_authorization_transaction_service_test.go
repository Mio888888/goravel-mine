package services

import (
	"testing"
	"time"

	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"
)

func TestSSOAuthorizationTransactionCreatesServerOwnedOIDCRequest(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	defer AllowLoopbackSSOEndpointsForTesting()()
	service := NewSSOAuthorizationTransactionService()
	provider := SSOProvider{
		Name:                  "oidc",
		Scene:                 "admin",
		Type:                  "oidc",
		AuthorizationEndpoint: "http://127.0.0.1:8080/authorize",
		ClientID:              "client-id",
		RedirectURI:           "https://console.example.test/login",
		Scope:                 "openid profile email",
		EnableNonce:           true,
	}

	result, err := service.Create(Tenant{ID: 7, Code: "acme"}, provider)

	require.NoError(t, err)
	require.NotEmpty(t, result.TransactionID)
	require.NotEmpty(t, result.State)
	require.NotEmpty(t, result.AuthorizationURL)
	require.NotContains(t, result.AuthorizationURL, "code_verifier")
	require.Contains(t, result.AuthorizationURL, "code_challenge=")

	transaction, err := service.Load(Tenant{ID: 7, Code: "acme"}, result.TransactionID)
	require.NoError(t, err)
	require.Equal(t, provider.Name, transaction.Provider)
	require.Equal(t, provider.Scene, transaction.Scene)
	require.Equal(t, provider.RedirectURI, transaction.RedirectURI)
	require.Equal(t, result.State, transaction.State)
	require.NotEmpty(t, transaction.Nonce)
	require.NotEmpty(t, transaction.CodeVerifier)
	require.LessOrEqual(t, time.Until(transaction.ExpiresAt), ssoAuthorizationTransactionMaxTTL)
}

func TestSSOAuthorizationTransactionRejectsStateMismatch(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID:          "transaction-id",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "expected-state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))

	_, err := service.ValidateCallback(Tenant{ID: 7, Code: "acme"}, "transaction-id", "wrong-state")

	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionInvalid)
}

func TestSSOAuthorizationTransactionRejectsExpiredTransaction(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	now := time.Now()
	originalNow := ssoAuthorizationTransactionNow
	ssoAuthorizationTransactionNow = func() time.Time { return now }
	t.Cleanup(func() { ssoAuthorizationTransactionNow = originalNow })
	transaction := SSOAuthorizationTransaction{
		ID:          "expired-transaction",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   now.Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))
	now = now.Add(2 * time.Minute)

	_, err := service.ValidateCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State)

	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionExpired)
}

func TestSSOAuthorizationTransactionConsumesCallbackOnlyOnce(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID:          "single-use-transaction",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))

	_, err := service.VerifyAndConsumeCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State, func(SSOAuthorizationTransaction) error {
		return nil
	})
	require.NoError(t, err)

	_, err = service.ValidateCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State)
	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionInvalid)
	_, err = service.VerifyAndConsumeCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State, func(SSOAuthorizationTransaction) error {
		return nil
	})
	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionReused)
}

func TestSSOAuthorizationTransactionRetainsFailedCallbackForRetry(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID:          "retry-transaction",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))

	_, err := service.VerifyAndConsumeCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State, func(SSOAuthorizationTransaction) error {
		return ErrSSOTokenInvalid
	})
	require.ErrorIs(t, err, ErrSSOTokenInvalid)

	loaded, err := service.Load(Tenant{ID: 7, Code: "acme"}, transaction.ID)
	require.NoError(t, err)
	require.Equal(t, transaction.ID, loaded.ID)
}

func TestSSOAuthorizationTransactionRetainsVerifiedClaimsForLoginRetry(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID: "verified-retry", TenantCode: "acme", Provider: "oidc", Scene: "admin",
		State: "state", RedirectURI: "https://console.example.test/login", ExpiresAt: time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))
	verified := ssoVerifiedAuthorization{
		TenantCode: "acme", ProviderID: 9, Provider: "oidc", Scene: "admin",
		Claims: ssoClaims{Subject: "subject-1", Email: "user@example.test"},
	}
	require.NoError(t, service.StoreVerified(transaction, verified))

	loaded, ok := service.LoadVerified(Tenant{ID: 7, Code: "acme"}, transaction)
	require.True(t, ok)
	require.Equal(t, verified.ProviderID, loaded.ProviderID)
	require.Equal(t, verified.Claims.Subject, loaded.Claims.Subject)

	service.ForgetVerified(transaction.ID)
	_, ok = service.LoadVerified(Tenant{ID: 7, Code: "acme"}, transaction)
	require.False(t, ok)
}

func TestSSOAuthorizationTransactionConsumesOnlyAfterSuccessfulCallback(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID:          "successful-transaction",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))

	_, err := service.VerifyAndConsumeCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State, func(SSOAuthorizationTransaction) error {
		return nil
	})
	require.NoError(t, err)

	_, err = service.Load(Tenant{ID: 7, Code: "acme"}, transaction.ID)
	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionInvalid)
}

func TestSSOAuthorizationTransactionConsumesBeforeLoginCompletion(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID:          "verified-transaction",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))

	_, err := service.VerifyAndConsumeCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State, func(SSOAuthorizationTransaction) error {
		return nil
	})
	require.NoError(t, err)

	_, err = service.Load(Tenant{ID: 7, Code: "acme"}, transaction.ID)
	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionInvalid)
}

func TestSSOAuthorizationTransactionRejectsConcurrentCallback(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID:          "locked-transaction",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))

	lock := ssoAuthorizationTransactionCache().Lock(ssoAuthorizationTransactionLockKey(transaction.ID), time.Minute)
	require.True(t, lock.Get())
	defer lock.Release()

	_, err := service.VerifyAndConsumeCallback(Tenant{ID: 7, Code: "acme"}, transaction.ID, transaction.State, func(SSOAuthorizationTransaction) error {
		return nil
	})

	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionReused)
}

func TestSSOAuthorizationTransactionLoadsOnlyForItsTenant(t *testing.T) {
	useSSOAuthorizationTransactionCache(t)
	service := NewSSOAuthorizationTransactionService()
	transaction := SSOAuthorizationTransaction{
		ID:          "tenant-bound-transaction",
		TenantCode:  "acme",
		Provider:    "oidc",
		Scene:       "admin",
		State:       "state",
		RedirectURI: "https://console.example.test/login",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, service.Store(transaction))

	_, err := service.Load(Tenant{ID: 8, Code: "other"}, transaction.ID)

	require.ErrorIs(t, err, ErrSSOAuthorizationTransactionInvalid)
}

func useSSOAuthorizationTransactionCache(t *testing.T) {
	t.Helper()
	cache := newTestCache()
	original := ssoAuthorizationTransactionCache
	ssoAuthorizationTransactionCache = func() contractscache.Driver { return cache }
	t.Cleanup(func() { ssoAuthorizationTransactionCache = original })
}
