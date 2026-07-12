package admin

import (
	"strings"
	"testing"
	"time"

	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/database/seeders"
	"goravel/tests"
)

type LogTestSuite struct {
	suite.Suite
	tests.TestCase
	token string
}

func TestLogTestSuite(t *testing.T) {
	suite.Run(t, new(LogTestSuite))
}

func (s *LogTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	s.token = s.loginAs("admin", "123456")
}

func (s *LogTestSuite) TestLoginLogListFilter() {
	s.loginAs("admin", "123456")
	s.failedLogin("admin", "bad-password")

	list := s.getJSON("/admin/user-login-log/list?username=admin&status=1")
	require.Equal(s.T(), float64(200), list["code"])
	data := list["data"].(map[string]any)
	require.Equal(s.T(), float64(2), data["total"])
	rows := data["list"].([]any)
	require.NotEmpty(s.T(), rows)
	row := rows[0].(map[string]any)
	require.Equal(s.T(), "admin", row["username"])
	require.Equal(s.T(), float64(1), row["status"])
	require.NotEmpty(s.T(), row["login_time"])
	require.NotContains(s.T(), row["login_time"].(string), "T")

}

func (s *LogTestSuite) TestDisabledLoginAttemptWritesFailureLog() {
	_, err := facades.Orm().Query().Table(`"user"`).Where("id", 1).Update("status", 2)
	require.NoError(s.T(), err)

	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(423), body["code"])

	_, err = facades.Orm().Query().Table(`"user"`).Where("id", 1).Update("status", 1)
	require.NoError(s.T(), err)

	list := s.getJSON("/admin/user-login-log/list?username=admin&status=2")
	require.Equal(s.T(), float64(200), list["code"])
	data := list["data"].(map[string]any)
	require.Equal(s.T(), float64(1), data["total"])
	row := data["list"].([]any)[0].(map[string]any)
	require.Equal(s.T(), "用户已停用", row["message"])
}

func (s *LogTestSuite) TestOperationLogRecordsProtectedMutations() {
	s.assertOK(s.Http(s.T()).WithToken(s.token).Post("/admin/role", strings.NewReader(`{
		"name": "审计员",
		"code": "Auditor",
		"status": 1,
		"sort": 60
	}`)))
	s.waitOperationLog("admin", "POST", "/admin/role")

	list := s.getJSON("/admin/user-operation-log/list?username=admin&router=/admin/role")
	require.Equal(s.T(), float64(200), list["code"])
	data := list["data"].(map[string]any)
	require.Equal(s.T(), float64(1), data["total"])
	rows := data["list"].([]any)
	require.Len(s.T(), rows, 1)
	row := rows[0].(map[string]any)
	require.Equal(s.T(), "admin", row["username"])
	require.Equal(s.T(), "POST", row["method"])
	require.Equal(s.T(), "/admin/role", row["router"])
	require.Equal(s.T(), "创建角色", row["service_name"])
	require.NotEmpty(s.T(), row["created_at"])
	require.NotContains(s.T(), row["created_at"].(string), "T")

}

func (s *LogTestSuite) getJSON(uri string) map[string]any {
	res, err := s.Http(s.T()).WithToken(s.token).Get(uri)
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *LogTestSuite) assertOK(response contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "body=%v", body)
}

func (s *LogTestSuite) loginAs(username, password string) string {
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

func (s *LogTestSuite) failedLogin(username, password string) {
	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, password)),
	)
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
}

func (s *LogTestSuite) waitOperationLog(username, method, router string) {
	require.Eventually(s.T(), func() bool {
		exists, err := facades.Orm().Query().Table("user_operation_log").
			Where("username", username).
			Where("method", method).
			Where("router", router).
			Exists()
		require.NoError(s.T(), err)
		return exists
	}, time.Second, 20*time.Millisecond)
}
