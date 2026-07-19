package admin

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
	"goravel/app/services"
	"goravel/database/migrations"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type ModuleLifecycleTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestModuleLifecycleTestSuite(t *testing.T) {
	suite.Run(t, new(ModuleLifecycleTestSuite))
}

func (s *ModuleLifecycleTestSuite) SetupTest() {
	s.RefreshDatabase()
	services.ResetEnterpriseSecurityControlForTest()
}

func (s *ModuleLifecycleTestSuite) TestLifecycleTablesExistAfterRefreshDatabase() {
	for _, table := range []string{
		"module_state",
		"module_lifecycle_run",
		"module_lifecycle_lock",
		"module_lifecycle_step",
		"enterprise_security_approval",
	} {
		require.True(s.T(), facades.Schema().HasTable(table), table)
	}
	require.True(s.T(), facades.Schema().HasColumn("module_lifecycle_step", "attempt_key"))
}

func (s *ModuleLifecycleTestSuite) TestAttemptKeyMigrationUpgradesLegacyStepTable() {
	_, err := facades.Orm().Query().Exec(`DROP INDEX module_lifecycle_step_attempt_key_unique`)
	require.NoError(s.T(), err)
	_, err = facades.Orm().Query().Exec(`ALTER TABLE module_lifecycle_step DROP COLUMN attempt_key`)
	require.NoError(s.T(), err)
	_, err = facades.Orm().Query().Exec(`ALTER TABLE module_lifecycle_step ADD CONSTRAINT module_lifecycle_step_run_key_step_name_unique UNIQUE (run_key, step_name)`)
	require.NoError(s.T(), err)
	now := time.Now()
	err = facades.Orm().Query().Table("module_lifecycle_step").Create(map[string]any{
		"run_key":    "legacy-run",
		"module_id":  "platform-rbac",
		"action":     modulecatalog.LifecycleActionUpgrade,
		"step_name":  "command",
		"command":    "migrate",
		"status":     modulecatalog.LifecycleStatusSucceeded,
		"created_at": now,
		"updated_at": now,
	})
	require.NoError(s.T(), err)

	err = (&migrations.M202607090005AddModuleLifecycleStepAttemptKey{}).Up()
	require.NoError(s.T(), err)
	require.True(s.T(), facades.Schema().HasIndex("module_lifecycle_step", "module_lifecycle_step_attempt_key_unique"))
	require.False(s.T(), facades.Schema().HasIndex("module_lifecycle_step", "module_lifecycle_step_run_key_step_name_unique"))

	var row modulecatalog.AdminStepRow
	err = facades.Orm().Query().Table("module_lifecycle_step").Where("run_key", "legacy-run").First(&row)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), row.AttemptKey)
	columns, err := facades.Schema().GetColumns("module_lifecycle_step")
	require.NoError(s.T(), err)
	for _, column := range columns {
		if column.Name == "attempt_key" {
			require.False(s.T(), column.Nullable)
			return
		}
	}
	s.Fail("attempt_key column not found")
}

func (s *ModuleLifecycleTestSuite) TestAttemptKeyMigrationDownRestoresLegacyConstraint() {
	err := (&migrations.M202607090005AddModuleLifecycleStepAttemptKey{}).Down()
	require.NoError(s.T(), err)
	require.False(s.T(), facades.Schema().HasColumn("module_lifecycle_step", "attempt_key"))
	require.True(s.T(), facades.Schema().HasIndex("module_lifecycle_step", "module_lifecycle_step_run_key_step_name_unique"))
}

func (s *ModuleLifecycleTestSuite) TestAttemptKeyMigrationDownKeepsLatestDuplicateAttempt() {
	now := time.Now()
	for _, attempt := range []struct {
		key    string
		status string
	}{
		{key: "attempt-old", status: modulecatalog.LifecycleStatusFailed},
		{key: "attempt-new", status: modulecatalog.LifecycleStatusSucceeded},
	} {
		err := facades.Orm().Query().Table("module_lifecycle_step").Create(map[string]any{
			"run_key":     "duplicate-run",
			"attempt_key": attempt.key,
			"module_id":   "platform-rbac",
			"action":      modulecatalog.LifecycleActionUpgrade,
			"step_name":   "command",
			"command":     "migrate",
			"status":      attempt.status,
			"created_at":  now,
			"updated_at":  now,
		})
		require.NoError(s.T(), err)
	}

	require.NoError(s.T(), (&migrations.M202607090005AddModuleLifecycleStepAttemptKey{}).Down())

	var statuses []string
	err := facades.Orm().Query().Table("module_lifecycle_step").
		Where("run_key", "duplicate-run").
		Where("step_name", "command").
		Pluck("status", &statuses)
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{modulecatalog.LifecycleStatusSucceeded}, statuses)
}

