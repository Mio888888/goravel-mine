package services

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

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
