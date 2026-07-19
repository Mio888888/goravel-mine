package admin

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type PlatformRBACTestSuite struct {
	suite.Suite
	tests.TestCase
	token string
}

func TestPlatformRBACTestSuite(t *testing.T) {
	suite.Run(t, new(PlatformRBACTestSuite))
}

func (s *PlatformRBACTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.PlatformAdminSeeder{})
	s.Seed(&seeders.PlatformMenuSeeder{})
	s.Seed(&seeders.PlatformCasbinSeeder{})
	s.token = s.loginAsPlatformAdmin()
}

func (s *PlatformRBACTestSuite) TestPlatformPassportReturnsPlatformMenusAndRoles() {
	infoRes, err := s.Http(s.T()).WithToken(s.token).Get("/admin/platform/passport/getInfo")
	s.assertOK(infoRes, err)

	menusRes, err := s.Http(s.T()).WithToken(s.token).Get("/admin/platform/permission/menus")
	s.assertOK(menusRes, err)
	menusBody, err := menusRes.Json()
	require.NoError(s.T(), err)
	menus := menusBody["data"].([]any)
	require.NotEmpty(s.T(), menus)
	tenantManage := findPlatformMenuItem(menus, "platform:tenantManage")
	require.NotNil(s.T(), tenantManage)
	system := findPlatformMenuItem(menus, "platform:system")
	require.NotNil(s.T(), system)
	require.NotNil(s.T(), findPlatformMenuItem(tenantManage["children"].([]any), "platform:tenant"))
	require.NotNil(s.T(), findPlatformMenuItem(tenantManage["children"].([]any), "platform:tenantPlan"))
	require.NotNil(s.T(), findPlatformMenuItem(system["children"].([]any), "platform:dictionary"))
	require.NotNil(s.T(), findPlatformMenuItem(system["children"].([]any), "platform:scheduledTask"))

	rolesRes, err := s.Http(s.T()).WithToken(s.token).Get("/admin/platform/permission/roles")
	s.assertOK(rolesRes, err)
	rolesBody, err := rolesRes.Json()
	require.NoError(s.T(), err)
	roles := rolesBody["data"].([]any)
	require.Equal(s.T(), "PlatformSuperAdmin", roles[0].(map[string]any)["code"])
}

func (s *PlatformRBACTestSuite) TestPlatformProfilePasswordValidationReturnsBusinessCode() {
	res, err := s.Http(s.T()).WithToken(s.token).Post("/admin/platform/permission/update", strings.NewReader(`{
		"old_password": "wrong-password",
		"new_password": "new-platform-password-123",
		"new_password_confirmation": "new-platform-password-123"
	}`))
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Equal(s.T(), "旧密码错误或新密码不一致", body["message"])
}

func (s *PlatformRBACTestSuite) TestPlatformProfilePersistsBackendSetting() {
	res, err := s.Http(s.T()).WithToken(s.token).Post("/admin/platform/permission/update", strings.NewReader(`{
		"backend_setting": {"app": {"useLocale": "zh_CN"}}
	}`))
	s.assertOK(res, err)

	var raw string
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_user").
		Where("username", "admin").
		Pluck("backend_setting::text", &raw)
	require.NoError(s.T(), err)
	require.JSONEq(s.T(), `{"app":{"useLocale":"zh_CN"}}`, raw)
}

