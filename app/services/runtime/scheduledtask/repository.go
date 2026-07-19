package scheduledtask

import (
	"encoding/json"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	frameworkerrors "github.com/goravel/framework/errors"

	"goravel/app/models"
)

func (s *ScheduledTaskService) setStatus(id uint64, status int8, operatorID uint64) (ScheduledTask, error) {
	task, err := s.find(id)
	if err != nil {
		return ScheduledTask{}, err
	}
	nextRunAt, err := taskNextRun(task, scheduledTaskNow())
	if err != nil {
		return ScheduledTask{}, err
	}
	_, err = s.query().Table("scheduled_task").Where("id", id).Update(map[string]any{
		"status": status, "next_run_at": nextRunAt, "updated_by": operatorID, "updated_at": scheduledTaskNow(),
	})
	if err != nil {
		return ScheduledTask{}, err
	}
	task.Status = status
	task.NextRunAt = &nextRunAt
	return task, nil
}

func (s *ScheduledTaskService) find(id uint64) (ScheduledTask, error) {
	var task ScheduledTask
	err := s.query().Table("scheduled_task").Where("id", id).First(&task)
	if err != nil {
		if err == frameworkerrors.OrmRecordNotFound {
			return ScheduledTask{}, BusinessError{Message: "计划任务不存在"}
		}
		return ScheduledTask{}, err
	}
	return task, nil
}

func (s *ScheduledTaskService) query() contractsorm.Query {
	return s.orm().Query()
}

func (s *ScheduledTaskService) orm() contractsorm.Orm {
	return OrmForConnectionWithContext(s.ctx, PlatformConnection())
}

func scheduledTaskScalar(task ScheduledTask) ScheduledTask {
	task.Payload = nil
	task.TargetIPs = nil
	task.TenantIDs = nil
	return task
}

func updateScheduledTaskJSON(query contractsorm.Query, id uint64, payload models.JSONMap, targetIPs models.JSONSlice, tenantIDs models.JSONSlice) error {
	encodedPayload, err := json.Marshal(nullIfEmpty(payload))
	if err != nil {
		return err
	}
	encodedTargets, err := json.Marshal(nullIfEmptySlice(targetIPs))
	if err != nil {
		return err
	}
	encodedTenants, err := json.Marshal(nullIfEmptySlice(tenantIDs))
	if err != nil {
		return err
	}
	_, err = query.Exec(
		"UPDATE scheduled_task SET payload = ?::jsonb, target_ips = ?::jsonb, tenant_ids = ?::jsonb WHERE id = ?",
		string(encodedPayload), string(encodedTargets), string(encodedTenants), id,
	)
	return err
}

func updateScheduledTaskLogJSON(query contractsorm.Query, id uint64, payload models.JSONMap, tenants models.JSONSlice) error {
	encoded, err := json.Marshal(nullIfEmpty(payload))
	if err != nil {
		return err
	}
	encodedTenants, err := json.Marshal(nullIfEmptySlice(tenants))
	if err != nil {
		return err
	}
	_, err = query.Exec("UPDATE scheduled_task_log SET payload = ?::jsonb, tenants = ?::jsonb WHERE id = ?", string(encoded), string(encodedTenants), id)
	return err
}

func nullIfEmptySlice(value models.JSONSlice) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
