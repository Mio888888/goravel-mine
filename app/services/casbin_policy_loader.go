package services

import (
	"fmt"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/persist"
	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/models"
)

func loadCasbinEnforcer(query contractsorm.Query, table string) (casbinAuthorizer, error) {
	var rules []models.CasbinRule
	if err := query.Table(table).OrderBy("id").Get(&rules); err != nil {
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

func casbinPolicyLine(rule models.CasbinRule) ([]string, error) {
	if rule.Ptype == "" {
		return nil, fmt.Errorf("ptype is empty")
	}
	line := []string{rule.Ptype, rule.V0, rule.V1, rule.V2, rule.V3, rule.V4, rule.V5}
	for len(line) > 1 && line[len(line)-1] == "" {
		line = line[:len(line)-1]
	}
	return line, nil
}
