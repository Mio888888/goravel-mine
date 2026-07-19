package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type ScheduledTaskTestSuite struct {
	suite.Suite
	tests.TestCase
	platformToken string
}

func TestScheduledTaskTestSuite(t *testing.T) {
	suite.Run(t, new(ScheduledTaskTestSuite))
}

func (s *ScheduledTaskTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.PlatformBootstrapSeeder{})
	s.platformToken = s.loginAsPlatformAdmin()
}

func (s *ScheduledTaskTestSuite) TestCreateListAndManualRunMethodTask() {
	require.Equal(s.T(), "platform:scheduledTask:list", services.PlatformPermissionForRoute("GET", "/admin/platform/scheduled-task/list"))
	require.Equal(s.T(), "platform:scheduledTask:run", services.PlatformPermissionForRoute("POST", "/admin/platform/scheduled-task/{id}/run"))

	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "健康检查",
		"code": "health_check",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	data := create["data"].(map[string]any)
	require.Equal(s.T(), "health_check", data["code"])
	require.NotEmpty(s.T(), data["next_run_at"])

	list := s.getJSON("/admin/platform/scheduled-task/list?code=health_check")
	require.Equal(s.T(), float64(200), list["code"])
	rows := list["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), rows, 1)

	id := uint64(data["id"].(float64))
	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)
	require.Equal(s.T(), float64(200), run["code"])
	require.Equal(s.T(), "success", run["data"].(map[string]any)["status"])

	logs := s.getJSON("/admin/platform/scheduled-task-log/list?task_id=" + itoa(id))
	require.Equal(s.T(), float64(200), logs["code"])
	logRows := logs["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), logRows, 1)
	require.Equal(s.T(), "manual", logRows[0].(map[string]any)["trigger_mode"])
}

func (s *ScheduledTaskTestSuite) TestTenantOptionsUseScheduledTaskPermission() {
	tenant := s.createScheduledTaskTenant("scheduled_task_scope", "计划租户")
	operatorToken := s.loginAsScheduledTaskOperator("platform:scheduledTask:list")

	tenantList := s.getJSONWithToken(operatorToken, "/admin/platform/tenant/list")
	require.Equal(s.T(), float64(403), tenantList["code"])

	res := s.getJSONWithToken(operatorToken, "/admin/platform/scheduled-task/tenant-options")
	require.Equal(s.T(), float64(200), res["code"])
	rows := res["data"].([]any)
	require.Len(s.T(), rows, 1)
	row := rows[0].(map[string]any)
	require.Equal(s.T(), float64(tenant.ID), row["id"])
	require.Equal(s.T(), "scheduled_task_scope", row["code"])
	require.Equal(s.T(), "计划租户", row["name"])
}

func (s *ScheduledTaskTestSuite) TestQueueFailedJobListUsesDedicatedPermissionAndDatabasePagination() {
	err := facades.Orm().Query().Table("failed_jobs").Create(map[string]any{
		"uuid": "failed-job-a", "connection": "database", "queue": "default",
		"payload": `{"signature":"operation_log"}`, "exception": "first failure",
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Query().Table("failed_jobs").Create(map[string]any{
		"uuid": "failed-job-b", "connection": "database", "queue": "default",
		"payload": `{"signature":"operation_log"}`, "exception": "second failure",
	})
	require.NoError(s.T(), err)

	oldPermissionToken := s.loginAsScheduledTaskOperator("platform:scheduledTask:log")
	forbidden := s.getJSONWithToken(oldPermissionToken, "/admin/platform/queue/failed-jobs")
	require.Equal(s.T(), float64(403), forbidden["code"])

	queueToken := s.loginAsPlatformOperator("queue_operator", "QueueOperator", "队列操作员", "platform:queueFailedJob:list")
	list := s.getJSONWithToken(queueToken, "/admin/platform/queue/failed-jobs?page=1&page_size=1")
	require.Equal(s.T(), float64(200), list["code"])
	data := list["data"].(map[string]any)
	require.Equal(s.T(), float64(2), data["total"])
	rows := data["list"].([]any)
	require.Len(s.T(), rows, 1)
	require.Equal(s.T(), "operation_log", rows[0].(map[string]any)["signature"])
}

func (s *ScheduledTaskTestSuite) TestInvalidCronRejected() {
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "坏表达式",
		"code": "bad_cron",
		"cron_expression": "* * * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"status": 1
	}`)
	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestInvalidStatusRejected() {
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "坏状态",
		"code": "bad_status",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"status": 9
	}`)

	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestURLTaskRejectsLoopbackTarget() {
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "内网 URL",
		"code": "loopback_url",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "url",
		"payload": {"method": "GET", "url": "http://127.0.0.1/admin"},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestFractionalTenantIDRejected() {
	tenant := s.createScheduledTaskTenant("fractional_tenant", "小数租户")
	res := s.postJSON("/admin/platform/scheduled-task", fmt.Sprintf(`{
		"name": "小数租户",
		"code": "fractional_tenant_scope",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"tenant_ids": [%d.9],
		"timeout_seconds": 5,
		"status": 1
	}`, tenant.ID))

	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestManualRunRecordsFailedLogWhenHandlerPanics() {
	services.RegisterScheduledTaskHandler("scheduler.panic_test", func(_ context.Context, _ models.JSONMap) services.ScheduledTaskExecutionResult {
		panic("boom")
	})
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "异常任务",
		"code": "panic_task",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.panic_test"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)

	require.Equal(s.T(), float64(200), run["code"])
	require.Equal(s.T(), "failed", run["data"].(map[string]any)["status"])
	require.Contains(s.T(), run["data"].(map[string]any)["error_message"], "panic: boom")
}

func (s *ScheduledTaskTestSuite) TestManualRunDoesNotReleaseExistingScheduleLock() {
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "锁保护",
		"code": "manual_lock",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))
	lockUntil := time.Now().Add(time.Hour)
	_, err := facades.Orm().Connection(services.PlatformConnection()).Query().Exec(
		"UPDATE scheduled_task SET locked_until = ?, lock_owner = ?, run_token = ? WHERE id = ?",
		lockUntil, "node-a", "schedule-token", id,
	)
	require.NoError(s.T(), err)

	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)

	require.Equal(s.T(), float64(200), run["code"])
	var task models.ScheduledTask
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("scheduled_task").Where("id", id).First(&task)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "node-a", task.LockOwner)
	require.NotNil(s.T(), task.LockedUntil)
}

