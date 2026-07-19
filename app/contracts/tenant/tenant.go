package tenant

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"goravel/app/models"
)

const (
	StatusActive    int8 = 1
	StatusSuspended int8 = 2
	StatusArchived  int8 = 3
)

type Tenant = models.Tenant

var (
	ErrRequired      = errors.New("tenant is required")
	ErrNotFound      = errors.New("tenant not found")
	ErrSuspended     = errors.New("tenant is not active")
	ErrQuotaExceeded = errors.New("tenant quota exceeded")
)

func ConnectionName(tenant Tenant) string {
	code := strings.ToLower(strings.TrimSpace(tenant.Code))
	code = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(code, "_")
	code = strings.Trim(code, "_")
	if code == "" {
		code = "default"
	}
	return "tenant_" + strconv.FormatUint(tenant.ID, 10) + "_" + code
}
