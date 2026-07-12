package admin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests"
)

type TenantPlatformTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestTenantPlatformTestSuite(t *testing.T) {
	suite.Run(t, new(TenantPlatformTestSuite))
}

func (s *TenantPlatformTestSuite) SetupTest() {
	s.RefreshDatabase()
	_ = facades.Cache().Flush()
	services.ResetEnterpriseSecurityControlForTest()
	services.ResetCasbinEnforcerCacheForTest()
	services.ResetTenantConnectionRegistryForTest()
	s.Seed(&seeders.TenantPlanSeeder{})
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.DictionarySeeder{})
	require.NoError(s.T(), (&seeders.PlatformDictionarySeeder{}).Run())
	require.NoError(s.T(), (&seeders.PlatformAdminSeeder{}).Run())
	require.NoError(s.T(), (&seeders.PlatformMenuSeeder{}).Run())
	require.NoError(s.T(), (&seeders.PlatformCasbinSeeder{}).Run())
}

func (s *TenantPlatformTestSuite) TestPlatformTenantListRejectsMissingAdminToken() {
	res, err := s.Http(s.T()).Get("/admin/platform/tenant/list")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(401), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantListUsesPageResultShape() {
	token := s.loginAsPlatformAdmin()
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/tenant/list")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	data := body["data"].(map[string]any)
	require.Contains(s.T(), data, "list")
	require.Contains(s.T(), data, "total")
	require.GreaterOrEqual(s.T(), data["total"].(float64), float64(1))
}

func (s *TenantPlatformTestSuite) TestPlatformTenantExportRequiresBoundApproval() {
	token := s.loginAsPlatformAdmin()
	resource := "tenant-data:export:1:users:jsonl:sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	requestBody := `{"dataset":"users","format":"jsonl","filters":{}}`

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant/1/exports", strings.NewReader(requestBody))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Equal(s.T(), int64(0), s.platformTableCount("tenant_governance_run"))
	require.Equal(s.T(), int64(0), s.platformTableCount("queue_outbox"))

	approvalID := s.createApprovedBoundApproval(1, "tenant.data.export", resource)
	reauthToken := s.issuePlatformSensitiveReAuth(token, "tenant.data.export", resource)
	requestBody = fmt.Sprintf(`{"dataset":"users","format":"jsonl","filters":{},"reauth_token":%q,"approval_id":%q}`, reauthToken, approvalID)
	res, err = s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant/1/exports", strings.NewReader(requestBody))
	s.assertOK(res, err)
	body, err = res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	require.Equal(s.T(), int64(1), s.platformTableCount("tenant_governance_run"))
	require.Equal(s.T(), int64(1), s.platformTableCount("queue_outbox"))

	res, err = s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant/1/exports", strings.NewReader(requestBody))
	s.assertOK(res, err)
	require.Equal(s.T(), int64(1), s.platformTableCount("tenant_governance_run"))
	require.Equal(s.T(), int64(1), s.platformTableCount("queue_outbox"))
}

func (s *TenantPlatformTestSuite) platformTableCount(table string) int64 {
	count, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table(table).Count()
	require.NoError(s.T(), err)
	return count
}

func (s *TenantPlatformTestSuite) TestTenantAdminTokenCannotAccessPlatformTenantList() {
	token := s.loginAsTenantAdmin()
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/tenant/list")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(401), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantListRequiresPlatformPermission() {
	token := s.loginAsPlatformAuditor()
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/tenant/list")
	s.assertOK(res, err)

	createRes, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant", strings.NewReader(`{
		"code": "denied_create",
		"name": "Denied Create",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "denied_create",
		"db_schema": "public"
	}`))
	require.NoError(s.T(), err)
	createRes.AssertOk()

	body, err := createRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformDictionaryRequiresDictionaryPermission() {
	token := s.loginAsPlatformAuditor()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/dictionary", strings.NewReader(`{
		"code": "denied_dict",
		"name": "Denied Dictionary",
		"status": 1,
		"items": [
			{ "label": "One", "value": "1", "status": 1 }
		]
	}`))
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformDictionaryOptionsRequiresDictionaryPermission() {
	token := s.loginAsPlatformAuditor()
	res, err := s.Http(s.T()).
		WithToken(token).
		Get("/admin/platform/dictionary/options?code=system-status")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyPermissionCannotIssueModuleLifecycleReAuth() {
	token := s.loginAsPlatformTenantDestroyer()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/reauth-token", strings.NewReader(`{
		"password": "123456",
		"operation": "module.lifecycle.execute",
		"resource": "module-lifecycle:platform-rbac:upgrade"
	}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyPermissionCanIssueTenantDeletionReAuth() {
	token := s.loginAsPlatformTenantDestroyer()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/reauth-token", strings.NewReader(`{
		"password": "123456",
		"operation": "tenant.data.delete",
		"resource": "tenant-data:delete:1:metadata"
	}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	require.NotEmpty(s.T(), body["data"].(map[string]any)["reauth_token"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyPermissionCannotCreateModuleLifecycleApproval() {
	token := s.loginAsPlatformTenantDestroyer()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/approvals", strings.NewReader(`{
		"scope": "module.lifecycle.execute",
		"resource": "module-lifecycle:platform-rbac:upgrade",
		"reason": "cross-domain escalation"
	}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyPermissionCanApproveTenantDeletion() {
	token := s.loginAsPlatformTenantDestroyer()
	approvalID := s.createPendingPlatformApproval(99, "tenant.data.delete", "tenant-data:delete:1:metadata")

	res, err := s.Http(s.T()).WithToken(token).Put("/admin/platform/security/approvals/"+approvalID+"/approve", strings.NewReader(`{}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyPermissionCannotApproveModuleLifecycle() {
	token := s.loginAsPlatformTenantDestroyer()
	approvalID := s.createPendingPlatformApproval(99, "module.lifecycle.execute", "module-lifecycle:platform-rbac:upgrade")

	res, err := s.Http(s.T()).WithToken(token).Put("/admin/platform/security/approvals/"+approvalID+"/approve", strings.NewReader(`{}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformApprovalRejectsUnknownScopeForSuperAdmin() {
	token := s.loginAsPlatformAdmin()
	approvalID := s.createPendingPlatformApproval(99, "unknown.sensitive.scope", "unknown:resource")

	res, err := s.Http(s.T()).WithToken(token).Put("/admin/platform/security/approvals/"+approvalID+"/approve", strings.NewReader(`{}`))

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformApprovalCannotExpireBetweenReadAndUpdate() {
	service := services.NewEnterpriseSecurityControlService()
	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	record, err := service.CreatePlatformApproval(s.T().Context(), services.PlatformApprovalCreateRequest{
		RequesterID: 99,
		Scope:       "tenant.data.delete",
		Resource:    "tenant-data:delete:1:metadata",
		Reason:      "expiry race regression",
		ExpiresAt:   base.Add(time.Minute),
	})
	require.NoError(s.T(), err)

	call := 0
	restoreClock := services.SetEnterpriseSecurityNowForTest(func() time.Time {
		call++
		if call == 1 {
			return base
		}
		return base.Add(2 * time.Minute)
	})
	s.T().Cleanup(restoreClock)

	_, err = service.ApprovePlatformApproval(s.T().Context(), services.PlatformApprovalApproveRequest{
		ApprovalID: record.ApprovalID,
		ApproverID: 1,
	})

	require.ErrorIs(s.T(), err, services.ErrApprovalRequired)
	var approval struct {
		Status string `gorm:"column:status"`
	}
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").
		Where("approval_id", record.ApprovalID).First(&approval))
	require.Equal(s.T(), "pending", approval.Status)
}

func (s *TenantPlatformTestSuite) TestPlatformApprovalDetailReturnsPendingApproval() {
	token := s.loginAsPlatformAdmin()
	approvalID := s.createPendingPlatformApproval(99, "tenant.data.delete", "tenant-data:delete:1:metadata")

	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/security/approvals/" + approvalID)

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	data := body["data"].(map[string]any)
	require.Equal(s.T(), approvalID, data["approval_id"])
	require.Equal(s.T(), float64(99), data["requester_id"])
	require.Equal(s.T(), "tenant.data.delete", data["scope"])
	require.Equal(s.T(), "tenant-data:delete:1:metadata", data["resource"])
	require.Equal(s.T(), "pending", data["status"])
}

func (s *TenantPlatformTestSuite) TestPlatformApprovalDetailRejectsUnknownApproval() {
	token := s.loginAsPlatformAdmin()

	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/security/approvals/approval-missing")

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyPermissionCannotReadModuleLifecycleApproval() {
	token := s.loginAsPlatformTenantDestroyer()
	approvalID := s.createPendingPlatformApproval(99, "module.lifecycle.execute", "module-lifecycle:platform-rbac:upgrade")

	res, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/security/approvals/" + approvalID)

	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantPlanLifecycleAndTenantQuotaOverride() {
	token := s.loginAsPlatformAdmin()
	s.assertOK(s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant-plan", strings.NewReader(`{
		"code": "growth",
		"name": "成长版",
		"status": 1,
		"sort": 20,
		"quotas": {
			"api_rate_per_minute": 300,
			"max_users": 20,
			"max_roles": 8,
			"max_storage_mb": 4096
		},
		"remark": "适合成长团队"
	}`)))

	listRes, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/tenant-plan/list")
	s.assertOK(listRes, err)
	listBody, err := listRes.Json()
	require.NoError(s.T(), err)
	list := listBody["data"].(map[string]any)["list"].([]any)
	require.NotEmpty(s.T(), list)

	id := s.createTenantWithPlanAndQuotas(token, "tenant_growth", "growth", `{
		"api_rate_per_minute": 300,
		"max_users": 35,
		"max_roles": 8,
		"max_storage_mb": 8192
	}`)
	var tenant services.Tenant
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Where("id", id).
		First(&tenant)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "growth", tenant.Plan)
	require.Equal(s.T(), float64(35), tenant.Quotas["max_users"])
	require.Equal(s.T(), float64(8192), tenant.Quotas["max_storage_mb"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantPayloadCannotManageSSOProviders() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_platform_no_sso")
	payload := s.tenantJSON(`{
		"code": "tenant_platform_no_sso",
		"name": "平台不可配置 SSO",
		"plan": "standard",
		"status": 1,
		"db_host": "__TEST_DB_HOST__",
		"db_port": __TEST_DB_PORT__,
		"db_database": "__TEST_DB__",
		"db_username": "__TEST_DB_USER__",
		"db_password": "__TEST_DB_PASSWORD__",
		"db_schema": "__TEST_DB_SCHEMA__",
		"features": {
			"sso": {
				"password_login": false,
				"providers": [{
					"name": "okta",
					"type": "oidc",
					"enabled": true,
					"issuer": "https://issuer.example.test",
					"jwt_secret": "secret"
				}]
			}
		}
	}`)
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	tenant, err := services.NewTenantService().Resolve("tenant_platform_no_sso")
	require.NoError(s.T(), err)
	require.NotContains(s.T(), tenant.Features, "sso")
}

func (s *TenantPlatformTestSuite) TestPlatformTenantPayloadCannotManagePermissionSnapshot() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_platform_no_permission_bypass")

	permissionPayload := map[string]any{
		"allowed": []string{"permission", "permission:user", "permission:user:index"},
	}
	s.assertOK(s.Http(s.T()).
		WithToken(token).
		Put(fmt.Sprintf("/admin/platform/tenant/%d/permissions", id), strings.NewReader(s.boundTenantChangePayload(
			token, "tenant.permissions.sync", "permissions", id, permissionPayload,
		))))

	payload := s.tenantJSON(`{
		"code": "tenant_platform_no_permission_bypass",
		"name": "平台常规编辑不可改权限",
		"plan": "standard",
		"status": 1,
		"db_host": "__TEST_DB_HOST__",
		"db_port": __TEST_DB_PORT__,
		"db_database": "__TEST_DB__",
		"db_username": "__TEST_DB_USER__",
		"db_password": "__TEST_DB_PASSWORD__",
		"db_schema": "__TEST_DB_SCHEMA__",
		"features": {
			"permissions": {
				"allowed": ["permission:user:delete"]
			}
		}
	}`)
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	tenant, err := services.NewTenantService().Resolve("tenant_platform_no_permission_bypass")
	require.NoError(s.T(), err)
	permissions := services.TenantPermissionPayloadFromTenant(tenant)
	require.Contains(s.T(), permissions.Allowed, "permission:user:index")
	require.NotContains(s.T(), permissions.Allowed, "permission:user:delete")
}

func (s *TenantPlatformTestSuite) TestPlatformTenantPermissionUpdateWritesAudit() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_permission_audit")
	payload := map[string]any{
		"allowed": []string{"permission", "permission:user", "permission:user:index"},
	}

	res, err := s.Http(s.T()).
		WithToken(token).
		Put(fmt.Sprintf("/admin/platform/tenant/%d/permissions", id), strings.NewReader(s.boundTenantChangePayload(
			token, "tenant.permissions.sync", "permissions", id, payload,
		)))
	s.assertOK(res, err)

	var audit models.TenantPermissionAudit
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant_permission_audit").
		Where("tenant_id", id).
		Where("operation", services.TenantPermissionAuditOperationUpdate).
		First(&audit)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "tenant_permission_audit", audit.TenantCode)
	require.Equal(s.T(), services.TenantPermissionAuditSourcePlatform, audit.Source)
	require.Equal(s.T(), uint64(1), audit.OperatorID)
	require.Equal(s.T(), "admin", audit.OperatorName)
	require.Contains(s.T(), audit.AfterSnapshot["allowed"], "permission:user:index")
	require.Contains(s.T(), audit.Diff["unchanged"], "permission:user:index")
	require.Contains(s.T(), audit.Diff["removed"], "permission:role:index")
}

func (s *TenantPlatformTestSuite) TestPlatformTenantPermissionUpdateImmediatelyFiltersTenantMenus() {
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	platformToken := s.loginAsPlatformAdmin()
	tenantToken := s.loginAsTenantAdmin()

	beforeRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/permission/menus")
	s.assertOK(beforeRes, err)

	var before menuResponse
	require.NoError(s.T(), beforeRes.Bind(&before))
	require.NotNil(s.T(), findMenuItem(before.Data, "permission:role"))

	firstPayload := map[string]any{
		"allowed": []string{"permission", "permission:user", "permission:user:index"},
	}
	_, err = s.Http(s.T()).
		WithToken(platformToken).
		Put("/admin/platform/tenant/1/permissions", strings.NewReader(s.boundTenantChangePayload(
			platformToken, "tenant.permissions.sync", "permissions", 1, firstPayload,
		)))
	require.NoError(s.T(), err)

	afterRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/permission/menus")
	s.assertOK(afterRes, err)

	var after menuResponse
	require.NoError(s.T(), afterRes.Bind(&after))
	require.NotNil(s.T(), findMenuItem(after.Data, "permission"))
	require.NotNil(s.T(), findMenuItem(after.Data, "permission:user"))
	require.NotNil(s.T(), findMenuItem(after.Data, "permission:user:index"))
	require.Nil(s.T(), findMenuItem(after.Data, "permission:role"))
	require.Nil(s.T(), findMenuItem(after.Data, "permission:menu"))

	secondPayload := map[string]any{"allowed": []string{"permission:user:index"}}
	_, err = s.Http(s.T()).
		WithToken(platformToken).
		Put("/admin/platform/tenant/1/permissions", strings.NewReader(s.boundTenantChangePayload(
			platformToken, "tenant.permissions.sync", "permissions", 1, secondPayload,
		)))
	require.NoError(s.T(), err)

	legacyRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/permission/menus")
	s.assertOK(legacyRes, err)

	var legacy menuResponse
	require.NoError(s.T(), legacyRes.Bind(&legacy))
	require.Nil(s.T(), findMenuItem(legacy.Data, "permission"))
	require.Nil(s.T(), findMenuItem(legacy.Data, "permission:user"))
	require.Nil(s.T(), findMenuItem(legacy.Data, "permission:user:index"))
}

func (s *TenantPlatformTestSuite) TestPlatformTenantLegacyPermissionsReturnFullAccessPayload() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_permission_legacy_payload")

	res, err := s.Http(s.T()).
		WithToken(token).
		Get(fmt.Sprintf("/admin/platform/tenant/%d/permissions", id))
	s.assertOK(res, err)

	body, err := res.Json()
	require.NoError(s.T(), err)
	data := body["data"].(map[string]any)
	require.Contains(s.T(), data["allowed"], "permission:user:index")
}

func (s *TenantPlatformTestSuite) TestSnapshotLegacyPermissionsWritesFullSnapshotAndAudit() {
	id := s.createTenant(s.loginAsPlatformAdmin(), "tenant_legacy_snapshot")
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec("UPDATE tenant SET features = '{}'::jsonb WHERE id = ?", id)
	require.NoError(s.T(), err)

	count, err := services.NewTenantService().SnapshotLegacyPermissions(false)
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), count, 1)

	var tenant services.Tenant
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant").
		Where("id", id).
		First(&tenant)
	require.NoError(s.T(), err)
	require.True(s.T(), services.TenantAllowsPermission(tenant, "permission:user:index"))
	require.False(s.T(), services.TenantPermissionSnapshotFromFeatures(tenant.Features).LegacyFullAccess)

	var audit models.TenantPermissionAudit
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant_permission_audit").
		Where("tenant_id", id).
		Where("operation", services.TenantPermissionAuditOperationLegacySnapshot).
		First(&audit)
	require.NoError(s.T(), err)
	require.Equal(s.T(), services.TenantPermissionAuditSourceLegacyMigration, audit.Source)
	require.Contains(s.T(), audit.AfterSnapshot["allowed"], "permission:user:index")
}

func (s *TenantPlatformTestSuite) TestTenantPlanSwitchDefaultsToNewPlanPermissions() {
	token := s.loginAsPlatformAdmin()
	s.createPermissionPlan("limited", []string{"permission", "permission:user", "permission:user:index"})
	s.createPermissionPlan("roles_only", []string{"permission", "permission:role", "permission:role:index"})
	id := s.createTenant(token, "tenant_plan_permission_switch")

	permissionPayload := map[string]any{
		"allowed": []string{"permission", "permission:user", "permission:user:index"},
	}
	s.assertOK(s.Http(s.T()).
		WithToken(token).
		Put(fmt.Sprintf("/admin/platform/tenant/%d/permissions", id), strings.NewReader(s.boundTenantChangePayload(
			token, "tenant.permissions.sync", "permissions", id, permissionPayload,
		))))

	planPayload := map[string]any{"plan": "roles_only"}
	res, err := s.Http(s.T()).
		WithToken(token).
		Put(fmt.Sprintf("/admin/platform/tenant/%d/plan", id), strings.NewReader(s.boundTenantChangePayload(
			token, "tenant.plan.change", "plan", id, planPayload,
		)))
	s.assertOK(res, err)

	tenant, err := services.NewTenantService().FindByID(id)
	require.NoError(s.T(), err)
	permissions := services.TenantPermissionPayloadFromTenant(tenant)
	require.Contains(s.T(), permissions.Allowed, "permission:role:index")
	require.NotContains(s.T(), permissions.Allowed, "permission:user:index")
}

func (s *TenantPlatformTestSuite) TestTenantEditPlanSwitchDefaultsToNewPlanPermissions() {
	token := s.loginAsPlatformAdmin()
	s.createPermissionPlan("limited", []string{"permission", "permission:user", "permission:user:index"})
	s.createPermissionPlan("roles_only", []string{"permission", "permission:role", "permission:role:index"})
	id := s.createTenantWithPlanAndQuotas(token, "tenant_edit_plan_permission_switch", "limited", `{}`)

	payload := s.tenantJSON(`{
		"code": "tenant_edit_plan_permission_switch",
		"name": "普通编辑套餐权限切换",
		"plan": "limited",
		"status": 1,
		"db_host": "__TEST_DB_HOST__",
		"db_port": __TEST_DB_PORT__,
		"db_database": "__TEST_DB__",
		"db_username": "__TEST_DB_USER__",
		"db_password": "__TEST_DB_PASSWORD__",
		"db_schema": "__TEST_DB_SCHEMA__"
	}`)
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))
	planPayload := map[string]any{"plan": "roles_only"}
	s.assertOK(s.Http(s.T()).WithToken(token).Put(
		fmt.Sprintf("/admin/platform/tenant/%d/plan", id),
		strings.NewReader(s.boundTenantChangePayload(token, "tenant.plan.change", "plan", id, planPayload)),
	))

	tenant, err := services.NewTenantService().FindByID(id)
	require.NoError(s.T(), err)
	permissions := services.TenantPermissionPayloadFromTenant(tenant)
	require.Contains(s.T(), permissions.Allowed, "permission:role:index")
	require.NotContains(s.T(), permissions.Allowed, "permission:user:index")
}

func (s *TenantPlatformTestSuite) TestTenantPlanDiffExpandsLegacyFullAccessBaseline() {
	token := s.loginAsPlatformAdmin()
	s.createPermissionPlan("limited", []string{"permission", "permission:user", "permission:user:index"})
	id := s.createTenant(token, "tenant_plan_diff_legacy")

	res, err := s.Http(s.T()).
		WithToken(token).
		Post(fmt.Sprintf("/admin/platform/tenant/%d/permissions/plan-diff", id), strings.NewReader(`{
			"plan": "limited"
		}`))
	s.assertOK(res, err)

	body, err := res.Json()
	require.NoError(s.T(), err)
	data := body["data"].(map[string]any)
	require.Contains(s.T(), data["unchanged"], "permission:user:index")
	require.Contains(s.T(), data["removed"], "permission:user:delete")
	require.Empty(s.T(), data["added"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantSensitiveMutationsRequireEvidence() {
	token := s.loginAsPlatformAdmin()

	s.Run("permissions", func() {
		id := s.createTenant(token, "tenant_sensitive_permissions")
		res, err := s.Http(s.T()).WithToken(token).Put(
			fmt.Sprintf("/admin/platform/tenant/%d/permissions", id),
			strings.NewReader(`{"allowed":["permission","permission:user","permission:user:index"]}`),
		)
		s.assertSensitiveEvidenceRejected(res, err)

		tenant, err := services.NewTenantService().FindByID(id)
		require.NoError(s.T(), err)
		require.True(s.T(), services.TenantAllowsPermission(tenant, "permission:role:index"))
	})

	s.Run("plan", func() {
		s.createPermissionPlan("sensitive_plan", []string{"permission", "permission:user", "permission:user:index"})
		id := s.createTenant(token, "tenant_sensitive_plan")
		res, err := s.Http(s.T()).WithToken(token).Put(
			fmt.Sprintf("/admin/platform/tenant/%d/plan", id),
			strings.NewReader(`{"plan":"sensitive_plan"}`),
		)
		s.assertSensitiveEvidenceRejected(res, err)
		s.assertTenantField(id, "plan", "standard")
	})

	s.Run("governance", func() {
		id := s.createTenant(token, "tenant_sensitive_governance")
		res, err := s.Http(s.T()).WithToken(token).Put(
			fmt.Sprintf("/admin/platform/tenant/%d/governance", id),
			strings.NewReader(`{"modules":{"scheduled-task":false}}`),
		)
		s.assertSensitiveEvidenceRejected(res, err)

		count, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("tenant_governance").
			Where("tenant_id", id).Count()
		require.NoError(s.T(), err)
		require.Equal(s.T(), int64(0), count)
	})

	s.Run("status", func() {
		id := s.createTenant(token, "tenant_sensitive_status")
		res, err := s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d/suspend", id), nil)
		s.assertSensitiveEvidenceRejected(res, err)
		s.assertTenantStatus(id, services.TenantStatusActive)
	})
}

func (s *TenantPlatformTestSuite) TestPlatformTenantPermissionChangeAcceptsBoundEvidenceOnce() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_sensitive_success")
	desired := services.TenantPermissionPayload{Allowed: []string{"permission", "permission:user", "permission:user:index"}}
	selector := tenantChangeSelector(s.T(), "permissions", id, desired)
	approvalID := s.createApprovedBoundApproval(1, "tenant.permissions.sync", selector)
	reauthToken := s.issuePlatformSensitiveReAuth(token, "tenant.permissions.sync", selector)
	payload := fmt.Sprintf(`{"allowed":["permission","permission:user","permission:user:index"],"reauth_token":%q,"approval_id":%q}`, reauthToken, approvalID)

	res, err := s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d/permissions", id), strings.NewReader(payload))
	s.assertOK(res, err)
	tenant, err := services.NewTenantService().FindByID(id)
	require.NoError(s.T(), err)
	require.True(s.T(), services.TenantAllowsPermission(tenant, "permission:user:index"))

	res, err = s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d/permissions", id), strings.NewReader(payload))
	s.assertSensitiveEvidenceRejected(res, err)
}

func (s *TenantPlatformTestSuite) TestTenantPlanEditorCanLoadPermissionCatalog() {
	token := s.loginAsPlatformPlanEditor()

	res, err := s.Http(s.T()).
		WithToken(token).
		Get("/admin/platform/tenant/permission-catalog")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
}

func (s *TenantPlatformTestSuite) TestDictionaryTemplateDispatchKeepsTenantDisplayOverrides() {
	token := s.loginAsPlatformAdmin()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/dictionary", strings.NewReader(`{
		"code": "priority",
		"name": "优先级",
		"status": 1,
		"sort": 60,
		"items": [
			{ "label": "高", "value": "high", "color": "danger", "status": 1, "sort": 10 },
			{ "label": "低", "value": "low", "color": "info", "status": 1, "sort": 20 }
		]
	}`))
	s.assertOK(res, err)

	s.assertOK(s.Http(s.T()).WithToken(token).Post("/admin/platform/dictionary/dispatch", nil))

	tenantToken := s.loginAsTenantAdmin()
	optionsRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/dictionary/options?code=priority")
	s.assertOK(optionsRes, err)
	optionsBody, err := optionsRes.Json()
	require.NoError(s.T(), err)
	options := optionsBody["data"].([]any)
	require.Len(s.T(), options, 2)
	require.Equal(s.T(), "high", options[0].(map[string]any)["value"])

	var itemID uint64
	err = facades.Orm().
		Query().
		Table("dict_item").
		Where("type_code", "priority").
		Where("value", "high").
		Pluck("id", &itemID)
	require.NoError(s.T(), err)
	require.NotZero(s.T(), itemID)

	s.assertOK(s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Put(fmt.Sprintf("/admin/dictionary-item/%d", itemID), strings.NewReader(`{
			"label": "紧急",
			"value": "changed",
			"color": "warning",
			"status": 1,
			"sort": 5
		}`)))

	var label string
	err = facades.Orm().Query().Table("dict_item").Where("id", itemID).Pluck("label", &label)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "紧急", label)
	var value string
	err = facades.Orm().Query().Table("dict_item").Where("id", itemID).Pluck("value", &value)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "high", value)

	s.assertOK(s.Http(s.T()).WithToken(token).Post("/admin/platform/dictionary/dispatch", nil))
	err = facades.Orm().Query().Table("dict_item").Where("id", itemID).Pluck("label", &label)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "紧急", label)
}

func (s *TenantPlatformTestSuite) TestDictionaryOptionsKeepNumericValuesForBuiltinDictionaries() {
	tenantToken := s.loginAsTenantAdmin()
	optionsRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/dictionary/options?code=system-status")
	s.assertOK(optionsRes, err)

	body, err := optionsRes.Json()
	require.NoError(s.T(), err)
	options := body["data"].([]any)
	require.Len(s.T(), options, 2)
	require.Equal(s.T(), float64(1), options[0].(map[string]any)["value"])
}

func (s *TenantPlatformTestSuite) TestDictionaryTemplateDeleteFailsWhenTenantLookupFails() {
	token := s.loginAsPlatformAdmin()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/dictionary", strings.NewReader(`{
		"code": "guard_delete",
		"name": "删除保护",
		"status": 1,
		"items": [
			{ "label": "启用", "value": "enabled", "status": 1 }
		]
	}`))
	s.assertOK(res, err)

	body, err := res.Json()
	require.NoError(s.T(), err)
	data := body["data"].(map[string]any)
	id := uint64(data["id"].(float64))

	_, err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant").
		Where("code", "default").
		Update(map[string]any{
			"code":        "tenant_lookup_failure",
			"db_database": "missing_tenant_database",
		})
	require.NoError(s.T(), err)

	deleteRes, err := s.Http(s.T()).
		WithToken(token).
		Delete("/admin/platform/dictionary", strings.NewReader(fmt.Sprintf(`[%d]`, id)))
	require.NoError(s.T(), err)
	deleteRes.AssertOk()
	deleteBody, err := deleteRes.Json()
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), float64(200), deleteBody["code"])

	var count int64
	count, err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_dict_type").
		Where("id", id).
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantRejectsUnknownPlan() {
	token := s.loginAsPlatformAdmin()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant", strings.NewReader(`{
		"code": "unknown_plan",
		"name": "未知套餐租户",
		"plan": "missing",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "unknown_plan",
		"db_schema": "public"
	}`))
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
}

func (s *TenantPlatformTestSuite) TestTenantUsesPlanQuotaWhenTenantQuotaIsEmpty() {
	token := s.loginAsPlatformAdmin()
	s.assertOK(s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant-plan", strings.NewReader(`{
		"code": "single_user",
		"name": "单用户版",
		"status": 1,
		"sort": 30,
		"quotas": {
			"max_users": 1
		}
	}`)))

	id := s.createTenantWithPlanAndQuotas(token, "tenant_plan_quota", "single_user", `{}`)
	payload := s.tenantJSON(`{
		"code": "tenant_plan_quota",
		"name": "套餐配额生效租户",
		"plan": "single_user",
		"status": 1,
		"db_host": "__TEST_DB_HOST__",
		"db_port": __TEST_DB_PORT__,
		"db_database": "__TEST_DB__",
		"db_username": "__TEST_DB_USER__",
		"db_password": "__TEST_DB_PASSWORD__",
		"db_schema": "__TEST_DB_SCHEMA__",
		"quotas": {}
	}`)
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	tenant, err := services.NewTenantService().Resolve("tenant_plan_quota")
	require.NoError(s.T(), err)
	conn := services.RegisterTenantConnection(tenant)
	_, err = facades.Orm().Connection(conn).Query().Exec(`
		INSERT INTO "user" (
			id, username, password, user_type, nickname, status, backend_setting,
			created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (1, 'admin', '$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe',
			'100', '管理员', 1, '{}'::jsonb, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
		ON CONFLICT (id) DO NOTHING
	`)
	require.NoError(s.T(), err)

	adminToken := s.loginAsTenant("tenant_plan_quota")
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "tenant_plan_quota").
		WithToken(adminToken).
		Post("/admin/user", strings.NewReader(`{
			"username": "blocked_by_plan",
			"password": "123456",
			"user_type": "100",
			"nickname": "Blocked By Plan",
			"status": 1
		}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(429), body["code"])
}

func (s *TenantPlatformTestSuite) TestPlatformTenantLifecycleUpdateStatusAndDestroy() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_lifecycle")
	s.createPermissionPlan("enterprise", []string{"permission", "permission:user", "permission:user:index"})

	payload := `{
		"code": "tenant_lifecycle",
		"name": "生命周期租户更新",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "tenant_lifecycle_updated",
		"db_username": "tenant_user",
		"db_password": "tenant_secret",
		"db_schema": "public",
		"custom_domain": "tenant-lifecycle.example.test",
		"remark": "updated"
	}`
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))
	s.assertTenantField(id, "name", "生命周期租户更新")
	planPayload := map[string]any{"plan": "enterprise"}
	s.assertOK(s.Http(s.T()).WithToken(token).Put(
		fmt.Sprintf("/admin/platform/tenant/%d/plan", id),
		strings.NewReader(s.boundTenantChangePayload(token, "tenant.plan.change", "plan", id, planPayload)),
	))
	s.assertTenantField(id, "plan", "enterprise")
	s.assertTenantField(id, "db_database", "tenant_lifecycle_updated")

	s.assertOK(s.Http(s.T()).WithToken(token).Put(
		fmt.Sprintf("/admin/platform/tenant/%d/suspend", id),
		strings.NewReader(s.boundTenantChangePayload(token, "tenant.status.change", "status", id, services.TenantStatusSuspended)),
	))
	s.assertTenantStatus(id, 2)
	_, err := services.NewTenantService().Resolve("tenant_lifecycle")
	require.ErrorIs(s.T(), err, services.ErrTenantSuspended)

	s.assertOK(s.Http(s.T()).WithToken(token).Put(
		fmt.Sprintf("/admin/platform/tenant/%d/resume", id),
		strings.NewReader(s.boundTenantChangePayload(token, "tenant.status.change", "status", id, services.TenantStatusActive)),
	))
	s.assertTenantStatus(id, 1)
	_, err = services.NewTenantService().Resolve("tenant_lifecycle")
	require.NoError(s.T(), err)

	s.assertOK(s.Http(s.T()).WithToken(token).Put(
		fmt.Sprintf("/admin/platform/tenant/%d/archive", id),
		strings.NewReader(s.boundTenantChangePayload(token, "tenant.status.change", "status", id, services.TenantStatusArchived)),
	))
	s.assertTenantStatus(id, 3)
	_, err = services.NewTenantService().Resolve("tenant_lifecycle")
	require.ErrorIs(s.T(), err, services.ErrTenantSuspended)

	reAuthToken := s.issueTenantDeletionReAuth(token, false, id)
	approvalID := s.approvedTenantDeletion(token, false, id)
	s.assertOK(s.Http(s.T()).WithToken(token).Delete("/admin/platform/tenant", strings.NewReader(fmt.Sprintf(`{
		"ids": [%d],
		"confirm_code": "tenant_lifecycle",
		"reauth_token": %q,
		"approval_id": %q
	}`, id, reAuthToken, approvalID))))

	total, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant").
		Where("id", id).
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(0), total)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyRequiresBoundApproval() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_destroy_approval")
	reAuthToken := s.issueTenantDeletionReAuth(token, false, id)

	res, err := s.Http(s.T()).WithToken(token).Delete("/admin/platform/tenant", strings.NewReader(fmt.Sprintf(`{
		"ids": [%d],
		"confirm_code": "tenant_destroy_approval",
		"reauth_token": %q,
		"approval_id": "approval-missing"
	}`, id, reAuthToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "approval")

	total, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("tenant").Where("id", id).Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), total)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyBindsDatabaseDeletionMode() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_destroy_mode")
	reAuthToken := s.issueTenantDeletionReAuth(token, true, id)
	metadataApprovalID := s.approvedTenantDeletion(token, false, id)

	res, err := s.Http(s.T()).WithToken(token).Delete("/admin/platform/tenant", strings.NewReader(fmt.Sprintf(`{
		"ids": [%d],
		"confirm_code": "tenant_destroy_mode",
		"drop_database": true,
		"reauth_token": %q,
		"approval_id": %q
	}`, id, reAuthToken, metadataApprovalID)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "approval")

	total, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("tenant").Where("id", id).Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), total)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyConsumesEvidenceWhenDatabaseDropFails() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_destroy_retry")
	reAuthToken := s.issueTenantDeletionReAuth(token, true, id)
	approvalID := s.approvedTenantDeletion(token, true, id)

	res, err := s.Http(s.T()).WithToken(token).Delete("/admin/platform/tenant", strings.NewReader(fmt.Sprintf(`{
		"ids": [%d],
		"confirm_code": "tenant_destroy_retry",
		"drop_database": true,
		"reauth_token": %q,
		"approval_id": %q
	}`, id, reAuthToken, approvalID)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "拒绝删除平台数据库")

	unusedApprovals, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").
		Where("approval_id", approvalID).
		WhereNull("used_at").
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(0), unusedApprovals)
	resource := tenantDeletionResource([]uint64{id}, true)
	require.Error(s.T(), services.NewEnterpriseSecurityControlService().ValidateSensitiveOperationWithApproval(
		s.T().Context(),
		services.SensitiveOperationRequest{
			UserID:      1,
			Operation:   "tenant.data.delete",
			Resource:    resource,
			ReAuthToken: reAuthToken,
		},
		approvalID,
		1,
		"tenant.data.delete",
		resource,
	))
}

func (s *TenantPlatformTestSuite) TestPlatformTenantDestroyRejectsBatchDatabaseDeletionBeforeConsumingApproval() {
	token := s.loginAsPlatformAdmin()
	firstID := s.createTenant(token, "tenant_destroy_batch_one")
	secondID := s.createTenant(token, "tenant_destroy_batch_two")
	reAuthToken := s.issueTenantDeletionReAuth(token, true, firstID, secondID)
	approvalID := s.approvedTenantDeletion(token, true, firstID, secondID)

	res, err := s.Http(s.T()).WithToken(token).Delete("/admin/platform/tenant", strings.NewReader(fmt.Sprintf(`{
		"ids": [%d, %d],
		"confirm_code": "tenant_destroy_batch_one",
		"drop_database": true,
		"reauth_token": %q,
		"approval_id": %q
	}`, firstID, secondID, reAuthToken, approvalID)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "物理库删除仅支持单租户")

	unusedApprovals, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").
		Where("approval_id", approvalID).
		WhereNull("used_at").
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), unusedApprovals)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantUpdateKeepsPasswordWhenOmitted() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_password")

	payload := `{
		"code": "tenant_password",
		"name": "密码保留租户",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "tenant_password",
		"db_username": "tenant_user",
		"db_schema": "public"
	}`
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	var password string
	err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant").
		Where("id", id).
		Pluck("db_password", &password)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "tenant_secret", password)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantUpdateStoresEnterpriseSettings() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_enterprise")
	s.createPermissionPlan("enterprise", []string{"permission", "permission:user", "permission:user:index"})

	payload := `{
		"code": "tenant_enterprise",
		"name": "企业能力租户",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "tenant_enterprise",
		"db_username": "tenant_user",
		"db_schema": "public",
		"custom_domain": "enterprise.example.test",
		"billing": {
			"subscription_status": "active",
			"currency": "CNY",
			"amount_cents": 19900,
			"expires_at": "2099-01-01T00:00:00Z"
		},
		"quotas": {
			"api_rate_per_minute": 120,
			"max_users": 50,
			"max_roles": 10,
			"max_storage_mb": 2048
		},
		"branding": {
			"app_name": "Acme Console",
			"logo_url": "https://cdn.example.test/acme.svg",
			"primary_color": "#246BFE",
			"mail_from_name": "Acme Ops"
		}
	}`
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))
	planPayload := map[string]any{"plan": "enterprise"}
	s.assertOK(s.Http(s.T()).WithToken(token).Put(
		fmt.Sprintf("/admin/platform/tenant/%d/plan", id),
		strings.NewReader(s.boundTenantChangePayload(token, "tenant.plan.change", "plan", id, planPayload)),
	))

	var tenant services.Tenant
	err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Where("id", id).
		First(&tenant)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "active", tenant.Billing["subscription_status"])
	require.Equal(s.T(), float64(120), tenant.Quotas["api_rate_per_minute"])
	require.Equal(s.T(), "Acme Console", tenant.Branding["app_name"])
	require.Equal(s.T(), "enterprise.example.test", *tenant.CustomDomain)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantUsageReportsBillingQuotasAndUsage() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_usage")

	payload := s.tenantJSON(`{
		"code": "tenant_usage",
		"name": "使用量租户",
		"plan": "standard",
		"status": 1,
		"db_host": "__TEST_DB_HOST__",
		"db_port": __TEST_DB_PORT__,
		"db_database": "__TEST_DB__",
		"db_username": "__TEST_DB_USER__",
		"db_password": "__TEST_DB_PASSWORD__",
		"db_schema": "__TEST_DB_SCHEMA__",
		"billing": { "subscription_status": "active" },
		"quotas": { "max_users": 10, "max_storage_mb": 1024 }
	}`)
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	res, err := s.Http(s.T()).WithToken(token).Get(fmt.Sprintf("/admin/platform/tenant/%d/usage", id))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "tenant_usage", data["code"])
	require.Equal(s.T(), "standard", data["plan"])
	require.Equal(s.T(), "active", data["billing"].(map[string]any)["subscription_status"])
	require.Equal(s.T(), float64(10), data["quotas"].(map[string]any)["max_users"])
	require.Contains(s.T(), data["usage"].(map[string]any), "users")
	require.Contains(s.T(), data["usage"].(map[string]any), "storage_mb")
}

