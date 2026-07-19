package services

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"goravel/app/facades"
)

const tenantTokenSubject = "user"

type TokenInfo struct {
	UserID   uint64
	TenantID uint64
}

type jwtTokenRequirements struct {
	Subject       string
	Type          string
	RequireTenant bool
}

func issueApplicationToken(subject string, userID, tenantID uint64, tokenType string, ttlSeconds int) (string, error) {
	secret, err := JWTSecret()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  subject,
		"uid":  userID,
		"type": tokenType,
		"jti":  rand.Text(),
		"iat":  now.Unix(),
	}
	if tenantID != 0 {
		claims["tid"] = tenantID
	}
	if ttlSeconds > 0 {
		claims["exp"] = now.Add(time.Duration(ttlSeconds) * time.Second).Unix()
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func parseApplicationToken(authorization string, requirements jwtTokenRequirements) (TokenInfo, error) {
	tokenText := bearerToken(authorization)
	if tokenText == "" {
		return TokenInfo{}, ErrUnauthorized
	}

	secret, err := JWTSecret()
	if err != nil {
		return TokenInfo{}, err
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrUnauthorized
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return TokenInfo{}, ErrUnauthorized
	}
	if requirements.Subject != "" && claims["sub"] != requirements.Subject {
		return TokenInfo{}, ErrUnauthorized
	}
	if claims["type"] != requirements.Type {
		return TokenInfo{}, ErrUnauthorized
	}

	userID, err := jwtClaimUint64(claims["uid"])
	if err != nil || userID == 0 {
		return TokenInfo{}, ErrUnauthorized
	}

	var tenantID uint64
	if requirements.RequireTenant {
		tenantID, err = jwtClaimUint64(claims["tid"])
		if err != nil || tenantID == 0 {
			return TokenInfo{}, ErrUnauthorized
		}
	}

	return TokenInfo{UserID: userID, TenantID: tenantID}, nil
}

func JWTSecret() (string, error) {
	secret := facades.Config().GetString("jwt.secret")
	if secret != "" {
		return secret, nil
	}

	appKey := facades.Config().GetString("app.key")
	if appKey != "" {
		return appKey, nil
	}

	if facades.Config().GetString("app.env") == "local" {
		return "local-development-jwt-secret", nil
	}

	return "", ErrJWTSecretMissing
}

func AccessTokenTTLSeconds() int {
	return jwtTTLSeconds("jwt.ttl", 60)
}

func RefreshTokenTTLSeconds() int {
	return jwtTTLSeconds("jwt.refresh_ttl", 20160)
}

func jwtTTLSeconds(configKey string, defaultMinutes int) int {
	minutes := facades.Config().GetInt(configKey, defaultMinutes)
	if minutes <= 0 {
		return 0
	}
	return minutes * 60
}

func tokenBlacklistTTL(ttlSeconds int) time.Duration {
	if ttlSeconds <= 0 {
		return 0
	}
	return time.Duration(ttlSeconds) * time.Second
}

func bearerToken(authorization string) string {
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if token == authorization {
		return ""
	}
	return token
}

func jwtClaimUint64(value any) (uint64, error) {
	switch v := value.(type) {
	case float64:
		return uint64(v), nil
	case int:
		return uint64(v), nil
	case int64:
		return uint64(v), nil
	case uint64:
		return v, nil
	case string:
		return strconv.ParseUint(v, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported JWT claim type %T", value)
	}
}
