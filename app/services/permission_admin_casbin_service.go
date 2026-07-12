package services

import (
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/models"
)

func (s *PermissionAdminService) addCasbinRule(ptype, v0, v1, v2 string) error {
	return addCasbinRule(s.orm().Query(), ptype, v0, v1, v2)
}

func addCasbinRule(query contractsorm.Query, ptype, v0, v1, v2 string) error {
	now := time.Now()
	return query.Create(&models.CasbinRule{
		Ptype: ptype, V0: v0, V1: v1, V2: v2,
		Timestamps: models.Timestamps{CreatedAt: now, UpdatedAt: now},
	})
}
