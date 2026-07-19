package services

import (
	"strings"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

func applyStringFilter(query contractsorm.Query, column, value string) contractsorm.Query {
	if strings.TrimSpace(value) == "" {
		return query
	}
	return query.Where(column+" LIKE ?", "%"+value+"%")
}

func equalFilter(query contractsorm.Query, column, value string) contractsorm.Query {
	if strings.TrimSpace(value) == "" {
		return query
	}
	return query.Where(column, value)
}
