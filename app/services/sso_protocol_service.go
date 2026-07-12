package services

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/golang-jwt/jwt/v5"
	dsig "github.com/russellhaering/goxmldsig"
)

const ssoHTTPTimeout = 10 * time.Second

var ssoAllowLoopbackEndpoints bool

func AllowLoopbackSSOEndpointsForTesting() func() {
	previous := ssoAllowLoopbackEndpoints
	ssoAllowLoopbackEndpoints = true
	return func() {
		ssoAllowLoopbackEndpoints = previous
	}
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string   `json:"kty"`
	Use string   `json:"use"`
	Kid string   `json:"kid"`
	Alg string   `json:"alg"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5C []string `json:"x5c"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
}

type oidcDiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

func verifySSOClaims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	provider = withSSODiscoveryDefaults(provider)
	switch provider.Type {
	case "oidc":
		return verifyOIDCClaims(provider, payload)
	case "oauth2":
		return verifyOAuth2Claims(provider, payload)
	case "saml":
		return verifySAMLClaims(provider, payload)
	default:
		return ssoClaims{}, ErrSSOTokenInvalid
	}
}

func withSSODiscoveryDefaults(provider SSOProvider) SSOProvider {
	if strings.TrimSpace(provider.DiscoveryURL) == "" {
		return provider
	}
	body, err := fetchURL(provider.DiscoveryURL)
	if err != nil {
		return provider
	}
	var doc oidcDiscoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return provider
	}
	if provider.Issuer == "" {
		provider.Issuer = doc.Issuer
	}
	if provider.AuthorizationEndpoint == "" {
		provider.AuthorizationEndpoint = doc.AuthorizationEndpoint
	}
	if provider.TokenEndpoint == "" {
		provider.TokenEndpoint = doc.TokenEndpoint
	}
	if provider.UserinfoEndpoint == "" {
		provider.UserinfoEndpoint = doc.UserinfoEndpoint
	}
	if provider.JWKSURI == "" {
		provider.JWKSURI = doc.JWKSURI
	}
	return provider
}

func verifyOIDCClaims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	tokenText := strings.TrimSpace(payload.IDToken)
	if tokenText == "" && strings.TrimSpace(payload.Code) != "" {
		exchanged, err := exchangeOAuthCode(provider, payload)
		if err != nil {
			return ssoClaims{}, err
		}
		tokenText = exchanged.IDToken
	}
	if tokenText == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	return verifyIDToken(provider, payload, tokenText)
}

func verifyOAuth2Claims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	tokenText := strings.TrimSpace(payload.IDToken)
	accessToken := ""
	if tokenText == "" && strings.TrimSpace(payload.Code) != "" {
		exchanged, err := exchangeOAuthCode(provider, payload)
		if err != nil {
			return ssoClaims{}, err
		}
		tokenText = exchanged.IDToken
		accessToken = exchanged.AccessToken
	}
	if tokenText != "" {
		return verifyIDToken(provider, payload, tokenText)
	}
	if accessToken != "" {
		return fetchUserInfoClaims(provider, accessToken)
	}
	return ssoClaims{}, ErrSSOTokenInvalid
}

func fetchUserInfoClaims(provider SSOProvider, accessToken string) (ssoClaims, error) {
	if provider.UserinfoEndpoint == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	endpoint, err := ssoEndpointURL(provider.UserinfoEndpoint)
	if err != nil {
		return ssoClaims{}, err
	}
	client := ssoHTTPClient()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := client.Do(req)
	if err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	var claims map[string]any
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&claims); err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	subject := strings.TrimSpace(jsonString(claims, "sub"))
	if subject == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	return ssoClaims{
		Subject: subject,
		Email:   jsonString(claims, "email"),
		Name:    jsonString(claims, "name"),
		Issuer:  provider.Issuer,
		Raw:     claims,
	}, nil
}

