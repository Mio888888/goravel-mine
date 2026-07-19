package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"

	"goravel/app/http/request"
	"goravel/app/models"
)

const TenantPlanStatusEnabled int8 = 1

type TenantPlan = models.TenantPlan

type TenantPlanPayload struct {
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Status   int8           `json:"status"`
	Sort     int            `json:"sort"`
	Billing  models.JSONMap `json:"billing"`
	Quotas   models.JSONMap `json:"quotas"`
	Features models.JSONMap `json:"features"`
	Remark   string         `json:"remark"`
}

type TenantPlanService struct {
	ctx context.Context
}

func NewTenantPlanService() *TenantPlanService {
	return &TenantPlanService{}
}

func (s *TenantPlanService) WithContext(ctx context.Context) *TenantPlanService {
	clone := *s
	clone.ctx = contextOrBackground(ctx)
	return &clone
}

func (s *TenantPlanService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func (p TenantPlanPayload) TenantPlan() TenantPlan {
	status := p.Status
	if status == 0 {
		status = TenantPlanStatusEnabled
	}
	return TenantPlan{
		Code:     strings.TrimSpace(p.Code),
		Name:     strings.TrimSpace(p.Name),
		Status:   status,
		Sort:     p.Sort,
		Billing:  p.Billing,
		Quotas:   p.Quotas,
		Features: platformManagedFeatures(p.Features),
		Remark:   strings.TrimSpace(p.Remark),
	}
}

func (s *TenantPlanService) List(filters map[string]string, page, pageSize int) (request.PageResult[TenantPlan], error) {
	query := s.orm().Query().Table("tenant_plan")
	query = applyStringFilter(query, "code", filters["code"])
	query = applyStringFilter(query, "name", filters["name"])
	if filters["status"] != "" {
		query = query.Where("status", filters["status"])
	}
	return request.Paginate[TenantPlan](query.OrderBy("sort").OrderByDesc("id"), page, pageSize)
}

func (s *TenantPlanService) Options() ([]TenantPlan, error) {
	plans := make([]TenantPlan, 0)
	err := s.orm().
		Query().
		Table("tenant_plan").
		Where("status", TenantPlanStatusEnabled).
		OrderBy("sort").
		OrderBy("id").
		Get(&plans)
	return plans, err
}

func (s *TenantPlanService) Create(input TenantPlanPayload) (TenantPlan, error) {
	plan := input.TenantPlan()
	if err := validateTenantPlan(plan); err != nil {
		return TenantPlan{}, err
	}
	err := s.orm().Transaction(func(tx contractsorm.Query) error {
		row := TenantPlan{
			Code: plan.Code, Name: plan.Name, Status: plan.Status,
			Sort: plan.Sort, Remark: plan.Remark,
		}
		if err := tx.Table("tenant_plan").Create(&row); err != nil {
			return err
		}
		plan.ID = row.ID
		return updateTenantPlanJSONColumnsWithQuery(tx, plan.ID, plan)
	})
	if err != nil {
		return TenantPlan{}, err
	}
	return plan, nil
}

func (s *TenantPlanService) Update(id uint64, input TenantPlanPayload) (TenantPlan, error) {
	plan := input.TenantPlan()
	if err := validateTenantPlan(plan); err != nil {
		return TenantPlan{}, err
	}
	if err := s.ensureTenantPlanExists(id); err != nil {
		return TenantPlan{}, err
	}
	_, err := s.orm().
		Query().
		Table("tenant_plan").
		Where("id", id).
		Update(map[string]any{
			"code": plan.Code, "name": plan.Name, "status": plan.Status,
			"sort": plan.Sort, "remark": plan.Remark, "updated_at": time.Now(),
		})
	if err != nil {
		return TenantPlan{}, err
	}
	if err := s.updateTenantPlanJSONColumns(id, plan); err != nil {
		return TenantPlan{}, err
	}
	plan.ID = id
	return plan, nil
}

func (s *TenantPlanService) Delete(ids []uint64) error {
	if len(ids) == 0 {
		return nil
	}
	codes := make([]string, 0, len(ids))
	if err := s.orm().
		Query().
		Table("tenant_plan").
		WhereIn("id", uint64Any(ids)).
		Pluck("code", &codes); err != nil {
		return err
	}
	if len(codes) == 0 {
		return nil
	}
	count, err := s.orm().
		Query().
		Table("tenant").
		WhereIn("plan", stringAny(codes)).
		Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return BusinessError{Message: "套餐已被租户使用，不能删除"}
	}
	_, err = s.orm().
		Query().
		Table("tenant_plan").
		WhereIn("id", uint64Any(ids)).
		Delete()
	return err
}

func (s *TenantPlanService) ExistsActive(code string) (bool, error) {
	if strings.TrimSpace(code) == "" {
		return false, nil
	}
	count, err := s.orm().
		Query().
		Table("tenant_plan").
		Where("code", strings.TrimSpace(code)).
		Where("status", TenantPlanStatusEnabled).
		Count()
	return count > 0, err
}

func (s *TenantPlanService) ActiveByCode(code string) (TenantPlan, error) {
	var plan TenantPlan
	if strings.TrimSpace(code) == "" {
		return plan, nil
	}
	err := s.orm().
		Query().
		Table("tenant_plan").
		Where("code", strings.TrimSpace(code)).
		Where("status", TenantPlanStatusEnabled).
		First(&plan)
	return plan, err
}

func (s *TenantPlanService) ensureTenantPlanExists(id uint64) error {
	count, err := s.orm().
		Query().
		Table("tenant_plan").
		Where("id", id).
		Count()
	if err != nil {
		return err
	}
	if count == 0 {
		return BusinessError{Message: "套餐不存在"}
	}
	return nil
}

func validateTenantPlan(plan TenantPlan) error {
	if plan.Code == "" || plan.Name == "" {
		return BusinessError{Message: "套餐编码和名称不能为空"}
	}
	if plan.Status != TenantPlanStatusEnabled && plan.Status != 2 {
		return BusinessError{Message: "套餐状态无效"}
	}
	return nil
}

func (s *TenantPlanService) updateTenantPlanJSONColumns(id uint64, plan TenantPlan) error {
	return updateTenantPlanJSONColumnsWithQuery(s.orm().Query(), id, plan)
}

func updateTenantPlanJSONColumnsWithQuery(query contractsorm.Query, id uint64, plan TenantPlan) error {
	billing, err := json.Marshal(nullIfEmpty(plan.Billing))
	if err != nil {
		return err
	}
	quotas, err := json.Marshal(nullIfEmpty(plan.Quotas))
	if err != nil {
		return err
	}
	features, err := json.Marshal(nullIfEmpty(plan.Features))
	if err != nil {
		return err
	}
	_, err = query.Exec(
		"UPDATE tenant_plan SET billing = ?::jsonb, quotas = ?::jsonb, features = ?::jsonb WHERE id = ?",
		string(billing), string(quotas), string(features), id,
	)
	return err
}
