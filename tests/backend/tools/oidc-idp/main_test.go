package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	testClientID    = "goravel-oidc-e2e"
	testRedirectURI = "http://127.0.0.1:2889/#/login"
)

func TestAuthorizationCodeCanOnlyBeExchangedOnce(t *testing.T) {
	idp, server := newTestIDP(t)
	code := authorizeCode(t, server.URL, "nonce-1", "verifier-1")

	first := exchangeCode(t, server.URL, code, "verifier-1")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first token response status = %d, want %d", first.StatusCode, http.StatusOK)
	}
	_ = first.Body.Close()

	second := exchangeCode(t, server.URL, code, "verifier-1")
	if second.StatusCode != http.StatusBadRequest {
		t.Fatalf("reused token response status = %d, want %d", second.StatusCode, http.StatusBadRequest)
	}
	defer second.Body.Close()
	assertOAuthError(t, second.Body, "invalid_grant")

	events := idp.Events()
	if len(events) < 3 {
		t.Fatalf("events = %d, want authorization and two token attempts", len(events))
	}
}

func TestTokenRejectsMissingOrInvalidS256Verifier(t *testing.T) {
	_, server := newTestIDP(t)
	code := authorizeCode(t, server.URL, "nonce-1", "expected-verifier")

	missing := exchangeCode(t, server.URL, code, "")
	if missing.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing verifier status = %d, want %d", missing.StatusCode, http.StatusBadRequest)
	}
	assertOAuthError(t, missing.Body, "invalid_grant")
	_ = missing.Body.Close()

	wrong := exchangeCode(t, server.URL, code, "wrong-verifier")
	if wrong.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong verifier status = %d, want %d", wrong.StatusCode, http.StatusBadRequest)
	}
	assertOAuthError(t, wrong.Body, "invalid_grant")
	_ = wrong.Body.Close()

	valid := exchangeCode(t, server.URL, code, "expected-verifier")
	defer valid.Body.Close()
	if valid.StatusCode != http.StatusOK {
		t.Fatalf("valid verifier status = %d, want %d", valid.StatusCode, http.StatusOK)
	}
}

func TestJWKSRotationPublishesANewSigningKey(t *testing.T) {
	idp, server := newTestIDP(t)
	before := jwksKeyIDs(t, server.URL)
	if len(before) != 1 {
		t.Fatalf("initial key IDs = %v, want one key", before)
	}

	rotated := postControl(t, server.URL, "/control/rotate", nil)
	if rotated.StatusCode != http.StatusOK {
		t.Fatalf("rotation status = %d, want %d", rotated.StatusCode, http.StatusOK)
	}
	_ = rotated.Body.Close()

	after := jwksKeyIDs(t, server.URL)
	if len(after) != 1 || after[0] == before[0] {
		t.Fatalf("rotated key IDs = %v, want one different key from %v", after, before)
	}
	if ids := idp.KeyIDs(); len(ids) != 1 || ids[0] != after[0] {
		t.Fatalf("server key IDs = %v, want %v", ids, after)
	}
}

