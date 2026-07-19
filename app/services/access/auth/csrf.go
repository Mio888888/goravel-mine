package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"goravel/app/facades"
	"goravel/app/support/redaction"
)

func CSRFEnabled() bool {
	return facades.Config().GetBool("security.csrf.enabled", false)
}

func SecuritySameSite() string {
	return strings.ToLower(facades.Config().GetString("security.csrf.same_site", "lax"))
}

func CSRFCookieSecure() bool {
	value := facades.Config().Get("security.csrf.cookie_secure")
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		typed = strings.ToLower(strings.TrimSpace(typed))
		if typed == "" {
			return SecuritySameSite() == "none"
		}
		return typed == "true" || typed == "1" || typed == "yes" || typed == "on"
	default:
		text := strings.ToLower(strings.TrimSpace(fmt.Sprint(typed)))
		return text == "true" || text == "1" || text == "yes" || text == "on"
	}
}

func NewCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func CSRFTokenValid(headerToken, cookieToken string) bool {
	headerToken = strings.TrimSpace(headerToken)
	cookieToken = strings.TrimSpace(cookieToken)
	return headerToken != "" && cookieToken != "" &&
		subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookieToken)) == 1
}

func CSRFOriginAllowed(origin string) bool {
	origin = normalizeOrigin(origin)
	if origin == "" {
		return false
	}
	for _, allowed := range csrfTrustedOrigins() {
		allowed = normalizeOrigin(allowed)
		if allowed == origin {
			return true
		}
	}
	return false
}

func csrfTrustedOrigins() []string {
	origins := redaction.ConfigStringSlice("security.csrf.trusted_origins")
	if len(origins) > 0 {
		return origins
	}
	value := facades.Config().Get("cors.allowed_origins")
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeOrigin(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(value, "/")
	}
	return parsed.Scheme + "://" + parsed.Host
}
