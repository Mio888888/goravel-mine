package services

import (
	"strings"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

func (s *SSOAuditService) bindingRowsQuery() contractsorm.Query {
	return s.orm().Query().
		Table("sso_user_binding").
		Select(
			"sso_user_binding.id",
			"sso_user_binding.user_id",
			`"user".username`,
			`"user".nickname`,
			"sso_user_binding.provider_id",
			"sso_provider.name AS provider_name",
			"sso_provider.display_name AS provider_display_name",
			"sso_provider.type AS provider_type",
			"sso_provider.scene AS provider_scene",
			"sso_user_binding.sso_user_id",
			"sso_user_binding.sso_email",
			"sso_user_binding.sso_username",
			"sso_user_binding.sso_avatar",
			"sso_user_binding.login_count",
			"sso_user_binding.first_login_at",
			"sso_user_binding.last_login_at",
			"sso_user_binding.token_expires_at",
			"sso_user_binding.created_at",
			"sso_user_binding.updated_at",
		).
		Join(`LEFT JOIN "user" ON "user".id = sso_user_binding.user_id`).
		Join("LEFT JOIN sso_provider ON sso_provider.id = sso_user_binding.provider_id")
}

func (s *SSOAuditService) loginRowsQuery() contractsorm.Query {
	return s.orm().Query().
		Table("sso_login_log").
		Select(
			"sso_login_log.id",
			"sso_login_log.user_id",
			`"user".username`,
			"sso_login_log.provider_id",
			"sso_provider.name AS provider_name",
			"sso_provider.display_name AS provider_display_name",
			"sso_provider.type AS provider_type",
			"sso_provider.scene AS provider_scene",
			"sso_login_log.binding_id",
			"sso_login_log.sso_user_id",
			"sso_login_log.sso_email",
			"sso_login_log.status",
			"sso_login_log.failure_reason",
			"sso_login_log.ip",
			"sso_login_log.user_agent",
			"sso_login_log.device_type",
			"sso_login_log.login_at",
		).
		Join(`LEFT JOIN "user" ON "user".id = sso_login_log.user_id`).
		Join("LEFT JOIN sso_provider ON sso_provider.id = sso_login_log.provider_id")
}

func ssoBindingFilters(query contractsorm.Query, filters map[string]string) contractsorm.Query {
	query = uint64Filter(query, "sso_user_binding.user_id", filters["user_id"])
	query = uint64Filter(query, "sso_user_binding.provider_id", filters["provider_id"])
	query = likeFilter(query, "sso_user_binding.sso_user_id", filters["sso_user_id"])
	query = likeFilter(query, "sso_user_binding.sso_email", filters["sso_email"])
	query = likeFilter(query, "sso_user_binding.sso_username", filters["sso_username"])
	query = likeFilter(query, "sso_provider.name", filters["provider_name"])
	return likeFilter(query, `"user".username`, filters["username"])
}

func ssoLoginLogFilters(query contractsorm.Query, filters map[string]string) contractsorm.Query {
	query = uint64Filter(query, "sso_login_log.user_id", filters["user_id"])
	query = uint64Filter(query, "sso_login_log.provider_id", filters["provider_id"])
	query = equalFilter(query, "sso_login_log.status", filters["status"])
	query = likeFilter(query, "sso_login_log.sso_user_id", filters["sso_user_id"])
	query = likeFilter(query, "sso_login_log.sso_email", filters["sso_email"])
	query = likeFilter(query, "sso_provider.name", filters["provider_name"])
	query = likeFilter(query, `"user".username`, filters["username"])
	return dateRangeFilter(query, "sso_login_log.login_at", filters["start_date"], filters["end_date"])
}

func likeFilter(query contractsorm.Query, column, value string) contractsorm.Query {
	value = strings.TrimSpace(value)
	if value == "" {
		return query
	}
	return query.Where(column+" LIKE ?", "%"+value+"%")
}

func uint64Filter(query contractsorm.Query, column, value string) contractsorm.Query {
	value = strings.TrimSpace(value)
	if value == "" {
		return query
	}
	return query.Where(column, value)
}

func dateRangeFilter(query contractsorm.Query, column, start, end string) contractsorm.Query {
	if strings.TrimSpace(start) != "" {
		query = query.Where(column+" >= ?", start)
	}
	if strings.TrimSpace(end) != "" {
		query = query.Where(column+" <= ?", end)
	}
	return query
}
