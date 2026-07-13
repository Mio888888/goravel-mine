package services

import (
	"fmt"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/persist"
	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

type casbinPolicyRow struct {
	ID    uint64 `gorm:"column:id"`
	Ptype string `gorm:"column:ptype"`
	V0    string `gorm:"column:v0"`
	V1    string `gorm:"column:v1"`
	V2    string `gorm:"column:v2"`
	V3    string `gorm:"column:v3"`
	V4    string `gorm:"column:v4"`
	V5    string `gorm:"column:v5"`
}

func loadCasbinEnforcer(query contractsorm.Query, table string) (casbinAuthorizer, error) {
	var rules []casbinPolicyRow
	if err := query.Table(table).
		SelectRaw(`id,
			COALESCE(ptype, '') AS ptype,
			COALESCE(v0, '') AS v0,
			COALESCE(v1, '') AS v1,
			COALESCE(v2, '') AS v2,
			COALESCE(v3, '') AS v3,
			COALESCE(v4, '') AS v4,
			COALESCE(v5, '') AS v5`).
		OrderBy("id").Get(&rules); err != nil {
		return nil, err
	}

	casbinPolicy, err := casbinModel()
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		line, err := casbinPolicyLine(rule)
		if err != nil {
			return nil, fmt.Errorf("load %s policy %d: %w", table, rule.ID, err)
		}
		if err := persist.LoadPolicyArray(line, casbinPolicy); err != nil {
			return nil, fmt.Errorf("load %s policy %d: %w", table, rule.ID, err)
		}
	}

	enforcer, err := casbin.NewSyncedEnforcer(casbinPolicy)
	if err != nil {
		return nil, err
	}
	enforcer.EnableAutoSave(false)
	if err := enforcer.BuildRoleLinks(); err != nil {
		return nil, err
	}

	return enforcer, nil
}

func casbinPolicyLine(rule casbinPolicyRow) ([]string, error) {
	if rule.Ptype == "" {
		return nil, fmt.Errorf("ptype is empty")
	}
	line := []string{rule.Ptype, rule.V0, rule.V1, rule.V2, rule.V3, rule.V4, rule.V5}
	for len(line) > 1 && line[len(line)-1] == "" {
		line = line[:len(line)-1]
	}
	return line, nil
}