func (s *ModuleLifecycleTestSuite) TestEnterpriseApprovalMigrationAddsLegacyColumns() {
	require.NoError(s.T(), facades.Schema().DropIfExists("enterprise_security_approval"))
	require.NoError(s.T(), facades.Schema().Sql(`
		CREATE TABLE enterprise_security_approval (
			id BIGSERIAL PRIMARY KEY,
			approval_id VARCHAR(120) NOT NULL,
			requester_id BIGINT NOT NULL DEFAULT 0,
			approver_id BIGINT NOT NULL DEFAULT 0,
			scope VARCHAR(120) NOT NULL,
			status VARCHAR(30) NOT NULL DEFAULT 'pending',
			reason VARCHAR(255) NOT NULL DEFAULT '',
			expires_at TIMESTAMP NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL
		)
	`))
	require.NoError(s.T(), (&migrations.M202607090004CreateEnterpriseSecurityApprovalTable{}).Up())
	for _, column := range []string{"tenant_id", "resource", "before_snapshot", "after_snapshot", "used_at"} {
		require.True(s.T(), facades.Schema().HasColumn("enterprise_security_approval", column), column)
	}
	require.True(s.T(), facades.Schema().HasIndex("enterprise_security_approval", "enterprise_security_approval_tenant_id_index"))
	_, err := services.NewEnterpriseSecurityControlService().CreatePlatformApproval(context.Background(), services.PlatformApprovalCreateRequest{
		RequesterID: 1, Scope: "migration.compatibility", Resource: "enterprise-security-approval", Reason: "migration test",
	})
	require.NoError(s.T(), err)
}

func (s *ModuleLifecycleTestSuite) TestEnterpriseApprovalMigrationBackfillsMissingApprovalIDs() {
	require.NoError(s.T(), facades.Schema().DropIfExists("enterprise_security_approval"))
	require.NoError(s.T(), facades.Schema().Sql(`
		CREATE TABLE enterprise_security_approval (
			id BIGSERIAL PRIMARY KEY,
			scope VARCHAR(120) NOT NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL
		);
		INSERT INTO enterprise_security_approval (scope) VALUES ('legacy.one'), ('legacy.two');
	`))

	require.NoError(s.T(), (&migrations.M202607090004CreateEnterpriseSecurityApprovalTable{}).Up())

	var approvalIDs []string
	err := facades.Orm().Query().Table("enterprise_security_approval").OrderBy("id").Pluck("approval_id", &approvalIDs)
	require.NoError(s.T(), err)
	require.Equal(s.T(), []string{"legacy:1", "legacy:2"}, approvalIDs)
	require.True(s.T(), facades.Schema().HasIndex("enterprise_security_approval", "enterprise_security_approval_approval_id_unique"))

	columns, err := facades.Schema().GetColumns("enterprise_security_approval")
	require.NoError(s.T(), err)
	for _, column := range columns {
		if column.Name == "approval_id" {
			require.False(s.T(), column.Nullable)
			return
		}
	}
	s.Fail("approval_id column not found")
}

