package scheduledtask

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"goravel/app/models"
)

type scheduledTaskTenantScope struct {
	Tenants []Tenant
}

type ScheduledTaskTenantOption struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

func (s *ScheduledTaskService) TenantOptions() ([]ScheduledTaskTenantOption, error) {
	rows := make([]ScheduledTaskTenantOption, 0)
	err := s.query().
		Table("tenant").
		Select("id", "code", "name").
		Where("status", TenantStatusActive).
		OrderBy("id").
		Get(&rows)
	return rows, err
}

func (s scheduledTaskTenantScope) IDs() []uint64 {
	ids := make([]uint64, 0, len(s.Tenants))
	for _, tenant := range s.Tenants {
		ids = append(ids, tenant.ID)
	}
	return ids
}

func (s scheduledTaskTenantScope) Codes() []string {
	codes := make([]string, 0, len(s.Tenants))
	for _, tenant := range s.Tenants {
		codes = append(codes, tenant.Code)
	}
	return codes
}

func (s scheduledTaskTenantScope) JSONSlice() models.JSONSlice {
	values := make(models.JSONSlice, 0, len(s.Tenants))
	for _, tenant := range s.Tenants {
		values = append(values, map[string]any{
			"id":   tenant.ID,
			"code": tenant.Code,
			"name": tenant.Name,
		})
	}
	return values
}

func (s scheduledTaskTenantScope) SchedulerPayload(task ScheduledTask) map[string]any {
	return map[string]any{
		"task_id":      task.ID,
		"task_code":    task.Code,
		"task_name":    task.Name,
		"tenant_ids":   s.IDs(),
		"tenant_codes": s.Codes(),
		"tenants":      s.JSONSlice(),
	}
}

func (s scheduledTaskTenantScope) Env(task ScheduledTask) map[string]string {
	tenantsJSON, _ := json.Marshal(s.JSONSlice())
	return map[string]string{
		"SCHEDULED_TASK_ID":           fmt.Sprint(task.ID),
		"SCHEDULED_TASK_CODE":         task.Code,
		"SCHEDULED_TASK_NAME":         task.Name,
		"SCHEDULED_TASK_TENANT_IDS":   uint64CSV(s.IDs()),
		"SCHEDULED_TASK_TENANT_CODES": strings.Join(s.Codes(), ","),
		"SCHEDULED_TASK_TENANTS_JSON": string(tenantsJSON),
	}
}

func normalizeTenantIDs(values models.JSONSlice) models.JSONSlice {
	ids, valid := jsonSliceToUint64(values)
	if !valid {
		return values
	}
	result := make(models.JSONSlice, 0, len(ids))
	for _, id := range ids {
		result = append(result, id)
	}
	return result
}

func (s *ScheduledTaskService) validateScheduledTaskTenantIDs(values models.JSONSlice) error {
	ids, valid := jsonSliceToUint64(values)
	if !valid {
		return BusinessError{Message: "任务租户范围必须为正整数 ID"}
	}
	if len(ids) == 0 {
		return nil
	}
	count, err := s.query().
		Table("tenant").
		Where("status", TenantStatusActive).
		WhereIn("id", uint64Any(ids)).
		Count()
	if err != nil {
		return err
	}
	if count != int64(len(compactUniqueUint64(ids))) {
		return BusinessError{Message: "任务租户范围包含不存在或未启用的租户"}
	}
	return nil
}

func (s *ScheduledTaskService) scheduledTaskTenantScopeFor(task ScheduledTask) (scheduledTaskTenantScope, error) {
	ids, valid := jsonSliceToUint64(task.TenantIDs)
	if !valid {
		return scheduledTaskTenantScope{}, BusinessError{Message: "任务租户范围必须为正整数 ID"}
	}
	query := s.query().Table("tenant").Where("status", TenantStatusActive)
	if len(ids) > 0 {
		query = query.WhereIn("id", uint64Any(ids))
	}
	rows := make([]Tenant, 0)
	if err := query.OrderBy("id").Get(&rows); err != nil {
		return scheduledTaskTenantScope{}, err
	}
	return scheduledTaskTenantScope{Tenants: rows}, nil
}

func jsonSliceToUint64(values models.JSONSlice) ([]uint64, bool) {
	ids := make([]uint64, 0, len(values))
	valid := true
	for _, raw := range values {
		switch value := raw.(type) {
		case uint64:
			if value > 0 {
				ids = append(ids, value)
			} else {
				valid = false
			}
		case uint:
			if value > 0 {
				ids = append(ids, uint64(value))
			} else {
				valid = false
			}
		case int:
			if value > 0 {
				ids = append(ids, uint64(value))
			} else {
				valid = false
			}
		case int64:
			if value > 0 {
				ids = append(ids, uint64(value))
			} else {
				valid = false
			}
		case float64:
			if value > 0 && value == math.Trunc(value) {
				ids = append(ids, uint64(value))
			} else {
				valid = false
			}
		case string:
			parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err == nil && parsed > 0 {
				ids = append(ids, parsed)
			} else {
				valid = false
			}
		default:
			valid = false
		}
	}
	return compactUniqueUint64(ids), valid
}

func compactUniqueUint64(values []uint64) []uint64 {
	seen := map[uint64]struct{}{}
	result := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

func uint64CSV(values []uint64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	return strings.Join(parts, ",")
}