func TestControlEndpointsRequireTestMode(t *testing.T) {
	idp, err := NewServer(Config{
		ClientID:     testClientID,
		RedirectURIs: []string{testRedirectURI},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	server := httptest.NewServer(idp.Handler())
	defer server.Close()
	idp.SetIssuer(server.URL)

	res, err := http.Post(server.URL+"/control/rotate", "application/json", nil)
	if err != nil {
		t.Fatalf("POST control endpoint error = %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("control endpoint status = %d, want %d", res.StatusCode, http.StatusNotFound)
	}
}

func TestReplayControlReturnsTheLatestAuthorizationCode(t *testing.T) {
	_, server := newTestIDP(t)
	first := authorizeCode(t, server.URL, "nonce-1", "verifier-1")
	response := postControl(t, server.URL, "/control/fault", FaultControl{ReplayCode: true})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("replay control status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	_ = response.Body.Close()

	replayed := authorizeCode(t, server.URL, "nonce-2", "verifier-2")
	if replayed != first {
		t.Fatalf("replayed authorization code = %q, want %q", replayed, first)
	}
}

func TestTokenFaultControlsResetWithScenarioState(t *testing.T) {
	_, server := newTestIDP(t)
	response := postControl(t, server.URL, "/control/fault", FaultControl{
		NonceMismatch: true,
		TokenStatus:   http.StatusInternalServerError,
		TokenDelayMS:  20,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("fault control status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	_ = response.Body.Close()

	code := authorizeCode(t, server.URL, "nonce-1", "verifier-1")
	start := time.Now()
	faulted := exchangeCode(t, server.URL, code, "verifier-1")
	if faulted.StatusCode != http.StatusInternalServerError {
		t.Fatalf("faulted token status = %d, want %d", faulted.StatusCode, http.StatusInternalServerError)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("token delay control was not applied")
	}
	_ = faulted.Body.Close()

	failedExchange := exchangeCode(t, server.URL, code, "verifier-1")
	if failedExchange.StatusCode != http.StatusInternalServerError {
		t.Fatalf("retry after token fault status = %d, want %d", failedExchange.StatusCode, http.StatusInternalServerError)
	}
	_ = failedExchange.Body.Close()

	reset := postControl(t, server.URL, "/control/reset", nil)
	if reset.StatusCode != http.StatusOK {
		t.Fatalf("reset status = %d, want %d", reset.StatusCode, http.StatusOK)
	}
	_ = reset.Body.Close()

	freshCode := authorizeCode(t, server.URL, "nonce-2", "verifier-2")
	valid := exchangeCode(t, server.URL, freshCode, "verifier-2")
	defer valid.Body.Close()
	if valid.StatusCode != http.StatusOK {
		t.Fatalf("reset token status = %d, want %d", valid.StatusCode, http.StatusOK)
	}
}

func newTestIDP(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	idp, err := NewServer(Config{
		ClientID:     testClientID,
		RedirectURIs: []string{testRedirectURI},
		TestMode:     true,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	server := httptest.NewServer(idp.Handler())
	t.Cleanup(server.Close)
	idp.SetIssuer(server.URL)
	return idp, server
}

func authorizeCode(t *testing.T, baseURL, nonce, verifier string) string {
	t.Helper()
	endpoint, err := url.Parse(baseURL + "/authorize")
	if err != nil {
		t.Fatalf("parse authorize endpoint: %v", err)
	}
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", testClientID)
	query.Set("redirect_uri", testRedirectURI)
	query.Set("scope", "openid profile email")
	query.Set("state", "state-1")
	query.Set("nonce", nonce)
	query.Set("code_challenge", S256Challenge(verifier))
	query.Set("code_challenge_method", "S256")
	endpoint.RawQuery = query.Encode()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(endpoint.String())
	if err != nil {
		t.Fatalf("authorize request error = %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("authorize status = %d, want %d: %s", res.StatusCode, http.StatusFound, body)
	}
	callback, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize redirect: %v", err)
	}
	code := callback.Query().Get("code")
	if code == "" {
		t.Fatalf("authorization redirect has no code: %s", callback.String())
	}
	return code
}

func exchangeCode(t *testing.T, baseURL, code, verifier string) *http.Response {
	t.Helper()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {testClientID},
		"redirect_uri":  {testRedirectURI},
		"code":          {code},
		"code_verifier": {verifier},
	}
	res, err := http.Post(baseURL+"/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("token request error = %v", err)
	}
	return res
}

func jwksKeyIDs(t *testing.T, baseURL string) []string {
	t.Helper()
	res, err := http.Get(baseURL + "/jwks")
	if err != nil {
		t.Fatalf("JWKS request error = %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("JWKS status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	var document struct {
		Keys []struct {
			KID string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(res.Body).Decode(&document); err != nil {
		t.Fatalf("decode JWKS: %v", err)
	}
	ids := make([]string, 0, len(document.Keys))
	for _, key := range document.Keys {
		ids = append(ids, key.KID)
	}
	return ids
}

func postControl(t *testing.T, baseURL, path string, payload any) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode control payload: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	res, err := http.Post(baseURL+path, "application/json", body)
	if err != nil {
		t.Fatalf("POST control request error = %v", err)
	}
	return res
}

func assertOAuthError(t *testing.T, body io.Reader, expected string) {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		t.Fatalf("decode OAuth error: %v", err)
	}
	if payload.Error != expected {
		t.Fatalf("OAuth error = %q, want %q", payload.Error, expected)
	}
}