func (s *ModuleLifecycleTestSuite) TestRetryPersistsEveryLifecycleStepAttempt() {
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	call := 0
	service.SetRunnerForTest(func(context.Context, string) error {
		call++
		if call == 1 {
			return fmt.Errorf("first attempt failed")
		}
		return nil
	})
	opts := modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  true,
		Owner:    "feature-test",
		Reason:   "verify retry step history",
	}

	first, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, opts)
	require.ErrorContains(s.T(), err, "first attempt failed")
	second, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, opts)
	require.NoError(s.T(), err)
	require.Equal(s.T(), first.Items[0].IdempotencyKey, second.Items[0].IdempotencyKey)

	var rows []modulecatalog.AdminStepRow
	err = facades.Orm().Query().Table("module_lifecycle_step").
		Where("run_key", first.Items[0].IdempotencyKey).
		OrderBy("id").
		Get(&rows)
	require.NoError(s.T(), err)
	require.Len(s.T(), rows, 3)
	require.Equal(s.T(), modulecatalog.LifecycleStatusFailed, rows[0].Status)
	require.Equal(s.T(), modulecatalog.LifecycleStatusSucceeded, rows[1].Status)
	require.Equal(s.T(), modulecatalog.LifecycleStatusSucceeded, rows[2].Status)
	require.NotEqual(s.T(), rows[0].AttemptKey, rows[1].AttemptKey)
}

func (s *ModuleLifecycleTestSuite) TestExecutePersistsStateRunAndIdempotentSkip() {
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	calls := 0
	service.SetRunnerForTest(func(context.Context, string) error {
		calls++
		return nil
	})

	first, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  true,
		Owner:    "feature-test",
		Reason:   "verify lifecycle persistence",
	})
	require.NoError(s.T(), err)
	require.False(s.T(), first.DryRun)
	require.Len(s.T(), first.Items, 1)
	require.Equal(s.T(), modulecatalog.LifecycleStatusSucceeded, first.Items[0].Status)
	require.Equal(s.T(), 2, calls)

	var stateStatus string
	err = facades.Orm().Query().Table("module_state").
		Where("module_id", "platform-rbac").
		Pluck("status", &stateStatus)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "upgraded", stateStatus)

	var runStatus string
	err = facades.Orm().Query().Table("module_lifecycle_run").
		Where("idempotency_key", first.Items[0].IdempotencyKey).
		Pluck("status", &runStatus)
	require.NoError(s.T(), err)
	require.Equal(s.T(), modulecatalog.LifecycleStatusSucceeded, runStatus)

	var stepCount int64
	stepCount, err = facades.Orm().Query().Table("module_lifecycle_step").
		Where("run_key", first.Items[0].IdempotencyKey).
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(2), stepCount)

	stateManifest, err := modulecatalog.NewService(moduleboot.Modules()).ModuleStateManifest()
	require.NoError(s.T(), err)
	var persisted *modulecatalog.PersistedModuleState
	for _, item := range stateManifest {
		if item.ID == "platform-rbac" {
			persisted = item.Persisted
			break
		}
	}
	require.NotNil(s.T(), persisted)
	require.Equal(s.T(), "upgraded", persisted.Status)
	require.Equal(s.T(), "upgrade", persisted.LastAction)

	second, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  true,
		Owner:    "feature-test",
		Reason:   "verify idempotent skip",
	})
	require.NoError(s.T(), err)
	require.Len(s.T(), second.Items, 1)
	require.Equal(s.T(), modulecatalog.LifecycleStatusSkipped, second.Items[0].Status)
	require.True(s.T(), second.Items[0].Skipped)
	require.Equal(s.T(), 2, calls)
}

func (s *ModuleLifecycleTestSuite) TestExecuteReturnsLockBlockedWhenModuleLocked() {
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	now := time.Now()
	err := facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
		"key":        "module-lifecycle:platform-rbac",
		"owner":      "other-worker",
		"run_key":    "other-run",
		"expires_at": now.Add(time.Minute),
		"created_at": now,
		"updated_at": now,
	})
	require.NoError(s.T(), err)

	result, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  true,
		Owner:    "feature-test",
		Reason:   "verify lock handling",
	})

	require.ErrorContains(s.T(), err, "module lifecycle lock held by other-worker")
	require.Len(s.T(), result.Items, 1)
	require.Equal(s.T(), modulecatalog.LifecycleStatusLockBlocked, result.Items[0].Status)
	require.True(s.T(), result.Items[0].Skipped)
}

