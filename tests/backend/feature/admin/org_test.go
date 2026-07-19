package admin

import (
	"strings"
	"testing"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type OrgTestSuite struct {
	suite.Suite
	tests.TestCase
	token string
}

func TestOrgTestSuite(t *testing.T) {
	suite.Run(t, new(OrgTestSuite))
}

func (s *OrgTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	s.token = s.loginAs("admin", "123456")
}

func (s *OrgTestSuite) TestDepartmentTreeCrudAndUserSync() {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/department", strings.NewReader(`{
		"name": "研发部",
		"parent_id": 0,
		"department_users": [1],
		"leader": [1]
	}`)))

	var deptID uint64
	require.NoError(s.T(), facades.Orm().Query().Table("department").Where("name", "研发部").Pluck("id", &deptID))
	require.NotZero(s.T(), deptID)

	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/department", strings.NewReader(`{
		"name": "后端组",
		"parent_id": `+itoa(deptID)+`
	}`)))

	tree := s.getJSON("/admin/department/list?level=1&name=研发")
	require.Equal(s.T(), float64(200), tree["code"])
	list := tree["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), list, 1)
	root := list[0].(map[string]any)
	require.Equal(s.T(), "研发部", root["name"])
	require.NotEmpty(s.T(), root["children"])
	require.NotEmpty(s.T(), root["department_users"])
	require.NotEmpty(s.T(), root["leader"])

	info := s.getJSON("/admin/passport/getInfo")
	require.NotEmpty(s.T(), info["data"].(map[string]any)["departments"])

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/department/"+itoa(deptID), strings.NewReader(`{
		"name": "研发中心",
		"parent_id": 0,
		"department_users": [],
		"leader": []
	}`)))
	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/department", strings.NewReader(`[`+itoa(deptID)+`]`)))
}

func (s *OrgTestSuite) TestPositionCrudAndDataPermission() {
	deptID := s.createDepartment("产品部")
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/position", strings.NewReader(`{
		"name": "产品经理",
		"dept_id": `+itoa(deptID)+`
	}`)))

	var positionID uint64
	require.NoError(s.T(), facades.Orm().Query().Table("position").Where("name", "产品经理").Pluck("id", &positionID))
	require.NotZero(s.T(), positionID)

	list := s.getJSON("/admin/position/list?name=产品")
	require.Equal(s.T(), float64(200), list["code"])
	rows := list["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), rows, 1)
	require.Equal(s.T(), "产品部", rows[0].(map[string]any)["dept_name"])

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/position/"+itoa(positionID)+"/data_permission", strings.NewReader(`{
		"policy_type": "CUSTOM_DEPT",
		"value": [`+itoa(deptID)+`]
	}`)))

	var policyType string
	require.NoError(s.T(), facades.Orm().Query().Table("data_permission_policy").Where("position_id", positionID).Pluck("policy_type", &policyType))
	require.Equal(s.T(), "CUSTOM_DEPT", policyType)
	list = s.getJSON("/admin/position/list?name=产品")
	rows = list["data"].(map[string]any)["list"].([]any)
	policy := rows[0].(map[string]any)["policy"].(map[string]any)
	require.Equal(s.T(), "CUSTOM_DEPT", policy["policy_type"])
	require.Equal(s.T(), []any{float64(deptID)}, policy["value"])

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/position/"+itoa(positionID), strings.NewReader(`{
		"name": "高级产品经理",
		"dept_id": `+itoa(deptID)+`
	}`)))
	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/position", strings.NewReader(`[`+itoa(positionID)+`]`)))
}

func (s *OrgTestSuite) TestLeaderCreateListUpdateAndDeleteByDoubleKey() {
	deptID := s.createDepartment("运营部")

	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/leader", strings.NewReader(`{
		"dept_id": `+itoa(deptID)+`,
		"user_id": [1]
	}`)))

	list := s.getJSON("/admin/leader/list?dept_id=" + itoa(deptID))
	require.Equal(s.T(), float64(200), list["code"])
	rows := list["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), rows, 1)
	require.Equal(s.T(), "运营部", rows[0].(map[string]any)["dept_name"])
	user := rows[0].(map[string]any)["user"].(map[string]any)
	require.Equal(s.T(), "admin", user["username"])
	require.NotEmpty(s.T(), user["phone"])
	require.NotEmpty(s.T(), rows[0].(map[string]any)["users"])

	s.assertOK(s.Http(s.T()).WithToken(s.token).Put("/admin/leader/"+itoa(deptID), strings.NewReader(`{
		"dept_id": `+itoa(deptID)+`,
		"user_id": [1]
	}`)))
	s.assertOK(s.Http(s.T()).WithToken(s.token).Delete("/admin/leader", strings.NewReader(`{
		"dept_id": `+itoa(deptID)+`,
		"user_ids": [1]
	}`)))

	count, err := facades.Orm().Query().Table("dept_leader").Where("dept_id", deptID).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)
}

func (s *OrgTestSuite) TestUserListAppliesDepartmentSelfDataPermission() {
	deptA := s.createDepartment("一部")
	deptB := s.createDepartment("二部")
	roleID := s.createRole("DeptAuditor", "部门审计")
	s.grantRolePermissions(roleID, []string{"permission:user:index"})

	userA := s.createUser("dept_user_a", deptA)
	userB := s.createUser("dept_user_b", deptB)
	s.assignRole(userA, "DeptAuditor")
	s.assignRole(userB, "DeptAuditor")
	s.setUserPolicy(userA, "DEPT_SELF", []uint64{deptA})

	token := s.loginAs("dept_user_a", "123456")
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/user/list")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	rows := body["data"].(map[string]any)["list"].([]any)
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.(map[string]any)["username"].(string))
	}
	require.Contains(s.T(), names, "dept_user_a")
	require.NotContains(s.T(), names, "dept_user_b")
}

func (s *OrgTestSuite) TestUserListDepartmentSelfPolicyUsesCurrentUserDepartments() {
	deptA := s.createDepartment("三部")
	deptB := s.createDepartment("四部")
	roleID := s.createRole("DeptSelfAuditor", "本部门审计")
	s.grantRolePermissions(roleID, []string{"permission:user:index"})

	userA := s.createUser("dept_self_a", deptA)
	userB := s.createUser("dept_self_b", deptB)
	s.assignRole(userA, "DeptSelfAuditor")
	s.assignRole(userB, "DeptSelfAuditor")
	s.setUserPolicy(userA, "DEPT_SELF", nil)

	token := s.loginAs("dept_self_a", "123456")
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/user/list")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	rows := body["data"].(map[string]any)["list"].([]any)
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.(map[string]any)["username"].(string))
	}
	require.Contains(s.T(), names, "dept_self_a")
	require.NotContains(s.T(), names, "dept_self_b")
}

func (s *OrgTestSuite) TestUserListDepartmentTreePolicyIncludesChildDepartments() {
	parentDept := s.createDepartment("五部")
	childDept := s.createChildDepartment(parentDept, "五部一组")
	otherDept := s.createDepartment("六部")
	roleID := s.createRole("DeptTreeAuditor", "部门树审计")
	s.grantRolePermissions(roleID, []string{"permission:user:index"})

	parentUser := s.createUser("dept_tree_parent", parentDept)
	childUser := s.createUser("dept_tree_child", childDept)
	otherUser := s.createUser("dept_tree_other", otherDept)
	s.assignRole(parentUser, "DeptTreeAuditor")
	s.assignRole(childUser, "DeptTreeAuditor")
	s.assignRole(otherUser, "DeptTreeAuditor")
	s.setUserPolicy(parentUser, "DEPT_TREE", nil)

	token := s.loginAs("dept_tree_parent", "123456")
	res, err := s.Http(s.T()).WithToken(token).Get("/admin/user/list")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	rows := body["data"].(map[string]any)["list"].([]any)
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.(map[string]any)["username"].(string))
	}
	require.Contains(s.T(), names, "dept_tree_parent")
	require.Contains(s.T(), names, "dept_tree_child")
	require.NotContains(s.T(), names, "dept_tree_other")
}

func (s *OrgTestSuite) createDepartment(name string) uint64 {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/department", strings.NewReader(`{
		"name": "`+name+`",
		"parent_id": 0
	}`)))
	var id uint64
	require.NoError(s.T(), facades.Orm().Query().Table("department").Where("name", name).Pluck("id", &id))
	require.NotZero(s.T(), id)
	return id
}

func (s *OrgTestSuite) createChildDepartment(parentID uint64, name string) uint64 {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/department", strings.NewReader(`{
		"name": "`+name+`",
		"parent_id": `+itoa(parentID)+`
	}`)))
	var id uint64
	require.NoError(s.T(), facades.Orm().Query().Table("department").Where("name", name).Pluck("id", &id))
	require.NotZero(s.T(), id)
	return id
}

func (s *OrgTestSuite) createRole(code, name string) uint64 {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/role", strings.NewReader(`{
		"name": "`+name+`",
		"code": "`+code+`",
		"status": 1,
		"sort": 50
	}`)))
	var id uint64
	require.NoError(s.T(), facades.Orm().Query().Table("role").Where("code", code).Pluck("id", &id))
	require.NotZero(s.T(), id)
	return id
}

func (s *OrgTestSuite) grantRolePermissions(roleID uint64, permissions []string) {
	require.NoError(s.T(), services.NewPermissionAdminService().SyncRolePermissions(roleID, permissions))
}

func (s *OrgTestSuite) createUser(username string, deptID uint64) uint64 {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/user", strings.NewReader(`{
		"username": "`+username+`",
		"password": "123456",
		"nickname": "`+username+`",
		"user_type": "100",
		"status": 1,
		"department": [`+itoa(deptID)+`]
	}`)))
	var id uint64
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("username", username).Pluck("id", &id))
	require.NotZero(s.T(), id)
	return id
}

func (s *OrgTestSuite) assignRole(userID uint64, roleCode string) {
	require.NoError(s.T(), services.NewPermissionAdminService().SyncUserRoles(userID, []string{roleCode}))
}

func (s *OrgTestSuite) setUserPolicy(userID uint64, policyType string, deptIDs []uint64) {
	values := make([]string, 0, len(deptIDs))
	for _, id := range deptIDs {
		values = append(values, itoa(id))
	}
	value := "[]"
	if deptIDs != nil {
		value = "[" + strings.Join(values, ",") + "]"
	}
	_, err := facades.Orm().Query().Exec(`
		INSERT INTO data_permission_policy (user_id, policy_type, is_default, value, created_at, updated_at)
		VALUES (?, ?, true, ?::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, userID, policyType, value)
	require.NoError(s.T(), err)
}

func (s *OrgTestSuite) getJSON(uri string) map[string]any {
	res, err := s.Http(s.T()).WithToken(s.token).Get(uri)
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *OrgTestSuite) assertOK(response contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "body=%v", body)
}

func (s *OrgTestSuite) loginAs(username, password string) string {
	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, password)),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}