func verifyIDToken(provider SSOProvider, payload SSOLoginPayload, tokenText string) (ssoClaims, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if method, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
			if method != jwt.SigningMethodHS256 || provider.JWTSecret == "" {
				return nil, ErrSSOTokenInvalid
			}
			return []byte(provider.JWTSecret), nil
		}
		if _, ok := token.Method.(*jwt.SigningMethodRSA); ok {
			key, err := publicKeyFromJWKS(provider, fmt.Sprint(token.Header["kid"]))
			if err != nil {
				return nil, err
			}
			return key, nil
		}
		return nil, ErrSSOTokenInvalid
	}, jwt.WithValidMethods([]string{"HS256", "RS256", "RS384", "RS512"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.Issuer != "" && claimsString(claims, "iss") != provider.Issuer {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.Audience != "" && !audienceMatches(claims["aud"], provider.Audience) {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.EnableNonce {
		nonce := strings.TrimSpace(payload.Nonce)
		if nonce == "" || claimsString(claims, "nonce") != nonce {
			return ssoClaims{}, ErrSSOTokenInvalid
		}
	}
	subject := strings.TrimSpace(claimsString(claims, "sub"))
	if subject == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	return ssoClaims{
		Subject: subject,
		Email:   claimsString(claims, "email"),
		Name:    claimsString(claims, "name"),
		Issuer:  claimsString(claims, "iss"),
		Raw:     mapClaims(claims),
	}, nil
}

func exchangeOAuthCode(provider SSOProvider, payload SSOLoginPayload) (tokenResponse, error) {
	if provider.TokenEndpoint == "" || provider.ClientID == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	if strings.TrimSpace(payload.Code) == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	endpoint, err := ssoEndpointURL(provider.TokenEndpoint)
	if err != nil {
		return tokenResponse{}, err
	}
	redirectURI := strings.TrimSpace(provider.RedirectURI)
	if redirectURI == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	if requestedRedirect := strings.TrimSpace(payload.RedirectURI); requestedRedirect != "" && requestedRedirect != redirectURI {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	codeVerifier := strings.TrimSpace(payload.CodeVerifier)
	if (provider.EnablePKCE || provider.Type == "oidc") && codeVerifier == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(payload.Code))
	form.Set("client_id", provider.ClientID)
	if provider.ClientSecret != "" {
		form.Set("client_secret", provider.ClientSecret)
	}
	form.Set("redirect_uri", redirectURI)
	if provider.EnablePKCE || provider.Type == "oidc" {
		form.Set("code_verifier", codeVerifier)
	}

	client := ssoHTTPClient()
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	var payloadRes tokenResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payloadRes); err != nil {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	if payloadRes.IDToken == "" && payloadRes.AccessToken == "" {
		return tokenResponse{}, ErrSSOTokenInvalid
	}
	return payloadRes, nil
}

func publicKeyFromJWKS(provider SSOProvider, kid string) (*rsa.PublicKey, error) {
	rawJWKS := strings.TrimSpace(provider.JWKSJSON)
	if rawJWKS == "" {
		if provider.JWKSURI == "" {
			return nil, ErrSSOTokenInvalid
		}
		body, err := fetchURL(provider.JWKSURI)
		if err != nil {
			return nil, err
		}
		rawJWKS = string(body)
	}
	var doc jwksDocument
	if err := json.Unmarshal([]byte(rawJWKS), &doc); err != nil {
		return nil, ErrSSOTokenInvalid
	}
	for _, key := range doc.Keys {
		if key.Kty != "RSA" || (kid != "" && key.Kid != kid) {
			continue
		}
		if len(key.X5C) > 0 {
			certDER, err := base64.StdEncoding.DecodeString(key.X5C[0])
			if err != nil {
				return nil, ErrSSOTokenInvalid
			}
			cert, err := x509.ParseCertificate(certDER)
			if err != nil {
				return nil, ErrSSOTokenInvalid
			}
			if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
				return rsaKey, nil
			}
		}
		rsaKey, err := jwkRSAPublicKey(key)
		if err == nil {
			return rsaKey, nil
		}
	}
	return nil, ErrSSOTokenInvalid
}

func jwkRSAPublicKey(key jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}
	e := new(big.Int).SetBytes(eBytes).Int64()
	if e <= 0 {
		return nil, ErrSSOTokenInvalid
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: int(e)}, nil
}

func fetchURL(uri string) ([]byte, error) {
	endpoint, err := ssoEndpointURL(uri)
	if err != nil {
		return nil, err
	}
	client := ssoHTTPClient()
	res, err := client.Get(endpoint.String())
	if err != nil {
		return nil, ErrSSOTokenInvalid
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, ErrSSOTokenInvalid
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, ErrSSOTokenInvalid
	}
	return body, nil
}

func ssoEndpointURL(uri string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, ErrSSOTokenInvalid
	}
	if endpoint.Scheme != "https" && endpoint.Scheme != "http" {
		return nil, ErrSSOTokenInvalid
	}
	if endpoint.User != nil {
		return nil, ErrSSOTokenInvalid
	}
	host := endpoint.Hostname()
	if strings.TrimSpace(host) == "" {
		return nil, ErrSSOTokenInvalid
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, ErrSSOTokenInvalid
	}
	if endpoint.Scheme == "http" && !allowHTTPSSOEndpoint(ips) {
		return nil, ErrSSOTokenInvalid
	}
	for _, ip := range ips {
		if isPrivateSSOEndpointIP(ip) {
			return nil, ErrSSOTokenInvalid
		}
	}
	return endpoint, nil
}

func allowHTTPSSOEndpoint(ips []net.IP) bool {
	if !allowLoopbackSSOEndpoints() || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false
		}
	}
	return true
}

func allowLoopbackSSOEndpoints() bool {
	return ssoAllowLoopbackEndpoints ||
		(os.Getenv("APP_ENV") == "testing" && os.Getenv("SSO_TEST_ALLOW_LOOPBACK") == "true")
}

func ssoHTTPClient() http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = ssoSafeDialContext
	return http.Client{
		Timeout:   ssoHTTPTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			_, err := ssoEndpointURL(req.URL.String())
			return err
		},
	}
}

func ssoSafeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrSSOTokenInvalid
	}
	ips, err := ssoHostIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isPrivateSSOEndpointIP(ip) {
			return nil, ErrSSOTokenInvalid
		}
	}

	var lastErr error
	dialer := net.Dialer{}
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrSSOTokenInvalid
}

func ssoHostIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, ErrSSOTokenInvalid
	}
	return ips, nil
}

func isPrivateSSOEndpointIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if allowLoopbackSSOEndpoints() && ip.IsLoopback() {
		return false
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

func verifySAMLClaims(provider SSOProvider, payload SSOLoginPayload) (ssoClaims, error) {
	raw := strings.TrimSpace(payload.SAMLResponse)
	if raw == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && strings.Contains(string(decoded), "<") {
		raw = string(decoded)
	}
	certs, err := parseSAMLCertificates(provider.SAMLCertificate)
	if err != nil || len(certs) == 0 {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	validator := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: certs})
	validator.IdAttribute = "ID"
	validated, err := validator.Validate(doc.Root())
	if err != nil {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.Issuer != "" && samlFirstText(validated, "Issuer") != provider.Issuer {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if provider.SAMLEntityID != "" && !samlAudienceMatches(validated, provider.SAMLEntityID) {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	if !samlConditionsValid(validated, time.Now()) {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	subject := strings.TrimSpace(samlFirstText(validated, "NameID"))
	if subject == "" {
		return ssoClaims{}, ErrSSOTokenInvalid
	}
	rawClaims := samlAttributes(validated)
	rawClaims["sub"] = subject
	rawClaims["subject"] = subject
	rawClaims["email"] = samlAttribute(validated, "email")
	rawClaims["name"] = samlAttribute(validated, "name")
	rawClaims["iss"] = samlFirstText(validated, "Issuer")
	return ssoClaims{
		Subject: subject,
		Email:   jsonString(rawClaims, "email"),
		Name:    jsonString(rawClaims, "name"),
		Issuer:  jsonString(rawClaims, "iss"),
		Raw:     rawClaims,
	}, nil
}

func mapClaims(claims jwt.MapClaims) map[string]any {
	out := make(map[string]any, len(claims))
	for key, value := range claims {
		out[key] = value
	}
	return out
}

func parseSAMLCertificates(value string) ([]*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0)
	rest := []byte(strings.TrimSpace(value))
	for len(rest) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	if len(certs) > 0 {
		return certs, nil
	}
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return []*x509.Certificate{cert}, nil
}

func claimsString(claims jwt.MapClaims, key string) string {
	value, ok := claims[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func samlFirstText(root *etree.Element, tag string) string {
	for _, el := range root.FindElements(".//*") {
		if localName(el.Tag) == tag {
			return strings.TrimSpace(el.Text())
		}
	}
	return ""
}

func samlAttribute(root *etree.Element, name string) string {
	for _, attr := range root.FindElements(".//*") {
		if localName(attr.Tag) != "Attribute" || attr.SelectAttrValue("Name", "") != name {
			continue
		}
		for _, child := range attr.ChildElements() {
			if localName(child.Tag) == "AttributeValue" {
				return strings.TrimSpace(child.Text())
			}
		}
	}
	return ""
}

func samlAttributes(root *etree.Element) map[string]any {
	out := map[string]any{}
	for _, attr := range root.FindElements(".//*") {
		if localName(attr.Tag) != "Attribute" {
			continue
		}
		name := strings.TrimSpace(attr.SelectAttrValue("Name", ""))
		if name == "" {
			continue
		}
		values := make([]string, 0)
		for _, child := range attr.ChildElements() {
			if localName(child.Tag) == "AttributeValue" {
				if value := strings.TrimSpace(child.Text()); value != "" {
					values = append(values, value)
				}
			}
		}
		if len(values) == 1 {
			out[name] = values[0]
		} else if len(values) > 1 {
			out[name] = values
		}
	}
	return out
}

func samlAudienceMatches(root *etree.Element, expected string) bool {
	for _, el := range root.FindElements(".//*") {
		if localName(el.Tag) == "Audience" && strings.TrimSpace(el.Text()) == expected {
			return true
		}
	}
	return false
}

func samlConditionsValid(root *etree.Element, now time.Time) bool {
	for _, el := range root.FindElements(".//*") {
		if localName(el.Tag) != "Conditions" {
			continue
		}
		if notBefore := el.SelectAttrValue("NotBefore", ""); notBefore != "" {
			parsed, err := time.Parse(time.RFC3339, notBefore)
			if err != nil || now.Before(parsed.Add(-time.Minute)) {
				return false
			}
		}
		if notOnOrAfter := el.SelectAttrValue("NotOnOrAfter", ""); notOnOrAfter != "" {
			parsed, err := time.Parse(time.RFC3339, notOnOrAfter)
			if err != nil || !now.Before(parsed.Add(time.Minute)) {
				return false
			}
		}
	}
	return true
}

func localName(tag string) string {
	if idx := strings.LastIndex(tag, ":"); idx >= 0 {
		return tag[idx+1:]
	}
	return tag
}