func (s *ModuleLifecycleTestSuite) TestExecuteReturnsLockBlockedWhenSameOwnerModuleLocked() {
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	now := time.Now()
	err := facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
		"key":        "module-lifecycle:platform-rbac",
		"owner":      "feature-test",
		"run_key":    "in-flight-run",
		"expires_at": now.Add(time.Minute),
		"created_at": now,
		"updated_at": now,
	})
	require.NoError(s.T(), err)

	result, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  true,
		Owner:    "feature-test",
		Reason:   "verify same owner lock handling",
	})

	require.ErrorContains(s.T(), err, "module lifecycle lock held by feature-test")
	require.Len(s.T(), result.Items, 1)
	require.Equal(s.T(), modulecatalog.LifecycleStatusLockBlocked, result.Items[0].Status)
	require.True(s.T(), result.Items[0].Skipped)
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminLockConflictDoesNotConsumeSecurityEvidence() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	approverToken := s.seedAndLoginPlatformApprover()
	resource := "module-lifecycle:platform-rbac:upgrade"
	reAuthToken, approvalID := s.moduleLifecycleSecurityEvidence(token, approverToken, "module.lifecycle.execute", resource)
	now := time.Now()
	err := facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
		"key":        "module-lifecycle:platform-rbac",
		"owner":      "other-worker",
		"run_key":    "other-run",
		"expires_at": now.Add(time.Minute),
		"created_at": now,
		"updated_at": now,
	})
	require.NoError(s.T(), err)
	payload := modulecatalog.AdminExecutePayload{
		Action:       modulecatalog.LifecycleActionUpgrade,
		ModuleID:     "platform-rbac",
		Execute:      true,
		Owner:        "feature-test",
		Reason:       "retry after lock conflict",
		ConfirmToken: "platform-rbac:upgrade",
		ReAuthToken:  reAuthToken,
		ApprovalID:   approvalID,
		OperatorID:   1,
	}
	service := modulecatalog.NewAdminService(moduleboot.Modules())

	first, err := service.Execute(payload)
	require.ErrorContains(s.T(), err, "module lifecycle lock held by other-worker")
	require.Equal(s.T(), modulecatalog.LifecycleStatusLockBlocked, first.Items[0].Status)

	_, err = facades.Orm().Query().Table("module_lifecycle_lock").Where("key", "module-lifecycle:platform-rbac").Delete()
	require.NoError(s.T(), err)
	second, err := service.Execute(payload)
	require.NoError(s.T(), err)
	require.Equal(s.T(), modulecatalog.LifecycleStatusSucceeded, second.Items[0].Status)
}

