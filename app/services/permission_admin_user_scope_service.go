package services

import (
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/models"
)

func (s *PermissionAdminService) applyUserDataScope(query contractsorm.Query, currentUserID uint64) (contractsorm.Query, error) {
	if currentUserID == 1 {
		return query, nil
	}
	policy, err := s.resolveUserListPolicy(currentUserID)
	if err != nil {
		return query, err
	}
	scope, err := BuildDataScope(policy, DataScopeContext{
		UserID: currentUserID, DeptColumn: "ud.dept_id", OwnerColumn: `"user".id`,
	})
	if err != nil {
		return query, err
	}
	if scope.Condition == "" {
		return query, nil
	}
	query = query.Join("LEFT JOIN user_dept ud ON ud.user_id = \"user\".id AND ud.deleted_at IS NULL")
	return query.Where(scope.Condition, scope.Args...), nil
}

func (s *PermissionAdminService) resolveUserListPolicy(userID uint64) (DataPolicy, error) {
	userPolicy, err := s.policyByOwner("user_id", userID)
	if err != nil {
		return DataPolicy{}, err
	}
	positionPolicies, err := s.positionPolicies(userID)
	if err != nil {
		return DataPolicy{}, err
	}
	return s.resolveDepartmentPolicy(ResolveDataPolicy(userPolicy, positionPolicies), userID)
}

func (s *PermissionAdminService) resolveDepartmentPolicy(policy DataPolicy, userID uint64) (DataPolicy, error) {
	switch policy.Type {
	case PolicyDeptSelf:
		deptIDs, err := s.userDepartmentIDs(userID)
		if err != nil {
			return DataPolicy{}, err
		}
		policy.DeptIDs = deptIDs
	case PolicyDeptTree:
		deptIDs, err := s.userDepartmentIDs(userID)
		if err != nil {
			return DataPolicy{}, err
		}
		policy.DeptIDs, err = s.departmentTreeIDs(deptIDs)
		if err != nil {
			return DataPolicy{}, err
		}
	}

	return policy, nil
}

func (s *PermissionAdminService) policyByOwner(column string, id uint64) (DataPolicy, error) {
	var row struct {
		PolicyType string           `gorm:"column:policy_type"`
		Value      models.JSONSlice `gorm:"column:value;type:jsonb"`
	}
	err := s.orm().Query().Table("data_permission_policy").
		Select("policy_type", "value").
		Where(column, id).
		WhereNull("deleted_at").
		OrderByDesc("is_default").
		OrderBy("id").
		First(&row)
	if err != nil {
		return DataPolicy{}, nil
	}
	return DataPolicy{Type: PolicyType(row.PolicyType), DeptIDs: jsonSliceUint64(row.Value)}, nil
}

func (s *PermissionAdminService) positionPolicies(userID uint64) ([]DataPolicy, error) {
	rows := make([]struct {
		PolicyType string           `gorm:"column:policy_type"`
		Value      models.JSONSlice `gorm:"column:value;type:jsonb"`
	}, 0)
	err := s.orm().Query().Table("data_permission_policy").
		Select("data_permission_policy.policy_type", "data_permission_policy.value").
		Join("JOIN user_position up ON up.position_id = data_permission_policy.position_id").
		Where("up.user_id", userID).
		WhereNull("up.deleted_at").
		WhereNull("data_permission_policy.deleted_at").
		OrderByDesc("data_permission_policy.is_default").
		OrderBy("data_permission_policy.id").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	policies := make([]DataPolicy, 0, len(rows))
	for _, row := range rows {
		policies = append(policies, DataPolicy{Type: PolicyType(row.PolicyType), DeptIDs: jsonSliceUint64(row.Value)})
	}
	return policies, nil
}

func (s *PermissionAdminService) userDepartmentIDs(userID uint64) ([]uint64, error) {
	rows := make([]struct {
		DeptID uint64 `gorm:"column:dept_id"`
	}, 0)
	err := s.orm().Query().Table("user_dept").
		Select("dept_id").
		Where("user_id", userID).
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, err
	}
	deptIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		deptIDs = append(deptIDs, row.DeptID)
	}
	return deptIDs, nil
}

func (s *PermissionAdminService) departmentTreeIDs(rootIDs []uint64) ([]uint64, error) {
	seen := make(map[uint64]bool, len(rootIDs))
	queue := make([]uint64, 0, len(rootIDs))
	for _, id := range rootIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, id)
	}

	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]

		children := make([]struct {
			ID uint64 `gorm:"column:id"`
		}, 0)
		err := s.orm().Query().Table("department").
			Select("id").
			Where("parent_id", parentID).
			WhereNull("deleted_at").
			Scan(&children)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			queue = append(queue, child.ID)
		}
	}

	deptIDs := make([]uint64, 0, len(seen))
	for id := range seen {
		deptIDs = append(deptIDs, id)
	}
	return deptIDs, nil
}

func jsonSliceUint64(values models.JSONSlice) []uint64 {
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			out = append(out, uint64(v))
		case int:
			out = append(out, uint64(v))
		case uint64:
			out = append(out, v)
		}
	}
	return out
}
