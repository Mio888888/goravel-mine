package services

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"goravel/app/facades"
)

type KeyRotationFinding struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	RotatedAt string `json:"rotated_at,omitempty"`
	Message   string `json:"message"`
}

type StoredSecretRotationSource struct {
	Scope      string
	Name       string
	UpdatedAt  time.Time
	Configured bool
}

func CheckKeyRotation(now time.Time) []KeyRotationFinding {
	days := facades.Config().GetInt("security.key_rotation.days", 90)
	if days <= 0 {
		days = 90
	}
	keys := []string{
		"APP_KEY",
		"JWT_SECRET",
		"SSO_SAML_PRIVATE_KEY",
		"STORAGE_S3_SECRET_KEY",
	}
	findings := make([]KeyRotationFinding, 0, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			continue
		}
		rotatedAt := strings.TrimSpace(os.Getenv(key + "_ROTATED_AT"))
		if rotatedAt == "" {
			findings = append(findings, KeyRotationFinding{Name: key, Status: "unknown", Message: "缺少轮换时间元数据"})
			continue
		}
		parsed, err := time.Parse("2006-01-02", rotatedAt)
		if err != nil {
			findings = append(findings, KeyRotationFinding{Name: key, Status: "invalid", RotatedAt: rotatedAt, Message: "轮换时间格式应为 YYYY-MM-DD"})
			continue
		}
		age := int(now.Sub(parsed).Hours() / 24)
		switch {
		case age >= days:
			findings = append(findings, KeyRotationFinding{Name: key, Status: "expired", RotatedAt: rotatedAt, Message: "密钥已超过轮换周期"})
		case age >= days-7:
			findings = append(findings, KeyRotationFinding{Name: key, Status: "warning", RotatedAt: rotatedAt, Message: "密钥即将到达轮换周期"})
		default:
			findings = append(findings, KeyRotationFinding{Name: key, Status: "ok", RotatedAt: rotatedAt, Message: "密钥轮换状态正常"})
		}
	}
	return findings
}

func CheckDatabaseKeyRotation(ctx context.Context, now time.Time) ([]KeyRotationFinding, error) {
	days := facades.Config().GetInt("security.key_rotation.days", 90)
	if days <= 0 {
		days = 90
	}
	sources := make([]StoredSecretRotationSource, 0)
	platformConnection := PlatformConnection()
	platformSources, err := storedSecretRotationSources(ctx, platformConnection, "platform")
	if err != nil {
		return nil, err
	}
	sources = append(sources, platformSources...)

	tenants := make([]Tenant, 0)
	if err := OrmForConnectionWithContext(ctx, platformConnection).Query().Table("tenant").Get(&tenants); err != nil {
		return nil, err
	}
	for _, tenant := range tenants {
		RegisterTenantConnection(tenant)
		items, err := storedSecretRotationSources(ctx, TenantConnectionName(tenant), "tenant:"+tenant.Code)
		if err != nil {
			return nil, err
		}
		sources = append(sources, items...)
	}
	return StoredSecretRotationFindings(sources, now, days), nil
}

func StoredSecretRotationFindings(sources []StoredSecretRotationSource, now time.Time, days int) []KeyRotationFinding {
	if days <= 0 {
		days = 90
	}
	findings := make([]KeyRotationFinding, 0, len(sources))
	for _, source := range sources {
		if !source.Configured {
			continue
		}
		name := source.Scope + "/" + source.Name
		if source.UpdatedAt.IsZero() {
			findings = append(findings, KeyRotationFinding{Name: name, Status: "unknown", Message: "缺少数据库密钥轮换时间元数据"})
			continue
		}
		age := int(now.Sub(source.UpdatedAt).Hours() / 24)
		rotatedAt := source.UpdatedAt.Format("2006-01-02")
		switch {
		case age >= days:
			findings = append(findings, KeyRotationFinding{Name: name, Status: "expired", RotatedAt: rotatedAt, Message: "数据库密钥已超过轮换周期"})
		case age >= days-7:
			findings = append(findings, KeyRotationFinding{Name: name, Status: "warning", RotatedAt: rotatedAt, Message: "数据库密钥即将到达轮换周期"})
		default:
			findings = append(findings, KeyRotationFinding{Name: name, Status: "ok", RotatedAt: rotatedAt, Message: "数据库密钥轮换状态正常"})
		}
	}
	return findings
}

func storedSecretRotationSources(ctx context.Context, connection, scope string) ([]StoredSecretRotationSource, error) {
	sources := make([]StoredSecretRotationSource, 0)
	if items, err := ssoProviderSecretSources(ctx, connection, scope); err != nil {
		return nil, err
	} else {
		sources = append(sources, items...)
	}
	if items, err := storageConfigSecretSources(ctx, connection, scope); err != nil {
		return nil, err
	} else {
		sources = append(sources, items...)
	}
	return sources, nil
}

func ssoProviderSecretSources(ctx context.Context, connection, scope string) ([]StoredSecretRotationSource, error) {
	if !schemaHasTable(connection, "sso_provider") {
		return nil, nil
	}
	rows := make([]struct {
		ID                    uint64
		ClientSecret          string
		ClientSecretRotatedAt time.Time
		JWTSecret             string
		JWTSecretRotatedAt    time.Time
	}, 0)
	if err := OrmForConnectionWithContext(ctx, connection).Query().Table("sso_provider").Get(&rows); err != nil {
		return nil, err
	}
	sources := make([]StoredSecretRotationSource, 0, len(rows)*2)
	for _, row := range rows {
		prefix := scope + ":sso_provider:" + strings.TrimSpace(rowID(row.ID))
		sources = append(sources,
			StoredSecretRotationSource{Scope: prefix, Name: "client_secret", UpdatedAt: row.ClientSecretRotatedAt, Configured: strings.TrimSpace(row.ClientSecret) != ""},
			StoredSecretRotationSource{Scope: prefix, Name: "jwt_secret", UpdatedAt: row.JWTSecretRotatedAt, Configured: strings.TrimSpace(row.JWTSecret) != ""},
		)
	}
	return sources, nil
}

func storageConfigSecretSources(ctx context.Context, connection, scope string) ([]StoredSecretRotationSource, error) {
	if !schemaHasTable(connection, "storage_config") {
		return nil, nil
	}
	rows := make([]struct {
		ID                 uint64
		SecretKey          string
		SecretKeyRotatedAt time.Time
	}, 0)
	if err := OrmForConnectionWithContext(ctx, connection).Query().Table("storage_config").Get(&rows); err != nil {
		return nil, err
	}
	sources := make([]StoredSecretRotationSource, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, StoredSecretRotationSource{
			Scope:      scope + ":storage_config:" + strings.TrimSpace(rowID(row.ID)),
			Name:       "secret_key",
			UpdatedAt:  row.SecretKeyRotatedAt,
			Configured: strings.TrimSpace(row.SecretKey) != "",
		})
	}
	return sources, nil
}

func rowID(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func schemaHasTable(connection, table string) bool {
	previous := facades.Schema().GetConnection()
	facades.Schema().SetConnection(connection)
	defer facades.Schema().SetConnection(previous)
	return facades.Schema().HasTable(table)
}