func (s *PlatformRBACTestSuite) TestPlatformSelfEndpointsDoNotRequireAdminPermissionMap() {
	token := s.loginAsPlatformOperator("self_operator", "PlatformSelfOperator", "平台自助操作员", "platform:tenant:list")

	menusRes, err := s.Http(s.T()).WithToken(token).Get("/admin/platform/permission/menus")
	s.assertOK(menusRes, err)

	updateRes, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/permission/update", strings.NewReader(`{
		"backend_setting": {"app": {"sidebarCollapse": true}}
	}`))
	s.assertOK(updateRes, err)
}

func (s *PlatformRBACTestSuite) TestPlatformUserRoleMenuLifecycle() {
	userID := s.createPlatformUser()
	roleID := s.createPlatformRole()

	s.assertOK(s.platformSensitivePut(fmt.Sprintf("/admin/platform/user/%d/roles", userID), "user.roles.sync",
		rbacTestSelector("user", userID, "roles", []string{"PlatformOperator"}), `{
			"role_codes": ["PlatformOperator"]
		}`))

	userRolesRes, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/admin/platform/user/%d/roles", userID))
	s.assertOK(userRolesRes, err)
	userRolesBody, err := userRolesRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "PlatformOperator", userRolesBody["data"].([]any)[0].(map[string]any)["code"])

	s.assertOK(s.platformSensitivePut(fmt.Sprintf("/admin/platform/role/%d/permissions", roleID), "role.permissions.sync",
		rbacTestSelector("role", roleID, "permissions", []string{"platform:tenant:list"}), `{
			"permissions": ["platform:tenant:list"]
		}`))

	rolePermissionsRes, err := s.Http(s.T()).WithToken(s.token).Get(fmt.Sprintf("/admin/platform/role/%d/permissions", roleID))
	s.assertOK(rolePermissionsRes, err)
	rolePermissionsBody, err := rolePermissionsRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), "platform:tenant:list", rolePermissionsBody["data"].([]any)[0].(map[string]any)["name"])
}

func (s *PlatformRBACTestSuite) TestPlatformRBACSensitiveMutationsRequireEvidence() {
	userID := s.createPlatformUser()
	roleID := s.createPlatformRole()
	var passwordBefore string
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("platform_user").Where("id", userID).Pluck("password", &passwordBefore))

	s.assertSensitiveRejected(s.Http(s.T()).WithToken(s.token).Put(fmt.Sprintf("/admin/platform/user/%d/roles", userID), strings.NewReader(`{"role_codes":["PlatformOperator"]}`)))
	s.assertSensitiveRejected(s.Http(s.T()).WithToken(s.token).Put(fmt.Sprintf("/admin/platform/role/%d/permissions", roleID), strings.NewReader(`{"permissions":["platform:tenant:list"]}`)))
	s.assertSensitiveRejected(s.Http(s.T()).WithToken(s.token).Put("/admin/platform/user/password", strings.NewReader(fmt.Sprintf(`{"id":%d}`, userID))))

	query := facades.Orm().Connection(services.PlatformConnection()).Query()
	userRoles, err := query.Table("platform_user_belongs_role").Where("user_id", userID).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), userRoles)
	rolePermissions, err := query.Table("platform_role_belongs_menu").Where("role_id", roleID).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), rolePermissions)
	var passwordAfter string
	require.NoError(s.T(), query.Table("platform_user").Where("id", userID).Pluck("password", &passwordAfter))
	require.Equal(s.T(), passwordBefore, passwordAfter)
}

func (s *PlatformRBACTestSuite) createPlatformUser() uint64 {
	res, err := s.Http(s.T()).WithToken(s.token).Post("/admin/platform/user", strings.NewReader(`{
		"username": "operator",
		"password": "123456",
		"nickname": "平台操作员",
		"email": "operator@example.test",
		"phone": "16800000001",
		"dashboard": "platform:tenant",
		"status": 1
	}`))
	s.assertOK(res, err)

	var id uint64
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_user").
		Where("username", "operator").
		Pluck("id", &id)
	require.NoError(s.T(), err)
	return id
}

func (s *PlatformRBACTestSuite) createPlatformRole() uint64 {
	res, err := s.Http(s.T()).WithToken(s.token).Post("/admin/platform/role", strings.NewReader(`{
		"name": "平台操作员",
		"code": "PlatformOperator",
		"status": 1,
		"sort": 10
	}`))
	s.assertOK(res, err)

	var id uint64
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_role").
		Where("code", "PlatformOperator").
		Pluck("id", &id)
	require.NoError(s.T(), err)
	return id
}

func (s *PlatformRBACTestSuite) loginAsPlatformAdmin() string {
	return s.loginAsPlatformUser("admin", "123456")
}

func (s *PlatformRBACTestSuite) loginAsPlatformOperator(username, roleCode, roleName string, permissions ...string) string {
	adminService := services.NewPlatformPermissionAdminService()
	require.NoError(s.T(), adminService.CreateUser(services.UserPayload{
		Username:  username,
		Password:  "123456",
		Nickname:  roleName,
		Email:     username + "@example.test",
		Phone:     "16800000002",
		Dashboard: "platform:tenant",
		Status:    1,
	}, 1))
	require.NoError(s.T(), adminService.CreateRole(services.RolePayload{
		Name:   roleName,
		Code:   roleCode,
		Status: 1,
		Sort:   10,
	}, 1))

	var userID uint64
	err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_user").
		Where("username", username).
		Pluck("id", &userID)
	require.NoError(s.T(), err)

	var roleID uint64
	err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_role").
		Where("code", roleCode).
		Pluck("id", &roleID)
	require.NoError(s.T(), err)

	require.NoError(s.T(), adminService.SyncRolePermissions(roleID, permissions))
	require.NoError(s.T(), adminService.SyncUserRoles(userID, []string{roleCode}))
	return s.loginAsPlatformUser(username, "123456")
}

func (s *PlatformRBACTestSuite) loginAsPlatformUser(username, password string) string {
	res, err := s.Http(s.T()).
		Post(
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

func (s *PlatformRBACTestSuite) assertOK(res contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	if !res.IsSuccessful() {
		content, contentErr := res.Content()
		require.NoError(s.T(), contentErr)
		require.Failf(s.T(), "unexpected http status", "response body: %s", content)
	}
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
}

func (s *PlatformRBACTestSuite) assertSensitiveRejected(res contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	res.AssertOk()
	body, bindErr := res.Json()
	require.NoError(s.T(), bindErr)
	require.Equal(s.T(), float64(422), body["code"])
}

func (s *PlatformRBACTestSuite) platformSensitivePut(uri, policyKey, resource, body string) (contractshttp.Response, error) {
	evidence := s.platformSensitiveEvidence(policyKey, resource)
	var payload map[string]any
	require.NoError(s.T(), json.Unmarshal([]byte(body), &payload))
	payload["reauth_token"] = evidence.ReAuthToken
	payload["approval_id"] = evidence.ApprovalID
	encoded, err := json.Marshal(payload)
	require.NoError(s.T(), err)
	return s.Http(s.T()).WithToken(s.token).Put(uri, strings.NewReader(string(encoded)))
}

func (s *PlatformRBACTestSuite) platformSensitiveEvidence(policyKey, resource string) services.SensitiveOperationEvidence {
	var requesterID uint64
	require.NoError(s.T(), facades.Orm().Connection(services.PlatformConnection()).Query().
		Table("platform_user").Where("username", "admin").Pluck("id", &requesterID))
	security := services.NewEnterpriseSecurityControlService()
	approval, err := security.CreatePlatformApproval(s.T().Context(), services.PlatformApprovalCreateRequest{
		RequesterID: requesterID, PolicyKey: policyKey, Resource: resource, Reason: "platform RBAC feature test",
	})
	require.NoError(s.T(), err)
	result, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").
		Where("approval_id", approval.ApprovalID).Where("status", "pending").Update(map[string]any{
		"approver_id": 99, "status": "approved", "updated_at": time.Now(),
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), result.RowsAffected)
	res, err := s.Http(s.T()).WithToken(s.token).Post("/admin/platform/security/reauth-token", strings.NewReader(fmt.Sprintf(`{
		"password":"123456","operation":%q,"resource":%q
	}`, policyKey, resource)))
	s.assertOK(res, err)
	response, err := res.Json()
	require.NoError(s.T(), err)
	return services.SensitiveOperationEvidence{
		ReAuthToken: response["data"].(map[string]any)["reauth_token"].(string),
		ApprovalID:  approval.ApprovalID,
	}
}

func findPlatformMenuItem(items []any, name string) map[string]any {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["name"] == name {
			return item
		}
		children, ok := item["children"].([]any)
		if !ok {
			continue
		}
		if found := findPlatformMenuItem(children, name); found != nil {
			return found
		}
	}
	return nil
}