func (s *ModuleLifecycleTestSuite) TestExecuteDoesNotWriteNestedCommandOutputToStdout() {
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	service.SetRunnerForTest(func(context.Context, string) error {
		return nil
	})

	output, err := captureStdout(s.T(), func() error {
		_, runErr := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, modulecatalog.LifecycleOptions{
			ModuleID: "platform-rbac",
			Execute:  true,
			Owner:    "feature-test",
			Reason:   "verify clean lifecycle stdout",
		})
		return runErr
	})

	require.NoError(s.T(), err)
	require.Empty(s.T(), output)
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminCanViewLifecycleState() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()

	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/module-lifecycle/state")

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"], "response body: %#v", body)
	items := body["data"].(map[string]any)["list"].([]any)
	require.NotEmpty(s.T(), items)
	first := items[0].(map[string]any)
	require.NotEmpty(s.T(), first["id"])
	require.Contains(s.T(), first, "lifecycle")
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminCanDryRunLifecycleAction() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/execute", strings.NewReader(`{
		"action": "upgrade",
		"module_id": "platform-rbac"
	}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"], "response body: %#v", body)
	data := body["data"].(map[string]any)
	require.Equal(s.T(), true, data["dry_run"])
	require.Len(s.T(), data["items"].([]any), 1)

	var count int64
	count, err = facades.Orm().Query().Table("module_lifecycle_run").Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(0), count)
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminExecuteRequiresSafetyGate() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/execute", strings.NewReader(`{
		"action": "upgrade",
		"module_id": "platform-rbac",
		"execute": true,
		"owner": "feature-test",
		"reason": "verify safety gate"
	}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "confirm token")
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminExecuteRequiresRegisteredApprovalAndBoundReAuth() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/execute", strings.NewReader(`{
		"action": "upgrade",
		"module_id": "platform-rbac",
		"execute": true,
		"owner": "feature-test",
		"reason": "verify safety gate",
		"confirm_token": "platform-rbac:upgrade",
		"reauth_token": "unregistered",
		"approval_id": "approval-1"
	}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "valid re-auth token")
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminExecuteTreatsMissingPersistentApprovalAsGateError() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	reAuthToken := s.issueLifecycleReAuthToken(token, "module.lifecycle.execute", "module-lifecycle:platform-rbac:upgrade")

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/execute", strings.NewReader(`{
		"action": "upgrade",
		"module_id": "platform-rbac",
		"execute": true,
		"owner": "feature-test",
		"reason": "verify missing persistent approval gate",
		"confirm_token": "platform-rbac:upgrade",
		"reauth_token": "`+reAuthToken+`",
		"approval_id": "approval-missing"
	}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "approved approval record")
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminCanExecuteWithPersistentApproval() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	approverToken := s.seedAndLoginPlatformApprover()
	reAuthToken, approvalID := s.moduleLifecycleSecurityEvidence(token, approverToken, "module.lifecycle.execute", "module-lifecycle:platform-rbac:upgrade")

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/execute", strings.NewReader(`{
		"action": "upgrade",
		"module_id": "platform-rbac",
		"execute": true,
		"owner": "feature-test",
		"reason": "verify persistent approval",
		"confirm_token": "platform-rbac:upgrade",
		"reauth_token": "`+reAuthToken+`",
		"approval_id": "`+approvalID+`"
	}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"], "response body: %#v", body)
	data := body["data"].(map[string]any)
	require.Equal(s.T(), false, data["dry_run"])
	require.Len(s.T(), data["items"].([]any), 1)

	reuseReAuthToken := s.issueLifecycleReAuthToken(token, "module.lifecycle.execute", "module-lifecycle:platform-rbac:upgrade")
	reuse, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/execute", strings.NewReader(`{
		"action": "upgrade",
		"module_id": "platform-rbac",
		"execute": true,
		"owner": "feature-test",
		"reason": "verify approval reuse blocked",
		"confirm_token": "platform-rbac:upgrade",
		"reauth_token": "`+reuseReAuthToken+`",
		"approval_id": "`+approvalID+`"
	}`))
	require.NoError(s.T(), err)
	reuse.AssertOk()
	reuseBody, err := reuse.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), reuseBody["code"])
	require.Contains(s.T(), reuseBody["message"], "approved approval record")
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminCanViewLifecycleSteps() {
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	service.SetRunnerForTest(func(context.Context, string) error {
		return nil
	})
	result, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  true,
		Owner:    "feature-test",
		Reason:   "verify lifecycle steps",
	})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), result.Items)

	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/module-lifecycle/steps?run_key=" + result.Items[0].IdempotencyKey)

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	items := body["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), items, 2)
	first := items[0].(map[string]any)
	require.NotEmpty(s.T(), first["step_name"])
	require.NotEmpty(s.T(), first["command"])
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminCanViewLifecycleDiff() {
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	service.SetRunnerForTest(func(context.Context, string) error {
		return nil
	})
	_, err := service.Execute(context.Background(), modulecatalog.LifecycleActionUpgrade, modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  true,
		Owner:    "feature-test",
		Reason:   "verify lifecycle diff",
	})
	require.NoError(s.T(), err)

	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/module-lifecycle/diff")

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	items := body["data"].(map[string]any)["list"].([]any)
	require.NotEmpty(s.T(), items)
	first := items[0].(map[string]any)
	require.Contains(s.T(), first, "drift")
	require.Contains(s.T(), first, "manifest_version")
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminLifecycleDiffIncludesOrphanPersistedState() {
	now := time.Now()
	for _, moduleID := range []string{"zulu-removed", "alpha-removed"} {
		err := facades.Orm().Query().Table("module_state").Create(map[string]any{
			"module_id": moduleID, "name": moduleID, "status": "upgraded", "enabled": true,
			"target_version": "9.9.9", "last_action": "upgrade", "last_run_key": moduleID + "-run",
			"last_run_at": now, "installed_at": now, "upgraded_at": now,
			"created_at": now, "updated_at": now,
		})
		require.NoError(s.T(), err)
	}

	result, err := modulecatalog.NewAdminService(moduleboot.Modules()).StateDiff()
	require.NoError(s.T(), err)

	expectedIDs := make([]string, 0, len(result.List))
	for _, state := range moduleboot.Modules().ModuleStates() {
		expectedIDs = append(expectedIDs, state.ID)
	}
	expectedIDs = append(expectedIDs, "alpha-removed", "zulu-removed")
	actualIDs := make([]string, 0, len(result.List))
	for _, item := range result.List {
		actualIDs = append(actualIDs, item.ModuleID)
	}
	require.Equal(s.T(), expectedIDs, actualIDs)
	require.Equal(s.T(), "missing_manifest", result.List[len(result.List)-1].Drift)
	require.Equal(s.T(), "9.9.9", result.List[len(result.List)-1].PersistedVersion)
}

func (s *ModuleLifecycleTestSuite) TestStaleLockReleaseConsumesSecurityEvidenceOnce() {
	now := time.Now()
	require.NoError(s.T(), facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
		"key": "module-lifecycle:stale", "owner": "feature-test", "run_key": "stale-run",
		"expires_at": now.Add(-time.Minute), "created_at": now, "updated_at": now,
	}))
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	approverToken := s.seedAndLoginPlatformApprover()
	operation := "module.lifecycle.release-lock"
	resource := "module-lifecycle:stale-locks:all"
	reAuthToken, approvalID := s.moduleLifecycleSecurityEvidence(token, approverToken, operation, resource)
	service := modulecatalog.NewAdminService(moduleboot.Modules())
	payload := modulecatalog.AdminLockReleasePayload{
		ConfirmToken: "release-stale-locks", ReAuthToken: reAuthToken, ApprovalID: approvalID, OperatorID: 1,
	}

	result, err := service.ReleaseStaleLocks(payload)
	require.NoError(s.T(), err)
	require.Len(s.T(), result.Released, 1)

	_, err = service.ReleaseStaleLocks(payload)
	require.ErrorContains(s.T(), err, "valid re-auth token")
	payload.ReAuthToken = s.issueLifecycleReAuthToken(token, operation, resource)
	_, err = service.ReleaseStaleLocks(payload)
	require.ErrorContains(s.T(), err, "approved approval record")
}

func (s *ModuleLifecycleTestSuite) TestModuleStateReadHonorsCanceledContextWhenTableMissing() {
	_, err := facades.Orm().Query().Exec(`DROP TABLE module_state`)
	require.NoError(s.T(), err)
	_, err = modulecatalog.NewAdminService(moduleboot.Modules()).State()
	require.NoError(s.T(), err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = modulecatalog.NewAdminService(moduleboot.Modules()).WithContext(ctx).State()

	require.ErrorIs(s.T(), err, context.Canceled)
}

func (s *ModuleLifecycleTestSuite) TestPlatformAdminCanDryRunAndReleaseStaleLocks() {
	now := time.Now()
	err := facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
		"key":        "module-lifecycle:stale",
		"owner":      "feature-test",
		"run_key":    "stale-run",
		"expires_at": now.Add(-time.Minute),
		"created_at": now,
		"updated_at": now,
	})
	require.NoError(s.T(), err)

	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	dryRun, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/locks/release-stale", strings.NewReader(`{
		"dry_run": true
	}`))
	require.NoError(s.T(), err)
	dryRun.AssertOk()
	dryBody, err := dryRun.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), dryBody["code"])
	require.Len(s.T(), dryBody["data"].(map[string]any)["released"].([]any), 1)

	approverToken := s.seedAndLoginPlatformApprover()
	reAuthToken, approvalID := s.moduleLifecycleSecurityEvidence(token, approverToken, "module.lifecycle.release-lock", "module-lifecycle:stale-locks:all")
	release, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/module-lifecycle/locks/release-stale", strings.NewReader(`{
		"dry_run": false,
		"confirm_token": "release-stale-locks",
		"reauth_token": "`+reAuthToken+`",
		"approval_id": "`+approvalID+`"
	}`))
	require.NoError(s.T(), err)
	release.AssertOk()
	body, err := release.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])

	count, err := facades.Orm().Query().Table("module_lifecycle_lock").Where("key", "module-lifecycle:stale").Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(0), count)
}

func (s *ModuleLifecycleTestSuite) TestStaleLockSecurityEvidenceIsBoundToRequestedKey() {
	now := time.Now()
	for _, key := range []string{"module-lifecycle:alpha", "module-lifecycle:beta"} {
		err := facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
			"key":        key,
			"owner":      "feature-test",
			"run_key":    key + ":run",
			"expires_at": now.Add(-time.Minute),
			"created_at": now,
			"updated_at": now,
		})
		require.NoError(s.T(), err)
	}
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	approverToken := s.seedAndLoginPlatformApprover()
	resource := "module-lifecycle:stale-locks:module-lifecycle:alpha"
	reAuthToken, approvalID := s.moduleLifecycleSecurityEvidence(token, approverToken, "module.lifecycle.release-lock", resource)
	service := modulecatalog.NewAdminService(moduleboot.Modules())
	payload := modulecatalog.AdminLockReleasePayload{
		Key:          "module-lifecycle:beta",
		ConfirmToken: "release-stale-locks",
		ReAuthToken:  reAuthToken,
		ApprovalID:   approvalID,
		OperatorID:   1,
	}

	_, err := service.ReleaseStaleLocks(payload)
	require.ErrorContains(s.T(), err, "valid re-auth token")
	payload.Key = ""
	_, err = service.ReleaseStaleLocks(payload)
	require.ErrorContains(s.T(), err, "valid re-auth token")
	payload.Key = "module-lifecycle:alpha"
	result, err := service.ReleaseStaleLocks(payload)
	require.NoError(s.T(), err)
	require.Len(s.T(), result.Released, 1)
	require.Equal(s.T(), "module-lifecycle:alpha", result.Released[0].Key)

	count, err := facades.Orm().Query().Table("module_lifecycle_lock").Where("key", "module-lifecycle:beta").Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *ModuleLifecycleTestSuite) TestReleaseStaleLocksKeepsNewlyReacquiredLock() {
	now := time.Now()
	expiredAt := now.Add(-time.Minute)
	err := facades.Orm().Query().Table("module_lifecycle_lock").Create(map[string]any{
		"key":        "module-lifecycle:stale",
		"owner":      "old-worker",
		"run_key":    "old-run",
		"expires_at": expiredAt,
		"created_at": now,
		"updated_at": now,
	})
	require.NoError(s.T(), err)

	service := modulecatalog.NewAdminService(moduleboot.Modules())
	service.SetAfterStaleLockReadForTest(func(context.Context, []modulecatalog.AdminLockRow) error {
		_, updateErr := facades.Orm().Query().Table("module_lifecycle_lock").
			Where("key", "module-lifecycle:stale").
			Update(map[string]any{
				"owner":      "new-worker",
				"run_key":    "new-run",
				"expires_at": now.Add(time.Minute),
				"updated_at": now,
			})
		return updateErr
	})
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	approverToken := s.seedAndLoginPlatformApprover()
	reAuthToken, approvalID := s.moduleLifecycleSecurityEvidence(token, approverToken, "module.lifecycle.release-lock", "module-lifecycle:stale-locks:all")
	result, err := service.ReleaseStaleLocks(modulecatalog.AdminLockReleasePayload{
		ConfirmToken: "release-stale-locks",
		ReAuthToken:  reAuthToken,
		ApprovalID:   approvalID,
		OperatorID:   1,
	})
	require.NoError(s.T(), err)
	require.Empty(s.T(), result.Released)

	var lock modulecatalog.AdminLockRow
	err = facades.Orm().Query().Table("module_lifecycle_lock").
		Where("key", "module-lifecycle:stale").
		First(&lock)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "new-worker", lock.Owner)
	require.Equal(s.T(), "new-run", lock.RunKey)

	_, err = facades.Orm().Query().Table("module_lifecycle_lock").
		Where("key", "module-lifecycle:stale").
		Update(map[string]any{
			"expires_at": time.Now().Add(-time.Minute),
			"updated_at": time.Now(),
		})
	require.NoError(s.T(), err)
	service.SetAfterStaleLockReadForTest(nil)
	_, err = service.ReleaseStaleLocks(modulecatalog.AdminLockReleasePayload{
		ConfirmToken: "release-stale-locks",
		ReAuthToken:  reAuthToken,
		ApprovalID:   approvalID,
		OperatorID:   1,
	})
	require.ErrorContains(s.T(), err, "approved approval record")

	reAuthToken, approvalID = s.moduleLifecycleSecurityEvidence(token, approverToken, "module.lifecycle.release-lock", "module-lifecycle:stale-locks:all")
	retry, err := service.ReleaseStaleLocks(modulecatalog.AdminLockReleasePayload{
		ConfirmToken: "release-stale-locks",
		ReAuthToken:  reAuthToken,
		ApprovalID:   approvalID,
		OperatorID:   1,
	})
	require.NoError(s.T(), err)
	require.Len(s.T(), retry.Released, 1)
}

func (s *ModuleLifecycleTestSuite) TestPlatformSecurityControlRejectsSelfApproval() {
	s.seedPlatformAccess()
	token := s.loginAsPlatformAdmin()
	approvalID := s.createLifecycleApproval(token, "module.lifecycle.execute", "module-lifecycle:platform-rbac:upgrade")

	res, err := s.Http(s.T()).WithToken(token).Put("/admin/platform/security/approvals/"+approvalID+"/approve", strings.NewReader(`{}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "approver must differ")
}

