package services

import (
	"encoding/json"
	"time"

	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

func (s *OrgAdminService) ListPositions(filters map[string]string, page, pageSize int) (request.PageResult[PositionRow], error) {
	query := s.orm().Query().Table("position").
		Select("position.id", "position.name", "position.dept_id", "department.name AS dept_name", "policy.policy_type", "policy.value").
		Join("LEFT JOIN department ON department.id = position.dept_id").
		Join("LEFT JOIN data_permission_policy policy ON policy.position_id = position.id AND policy.deleted_at IS NULL").
		WhereNull("position.deleted_at")
	query = query.Scopes(scopes.Contains("position.name", filters["name"]))
	query = query.Scopes(scopes.EqualIfPresent("position.dept_id", filters["dept_id"]))
	result, err := request.Paginate[PositionRow](query.OrderBy("position.id"), page, pageSize)
	if err != nil {
		return request.PageResult[PositionRow]{}, err
	}
	for i := range result.List {
		if result.List[i].PolicyType != "" {
			result.List[i].Policy = &PositionPolicy{
				PolicyType: result.List[i].PolicyType,
				Value:      result.List[i].Value,
			}
		}
	}
	return result, nil
}

func (s *OrgAdminService) CreatePosition(input PositionPayload) error {
	return s.orm().Query().Create(&models.Position{
		Name: input.Name, DeptID: input.DeptID, Timestamps: nowTimestamps(),
	})
}

func (s *OrgAdminService) UpdatePosition(id uint64, input PositionPayload) error {
	_, err := s.orm().Query().Table("position").Where("id", id).Update(map[string]any{
		"name": input.Name, "dept_id": input.DeptID, "updated_at": time.Now(),
	})
	return err
}

func (s *OrgAdminService) DeletePositions(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.orm().Query().Table("position").WhereIn("id", uint64Any(ids)).Update("deleted_at", time.Now())
	return err
}

func (s *OrgAdminService) SetPositionPolicy(positionID uint64, input PositionPayload) error {
	if input.PolicyType == PolicyCustomFunc {
		return BusinessError{Message: "自定义数据权限函数未注册"}
	}
	encoded, err := json.Marshal(mapOrEmptySlice(input.Value))
	if err != nil {
		return err
	}
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("data_permission_policy").Where("position_id", positionID).Delete()
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO data_permission_policy (position_id, policy_type, is_default, value, created_at, updated_at)
			VALUES (?, ?, true, ?::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, positionID, string(input.PolicyType), string(encoded))
		return err
	})
}

func mapOrEmptySlice(value models.JSONSlice) models.JSONSlice {
	if value == nil {
		return models.JSONSlice{}
	}
	return value
}
