package application

import (
	"github.com/golang-jwt/jwt/v5"
	contractscache "github.com/goravel/framework/contracts/cache"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// Source: sso_authorization_transaction_service_test.go
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

// Source: sso_protocol_service_test.go
func TestSSOEndpointURLRejectsPrivateAndLocalTargets(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/.well-known/openid-configuration",
		"http://10.0.0.1/token",
		"http://172.16.0.1/token",
		"http://192.168.1.10/userinfo",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/jwks",
		"http://localhost/token",
		"file:///etc/passwd",
	}

	for _, uri := range cases {
		_, err := ssoEndpointURL(uri)
		require.ErrorIs(t, err, ErrSSOTokenInvalid, uri)
	}
}

func TestFetchURLAllowsLoopbackOnlyForTestServers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	t.Cleanup(func() { ssoAllowLoopbackEndpoints = false })
	ssoAllowLoopbackEndpoints = true

	body, err := fetchURL(server.URL)

	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(body))
}

func TestSSOEndpointURLAllowsLoopbackOnlyWithTestingEnvironmentGate(t *testing.T) {
	t.Setenv("APP_ENV", "testing")
	t.Setenv("SSO_TEST_ALLOW_LOOPBACK", "true")

	endpoint, err := ssoEndpointURL("http://127.0.0.1:19090/authorize")

	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:19090", endpoint.Host)
}

func TestFetchURLRejectsRedirectToPrivateTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer server.Close()
	t.Cleanup(func() { ssoAllowLoopbackEndpoints = false })
	ssoAllowLoopbackEndpoints = true

	_, err := fetchURL(server.URL)

	require.ErrorIs(t, err, ErrSSOTokenInvalid)
}

func TestVerifyIDTokenRequiresNonceWhenEnabled(t *testing.T) {
	tokenText, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "external-user",
		"nonce": "expected-nonce",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("secret"))
	require.NoError(t, err)

	provider := SSOProvider{
		Type:        "oidc",
		JWTSecret:   "secret",
		EnableNonce: true,
	}

	_, err = verifyIDToken(provider, SSOLoginPayload{}, tokenText)
	require.ErrorIs(t, err, ErrSSOTokenInvalid)

	_, err = verifyIDToken(provider, SSOLoginPayload{Nonce: "wrong-nonce"}, tokenText)
	require.ErrorIs(t, err, ErrSSOTokenInvalid)

	claims, err := verifyIDToken(provider, SSOLoginPayload{Nonce: "expected-nonce"}, tokenText)
	require.NoError(t, err)
	require.Equal(t, "external-user", claims.Subject)
}

func TestExchangeOAuthCodeRejectsMissingPKCEVerifier(t *testing.T) {
	provider := SSOProvider{
		TokenEndpoint: "https://idp.example.test/token",
		ClientID:      "client-id",
		EnablePKCE:    true,
	}

	_, err := exchangeOAuthCode(provider, SSOLoginPayload{Code: "authorization-code"})

	require.ErrorIs(t, err, ErrSSOTokenInvalid)
}

func TestExchangeOAuthCodeRejectsRedirectMismatch(t *testing.T) {
	provider := SSOProvider{
		TokenEndpoint: "https://idp.example.test/token",
		ClientID:      "client-id",
		RedirectURI:   "https://console.example.test/login",
	}

	_, err := exchangeOAuthCode(provider, SSOLoginPayload{
		Code:        "authorization-code",
		RedirectURI: "https://attacker.example.test/callback",
	})

	require.ErrorIs(t, err, ErrSSOTokenInvalid)
}

func TestExchangeOAuthCodeUsesConfiguredRedirectURI(t *testing.T) {
	defer AllowLoopbackSSOEndpointsForTesting()()
	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		form = r.PostForm
		_, _ = io.WriteString(w, `{"access_token":"access-token"}`)
	}))
	defer server.Close()

	provider := SSOProvider{
		TokenEndpoint: server.URL + "/token",
		ClientID:      "client-id",
		RedirectURI:   "https://console.example.test/login",
	}

	_, err := exchangeOAuthCode(provider, SSOLoginPayload{Code: "authorization-code"})

	require.NoError(t, err)
	require.Equal(t, "https://console.example.test/login", form.Get("redirect_uri"))
}

func TestExchangeOAuthCodeRequiresOIDCPKCEVerifierEvenWhenProviderFlagIsDisabled(t *testing.T) {
	provider := SSOProvider{
		Type:          "oidc",
		TokenEndpoint: "https://idp.example.test/token",
		ClientID:      "client-id",
		RedirectURI:   "https://console.example.test/login",
	}

	_, err := exchangeOAuthCode(provider, SSOLoginPayload{Code: "authorization-code"})

	require.ErrorIs(t, err, ErrSSOTokenInvalid)
}

func TestSSORotationMetadataSetOnlyWhenSecretProvided(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	provider := withSSORotationMetadata(SSOProvider{JWTSecret: "jwt", ClientSecret: "client"}, now)
	require.Equal(t, now, provider.JWTSecretRotatedAt)
	require.Equal(t, now, provider.ClientSecretRotatedAt)

	provider = withSSORotationMetadata(SSOProvider{Name: "demo"}, now)
	require.True(t, provider.JWTSecretRotatedAt.IsZero())
	require.True(t, provider.ClientSecretRotatedAt.IsZero())
}