func (s *ScheduledTaskTestSuite) TestManualRunRejectsNonTargetNode() {
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "节点限制",
		"code": "manual_target_node",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"target_ips": ["10.0.0.99"],
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	_, err := services.NewScheduledTaskServiceForNode("10.0.0.8").ManualRun(context.Background(), id)

	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "当前节点不在任务指定 IP 范围")
	var count int64
	count, err = facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("scheduled_task_log").
		Where("task_id", id).
		Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)
}

func (s *ScheduledTaskTestSuite) TestScriptTaskRejectsAbsoluteSystemCommand() {
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "危险脚本",
		"code": "danger_script",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "/bin/sh", "args": ["-c", "id"]},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestScriptTaskRejectsShellWrappedCommand() {
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "命令注入",
		"code": "script_shell_injection",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "storage/scripts/example.sh && id"},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestNonSuperAdminCannotConfigureScriptOrBackupTasks() {
	operatorToken := s.loginAsScheduledTaskOperator("platform:scheduledTask:save", "platform:scheduledTask:update")
	scriptPath := facades.App().BasePath("storage/scripts/operator-script.sh")
	require.NoError(s.T(), os.MkdirAll(filepath.Dir(scriptPath), 0o755))
	require.NoError(s.T(), os.WriteFile(scriptPath, []byte("#!/bin/sh\necho operator\n"), 0o755))
	s.T().Cleanup(func() {
		_ = os.Remove(scriptPath)
	})
	services.RegisterScheduledTaskHandler("scheduler.backup", func(context.Context, models.JSONMap) services.ScheduledTaskExecutionResult {
		return services.ScheduledTaskExecutionResult{Status: services.ScheduledTaskLogStatusSuccess}
	})
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.backup")
	})

	method := s.postJSONWithToken(operatorToken, "/admin/platform/scheduled-task", `{
		"name": "普通方法任务",
		"code": "operator_method",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), method["code"])
	id := uint64(method["data"].(map[string]any)["id"].(float64))

	script := s.postJSONWithToken(operatorToken, "/admin/platform/scheduled-task", `{
		"name": "普通脚本任务",
		"code": "operator_script",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "storage/scripts/operator-script.sh"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(403), script["code"])

	update := s.putJSONWithToken(operatorToken, "/admin/platform/scheduled-task/"+itoa(id), `{
		"name": "升级脚本任务",
		"code": "operator_method",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "storage/scripts/operator-script.sh"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(403), update["code"])

	backup := s.postJSONWithToken(operatorToken, "/admin/platform/scheduled-task", `{
		"name": "普通备份任务",
		"code": "operator_backup",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "backup",
		"payload": {"connection": "postgres", "target": "storage/backups"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(403), backup["code"])
}

func (s *ScheduledTaskTestSuite) TestNonSuperAdminCannotBypassScriptRestrictionWithWhitespaceTaskType() {
	operatorToken := s.loginAsScheduledTaskOperator("platform:scheduledTask:save")
	scriptPath := facades.App().BasePath("storage/scripts/operator-whitespace-script.sh")
	require.NoError(s.T(), os.MkdirAll(filepath.Dir(scriptPath), 0o755))
	require.NoError(s.T(), os.WriteFile(scriptPath, []byte("#!/bin/sh\necho operator\n"), 0o755))
	s.T().Cleanup(func() {
		_ = os.Remove(scriptPath)
	})

	script := s.postJSONWithToken(operatorToken, "/admin/platform/scheduled-task", `{
		"name": "空白脚本任务",
		"code": "operator_whitespace_script",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": " script ",
		"payload": {"command": "storage/scripts/operator-whitespace-script.sh"},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(403), script["code"])
}

func (s *ScheduledTaskTestSuite) TestNonSuperAdminCannotUseBackupHandlerThroughMethodTask() {
	operatorToken := s.loginAsScheduledTaskOperator("platform:scheduledTask:save")
	services.RegisterScheduledTaskHandler("scheduler.backup", func(context.Context, models.JSONMap) services.ScheduledTaskExecutionResult {
		return services.ScheduledTaskExecutionResult{Status: services.ScheduledTaskLogStatusSuccess}
	})
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.backup")
	})

	method := s.postJSONWithToken(operatorToken, "/admin/platform/scheduled-task", `{
		"name": "方法绕过备份",
		"code": "operator_backup_method",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.backup"},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(403), method["code"])
}

func (s *ScheduledTaskTestSuite) TestScriptTaskRejectsStorageScriptSymlinkEscape() {
	outside := filepath.Join(s.T().TempDir(), "escaped.sh")
	require.NoError(s.T(), os.WriteFile(outside, []byte("#!/bin/sh\necho escaped\n"), 0o755))
	link := facades.App().BasePath("storage/scripts/escape-link.sh")
	require.NoError(s.T(), os.MkdirAll(filepath.Dir(link), 0o755))
	_ = os.Remove(link)
	if err := os.Symlink(outside, link); err != nil {
		s.T().Skipf("symlink unavailable: %v", err)
	}
	s.T().Cleanup(func() {
		_ = os.Remove(link)
	})

	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "链接逃逸",
		"code": "script_symlink_escape",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "storage/scripts/escape-link.sh"},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestScriptTaskIgnoresWorkdirForScriptPathResolution() {
	tmp := s.T().TempDir()
	scriptName := "missing-" + filepath.Base(tmp) + ".sh"
	script := filepath.Join(tmp, "storage", "scripts", scriptName)
	require.NoError(s.T(), os.MkdirAll(filepath.Dir(script), 0o755))
	require.NoError(s.T(), os.WriteFile(script, []byte("#!/bin/sh\necho bypassed\n"), 0o755))
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "工作目录绕过",
		"code": "script_workdir",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "storage/scripts/`+scriptName+`", "workdir": "`+tmp+`"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(422), res["code"])

	task := s.createLegacyScriptTask(
		"script_workdir",
		models.JSONMap{"command": "storage/scripts/" + scriptName, "workdir": tmp},
		5,
		services.ScheduledTaskScopeGlobal,
		nil,
	)
	log, err := services.NewScheduledTaskService().ManualRun(context.Background(), task.ID)

	require.NoError(s.T(), err)
	require.Equal(s.T(), "failed", log.Status)
	require.NotContains(s.T(), log.Stdout, "bypassed")
}

func (s *ScheduledTaskTestSuite) TestScriptTaskStopsWhenContextCancelled() {
	scriptPath := facades.App().BasePath("storage/scripts/context-cancel-test.sh")
	require.NoError(s.T(), os.MkdirAll(filepath.Dir(scriptPath), 0o755))
	require.NoError(s.T(), os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 5\necho finished\n"), 0o755))
	s.T().Cleanup(func() {
		_ = os.Remove(scriptPath)
	})
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "取消脚本",
		"code": "script_context_cancel",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "storage/scripts/context-cancel-test.sh"},
		"timeout_seconds": 10,
		"status": 1
	}`)
	require.Equal(s.T(), float64(422), create["code"])
	task := s.createLegacyScriptTask(
		"script_context_cancel",
		models.JSONMap{"command": "storage/scripts/context-cancel-test.sh"},
		10,
		services.ScheduledTaskScopeGlobal,
		nil,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	log, err := services.NewScheduledTaskService().ManualRun(ctx, task.ID)
	elapsed := time.Since(started)

	require.NoError(s.T(), err)
	require.Equal(s.T(), "failed", log.Status)
	require.Less(s.T(), elapsed, 2*time.Second)
	require.NotContains(s.T(), log.Stdout, "finished")
}

func (s *ScheduledTaskTestSuite) TestManualRunRetriesShareLogicalExecutionAndOnlyFirstAttemptStoresIdempotencyKey() {
	calls := 0
	services.RegisterScheduledTaskHandler("scheduler.retry_idempotency", func(_ context.Context, _ models.JSONMap) services.ScheduledTaskExecutionResult {
		calls++
		if calls == 1 {
			return services.ScheduledTaskExecutionResult{
				Status: services.ScheduledTaskLogStatusFailed, ErrorMessage: "temporary",
			}
		}
		return services.ScheduledTaskExecutionResult{
			Status: services.ScheduledTaskLogStatusSuccess, Stdout: "recovered",
		}
	})
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.retry_idempotency")
	})
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "重试幂等",
		"code": "retry_idempotency",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"handler_key": "scheduler.retry_idempotency",
		"retry_policy": {
			"max_attempts": 2,
			"initial_delay_seconds": 1,
			"max_delay_seconds": 1
		},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	log, err := services.NewScheduledTaskService().ManualRunIdempotent(
		context.Background(), id, "manual-retry-key",
	)

	require.NoError(s.T(), err)
	require.Equal(s.T(), services.ScheduledTaskLogStatusSuccess, log.Status)
	require.Equal(s.T(), 2, calls)

	var attempts []models.ScheduledTaskLog
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("scheduled_task_log").
		Where("task_id", id).
		OrderBy("attempt").
		Get(&attempts))
	require.Len(s.T(), attempts, 2)
	require.NotEmpty(s.T(), attempts[0].LogicalExecutionID)
	require.Equal(s.T(), attempts[0].LogicalExecutionID, attempts[1].LogicalExecutionID)
	require.Equal(s.T(), "manual-retry-key", attempts[0].IdempotencyKey)
	require.Empty(s.T(), attempts[1].IdempotencyKey)
	require.Equal(s.T(), 1, attempts[0].Attempt)
	require.Equal(s.T(), 2, attempts[1].Attempt)

	duplicate, err := services.NewScheduledTaskService().ManualRunIdempotent(
		context.Background(), id, "manual-retry-key",
	)
	require.NoError(s.T(), err)
	require.Equal(s.T(), attempts[1].ID, duplicate.ID)
	require.Equal(s.T(), 2, calls)
}

func (s *ScheduledTaskTestSuite) TestConcurrentIdempotentRunDoesNotStartRetryExecution() {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	require.NoError(s.T(), services.RegisterScheduledTaskHandlerDefinition(services.ScheduledTaskHandlerDefinition{
		HandlerKey:           "scheduler.concurrent_idempotency",
		Description:          "并发幂等测试",
		DefaultTimeout:       5,
		TenantCapability:     services.ScheduledTaskTenantGlobalOnly,
		SupportsCancellation: true,
		Handler: func(ctx context.Context, _ models.JSONMap) services.ScheduledTaskExecutionResult {
			if calls.Add(1) == 1 {
				close(started)
			}
			select {
			case <-release:
				return services.ScheduledTaskExecutionResult{Status: services.ScheduledTaskLogStatusSuccess}
			case <-ctx.Done():
				return services.ScheduledTaskExecutionResult{
					Status: services.ScheduledTaskLogStatusFailed, ErrorMessage: ctx.Err().Error(),
				}
			}
		},
	}))
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.concurrent_idempotency")
	})
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "并发幂等",
		"code": "concurrent_idempotency",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"handler_key": "scheduler.concurrent_idempotency",
		"concurrency_policy": "REPLACE",
		"retry_policy": {
			"max_attempts": 2,
			"initial_delay_seconds": 1,
			"max_delay_seconds": 1
		},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	type runResult struct {
		log models.ScheduledTaskLog
		err error
	}
	firstResult := make(chan runResult, 1)
	go func() {
		log, err := services.NewScheduledTaskService().ManualRunIdempotent(
			context.Background(), id, "concurrent-run-key",
		)
		firstResult <- runResult{log: log, err: err}
	}()
	<-started

	duplicate, err := services.NewScheduledTaskService().ManualRunIdempotent(
		context.Background(), id, "concurrent-run-key",
	)
	require.NoError(s.T(), err)
	require.Equal(s.T(), services.ScheduledTaskLogStatusRunning, duplicate.Status)
	require.Equal(s.T(), int32(1), calls.Load())

	close(release)
	first := <-firstResult
	require.NoError(s.T(), first.err)
	require.Equal(s.T(), services.ScheduledTaskLogStatusSuccess, first.log.Status)
	require.Equal(s.T(), int32(1), calls.Load())

	attempts, err := facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("scheduled_task_log").
		Where("task_id", id).
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), attempts)
}

func (s *ScheduledTaskTestSuite) TestManualRunReturnsErrorWhenExecutionLogCannotBeCreated() {
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "日志失败",
		"code": "log_create_failure",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))
	_, err := facades.Orm().Connection(services.PlatformConnection()).Query().Exec("DROP TABLE scheduled_task_log")
	require.NoError(s.T(), err)

	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)

	require.Equal(s.T(), float64(500), run["code"])
}

func (s *ScheduledTaskTestSuite) TestCreateRollsBackWhenJSONColumnsCannotBeWritten() {
	_, err := facades.Orm().Connection(services.PlatformConnection()).Query().Exec(`
		CREATE OR REPLACE FUNCTION scheduled_task_json_write_failure()
		RETURNS trigger AS $$
		BEGIN
			IF NEW.code = 'json_write_failure' THEN
				RAISE EXCEPTION 'forced json write failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	require.NoError(s.T(), err)
	_, err = facades.Orm().Connection(services.PlatformConnection()).Query().Exec(`
		CREATE TRIGGER scheduled_task_json_write_failure
		BEFORE UPDATE OF payload, target_ips, tenant_ids ON scheduled_task
		FOR EACH ROW EXECUTE FUNCTION scheduled_task_json_write_failure();
	`)
	require.NoError(s.T(), err)

	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "事务回滚",
		"code": "json_write_failure",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.noop"},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(500), create["code"])
	var count int64
	count, err = facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("scheduled_task").
		Where("code", "json_write_failure").
		Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)
}

func (s *ScheduledTaskTestSuite) TestBackupTaskRejectedUntilRealHandlerConfigured() {
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "备份任务",
		"code": "backup_task",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "backup",
		"payload": {"connection": "postgres", "target": "storage/backups"},
		"timeout_seconds": 5,
		"status": 1
	}`)

	require.Equal(s.T(), float64(422), res["code"])
}

func (s *ScheduledTaskTestSuite) TestBackupTaskAllowedWhenRealHandlerConfigured() {
	services.RegisterScheduledTaskHandler("scheduler.backup", func(_ context.Context, payload models.JSONMap) services.ScheduledTaskExecutionResult {
		return services.ScheduledTaskExecutionResult{
			Status: services.ScheduledTaskLogStatusSuccess,
			Stdout: "backup:" + payload["task_code"].(string),
		}
	})
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.backup")
	})
	res := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "备份任务",
		"code": "backup_task_real",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "backup",
		"payload": {"connection": "postgres", "target": "storage/backups"},
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), res["code"])
	id := uint64(res["data"].(map[string]any)["id"].(float64))

	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)

	require.Equal(s.T(), float64(200), run["code"])
	body := run["data"].(map[string]any)
	require.Equal(s.T(), "success", body["status"])
	require.Equal(s.T(), "backup:backup_task_real", body["stdout"])
}

func (s *ScheduledTaskTestSuite) TestMethodTaskReceivesConfiguredTenantScope() {
	tenantA := s.createScheduledTaskTenant("tenant_scope_a", "租户A")
	tenantB := s.createScheduledTaskTenant("tenant_scope_b", "租户B")
	services.RegisterScheduledTaskHandler("scheduler.scope_test", func(_ context.Context, payload models.JSONMap) services.ScheduledTaskExecutionResult {
		scope := payload["_scheduler"].(map[string]any)
		return services.ScheduledTaskExecutionResult{
			Status: services.ScheduledTaskLogStatusSuccess,
			Stdout: fmt.Sprintf("%v|%v", scope["tenant_ids"], scope["tenant_codes"]),
		}
	})
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.scope_test")
	})
	create := s.postJSON("/admin/platform/scheduled-task", fmt.Sprintf(`{
		"name": "租户范围",
		"code": "tenant_scope_method",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.scope_test"},
		"tenant_ids": [%d, %d],
		"timeout_seconds": 5,
		"status": 1
	}`, tenantA.ID, tenantB.ID))
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)

	require.Equal(s.T(), float64(200), run["code"])
	stdout := run["data"].(map[string]any)["stdout"].(string)
	require.Contains(s.T(), stdout, "tenant_scope_a")
	require.Contains(s.T(), stdout, "tenant_scope_b")
}

func (s *ScheduledTaskTestSuite) TestMethodTaskReceivesTimeoutContext() {
	services.RegisterScheduledTaskHandler("scheduler.timeout_context", func(ctx context.Context, _ models.JSONMap) services.ScheduledTaskExecutionResult {
		deadline, ok := ctx.Deadline()
		if !ok {
			return services.ScheduledTaskExecutionResult{
				Status:       services.ScheduledTaskLogStatusFailed,
				ErrorMessage: "missing deadline",
			}
		}
		if time.Until(deadline) > 2*time.Second {
			return services.ScheduledTaskExecutionResult{
				Status:       services.ScheduledTaskLogStatusFailed,
				ErrorMessage: "deadline too far",
			}
		}
		return services.ScheduledTaskExecutionResult{Status: services.ScheduledTaskLogStatusSuccess}
	})
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.timeout_context")
	})
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "方法超时上下文",
		"code": "method_timeout_context",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.timeout_context"},
		"timeout_seconds": 1,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)

	require.Equal(s.T(), float64(200), run["code"])
	require.Equal(s.T(), "success", run["data"].(map[string]any)["status"])
}

func (s *ScheduledTaskTestSuite) TestScriptTaskReceivesTenantScopeEnvironment() {
	tenant := s.createScheduledTaskTenant("tenant_scope_script", "脚本租户")
	scriptPath := facades.App().BasePath("storage/scripts/tenant-scope-test.sh")
	require.NoError(s.T(), os.MkdirAll(filepath.Dir(scriptPath), 0o755))
	require.NoError(s.T(), os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"$SCHEDULED_TASK_TENANT_CODES|$SCHEDULED_TASK_TENANT_IDS\"\n"), 0o755))
	s.T().Cleanup(func() {
		_ = os.Remove(scriptPath)
	})
	create := s.postJSON("/admin/platform/scheduled-task", fmt.Sprintf(`{
		"name": "脚本租户范围",
		"code": "tenant_scope_script",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "script",
		"payload": {"command": "storage/scripts/tenant-scope-test.sh"},
		"tenant_ids": [%d],
		"timeout_seconds": 5,
		"status": 1
	}`, tenant.ID))
	require.Equal(s.T(), float64(422), create["code"])
	task := s.createLegacyScriptTask(
		"tenant_scope_script",
		models.JSONMap{"command": "storage/scripts/tenant-scope-test.sh"},
		5,
		services.ScheduledTaskScopePerTenant,
		models.JSONSlice{tenant.ID},
	)

	log, err := services.NewScheduledTaskService().ManualRun(context.Background(), task.ID)

	require.NoError(s.T(), err)
	require.Equal(s.T(), "success", log.Status)
	require.Contains(s.T(), log.Stdout, "tenant_scope_script")
	require.Contains(s.T(), log.Stdout, itoa(tenant.ID))
}

func (s *ScheduledTaskTestSuite) TestEmptyTenantScopeMeansAllActiveTenants() {
	tenantA := s.createScheduledTaskTenant("tenant_scope_all_a", "全量A")
	tenantB := s.createScheduledTaskTenant("tenant_scope_all_b", "全量B")
	_ = tenantA
	_ = tenantB
	services.RegisterScheduledTaskHandler("scheduler.scope_all_test", func(_ context.Context, payload models.JSONMap) services.ScheduledTaskExecutionResult {
		scope := payload["_scheduler"].(map[string]any)
		return services.ScheduledTaskExecutionResult{
			Status: services.ScheduledTaskLogStatusSuccess,
			Stdout: fmt.Sprint(scope["tenant_codes"]),
		}
	})
	s.T().Cleanup(func() {
		services.UnregisterScheduledTaskHandler("scheduler.scope_all_test")
	})
	create := s.postJSON("/admin/platform/scheduled-task", `{
		"name": "全部租户",
		"code": "tenant_scope_all",
		"cron_expression": "0 0 0 */10 * *",
		"task_type": "method",
		"payload": {"handler": "scheduler.scope_all_test"},
		"tenant_ids": [],
		"timeout_seconds": 5,
		"status": 1
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	run := s.postJSON("/admin/platform/scheduled-task/"+itoa(id)+"/run", `{}`)

	require.Equal(s.T(), float64(200), run["code"])
	stdout := run["data"].(map[string]any)["stdout"].(string)
	require.Contains(s.T(), stdout, "tenant_scope_all_a")
	require.Contains(s.T(), stdout, "tenant_scope_all_b")
}

func (s *ScheduledTaskTestSuite) TestRunDueScansPastNonTargetLimitedRows() {
	now := time.Now().Truncate(time.Second)
	for i := 0; i < 50; i++ {
		task := s.createScheduledTaskRecord("other_node_"+itoa(uint64(i)), now.Add(-2*time.Minute))
		_, err := facades.Orm().Connection(services.PlatformConnection()).Query().Exec(
			"UPDATE scheduled_task SET target_ips = ?::jsonb WHERE id = ?",
			`["10.0.0.99"]`, task.ID,
		)
		require.NoError(s.T(), err)
	}
	due := s.createScheduledTaskRecord("current_node", now.Add(-time.Minute))

	err := services.NewScheduledTaskServiceForNode("10.0.0.8").RunDue(context.Background(), now)

	require.NoError(s.T(), err)
	var task models.ScheduledTask
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("scheduled_task").Where("id", due.ID).First(&task)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "10.0.0.8", task.LockOwner)
}

