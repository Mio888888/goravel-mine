package org

import (
	"time"

	"goravel/app/http/request"
	"goravel/app/models"
	"goravel/app/scopes"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

func (s *OrgAdminService) ListDepartments(filters map[string]string) (request.PageResult[DepartmentRow], error) {
	rows := make([]DepartmentRow, 0)
	query := s.orm().Query().Table("department").WhereNull("deleted_at")
	query = query.Scopes(scopes.Contains("name", filters["name"]))
	if filters["level"] == "1" {
		query = query.Where("parent_id", 0)
	}
	err := query.OrderBy("id").Get(&rows)
	if err != nil {
		return request.PageResult[DepartmentRow]{}, err
	}
	for i := range rows {
		if err := s.fillDepartmentRelations(&rows[i]); err != nil {
			return request.PageResult[DepartmentRow]{}, err
		}
	}
	return request.PageResult[DepartmentRow]{List: rows, Total: int64(len(rows))}, nil
}

func (s *OrgAdminService) CreateDepartment(input DepartmentPayload) error {
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		dept := models.Department{ParentID: input.ParentID, Name: input.Name, Timestamps: nowTimestamps()}
		if err := tx.Create(&dept); err != nil {
			return err
		}
		return s.syncDepartmentRelations(tx, dept.ID, input)
	})
}

func (s *OrgAdminService) UpdateDepartment(id uint64, input DepartmentPayload) error {
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		_, err := tx.Table("department").Where("id", id).Update(map[string]any{
			"name": input.Name, "parent_id": input.ParentID, "updated_at": time.Now(),
		})
		if err != nil {
			return err
		}
		return s.syncDepartmentRelations(tx, id, input)
	})
}

func (s *OrgAdminService) DeleteDepartments(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	_, err := s.orm().Query().Table("department").WhereIn("id", uint64Any(ids)).Update("deleted_at", now)
	if err != nil {
		return err
	}
	_, err = s.orm().Query().Table("position").WhereIn("dept_id", uint64Any(ids)).Update("deleted_at", now)
	return err
}

func (s *OrgAdminService) fillDepartmentRelations(row *DepartmentRow) error {
	children := make([]DepartmentRow, 0)
	if err := s.orm().Query().Table("department").
		Where("parent_id", row.ID).WhereNull("deleted_at").OrderBy("id").Get(&children); err != nil {
		return err
	}
	for i := range children {
		if err := s.fillDepartmentRelations(&children[i]); err != nil {
			return err
		}
	}
	row.Children = children

	row.Positions = make([]PositionRow, 0)
	if err := s.orm().Query().Table("position").
		Select("id", "name", "dept_id").
		Where("dept_id", row.ID).WhereNull("deleted_at").OrderBy("id").Scan(&row.Positions); err != nil {
		return err
	}

	if err := s.fillDepartmentUsers(row); err != nil {
		return err
	}
	return s.fillDepartmentLeaders(row)
}

func (s *OrgAdminService) fillDepartmentUsers(row *DepartmentRow) error {
	row.DepartmentUsers = make([]DepartmentUser, 0)
	return s.orm().Query().Table(`"user"`).
		Select(`"user".id`, `"user".username`, `"user".nickname`, `"user".avatar`, `"user".phone`, `"user".email`).
		Join("JOIN user_dept ud ON ud.user_id = \"user\".id").
		Where("ud.dept_id", row.ID).WhereNull("ud.deleted_at").OrderBy(`"user".id`).
		Scan(&row.DepartmentUsers)
}

func (s *OrgAdminService) fillDepartmentLeaders(row *DepartmentRow) error {
	row.Leader = make([]DepartmentUser, 0)
	return s.orm().Query().Table(`"user"`).
		Select(`"user".id`, `"user".username`, `"user".nickname`, `"user".avatar`, `"user".phone`, `"user".email`).
		Join("JOIN dept_leader dl ON dl.user_id = \"user\".id").
		Where("dl.dept_id", row.ID).WhereNull("dl.deleted_at").OrderBy(`"user".id`).
		Scan(&row.Leader)
}

func (s *OrgAdminService) syncDepartmentRelations(tx contractsorm.Query, deptID uint64, input DepartmentPayload) error {
	if err := syncDepartmentUsers(tx, deptID, input.DepartmentUsers); err != nil {
		return err
	}
	return syncDepartmentLeaders(tx, deptID, input.Leader)
}

func syncDepartmentUsers(tx contractsorm.Query, deptID uint64, users []any) error {
	if users == nil {
		return nil
	}
	_, err := tx.Table("user_dept").Where("dept_id", deptID).Delete()
	if err != nil {
		return err
	}
	for _, userID := range payloadIDs(users, "id") {
		if err := tx.Create(&models.UserDept{UserID: userID, DeptID: deptID, Timestamps: nowTimestamps()}); err != nil {
			return err
		}
	}
	return nil
}

func syncDepartmentLeaders(tx contractsorm.Query, deptID uint64, leaders []any) error {
	if leaders == nil {
		return nil
	}
	_, err := tx.Table("dept_leader").Where("dept_id", deptID).Delete()
	if err != nil {
		return err
	}
	for _, userID := range payloadIDs(leaders, "id") {
		if err := tx.Create(&models.DeptLeader{UserID: userID, DeptID: deptID, Timestamps: nowTimestamps()}); err != nil {
			return err
		}
	}
	return nil
}
