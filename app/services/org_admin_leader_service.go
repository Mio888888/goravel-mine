package services

import (
	contractsorm "github.com/goravel/framework/contracts/database/orm"
	"goravel/app/http/request"
	"goravel/app/models"
)

func (s *OrgAdminService) ListLeaders(filters map[string]string, page, pageSize int) (request.PageResult[LeaderRow], error) {
	query := s.orm().Query().Table("dept_leader").
		Select("dept_leader.dept_id", "dept_leader.user_id", "department.name AS dept_name", `"user".username`, `"user".nickname`, `"user".phone`, `"user".email`).
		Join("LEFT JOIN department ON department.id = dept_leader.dept_id").
		Join(`LEFT JOIN "user" ON "user".id = dept_leader.user_id`).
		WhereNull("dept_leader.deleted_at")
	if filters["dept_id"] != "" {
		query = query.Where("dept_leader.dept_id", filters["dept_id"])
	}
	if filters["user_id"] != "" {
		query = query.Where("dept_leader.user_id", filters["user_id"])
	}
	result, err := request.Paginate[LeaderRow](query.OrderBy("dept_leader.dept_id").OrderBy("dept_leader.user_id"), page, pageSize)
	if err != nil {
		return request.PageResult[LeaderRow]{}, err
	}
	for i := range result.List {
		result.List[i].User = DepartmentUser{
			ID:       result.List[i].UserID,
			Username: result.List[i].Username,
			Nickname: result.List[i].Nickname,
			Phone:    result.List[i].Phone,
			Email:    result.List[i].Email,
		}
		result.List[i].Users = []DepartmentUser{result.List[i].User}
	}
	return result, nil
}

func (s *OrgAdminService) SaveLeaders(input LeaderPayload) error {
	userIDs := leaderUserIDs(input)
	return s.orm().Transaction(func(tx contractsorm.Query) error {
		for _, userID := range userIDs {
			_, err := tx.Table("dept_leader").Where("dept_id", input.DeptID).Where("user_id", userID).Delete()
			if err != nil {
				return err
			}
			err = tx.Create(&models.DeptLeader{
				DeptID: input.DeptID, UserID: userID, Timestamps: nowTimestamps(),
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *OrgAdminService) DeleteLeaders(input LeaderPayload) error {
	userIDs := leaderUserIDs(input)
	if len(userIDs) == 0 {
		return nil
	}
	_, err := s.orm().Query().Table("dept_leader").
		Where("dept_id", input.DeptID).
		WhereIn("user_id", uint64Any(userIDs)).
		Delete()
	return err
}

func leaderUserIDs(input LeaderPayload) []uint64 {
	userIDs := payloadIDs(input.UserIDs, "id")
	if len(userIDs) == 0 {
		userIDs = payloadIDs(input.UserID, "id")
	}
	return userIDs
}
