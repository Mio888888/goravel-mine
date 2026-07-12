package migrations

import (
	"fmt"
	"strconv"
	"strings"

	"goravel/app/facades"
	"goravel/app/models"
)

type M202607110009EnforceTenantDatabaseIsolation struct{}

func (r *M202607110009EnforceTenantDatabaseIsolation) Signature() string {
	return "202607110009_enforce_tenant_database_isolation"
}

func (r *M202607110009EnforceTenantDatabaseIsolation) Up() error {
	connection := facades.Config().GetString("tenant.platform_connection")
	if connection == "" {
		connection = facades.Config().GetString("database.default")
	}
	prefix := "database.connections." + connection
	platformDatabase := facades.Config().GetString(prefix + ".database")
	platformUsername := facades.Config().GetString(prefix + ".username")
	var tenants []models.Tenant
	if err := facades.Orm().Connection(connection).Query().Table("tenant").
		Where("db_database <> ''").Where("db_username <> ''").Get(&tenants); err != nil {
		return err
	}
	if err := restrictDatabaseConnect(connection, platformDatabase, platformUsername); err != nil {
		return err
	}
	for _, tenant := range tenants {
		if !samePostgresInstance(
			tenant.DBHost, tenant.DBPort,
			facades.Config().GetString(prefix+".host"), facades.Config().GetInt(prefix+".port", 5432),
		) {
			continue
		}
		if err := restrictDatabaseConnect(connection, tenant.DBDatabase, tenant.DBUsername); err != nil {
			return err
		}
	}
	return nil
}

func samePostgresInstance(firstHost string, firstPort int, secondHost string, secondPort int) bool {
	normalizeHost := func(host string) string {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "localhost" {
			return "127.0.0.1"
		}
		return host
	}
	normalizePort := func(port int) string {
		if port == 0 {
			port = 5432
		}
		return strconv.Itoa(port)
	}
	return normalizeHost(firstHost) == normalizeHost(secondHost) && normalizePort(firstPort) == normalizePort(secondPort)
}

func (r *M202607110009EnforceTenantDatabaseIsolation) Down() error {
	return nil
}

func restrictDatabaseConnect(connection, database, username string) error {
	database = strings.TrimSpace(database)
	username = strings.TrimSpace(username)
	if database == "" || username == "" {
		return fmt.Errorf("database isolation ACL requires database and username")
	}
	databaseIdentifier := `"` + strings.ReplaceAll(database, `"`, `""`) + `"`
	usernameIdentifier := `"` + strings.ReplaceAll(username, `"`, `""`) + `"`
	_, err := facades.Orm().Connection(connection).Query().Exec(fmt.Sprintf(
		"REVOKE CONNECT ON DATABASE %s FROM PUBLIC; GRANT CONNECT ON DATABASE %s TO %s",
		databaseIdentifier,
		databaseIdentifier,
		usernameIdentifier,
	))
	return err
}
