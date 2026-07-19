package admin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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

var errExpectedPolicy = errors.New("expected casbin policy")

type UserRoleMenuTestSuite struct {
	suite.Suite
	tests.TestCase
	token string
}

func TestUserRoleMenuTestSuite(t *testing.T) {
	suite.Run(t, new(UserRoleMenuTestSuite))
}

func (s *UserRoleMenuTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	s.token = s.loginAs("admin", "123456")
}

func (s *UserRoleMenuTestSuite) TestRoleCrudAndPermissionSyncsCasbinPolicy() {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/role", strings.NewReader(`{
		"name": "运营",
		"code": "Operator",
		"status": 1,
		"sort": 20,
		"remark": "后台运营"
	}`)))

	var roleID uint64
	require.NoError(s.T(), facades.Orm().Query().Table("role").Where("code", "Operator").Pluck("id", &roleID))
	require.NotZero(s.T(), roleID)

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/role/"+itoa(roleID), strings.NewReader(`{
		"name": "运营专员",
		"code": "Operator",
		"status": 1,
		"sort": 21
	}`)))

	list := s.getJSON("/admin/role/list?code=Operator")
	require.Equal(s.T(), float64(200), list["code"])
	require.Equal(s.T(), float64(1), list["data"].(map[string]any)["total"])

	s.assertOK(s.tenantSensitivePut("/admin/role/"+itoa(roleID)+"/permissions", "role.permissions.sync",
		rbacTestSelector("role", roleID, "permissions", []string{"permission:user:index", "permission:role:index"}), `{
			"permissions": ["permission:user:index", "permission:role:index"]
		}`))

	permissions := s.getJSON("/admin/role/" + itoa(roleID) + "/permissions")
	require.Equal(s.T(), float64(200), permissions["code"])
	require.Len(s.T(), permissions["data"], 2)
	require.NoError(s.T(), assertCasbinPolicy("role:Operator", "permission:user:index"))
}

func (s *UserRoleMenuTestSuite) TestRoleDeleteRejectsSuperAdmin() {
	response, err := s.Http(s.T()).WithToken(s.token).Delete("/admin/role", strings.NewReader(`[1]`))
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])

	count, err := facades.Orm().Query().Table("role").Where("code", "SuperAdmin").Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *UserRoleMenuTestSuite) TestRoleDeleteRemovesUserGroupingAndRolePolicies() {
	roleID := s.createRole("TempRole", "临时角色")
	s.assertOK(s.tenantSensitivePut("/admin/role/"+itoa(roleID)+"/permissions", "role.permissions.sync",
		rbacTestSelector("role", roleID, "permissions", []string{"permission:user:index"}), `{
			"permissions": ["permission:user:index"]
		}`))

	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/user", strings.NewReader(`{
		"username": "temp_user",
		"password": "123456",
		"nickname": "临时用户",
		"user_type": "100",
		"status": 1
	}`)))
	var userID uint64
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("username", "temp_user").Pluck("id", &userID))
	require.NotZero(s.T(), userID)

	s.assertOK(s.tenantSensitivePut("/admin/user/"+itoa(userID)+"/roles", "user.roles.sync",
		rbacTestSelector("user", userID, "roles", []string{"TempRole"}), `{
			"role_codes": ["TempRole"]
		}`))

	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/role", strings.NewReader(`[`+itoa(roleID)+`]`)))

	require.NoError(s.T(), assertNoCasbinPolicy("role:TempRole", "permission:user:index"))
	require.NoError(s.T(), assertNoCasbinGrouping("user:"+itoa(userID), "role:TempRole"))
}

func (s *UserRoleMenuTestSuite) TestRoleCodeUpdateMigratesCasbinRules() {
	roleID := s.createRole("OldCode", "旧角色")
	s.assertOK(s.tenantSensitivePut("/admin/role/"+itoa(roleID)+"/permissions", "role.permissions.sync",
		rbacTestSelector("role", roleID, "permissions", []string{"permission:user:index"}), `{
			"permissions": ["permission:user:index"]
		}`))

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/role/"+itoa(roleID), strings.NewReader(`{
		"name": "新角色",
		"code": "NewCode",
		"status": 1,
		"sort": 31
	}`)))

	require.NoError(s.T(), assertNoCasbinPolicy("role:OldCode", "permission:user:index"))
	require.NoError(s.T(), assertCasbinPolicy("role:NewCode", "permission:user:index"))
}

func (s *UserRoleMenuTestSuite) TestUserCrudRoleAssignmentAndPasswordReset() {
	roleID := s.createRole("Staff", "员工")

	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/user", strings.NewReader(`{
		"username": "staff",
		"password": "staff-pass",
		"nickname": "员工一号",
		"user_type": "100",
		"phone": "13800000000",
		"email": "staff@example.com",
		"status": 1,
		"remark": "demo"
	}`)))

	var userID uint64
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("username", "staff").Pluck("id", &userID))
	require.NotZero(s.T(), userID)

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/user/"+itoa(userID), strings.NewReader(`{
		"nickname": "员工一号更新",
		"status": 1,
		"email": "staff-new@example.com"
	}`)))

	list := s.getJSON("/admin/user/list?username=staff")
	require.Equal(s.T(), float64(200), list["code"])
	require.Equal(s.T(), float64(1), list["data"].(map[string]any)["total"])

	s.assertOK(s.tenantSensitivePut("/admin/user/"+itoa(userID)+"/roles", "user.roles.sync",
		rbacTestSelector("user", userID, "roles", []string{"Staff"}), `{
			"role_codes": ["Staff"]
		}`))
	roles := s.getJSON("/admin/user/" + itoa(userID) + "/roles")
	require.Equal(s.T(), float64(200), roles["code"])
	require.Len(s.T(), roles["data"], 1)
	require.NoError(s.T(), assertCasbinGrouping("user:"+itoa(userID), "role:Staff"))

	s.assertOK(s.tenantSensitivePut("/admin/user/password", "user.password.reset",
		"rbac:user:"+itoa(userID)+":password:reset", `{"id": `+itoa(userID)+`}`))
	resetLogin := s.loginResponse("staff", "123456")
	require.Equal(s.T(), 200, resetLogin.Code)
	require.NotEmpty(s.T(), resetLogin.Data.AccessToken)

	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/user", strings.NewReader(`[`+itoa(userID)+`]`)))
	list = s.getJSON("/admin/user/list?username=staff")
	require.Equal(s.T(), float64(0), list["data"].(map[string]any)["total"])

	_ = roleID
}

func (s *UserRoleMenuTestSuite) TestTenantRBACSensitiveMutationsRequireEvidence() {
	roleID := s.createRole("GuardRole", "门禁角色")
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/user", strings.NewReader(`{
		"username":"guard_user","password":"before-pass","nickname":"门禁用户","user_type":"100","status":1
	}`)))
	var userID uint64
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("username", "guard_user").Pluck("id", &userID))
	var passwordBefore string
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("id", userID).Pluck("password", &passwordBefore))

	s.assertSensitiveRejected(s.Http(s.T()).WithToken(s.token).Put("/admin/user/"+itoa(userID)+"/roles", strings.NewReader(`{"role_codes":["GuardRole"]}`)))
	s.assertSensitiveRejected(s.Http(s.T()).WithToken(s.token).Put("/admin/role/"+itoa(roleID)+"/permissions", strings.NewReader(`{"permissions":["permission:user:index"]}`)))
	s.assertSensitiveRejected(s.Http(s.T()).WithToken(s.token).Put("/admin/user/password", strings.NewReader(`{"id":`+itoa(userID)+`}`)))

	userRoles, err := facades.Orm().Query().Table("user_belongs_role").Where("user_id", userID).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), userRoles)
	rolePermissions, err := facades.Orm().Query().Table("role_belongs_menu").Where("role_id", roleID).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), rolePermissions)
	var passwordAfter string
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("id", userID).Pluck("password", &passwordAfter))
	require.Equal(s.T(), passwordBefore, passwordAfter)
}

func (s *UserRoleMenuTestSuite) TestUserCreateAcceptsEmptyArrayBackendSetting() {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/user", strings.NewReader(`{
		"password": "123456",
		"status": 1,
		"user_type": 100,
		"department": [],
		"position": [],
		"backend_setting": [],
		"username": "test",
		"nickname": "测试"
	}`)))

	var userID uint64
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("username", "test").Pluck("id", &userID))
	require.NotZero(s.T(), userID)

	login := s.loginResponse("test", "123456")
	require.Equal(s.T(), 200, login.Code)
	require.NotEmpty(s.T(), login.Data.AccessToken)
}

func (s *UserRoleMenuTestSuite) TestUserDeleteRejectsCurrentAdmin() {
	response, err := s.Http(s.T()).WithToken(s.token).Delete("/admin/user", strings.NewReader(`[1]`))
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])

	count, err := facades.Orm().Query().Table(`"user"`).Where("id", 1).Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *UserRoleMenuTestSuite) TestMenuCrudWithButtonPermission() {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/menu", strings.NewReader(`{
		"parent_id": 0,
		"name": "demo:menu",
		"path": "/demo",
		"component": "demo/index",
		"status": 1,
		"sort": 999,
		"meta": {"title": "演示菜单", "type": "M"},
		"btnPermission": [
			{"name": "demo:menu:create", "meta": {"title": "创建", "type": "B"}, "sort": 1}
		]
	}`)))

	var menuID uint64
	require.NoError(s.T(), facades.Orm().Query().Table("menu").Where("name", "demo:menu").Pluck("id", &menuID))
	require.NotZero(s.T(), menuID)

	tree := s.getJSON("/admin/menu/list")
	require.Equal(s.T(), float64(200), tree["code"])
	require.NotEmpty(s.T(), tree["data"])

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/menu/"+itoa(menuID), strings.NewReader(`{
		"name": "demo:menu",
		"path": "/demo-updated",
		"component": "demo/index",
		"status": 1,
		"sort": 998,
		"meta": {"title": "演示菜单更新", "type": "M"}
	}`)))

	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/menu", strings.NewReader(`[`+itoa(menuID)+`]`)))
	count, err := facades.Orm().Query().Table("menu").Where("name", "demo:menu").Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)
}

func (s *UserRoleMenuTestSuite) TestMenuDeleteRemovesRoleBindingsAndPolicies() {
	roleID := s.createRole("MenuCleaner", "菜单清理")
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/menu", strings.NewReader(`{
		"parent_id": 0,
		"name": "cleanup:menu",
		"path": "/cleanup",
		"component": "cleanup/index",
		"status": 1,
		"sort": 997,
		"meta": {"title": "清理菜单", "type": "M"},
		"btnPermission": [
			{"name": "cleanup:menu:index", "meta": {"title": "查看", "type": "B"}, "sort": 1}
		]
	}`)))

	var menuID uint64
	require.NoError(s.T(), facades.Orm().Query().Table("menu").Where("name", "cleanup:menu").Pluck("id", &menuID))
	require.NotZero(s.T(), menuID)
	s.assertOK(s.tenantSensitivePut("/admin/role/"+itoa(roleID)+"/permissions", "role.permissions.sync",
		rbacTestSelector("role", roleID, "permissions", []string{"cleanup:menu", "cleanup:menu:index"}), `{
			"permissions": ["cleanup:menu", "cleanup:menu:index"]
		}`))

	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/menu", strings.NewReader(`[`+itoa(menuID)+`]`)))

	require.NoError(s.T(), assertNoCasbinPolicy("role:MenuCleaner", "cleanup:menu"))
	require.NoError(s.T(), assertNoCasbinPolicy("role:MenuCleaner", "cleanup:menu:index"))
	count, err := facades.Orm().Query().Table("role_belongs_menu").Where("role_id", roleID).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)
}

func (s *UserRoleMenuTestSuite) createRole(code, name string) uint64 {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/role", strings.NewReader(`{
		"name": "`+name+`",
		"code": "`+code+`",
		"status": 1,
		"sort": 30
	}`)))

	var roleID uint64
	require.NoError(s.T(), facades.Orm().Query().Table("role").Where("code", code).Pluck("id", &roleID))
	require.NotZero(s.T(), roleID)

	return roleID
}

func (s *UserRoleMenuTestSuite) getJSON(uri string) map[string]any {
	res, err := s.Http(s.T()).WithToken(s.token).Get(uri)
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)

	return body
}

func (s *UserRoleMenuTestSuite) assertOK(response contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "body=%v", body)
}

func (s *UserRoleMenuTestSuite) assertSensitiveRejected(response contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	response.AssertOk()
	body, bindErr := response.Json()
	require.NoError(s.T(), bindErr)
	require.Equal(s.T(), float64(422), body["code"])
}

func (s *UserRoleMenuTestSuite) tenantSensitivePut(uri, policyKey, resource, body string) (contractshttp.Response, error) {
	evidence := s.tenantSensitiveEvidence(policyKey, resource)
	var payload map[string]any
	require.NoError(s.T(), json.Unmarshal([]byte(body), &payload))
	payload["reauth_token"] = evidence.ReAuthToken
	payload["approval_id"] = evidence.ApprovalID
	encoded, err := json.Marshal(payload)
	require.NoError(s.T(), err)
	return s.Http(s.T()).WithToken(s.token).Put(uri, strings.NewReader(string(encoded)))
}

func (s *UserRoleMenuTestSuite) tenantSensitiveEvidence(policyKey, resource string) services.SensitiveOperationEvidence {
	create, err := s.Http(s.T()).WithToken(s.token).Post("/admin/security/approvals", strings.NewReader(fmt.Sprintf(`{
		"policy_key":%q,"resource":%q,"reason":"RBAC feature test"
	}`, policyKey, resource)))
	s.assertOK(create, err)
	created, err := create.Json()
	require.NoError(s.T(), err)
	approvalID := created["data"].(map[string]any)["approval_id"].(string)
	result, err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("enterprise_security_approval").
		Where("approval_id", approvalID).Where("status", "pending").Update(map[string]any{
		"approver_id": 99, "status": "approved", "updated_at": time.Now(),
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), result.RowsAffected)
	reauth, err := s.Http(s.T()).WithToken(s.token).Post("/admin/security/reauth-token", strings.NewReader(fmt.Sprintf(`{
		"password":"123456","operation":%q,"resource":%q
	}`, policyKey, resource)))
	s.assertOK(reauth, err)
	reauthenticated, err := reauth.Json()
	require.NoError(s.T(), err)
	return services.SensitiveOperationEvidence{
		ReAuthToken: reauthenticated["data"].(map[string]any)["reauth_token"].(string),
		ApprovalID:  approvalID,
	}
}

func rbacTestSelector(subject string, id uint64, operation string, values []string) string {
	canonical := append([]string(nil), values...)
	sort.Strings(canonical)
	encoded, _ := json.Marshal(canonical)
	return fmt.Sprintf("rbac:%s:%d:%s:%s", subject, id, operation, base64.RawURLEncoding.EncodeToString(encoded))
}

func (s *UserRoleMenuTestSuite) loginResponse(username, password string) passportResponse {
	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, password)),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))

	return body
}

func (s *UserRoleMenuTestSuite) loginAs(username, password string) string {
	body := s.loginResponse(username, password)
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)

	return body.Data.AccessToken
}

func assertCasbinPolicy(role, permission string) error {
	exists, err := facades.Orm().Query().
		Table("casbin_rule").
		Where("ptype", "p").
		Where("v0", role).
		Where("v1", permission).
		Exists()
	if err != nil || exists {
		return err
	}

	return errExpectedPolicy
}

func assertNoCasbinPolicy(role, permission string) error {
	exists, err := facades.Orm().Query().
		Table("casbin_rule").
		Where("ptype", "p").
		Where("v0", role).
		Where("v1", permission).
		Exists()
	if err != nil || !exists {
		return err
	}

	return errors.New("unexpected casbin policy")
}

func assertCasbinGrouping(user, role string) error {
	exists, err := facades.Orm().Query().
		Table("casbin_rule").
		Where("ptype", "g").
		Where("v0", user).
		Where("v1", role).
		Exists()
	if err != nil || exists {
		return err
	}

	return errExpectedPolicy
}

func assertNoCasbinGrouping(user, role string) error {
	exists, err := facades.Orm().Query().
		Table("casbin_rule").
		Where("ptype", "g").
		Where("v0", user).
		Where("v1", role).
		Exists()
	if err != nil || !exists {
		return err
	}

	return errors.New("unexpected casbin grouping")
}

func itoa(id uint64) string {
	return strconv.FormatUint(id, 10)
}