func (s *TenantPlatformTestSuite) TestPlatformAdminCanManageTenantGovernance() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenantWithPlanAndQuotas(token, "tenant_governance", "standard", `{
		"api_rate_per_minute": 90,
		"max_users": 12
	}`)

	updatePayload := s.tenantJSON(`{
		"code": "tenant_governance",
		"name": "治理租户",
		"plan": "standard",
		"status": 1,
		"db_host": "__TEST_DB_HOST__",
		"db_port": __TEST_DB_PORT__,
		"db_database": "__TEST_DB__",
		"db_username": "__TEST_DB_USER__",
		"db_password": "__TEST_DB_PASSWORD__",
		"db_schema": "__TEST_DB_SCHEMA__",
		"quotas": {
			"api_rate_per_minute": 90,
			"max_users": 12
		},
		"features": {
			"modules": {
				"scheduled-task": true
			}
		}
	}`)
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(updatePayload)))

	getRes, err := s.Http(s.T()).
		WithToken(token).
		Get(fmt.Sprintf("/admin/platform/tenant/%d/governance", id))
	s.assertOK(getRes, err)
	getBody, err := getRes.Json()
	require.NoError(s.T(), err)
	data := getBody["data"].(map[string]any)
	require.Equal(s.T(), "tenant_governance", data["tenant_code"])
	require.Equal(s.T(), float64(90), data["rate_limit"].(map[string]any)["per_minute"])
	require.True(s.T(), data["modules"].(map[string]any)["scheduled-task"].(bool))

	governancePayload := map[string]any{
		"modules": map[string]bool{
			"scheduled-task": false,
			"security":       true,
		},
		"quotas": map[string]any{
			"api_rate_per_minute": 180,
			"max_users":           24,
		},
		"rate_limit": map[string]any{"per_minute": 180},
		"retention":  map[string]any{"audit_days": 400, "data_days": 800},
		"data_export": map[string]any{
			"enabled": true, "requires_approval": true,
		},
		"data_deletion": map[string]any{
			"enabled": false, "requires_approval": true,
		},
		"isolation_proof": map[string]any{
			"verified": true, "evidence": "s3://worm/tenant-governance/isolation.json", "digest": "sha256:tenant-governance",
		},
	}
	putRes, err := s.Http(s.T()).
		WithToken(token).
		Put(fmt.Sprintf("/admin/platform/tenant/%d/governance", id), strings.NewReader(s.boundTenantChangePayload(
			token, "tenant.governance.change", "governance", id, governancePayload,
		)))
	s.assertOK(putRes, err)
	putBody, err := putRes.Json()
	require.NoError(s.T(), err)
	updated := putBody["data"].(map[string]any)
	require.False(s.T(), updated["modules"].(map[string]any)["scheduled-task"].(bool))
	require.True(s.T(), updated["modules"].(map[string]any)["security"].(bool))
	require.Equal(s.T(), float64(800), updated["retention"].(map[string]any)["data_days"])
	require.False(s.T(), updated["data_deletion"].(map[string]any)["enabled"].(bool))
	require.Equal(s.T(), "sha256:tenant-governance", updated["isolation_proof"].(map[string]any)["digest"])

	total, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant_governance").
		Where("tenant_id", id).
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), total)
}

