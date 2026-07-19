package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user disabled")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrJWTSecretMissing   = errors.New("jwt secret is not configured")
	ErrAccountLocked      = errors.New("account locked")
	ErrLoginRiskBlocked   = errors.New("login risk blocked")
)