func (s *ScheduledTaskTestSuite) createScheduledTaskRecord(code string, nextRunAt time.Time) models.ScheduledTask {
	task := models.ScheduledTask{
		Name: code, Code: code, CronExpression: "0 0 0 */10 * *", Timezone: "UTC",
		NextRunAt: &nextRunAt, TaskType: services.ScheduledTaskTypeMethod,
		TimeoutSeconds: 5, MaxLogOutput: 4000, RunOnOneServer: true,
		Status: services.ScheduledTaskStatusEnabled,
	}
	err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("scheduled_task").Create(&task)
	require.NoError(s.T(), err)
	_, err = facades.Orm().Connection(services.PlatformConnection()).Query().Exec(
		"UPDATE scheduled_task SET payload = ?::jsonb WHERE id = ?",
		`{"handler":"scheduler.noop"}`, task.ID,
	)
	require.NoError(s.T(), err)
	return task
}

func (s *ScheduledTaskTestSuite) createLegacyScriptTask(
	code string,
	payload models.JSONMap,
	timeoutSeconds int,
	scope string,
	tenantIDs models.JSONSlice,
) models.ScheduledTask {
	now := time.Now()
	task := models.ScheduledTask{
		Name: code, Code: code, CronExpression: "0 0 0 */10 * *", Timezone: "UTC",
		NextRunAt: &now, TaskType: services.ScheduledTaskTypeScript,
		TimeoutSeconds: timeoutSeconds, MaxLogOutput: 4000, RunOnOneServer: true,
		ConcurrencyPolicy: services.ScheduledTaskConcurrencyForbid,
		MisfirePolicy:     services.ScheduledTaskMisfireSchedulerDefault,
		Scope:             scope, RuntimeState: services.ScheduledTaskRuntimeLegacyUnsafe, Version: 1,
		Status: services.ScheduledTaskStatusEnabled,
	}
	query := facades.Orm().Connection(services.PlatformConnection()).Query()
	require.NoError(s.T(), query.Table("scheduled_task").Create(&task))
	encodedPayload, err := json.Marshal(payload)
	require.NoError(s.T(), err)
	encodedTenants, err := json.Marshal(tenantIDs)
	require.NoError(s.T(), err)
	_, err = query.Exec(
		"UPDATE scheduled_task SET payload = ?::jsonb, tenant_ids = ?::jsonb WHERE id = ?",
		string(encodedPayload), string(encodedTenants), task.ID,
	)
	require.NoError(s.T(), err)
	return task
}

