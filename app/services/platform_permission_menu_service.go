package services

import (
	"encoding/json"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/models"
)

func (s *PlatformPermissionAdminService) ListMenus() ([]AdminMenuItem, error) {
	menus := make([]AdminMenuItem, 0)
	err := s.orm().Query().Table("platform_menu").Where("status", 1).
		OrderBy("sort").OrderBy("id").Scan(&menus)
	if err != nil {
		return nil, err
	}
	return buildAdminMenuTree(menus, 0), nil
}

func (s *PlatformPermissionAdminService) CreateMenu(input MenuPayload, operatorID uint64) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		menuID, err := s.saveMenu(tx, 0, input, operatorID)
		if err != nil {
			return err
		}
		return s.syncButtonPermissions(tx, menuID, input.BtnPermission, operatorID)
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) UpdateMenu(id uint64, input MenuPayload, operatorID uint64) error {
	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := s.saveMenu(tx, id, input, operatorID)
		if err != nil {
			return err
		}
		return s.syncButtonPermissions(tx, id, input.BtnPermission, operatorID)
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) DeleteMenus(ids []uint64) error {
	menuIDs, menuNames, err := s.deletedMenuTargets(ids)
	if err != nil {
		return err
	}
	if len(menuIDs) == 0 {
		return nil
	}

	if err := s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("platform_role_belongs_menu").WhereIn("menu_id", uint64Any(menuIDs)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_casbin_rule").Where("ptype", "p").WhereIn("v1", stringAny(menuNames)).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Table("platform_menu").WhereIn("id", uint64Any(menuIDs)).Delete()
		return err
	}); err != nil {
		return err
	}
	s.invalidateCasbinEnforcer()
	return nil
}

func (s *PlatformPermissionAdminService) deletedMenuTargets(ids []uint64) ([]uint64, []string, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}

	rows := make([]menuDeleteRow, 0)
	err := s.orm().Query().Table("platform_menu").
		Select("id", "parent_id", "name").
		Scan(&rows)
	if err != nil {
		return nil, nil, err
	}

	menuIDs, menuNames := collectMenuDeleteTargets(rows, ids)
	return menuIDs, menuNames, nil
}

func (s *PlatformPermissionAdminService) saveMenu(tx contractsorm.Query, id uint64, input MenuPayload, operatorID uint64) (uint64, error) {
	if input.Meta == nil {
		input.Meta = models.JSONMap{}
	}
	if input.Status == 0 {
		input.Status = 1
	}
	encodedMeta, err := json.Marshal(input.Meta)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		menu := models.Menu{
			ParentID: input.ParentID, Name: input.Name, Meta: nil,
			Path: input.Path, Component: input.Component, Redirect: input.Redirect,
			Status: input.Status, Sort: input.Sort, Remark: input.Remark,
			AuditColumns: models.AuditColumns{CreatedBy: operatorID, UpdatedBy: operatorID},
		}
		if err := tx.Table("platform_menu").Create(&menu); err != nil {
			return 0, err
		}
		_, err := tx.Exec("UPDATE platform_menu SET meta = ?::jsonb WHERE id = ?", string(encodedMeta), menu.ID)
		if err != nil {
			return 0, err
		}
		return menu.ID, nil
	}

	_, err = tx.Table("platform_menu").Where("id", id).Update(map[string]any{
		"parent_id": input.ParentID, "name": input.Name,
		"path": input.Path, "component": input.Component, "redirect": input.Redirect,
		"status": input.Status, "sort": input.Sort, "remark": input.Remark,
		"updated_by": operatorID, "updated_at": time.Now(),
	})
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("UPDATE platform_menu SET meta = ?::jsonb WHERE id = ?", string(encodedMeta), id)
	return id, err
}

func (s *PlatformPermissionAdminService) syncButtonPermissions(
	tx contractsorm.Query,
	parentID uint64,
	buttons []MenuPayload,
	operatorID uint64,
) error {
	if buttons == nil {
		return nil
	}
	_, err := tx.Exec(
		"DELETE FROM platform_menu WHERE parent_id = ? AND meta->>'type' = ?",
		parentID,
		"B",
	)
	if err != nil {
		return err
	}
	for _, button := range buttons {
		button.ParentID = parentID
		if button.Meta == nil {
			button.Meta = models.JSONMap{"type": "B"}
		}
		if _, ok := button.Meta["type"]; !ok {
			button.Meta["type"] = "B"
		}
		if _, err := s.saveMenu(tx, 0, button, operatorID); err != nil {
			return err
		}
	}
	return nil
}