func (s *TenantPlatformTestSuite) TestPlatformTenantCreateAllowsEmptyCredentialsBeforeProvisioning() {
	token := s.loginAsPlatformAdmin()
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant", strings.NewReader(`{
		"code": "auto_credentials",
		"name": "自动凭证租户",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "auto_credentials",
		"db_schema": "public",
		"initialize": false
	}`))
	s.assertOK(res, err)

	body, err := res.Json()
	require.NoError(s.T(), err)
	data := body["data"].(map[string]any)
	id := uint64(data["id"].(float64))

	var tenant services.Tenant
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Where("id", id).
		First(&tenant)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "", tenant.DBUsername)
	require.Equal(s.T(), "", tenant.DBPassword)

	prepared, err := services.ApplyPostgresProvisionDefaults(tenant)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "tenant_auto_credentials", prepared.DBUsername)
	require.NotEmpty(s.T(), prepared.DBPassword)
}

func (s *TenantPlatformTestSuite) TestTenantBrandingResolvesByCustomDomain() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_branding")
	s.createPermissionPlan("enterprise", []string{"permission", "permission:user", "permission:user:index"})
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(`{
		"code": "tenant_branding",
		"name": "品牌租户",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "tenant_branding",
		"db_username": "tenant_user",
		"db_schema": "public",
		"custom_domain": "brand.example.test",
		"branding": {
			"app_name": "Brand Console",
			"logo_url": "https://cdn.example.test/brand.svg",
			"primary_color": "#19A974"
		}
	}`)))
	planPayload := map[string]any{"plan": "enterprise"}
	s.assertOK(s.Http(s.T()).WithToken(token).Put(
		fmt.Sprintf("/admin/platform/tenant/%d/plan", id),
		strings.NewReader(s.boundTenantChangePayload(token, "tenant.plan.change", "plan", id, planPayload)),
	))

	res, err := s.Http(s.T()).
		WithHeader("Host", "brand.example.test").
		Get("/admin/passport/branding")
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "tenant_branding", data["code"])
	require.Equal(s.T(), "Brand Console", data["branding"].(map[string]any)["app_name"])
	require.Equal(s.T(), "https://cdn.example.test/brand.svg", data["branding"].(map[string]any)["logo_url"])
}

func (s *TenantPlatformTestSuite) TestBillingPastDueBlocksTenantRequests() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_past_due")

	payload := `{
		"code": "tenant_past_due",
		"name": "欠费租户",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "tenant_past_due",
		"db_username": "tenant_user",
		"db_schema": "public",
		"billing": { "subscription_status": "past_due" }
	}`
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "tenant_past_due").
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
		)
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(423), body["code"])
}

func (s *TenantPlatformTestSuite) TestTenantApiRateQuotaIsEnforced() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_rate_limit")

	payload := `{
		"code": "tenant_rate_limit",
		"name": "限流租户",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": "tenant_rate_limit",
		"db_username": "tenant_user",
		"db_schema": "public",
		"quotas": { "api_rate_per_minute": 1 }
	}`
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	first, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "tenant_rate_limit").
		Get("/admin/passport/branding")
	require.NoError(s.T(), err)
	first.AssertOk()
	firstBody, err := first.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), firstBody["code"])

	second, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "tenant_rate_limit").
		Get("/admin/passport/branding")
	require.NoError(s.T(), err)
	second.AssertOk()
	secondBody, err := second.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(429), secondBody["code"])
}

func (s *TenantPlatformTestSuite) TestTenantGovernanceModuleSwitchBlocksRuntimeRoute() {
	tenantToken := s.loginAsTenantAdmin()

	listBefore, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/attachment/list")
	s.assertOK(listBefore, err)

	disabled := false
	_, err = services.NewTenantGovernanceService().
		PatchPolicy(services.Tenant{ID: 1, Code: "default"}, services.TenantGovernancePatch{
			Modules: &map[string]bool{"data-center": disabled},
		})
	require.NoError(s.T(), err)

	listAfter, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/attachment/list")
	require.NoError(s.T(), err)
	listAfter.AssertOk()
	body, err := listAfter.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), body["code"])
}

func (s *TenantPlatformTestSuite) TestTenantGovernanceModuleSwitchBlocksSecurityAndRBACRoutes() {
	tenantToken := s.loginAsTenantAdmin()

	disabled := false
	_, err := services.NewTenantGovernanceService().
		PatchPolicy(services.Tenant{ID: 1, Code: "default"}, services.TenantGovernancePatch{
			Modules: &map[string]bool{
				"security":    disabled,
				"tenant-rbac": disabled,
			},
		})
	require.NoError(s.T(), err)

	mfa, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Post("/admin/security/mfa/setup", strings.NewReader(`{}`))
	require.NoError(s.T(), err)
	mfa.AssertOk()
	mfaBody, err := mfa.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), mfaBody["code"])

	menus, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(tenantToken).
		Get("/admin/permission/menus")
	require.NoError(s.T(), err)
	menus.AssertOk()
	menusBody, err := menus.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), menusBody["code"])
}

func (s *TenantPlatformTestSuite) TestTenantUserQuotaIsEnforcedOnCreate() {
	token := s.loginAsPlatformAdmin()
	id := s.createTenant(token, "tenant_user_quota")

	payload := s.tenantJSON(`{
		"code": "tenant_user_quota",
		"name": "用户配额租户",
		"plan": "standard",
		"status": 1,
		"db_host": "__TEST_DB_HOST__",
		"db_port": __TEST_DB_PORT__,
		"db_database": "__TEST_DB__",
		"db_username": "__TEST_DB_USER__",
		"db_password": "__TEST_DB_PASSWORD__",
		"db_schema": "__TEST_DB_SCHEMA__",
		"quotas": { "max_users": 1 }
	}`)
	s.assertOK(s.Http(s.T()).WithToken(token).Put(fmt.Sprintf("/admin/platform/tenant/%d", id), strings.NewReader(payload)))

	tenant, err := services.NewTenantService().Resolve("tenant_user_quota")
	require.NoError(s.T(), err)
	conn := services.RegisterTenantConnection(tenant)
	_, err = facades.Orm().Connection(conn).Query().Exec(`
		INSERT INTO "user" (
			id, username, password, user_type, nickname, status, backend_setting,
			created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (1, 'admin', '$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe',
			'100', '管理员', 1, '{}'::jsonb, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
		ON CONFLICT (id) DO NOTHING
	`)
	require.NoError(s.T(), err)

	adminToken := s.loginAsTenant("tenant_user_quota")
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "tenant_user_quota").
		WithToken(adminToken).
		Post("/admin/user", strings.NewReader(`{
			"username": "quota_user",
			"password": "123456",
			"user_type": "100",
			"nickname": "Quota User",
			"status": 1
		}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(429), body["code"])
}

