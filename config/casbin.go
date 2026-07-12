package config

import "goravel/app/facades"

func init() {
	config := facades.Config()
	config.Add("casbin", map[string]any{
		"model": "config/casbin/rbac_model.conf",
	})
}