func (s *ScheduledTaskTestSuite) createScheduledTaskTenant(code string, name string) models.Tenant {
	tenant := models.Tenant{
		Code: code, Name: name, Status: services.TenantStatusActive,
		Plan: "standard", DBDatabase: code, DBSchema: "public",
	}
	err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("tenant").Create(&tenant)
	require.NoError(s.T(), err)
	return tenant
}

func (s *ScheduledTaskTestSuite) postJSON(path string, body string) map[string]any {
	return s.postJSONWithToken(s.platformToken, path, body)
}

func (s *ScheduledTaskTestSuite) postJSONWithToken(token string, path string, body string) map[string]any {
	res, err := s.Http(s.T()).WithToken(token).Post(path, strings.NewReader(body))
	return s.jsonMap(res, err)
}

func (s *ScheduledTaskTestSuite) putJSONWithToken(token string, path string, body string) map[string]any {
	res, err := s.Http(s.T()).WithToken(token).Put(path, strings.NewReader(body))
	return s.jsonMap(res, err)
}

func (s *ScheduledTaskTestSuite) getJSON(path string) map[string]any {
	return s.getJSONWithToken(s.platformToken, path)
}

func (s *ScheduledTaskTestSuite) getJSONWithToken(token string, path string) map[string]any {
	res, err := s.Http(s.T()).WithToken(token).Get(path)
	return s.jsonMap(res, err)
}

