package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultListenAddress = "127.0.0.1:19090"
	defaultClientID      = "goravel-oidc-e2e"
	defaultRedirectURI   = "http://127.0.0.1:2889/#/login"
	codeLifetime         = 2 * time.Minute
)

type Config struct {
	Issuer       string
	ClientID     string
	RedirectURIs []string
	TestMode     bool
	EventLog     string
}

type FaultControl struct {
	WrongState    bool `json:"wrong_state"`
	NonceMismatch bool `json:"nonce_mismatch"`
	ReplayCode    bool `json:"replay_code"`
	TokenStatus   int  `json:"token_status"`
	TokenDelayMS  int  `json:"token_delay_ms"`
}

type Event struct {
	At       time.Time      `json:"at"`
	Kind     string         `json:"kind"`
	Code     string         `json:"code,omitempty"`
	KID      string         `json:"kid,omitempty"`
	Outcome  string         `json:"outcome"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type authorizationCode struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Nonce         string
	IssuedAt      time.Time
	Used          bool
}

type signingKey struct {
	KID     string
	Private *rsa.PrivateKey
}

type Server struct {
	mu           sync.Mutex
	issuer       string
	clientID     string
	redirectURIs map[string]struct{}
	testMode     bool
	eventLog     string
	codes        map[string]authorizationCode
	lastCode     string
	key          signingKey
	fault        FaultControl
	events       []Event
}

func NewServer(config Config) (*Server, error) {
	clientID := strings.TrimSpace(config.ClientID)
	if clientID == "" {
		clientID = defaultClientID
	}
	redirectURIs := make(map[string]struct{}, len(config.RedirectURIs))
	for _, redirectURI := range config.RedirectURIs {
		redirectURI = strings.TrimSpace(redirectURI)
		if redirectURI != "" {
			redirectURIs[redirectURI] = struct{}{}
		}
	}
	if len(redirectURIs) == 0 {
		return nil, errors.New("OIDC test IdP requires at least one redirect URI")
	}
	key, err := newSigningKey()
	if err != nil {
		return nil, err
	}
	return &Server{
		issuer:       strings.TrimRight(strings.TrimSpace(config.Issuer), "/"),
		clientID:     clientID,
		redirectURIs: redirectURIs,
		testMode:     config.TestMode,
		eventLog:     strings.TrimSpace(config.EventLog),
		codes:        make(map[string]authorizationCode),
		key:          key,
		events:       make([]Event, 0),
	}, nil
}

func (s *Server) SetIssuer(issuer string) {
	s.mu.Lock()
	s.issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	s.mu.Unlock()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.handleDiscovery)
	mux.HandleFunc("/authorize", s.handleAuthorize)
	mux.HandleFunc("/token", s.handleToken)
	mux.HandleFunc("/jwks", s.handleJWKS)
	if s.testMode {
		mux.HandleFunc("/control/reset", s.handleReset)
		mux.HandleFunc("/control/rotate", s.handleRotate)
		mux.HandleFunc("/control/fault", s.handleFault)
		mux.HandleFunc("/control/events", s.handleEvents)
	}
	return mux
}

func (s *Server) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...)
}

func (s *Server) KeyIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []string{s.key.KID}
}

func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	issuer := s.currentIssuer(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/authorize",
		"token_endpoint":                        issuer + "/token",
		"jwks_uri":                              issuer + "/jwks",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	redirectURI := strings.TrimSpace(query.Get("redirect_uri"))
	if query.Get("response_type") != "code" || query.Get("client_id") != s.clientID ||
		!s.hasRedirectURI(redirectURI) || query.Get("code_challenge_method") != "S256" ||
		strings.TrimSpace(query.Get("code_challenge")) == "" || strings.TrimSpace(query.Get("state")) == "" {
		s.record("authorize", "", "rejected", nil)
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	s.mu.Lock()
	fault := s.fault
	code := s.lastCode
	if !fault.ReplayCode || code == "" {
		var err error
		code, err = randomValue(32)
		if err == nil {
			s.codes[code] = authorizationCode{
				ClientID:      s.clientID,
				RedirectURI:   redirectURI,
				CodeChallenge: query.Get("code_challenge"),
				Nonce:         query.Get("nonce"),
				IssuedAt:      time.Now().UTC(),
			}
			s.lastCode = code
		}
		if err != nil {
			s.mu.Unlock()
			s.record("authorize", "", "error", nil)
			writeOAuthError(w, http.StatusInternalServerError, "server_error")
			return
		}
	}
	s.mu.Unlock()

	state := query.Get("state")
	if fault.WrongState {
		state = "wrong-" + state
	}
	callback, _ := url.Parse(redirectURI)
	callbackQuery := callback.Query()
	callbackQuery.Set("code", code)
	callbackQuery.Set("state", state)
	callback.RawQuery = callbackQuery.Encode()
	s.record("authorize", code, "issued", map[string]any{"state": state})
	http.Redirect(w, r, callback.String(), http.StatusFound)
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.record("token", "", "rejected", nil)
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	codeValue := strings.TrimSpace(r.PostForm.Get("code"))
	if r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("client_id") != s.clientID {
		s.record("token", codeValue, "rejected", nil)
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	s.mu.Lock()
	code, exists := s.codes[codeValue]
	fault := s.fault
	if fault.TokenStatus >= http.StatusInternalServerError {
		s.mu.Unlock()
		if fault.TokenDelayMS > 0 {
			time.Sleep(time.Duration(fault.TokenDelayMS) * time.Millisecond)
		}
		s.record("token", codeValue, "server_error", nil)
		writeOAuthError(w, fault.TokenStatus, "server_error")
		return
	}
	if !exists || code.Used || time.Since(code.IssuedAt) > codeLifetime ||
		code.RedirectURI != r.PostForm.Get("redirect_uri") ||
		code.CodeChallenge != S256Challenge(r.PostForm.Get("code_verifier")) {
		s.mu.Unlock()
		s.record("token", codeValue, "rejected", nil)
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	code.Used = true
	s.codes[codeValue] = code
	key := s.key
	issuer := s.issuer
	s.mu.Unlock()

	if fault.TokenDelayMS > 0 {
		time.Sleep(time.Duration(fault.TokenDelayMS) * time.Millisecond)
	}
	nonce := code.Nonce
	if fault.NonceMismatch {
		nonce = "wrong-" + nonce
	}
	idToken, err := s.idToken(key, issuer, nonce)
	if err != nil {
		s.record("token", codeValue, "error", nil)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	s.record("token", codeValue, "issued", map[string]any{"kid": key.KID})
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": "test-access-" + codeValue,
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   300,
	})
}

func (s *Server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	key := s.key
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"keys": []any{jwkForKey(key)}})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	s.codes = make(map[string]authorizationCode)
	s.lastCode = ""
	s.fault = FaultControl{}
	s.events = make([]Event, 0)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true})
}

func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	key, err := newSigningKey()
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	s.mu.Lock()
	s.key = key
	s.mu.Unlock()
	s.record("jwks.rotate", "", "rotated", map[string]any{"kid": key.KID})
	writeJSON(w, http.StatusOK, map[string]string{"kid": key.KID})
}

func (s *Server) handleFault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	fault := FaultControl{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&fault); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if fault.TokenStatus != 0 && (fault.TokenStatus < http.StatusInternalServerError || fault.TokenStatus > 599) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if fault.TokenDelayMS < 0 || fault.TokenDelayMS > 30_000 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	s.mu.Lock()
	s.fault = fault
	s.mu.Unlock()
	s.record("control.fault", "", "configured", nil)
	writeJSON(w, http.StatusOK, fault)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":  s.currentIssuer(r),
		"key_ids": s.KeyIDs(),
		"events":  s.Events(),
	})
}

func (s *Server) idToken(key signingKey, issuer, nonce string) (string, error) {
	if issuer == "" {
		return "", errors.New("OIDC test IdP issuer is not configured")
	}
	claims := jwt.MapClaims{
		"iss":   issuer,
		"sub":   "oidc-e2e-user",
		"aud":   s.clientID,
		"email": "oidc-e2e@example.test",
		"name":  "OIDC E2E User",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = key.KID
	return token.SignedString(key.Private)
}

func (s *Server) hasRedirectURI(redirectURI string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.redirectURIs[redirectURI]
	return exists
}

func (s *Server) currentIssuer(request *http.Request) string {
	s.mu.Lock()
	issuer := s.issuer
	s.mu.Unlock()
	if issuer != "" {
		return issuer
	}
	return "http://" + request.Host
}

func (s *Server) record(kind, code, outcome string, metadata map[string]any) {
	event := Event{At: time.Now().UTC(), Kind: kind, Code: code, Outcome: outcome, Metadata: metadata}
	s.mu.Lock()
	s.events = append(s.events, event)
	eventLog := s.eventLog
	s.mu.Unlock()
	if eventLog == "" {
		return
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}
	file, err := os.OpenFile(eventLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(encoded, '\n'))
}

func newSigningKey() (signingKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return signingKey{}, err
	}
	kid, err := randomValue(12)
	if err != nil {
		return signingKey{}, err
	}
	return signingKey{KID: kid, Private: privateKey}, nil
}

func jwkForKey(key signingKey) map[string]string {
	publicKey := key.Private.PublicKey
	exponent := big.NewInt(int64(publicKey.E)).Bytes()
	return map[string]string{
		"kty": "RSA",
		"use": "sig",
		"kid": key.KID,
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(exponent),
	}
}

func S256Challenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func randomValue(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeOAuthError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func main() {
	config, address, err := configFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	if !isLoopbackAddress(address) {
		log.Fatalf("OIDC test IdP must bind to a loopback address: %s", address)
	}
	idp, err := NewServer(config)
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	issuer := strings.TrimSpace(config.Issuer)
	if issuer == "" {
		issuer = "http://" + listener.Addr().String()
	}
	idp.SetIssuer(issuer)
	log.Printf("OIDC test IdP listening at %s", issuer)
	if err := http.Serve(listener, idp.Handler()); err != nil {
		log.Fatal(err)
	}
}

func configFromEnvironment() (Config, string, error) {
	address := strings.TrimSpace(os.Getenv("OIDC_IDP_ADDR"))
	if address == "" {
		address = defaultListenAddress
	}
	redirectURI := strings.TrimSpace(os.Getenv("OIDC_IDP_REDIRECT_URI"))
	if redirectURI == "" {
		redirectURI = defaultRedirectURI
	}
	testMode, err := strconv.ParseBool(envOrDefault("OIDC_IDP_TEST_MODE", "false"))
	if err != nil {
		return Config{}, "", fmt.Errorf("invalid OIDC_IDP_TEST_MODE: %w", err)
	}
	return Config{
		Issuer:       strings.TrimSpace(os.Getenv("OIDC_IDP_ISSUER")),
		ClientID:     envOrDefault("OIDC_IDP_CLIENT_ID", defaultClientID),
		RedirectURIs: splitValues(redirectURI),
		TestMode:     testMode,
		EventLog:     strings.TrimSpace(os.Getenv("OIDC_IDP_EVENT_LOG")),
	}, address, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitValues(value string) []string {
	values := strings.Split(value, ",")
	result := make([]string, 0, len(values))
	for _, item := range values {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
