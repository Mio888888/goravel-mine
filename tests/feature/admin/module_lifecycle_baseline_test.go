package admin

import (
	"context"
	"testing"
	"time"

	frameworkdb "github.com/goravel/framework/database/db"
	"github.com/stretchr/testify/require"

	"goravel/app/facades"
	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
	"goravel/tests"
)

func TestModuleLifecycleReadModelQueryBudgets(t *testing.T) {
	caseWithDB := &tests.TestCase{}
	caseWithDB.RefreshDatabase()
	seedLifecycleReadModels(t)

	tests := []struct {
		name string
		want int
		run  func(context.Context) error
	}{
		{name: "State", want: 1, run: func(ctx context.Context) error {
			_, err := modulecatalog.NewAdminService(moduleboot.Modules()).WithContext(ctx).State()
			return err
		}},
		{name: "Runs", want: 2, run: func(ctx context.Context) error {
			_, err := modulecatalog.NewAdminService(moduleboot.Modules()).WithContext(ctx).Runs(map[string]string{}, 1, 15)
			return err
		}},
		{name: "Steps", want: 2, run: func(ctx context.Context) error {
			_, err := modulecatalog.NewAdminService(moduleboot.Modules()).WithContext(ctx).Steps(map[string]string{}, 1, 15)
			return err
		}},
		{name: "Locks", want: 1, run: func(ctx context.Context) error {
			_, err := modulecatalog.NewAdminService(moduleboot.Modules()).WithContext(ctx).Locks()
			return err
		}},
		{name: "StateDiff", want: 1, run: func(ctx context.Context) error {
			_, err := modulecatalog.NewAdminService(moduleboot.Modules()).WithContext(ctx).StateDiff()
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := lifecycleQueryCount(t, tt.run)
			t.Logf("%s query count: %d", tt.name, count)
			require.Equal(t, tt.want, count)
		})
	}
}

func TestModuleLifecycleReadModelFiltersAndPagination(t *testing.T) {
	caseWithDB := &tests.TestCase{}
	caseWithDB.RefreshDatabase()
	seedLifecycleReadModelPages(t)
	service := modulecatalog.NewAdminService(moduleboot.Modules())

	runs, err := service.Runs(map[string]string{"ignored": "value"}, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), runs.Total)
	require.Equal(t, []string{"run-new", "run-old"}, []string{runs.List[0].IdempotencyKey, runs.List[1].IdempotencyKey})

	runs, err = service.Runs(map[string]string{"module_id": "alpha", "owner": "owner-new"}, 1, 15)
	require.NoError(t, err)
	require.Equal(t, int64(1), runs.Total)
	require.Equal(t, "run-new", runs.List[0].IdempotencyKey)

	runs, err = service.Runs(map[string]string{"action": "install", "status": "failed"}, 1, 15)
	require.NoError(t, err)
	require.Equal(t, int64(1), runs.Total)
	require.Equal(t, "run-old", runs.List[0].IdempotencyKey)

	runs, err = service.Runs(map[string]string{"run_key": "run-new"}, 1, 15)
	require.NoError(t, err)
	require.Equal(t, int64(1), runs.Total)
	require.Equal(t, "run-new", runs.List[0].IdempotencyKey)

	runs, err = service.Runs(map[string]string{}, 2, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), runs.Total)
	require.Equal(t, "run-old", runs.List[0].IdempotencyKey)

	steps, err := service.Steps(map[string]string{"ignored": "value"}, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), steps.Total)
	require.Equal(t, []string{"attempt-new", "attempt-old"}, []string{steps.List[0].AttemptKey, steps.List[1].AttemptKey})

	steps, err = service.Steps(map[string]string{"run_key": "run-new", "module_id": "alpha"}, 1, 15)
	require.NoError(t, err)
	require.Equal(t, int64(1), steps.Total)
	require.Equal(t, "attempt-new", steps.List[0].AttemptKey)

	steps, err = service.Steps(map[string]string{"action": "install", "status": "failed"}, 1, 15)
	require.NoError(t, err)
	require.Equal(t, int64(1), steps.Total)
	require.Equal(t, "attempt-old", steps.List[0].AttemptKey)

	steps, err = service.Steps(map[string]string{}, 2, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), steps.Total)
	require.Equal(t, "attempt-old", steps.List[0].AttemptKey)
}

func lifecycleQueryCount(t *testing.T, run func(context.Context) error) int {
	t.Helper()
	ctx := frameworkdb.EnableQueryLog(context.Background())
	require.NoError(t, run(ctx))
	return len(frameworkdb.GetQueryLog(ctx))
}

func seedLifecycleReadModels(t *testing.T) {
	t.Helper()
	now := time.Now()
	require.NoError(t, facades.Orm().Query().Table("module_state").Create(map[string]any{
		"module_id": "platform-rbac", "name": "Platform RBAC", "version": "1.0.0",
		"target_version": "1.0.0", "status": "upgraded", "enabled": true,
		"owner": "baseline", "last_action": "upgrade", "last_run_key": "upgrade:platform-rbac:1.0.0",
		"created_at": now, "updated_at": now,
	}))
	require.NoError(t, facades.Orm().Query().Table("module_lifecycle_run").Create(map[string]any{
		"idempotency_key": "upgrade:platform-rbac:1.0.0", "module_id": "platform-rbac",
		"action": "upgrade", "to_version": "1.0.0", "status": "succeeded",
		"owner": "baseline", "reason": "query budget", "created_at": now, "updated_at": now,
	}))
	require.NoError(t, facades.Orm().Query().Table("module_lifecycle_step").Create(map[string]any{
		"attempt_key": "baseline-attempt", "run_key": "upgrade:platform-rbac:1.0.0",
		"module_id": "platform-rbac", "action": "upgrade", "step_name": "command",
		"command": "migrate", "status": "succeeded", "created_at": now, "updated_at": now,
	}))
	require.NoError(t, facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
		"key": "module-lifecycle:baseline", "owner": "baseline", "run_key": "baseline-run",
		"expires_at": now.Add(time.Minute), "created_at": now, "updated_at": now,
	}))
}

func seedLifecycleReadModelPages(t *testing.T) {
	t.Helper()
	now := time.Now()
	for _, row := range []map[string]any{
		{"id": 10, "idempotency_key": "run-old", "module_id": "beta", "action": "install", "status": "failed", "owner": "owner-old", "created_at": now},
		{"id": 20, "idempotency_key": "run-new", "module_id": "alpha", "action": "upgrade", "status": "succeeded", "owner": "owner-new", "created_at": now},
	} {
		require.NoError(t, facades.Orm().Query().Table("module_lifecycle_run").Create(row))
	}
	for _, row := range []map[string]any{
		{"id": 10, "attempt_key": "attempt-old", "run_key": "run-old", "module_id": "beta", "action": "install", "step_name": "command", "command": "migrate", "status": "failed", "created_at": now},
		{"id": 20, "attempt_key": "attempt-new", "run_key": "run-new", "module_id": "alpha", "action": "upgrade", "step_name": "command", "command": "migrate", "status": "succeeded", "created_at": now},
	} {
		require.NoError(t, facades.Orm().Query().Table("module_lifecycle_step").Create(row))
	}
}