func (s *TenantPlatformTestSuite) loginAsTenant(code string) string {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", code).
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
		)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

func (s *TenantPlatformTestSuite) loginAsPlatformAdmin() string {
	res, err := s.Http(s.T()).
		Post(
			"/admin/platform/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
		)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

func (s *TenantPlatformTestSuite) issueTenantDeletionReAuth(token string, dropDatabase bool, ids ...uint64) string {
	resource := tenantDeletionResource(ids, dropDatabase)
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/reauth-token", strings.NewReader(fmt.Sprintf(`{
		"password": "123456",
		"operation": "tenant.data.delete",
		"resource": %q
	}`, resource)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	return body["data"].(map[string]any)["reauth_token"].(string)
}

func (s *TenantPlatformTestSuite) issuePlatformSensitiveReAuth(token, policyKey, resource string) string {
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/reauth-token", strings.NewReader(fmt.Sprintf(`{
		"password":"123456","operation":%q,"resource":%q
	}`, policyKey, resource)))
	s.assertOK(res, err)
	body, err := res.Json()
	require.NoError(s.T(), err)
	return body["data"].(map[string]any)["reauth_token"].(string)
}

func (s *TenantPlatformTestSuite) boundTenantChangePayload(token, policyKey, kind string, tenantID uint64, desired any) string {
	selector := tenantChangeSelector(s.T(), kind, tenantID, desired)
	approvalID := s.createApprovedBoundApproval(1, policyKey, selector)
	reauthToken := s.issuePlatformSensitiveReAuth(token, policyKey, selector)
	body := make(map[string]json.RawMessage)
	if kind != "status" {
		payload, err := json.Marshal(desired)
		require.NoError(s.T(), err)
		require.NoError(s.T(), json.Unmarshal(payload, &body))
	}
	body["reauth_token"] = json.RawMessage(fmt.Sprintf("%q", reauthToken))
	body["approval_id"] = json.RawMessage(fmt.Sprintf("%q", approvalID))
	payload, err := json.Marshal(body)
	require.NoError(s.T(), err)
	return string(payload)
}

func (s *TenantPlatformTestSuite) createApprovedBoundApproval(requesterID uint64, policyKey, resource string) string {
	record, err := services.NewEnterpriseSecurityControlService().CreatePlatformApproval(s.T().Context(), services.PlatformApprovalCreateRequest{
		RequesterID: requesterID, PolicyKey: policyKey, Resource: resource, Reason: "feature bound evidence",
	})
	require.NoError(s.T(), err)
	result, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").
		Where("approval_id", record.ApprovalID).Where("status", "pending").Update(map[string]any{
		"approver_id": 99, "status": "approved", "updated_at": time.Now(),
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), result.RowsAffected)
	return record.ApprovalID
}

func tenantChangeSelector(t *testing.T, kind string, tenantID uint64, desired any) string {
	t.Helper()
	desiredJSON, err := json.Marshal(normalizeTenantChangeSelector(t, kind, desired))
	require.NoError(t, err)
	payload, err := json.Marshal(struct {
		TenantID uint64          `json:"tenant_id"`
		Desired  json.RawMessage `json:"desired"`
	}{TenantID: tenantID, Desired: desiredJSON})
	require.NoError(t, err)
	return "tenant-change:" + kind + ":" + base64.RawURLEncoding.EncodeToString(payload)
}

func normalizeTenantChangeSelector(t *testing.T, kind string, desired any) any {
	t.Helper()
	payload, err := json.Marshal(desired)
	require.NoError(t, err)

	switch kind {
	case "permissions":
		var result services.TenantPermissionPayload
		require.NoError(t, json.Unmarshal(payload, &result))
		return result
	case "plan":
		var result services.TenantPlanUpdatePayload
		require.NoError(t, json.Unmarshal(payload, &result))
		return result
	case "governance":
		var result services.TenantGovernancePatch
		require.NoError(t, json.Unmarshal(payload, &result))
		return result
	case "status":
		var result int8
		require.NoError(t, json.Unmarshal(payload, &result))
		return result
	default:
		return desired
	}
}

func (s *TenantPlatformTestSuite) approvedTenantDeletion(token string, dropDatabase bool, ids ...uint64) string {
	resource := tenantDeletionResource(ids, dropDatabase)
	create, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/approvals", strings.NewReader(fmt.Sprintf(`{
		"scope": "tenant.data.delete",
		"resource": %q,
		"reason": "feature test tenant destruction"
	}`, resource)))
	require.NoError(s.T(), err)
	create.AssertOk()
	body, err := create.Json()
	require.NoError(s.T(), err)
	approvalID := body["data"].(map[string]any)["approval_id"].(string)

	now := time.Now()
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_user").Create(map[string]any{
		"id":              99,
		"username":        "destroy_approver",
		"password":        "$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe",
		"user_type":       "900",
		"nickname":        "Tenant Destroy Approver",
		"status":          1,
		"dashboard":       "platform:tenant",
		"backend_setting": "{}",
		"created_at":      now,
		"updated_at":      now,
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_user_belongs_role").Create(map[string]any{
		"user_id": 99, "role_id": 1, "created_at": now, "updated_at": now,
	})
	require.NoError(s.T(), err)
	approver, err := services.NewPlatformPassportService().Login("destroy_approver", "123456")
	require.NoError(s.T(), err)
	approve, err := s.Http(s.T()).WithToken(approver.AccessToken).Put("/admin/platform/security/approvals/"+approvalID+"/approve", strings.NewReader(`{}`))
	require.NoError(s.T(), err)
	approve.AssertOk()
	approveBody, err := approve.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), approveBody["code"])
	return approvalID
}

func (s *TenantPlatformTestSuite) createPendingPlatformApproval(requesterID uint64, scope string, resource string) string {
	if scope == "module.lifecycle.execute" {
		approvalID := fmt.Sprintf("approval-fixture-%d", time.Now().UnixNano())
		now := time.Now()
		require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").Create(map[string]any{
			"approval_id": approvalID, "requester_id": requesterID, "approver_id": 0,
			"policy_key": scope, "binding_digest": strings.Repeat("0", 64),
			"scope": scope, "resource": resource, "status": "pending", "reason": "feature test approval",
			"expires_at": now.Add(30 * time.Minute), "created_at": now, "updated_at": now,
		}))
		return approvalID
	}
	record, err := services.NewEnterpriseSecurityControlService().CreatePlatformApproval(s.T().Context(), services.PlatformApprovalCreateRequest{
		RequesterID: requesterID,
		Scope:       scope,
		Resource:    resource,
		Reason:      "feature test approval",
	})
	require.NoError(s.T(), err)
	return record.ApprovalID
}

func tenantDeletionResource(ids []uint64, dropDatabase bool) string {
	mode := "metadata"
	if dropDatabase {
		mode = "database"
	}
	return services.TenantDataActionResource("delete", ids, mode)
}

func (s *TenantPlatformTestSuite) loginAsTenantAdmin() string {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
		)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

func (s *TenantPlatformTestSuite) loginAsPlatformAuditor() string {
	s.createPlatformAuditor()
	res, err := s.Http(s.T()).
		Post(
			"/admin/platform/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "auditor", "123456")),
		)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

func (s *TenantPlatformTestSuite) loginAsPlatformPlanEditor() string {
	s.createPlatformPlanEditor()
	res, err := s.Http(s.T()).
		Post(
			"/admin/platform/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "plan_editor", "123456")),
		)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

func (s *TenantPlatformTestSuite) loginAsPlatformTenantDestroyer() string {
	s.createPlatformTenantDestroyer()
	res, err := s.Http(s.T()).Post(
		"/admin/platform/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "tenant_destroyer", "123456")),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}

func (s *TenantPlatformTestSuite) createPlatformTenantDestroyer() {
	query := facades.Orm().Connection(services.PlatformConnection()).Query()
	_, err := query.Exec(`
		INSERT INTO platform_user (
			id, username, password, user_type, nickname, email, phone, signed, dashboard,
			status, login_ip, login_time, backend_setting, created_by, updated_by,
			created_at, updated_at, remark
		) VALUES (
			4, 'tenant_destroyer', '$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe',
			'900', '租户销毁员', 'destroyer@example.test', '16800000002', '',
			'platform:tenant', 1, '127.0.0.1', CURRENT_TIMESTAMP, '{}'::jsonb,
			1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
		) ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			password = EXCLUDED.password,
			status = EXCLUDED.status,
			updated_at = CURRENT_TIMESTAMP;
		INSERT INTO platform_role (id, name, code, status, sort, created_by, updated_by, created_at, updated_at, remark)
		VALUES (4, '租户销毁员', 'TenantDestroyer', 1, 20, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, code = EXCLUDED.code, status = EXCLUDED.status;
		INSERT INTO platform_user_belongs_role (user_id, role_id, created_at, updated_at)
		VALUES (4, 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, role_id) DO NOTHING;
		INSERT INTO platform_casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		VALUES
			('g', 'user:4', 'role:TenantDestroyer', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('p', 'role:TenantDestroyer', 'platform:tenant:destroy', '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	require.NoError(s.T(), err)
}

func (s *TenantPlatformTestSuite) createPlatformAuditor() {
	query := facades.Orm().Connection(services.PlatformConnection()).Query()
	_, err := query.Exec(`
		INSERT INTO platform_user (
			id, username, password, user_type, nickname, email, phone, signed, dashboard,
			status, login_ip, login_time, backend_setting, created_by, updated_by,
			created_at, updated_at, remark
		)
		VALUES (
			2, 'auditor', '$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe',
			'900', '平台审计员', 'auditor@example.test', '16800000000', '',
			'platform:tenant', 1, '127.0.0.1', CURRENT_TIMESTAMP, '{}'::jsonb,
			1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
		)
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			password = EXCLUDED.password,
			status = EXCLUDED.status,
			updated_at = CURRENT_TIMESTAMP
	`)
	require.NoError(s.T(), err)

	_, err = query.Exec(`
		INSERT INTO platform_role (id, name, code, status, sort, created_by, updated_by, created_at, updated_at, remark)
		VALUES (2, '租户审计员', 'TenantAuditor', 1, 10, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			code = EXCLUDED.code,
			status = EXCLUDED.status,
			sort = EXCLUDED.sort,
			updated_at = CURRENT_TIMESTAMP
	`)
	require.NoError(s.T(), err)

	_, err = query.Exec(`
		INSERT INTO platform_user_belongs_role (user_id, role_id, created_at, updated_at)
		VALUES (2, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, role_id) DO NOTHING
	`)
	require.NoError(s.T(), err)

	_, err = query.Exec(`
		INSERT INTO platform_casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		VALUES
			('g', 'user:2', 'role:TenantAuditor', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('p', 'role:TenantAuditor', 'platform:tenant:list', '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	require.NoError(s.T(), err)
}

func (s *TenantPlatformTestSuite) createPlatformPlanEditor() {
	query := facades.Orm().Connection(services.PlatformConnection()).Query()
	_, err := query.Exec(`
		INSERT INTO platform_user (
			id, username, password, user_type, nickname, email, phone, signed, dashboard,
			status, login_ip, login_time, backend_setting, created_by, updated_by,
			created_at, updated_at, remark
		)
		VALUES (
			3, 'plan_editor', '$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe',
			'900', '套餐管理员', 'plan-editor@example.test', '16800000001', '',
			'platform:tenantPlan', 1, '127.0.0.1', CURRENT_TIMESTAMP, '{}'::jsonb,
			1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ''
		)
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			password = EXCLUDED.password,
			status = EXCLUDED.status,
			updated_at = CURRENT_TIMESTAMP
	`)
	require.NoError(s.T(), err)

	_, err = query.Exec(`
		INSERT INTO platform_role (id, name, code, status, sort, created_by, updated_by, created_at, updated_at, remark)
		VALUES (3, '套餐管理员', 'TenantPlanEditor', 1, 10, 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			code = EXCLUDED.code,
			status = EXCLUDED.status,
			sort = EXCLUDED.sort,
			updated_at = CURRENT_TIMESTAMP
	`)
	require.NoError(s.T(), err)

	_, err = query.Exec(`
		INSERT INTO platform_user_belongs_role (user_id, role_id, created_at, updated_at)
		VALUES (3, 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, role_id) DO NOTHING
	`)
	require.NoError(s.T(), err)

	_, err = query.Exec(`
		INSERT INTO platform_casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		VALUES
			('g', 'user:3', 'role:TenantPlanEditor', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('p', 'role:TenantPlanEditor', 'platform:tenantPlan:save', '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('p', 'role:TenantPlanEditor', 'platform:tenantPlan:update', '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('p', 'role:TenantPlanEditor', 'platform:tenantPlan:list', '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	require.NoError(s.T(), err)
}

func (s *TenantPlatformTestSuite) createTenant(token, code string) uint64 {
	testDatabase := facades.Config().GetString("database.connections." + services.PlatformConnection() + ".database")
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant", strings.NewReader(fmt.Sprintf(`{
		"code": %q,
		"name": "生命周期租户",
		"plan": "standard",
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": %q,
		"db_username": "tenant_user",
		"db_password": "tenant_secret",
		"db_schema": "public"
	}`, code, testDatabase)))
	s.assertOK(res, err)

	body, err := res.Json()
	require.NoError(s.T(), err)
	data := body["data"].(map[string]any)
	return uint64(data["id"].(float64))
}

func (s *TenantPlatformTestSuite) createTenantWithPlanAndQuotas(token, code, plan, quotas string) uint64 {
	testDatabase := facades.Config().GetString("database.connections." + services.PlatformConnection() + ".database")
	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/tenant", strings.NewReader(fmt.Sprintf(`{
		"code": %q,
		"name": "套餐配额租户",
		"plan": %q,
		"status": 1,
		"db_host": "127.0.0.1",
		"db_port": 5432,
		"db_database": %q,
		"db_username": "tenant_user",
		"db_password": "tenant_secret",
		"db_schema": "public",
		"quotas": %s
	}`, code, plan, testDatabase, quotas)))
	s.assertOK(res, err)

	body, err := res.Json()
	require.NoError(s.T(), err)
	data := body["data"].(map[string]any)
	return uint64(data["id"].(float64))
}

func (s *TenantPlatformTestSuite) createPermissionPlan(code string, allowed []string) {
	allowedJSON := `"` + strings.Join(allowed, `","`) + `"`
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec(`
			INSERT INTO tenant_plan (
				code, name, status, sort, billing, quotas, features,
				created_at, updated_at, remark
			)
			VALUES (?, ?, 1, 10, '{}'::jsonb, '{}'::jsonb, ?::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
			ON CONFLICT (code) DO UPDATE SET
				name = EXCLUDED.name,
				status = EXCLUDED.status,
				features = EXCLUDED.features,
				updated_at = CURRENT_TIMESTAMP
		`, code, code, fmt.Sprintf(`{"permissions":{"allowed":[%s]}}`, allowedJSON))
	require.NoError(s.T(), err)
}

func (s *TenantPlatformTestSuite) tenantJSON(value string) string {
	connection := "database.connections." + services.PlatformConnection()
	replacements := map[string]string{
		"__TEST_DB_HOST__":     facades.Config().GetString(connection + ".host"),
		"__TEST_DB_PORT__":     fmt.Sprintf("%d", facades.Config().GetInt(connection+".port", 5432)),
		"__TEST_DB__":          facades.Config().GetString(connection + ".database"),
		"__TEST_DB_USER__":     facades.Config().GetString(connection + ".username"),
		"__TEST_DB_PASSWORD__": facades.Config().GetString(connection + ".password"),
		"__TEST_DB_SCHEMA__":   facades.Config().GetString(connection+".schema", "public"),
	}
	for old, replacement := range replacements {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func (s *TenantPlatformTestSuite) assertOK(res contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	if !res.IsSuccessful() {
		content, contentErr := res.Content()
		require.NoError(s.T(), contentErr)
		require.Failf(s.T(), "unexpected http status", "response body: %s", content)
	}
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
}

func (s *TenantPlatformTestSuite) assertSensitiveEvidenceRejected(res contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Contains(s.T(), body["message"], "re-auth")
}

func (s *TenantPlatformTestSuite) assertTenantStatus(id uint64, status int8) {
	var tenant services.Tenant
	err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Where("id", id).
		First(&tenant)
	require.NoError(s.T(), err)
	require.Equal(s.T(), status, tenant.Status)
}

func (s *TenantPlatformTestSuite) assertTenantField(id uint64, column, expected string) {
	var actual string
	err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant").
		Where("id", id).
		Pluck(column, &actual)
	require.NoError(s.T(), err)
	require.Equal(s.T(), expected, actual)
}