func (s *ModuleLifecycleTestSuite) moduleLifecycleSecurityEvidence(requesterToken string, approverToken string, operation string, resource string) (string, string) {
	reAuthToken := s.issueLifecycleReAuthToken(requesterToken, operation, resource)
	approvalID := s.createLifecycleApproval(requesterToken, operation, resource)
	approve, err := s.Http(s.T()).WithToken(approverToken).Put("/admin/platform/security/approvals/"+approvalID+"/approve", strings.NewReader(`{}`))
	require.NoError(s.T(), err)
	approve.AssertOk()
	body, err := approve.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	return reAuthToken, approvalID
}

func (s *ModuleLifecycleTestSuite) issueLifecycleReAuthToken(token string, operation string, resource string) string {
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/reauth-token", strings.NewReader(fmt.Sprintf(`{
		"password": "123456",
		"operation": %q,
		"resource": %q
	}`, operation, resource)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	reAuthToken, ok := body["data"].(map[string]any)["reauth_token"].(string)
	require.True(s.T(), ok)
	require.NotEmpty(s.T(), reAuthToken)
	return reAuthToken
}

func (s *ModuleLifecycleTestSuite) createLifecycleApproval(token string, operation string, resource string) string {
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/approvals", strings.NewReader(fmt.Sprintf(`{
		"scope": %q,
		"resource": %q,
		"reason": "feature test"
	}`, operation, resource)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	approvalID, ok := body["data"].(map[string]any)["approval_id"].(string)
	require.True(s.T(), ok)
	require.NotEmpty(s.T(), approvalID)
	return approvalID
}

func (s *ModuleLifecycleTestSuite) seedPlatformAccess() {
	require.NoError(s.T(), (&seeders.PlatformAdminSeeder{}).Run())
	require.NoError(s.T(), (&seeders.PlatformMenuSeeder{}).Run())
	require.NoError(s.T(), (&seeders.PlatformCasbinSeeder{}).Run())
}

func (s *ModuleLifecycleTestSuite) loginAsPlatformAdmin() string {
	result, err := services.NewPlatformPassportService().Login("admin", "123456")
	require.NoError(s.T(), err)
	return result.AccessToken
}

func (s *ModuleLifecycleTestSuite) seedAndLoginPlatformApprover() string {
	now := time.Now()
	err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_user").Create(map[string]any{
		"id":              2,
		"username":        "approval_admin",
		"password":        "$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe",
		"user_type":       "900",
		"nickname":        "审批管理员",
		"status":          1,
		"dashboard":       "platform:tenant",
		"backend_setting": "{}",
		"created_at":      now,
		"updated_at":      now,
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_user_belongs_role").Create(map[string]any{
		"user_id":    2,
		"role_id":    1,
		"created_at": now,
		"updated_at": now,
	})
	require.NoError(s.T(), err)
	result, err := services.NewPlatformPassportService().Login("approval_admin", "123456")
	require.NoError(s.T(), err)
	return result.AccessToken
}