func (s *ScheduledTaskTestSuite) loginAsPlatformAdmin() string {
	return s.loginAsPlatformUser("admin", "123456")
}

func (s *ScheduledTaskTestSuite) loginAsScheduledTaskOperator(permissions ...string) string {
	return s.loginAsPlatformOperator("task_operator", "ScheduledTaskOperator", "计划任务操作员", permissions...)
}

func (s *ScheduledTaskTestSuite) loginAsPlatformOperator(username, roleCode, roleName string, permissions ...string) string {
	admin := services.NewPlatformPermissionAdminService()
	require.NoError(s.T(), admin.CreateUser(services.UserPayload{
		Username: username, Password: "123456", Nickname: roleName,
		Email: username + "@example.test", Phone: "16800000002", Dashboard: "platform:scheduledTask", Status: 1,
	}, 1))
	require.NoError(s.T(), admin.CreateRole(services.RolePayload{
		Name: roleName, Code: roleCode, Status: 1, Sort: 10,
	}, 1))
	var userID uint64
	err := facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("platform_user").Where("username", username).Pluck("id", &userID)
	require.NoError(s.T(), err)
	var roleID uint64
	err = facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("platform_role").Where("code", roleCode).Pluck("id", &roleID)
	require.NoError(s.T(), err)
	require.NoError(s.T(), admin.SyncRolePermissions(roleID, permissions))
	require.NoError(s.T(), admin.SyncUserRoles(userID, []string{roleCode}))
	return s.loginAsPlatformUser(username, "123456")
}

func (s *ScheduledTaskTestSuite) loginAsPlatformUser(username, password string) string {
	res, err := s.Http(s.T()).Post(
		"/admin/platform/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, password)),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

func (s *ScheduledTaskTestSuite) jsonMap(res contractshttp.Response, err error) map[string]any {
	require.NoError(s.T(), err)
	res.AssertOk()

	payload, err := res.Json()
	require.NoError(s.T(), err)
	return payload
}
