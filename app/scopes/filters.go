package scopes

import (
	"strings"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

func Equal(column, value string) func(contractsorm.Query) contractsorm.Query {
	return whenNonBlank(value, func(query contractsorm.Query) contractsorm.Query {
		return query.Where(column, value)
	})
}

func EqualIfPresent(column, value string) func(contractsorm.Query) contractsorm.Query {
	return whenPresent(value, func(query contractsorm.Query) contractsorm.Query {
		return query.Where(column, value)
	})
}

func Contains(column, value string) func(contractsorm.Query) contractsorm.Query {
	return whenNonBlank(value, func(query contractsorm.Query) contractsorm.Query {
		return query.Where(column+" LIKE ?", "%"+value+"%")
	})
}

func ContainsFold(column, value string) func(contractsorm.Query) contractsorm.Query {
	return whenNonBlank(value, func(query contractsorm.Query) contractsorm.Query {
		return query.Where(column+" ILIKE ?", "%"+value+"%")
	})
}

func ContainsFoldIfPresent(column, value string) func(contractsorm.Query) contractsorm.Query {
	return whenPresent(value, func(query contractsorm.Query) contractsorm.Query {
		return query.Where(column+" ILIKE ?", "%"+value+"%")
	})
}

func GreaterThanOrEqual(column, value string) func(contractsorm.Query) contractsorm.Query {
	return whenNonBlank(value, func(query contractsorm.Query) contractsorm.Query {
		return query.Where(column+" >= ?", value)
	})
}

func LessThanOrEqual(column, value string) func(contractsorm.Query) contractsorm.Query {
	return whenNonBlank(value, func(query contractsorm.Query) contractsorm.Query {
		return query.Where(column+" <= ?", value)
	})
}

func whenNonBlank(
	value string,
	scope func(contractsorm.Query) contractsorm.Query,
) func(contractsorm.Query) contractsorm.Query {
	return func(query contractsorm.Query) contractsorm.Query {
		if strings.TrimSpace(value) == "" {
			return query
		}
		return scope(query)
	}
}

func whenPresent(
	value string,
	scope func(contractsorm.Query) contractsorm.Query,
) func(contractsorm.Query) contractsorm.Query {
	return func(query contractsorm.Query) contractsorm.Query {
		if value == "" {
			return query
		}
		return scope(query)
	}
}
