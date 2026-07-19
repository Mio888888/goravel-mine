package admin

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type AuditLogImmutabilityTestSuite struct {
	suite.Suite
	tests.TestCase
	token string
}

func TestAuditLogImmutabilityTestSuite(t *testing.T) {
	suite.Run(t, new(AuditLogImmutabilityTestSuite))
}

func (s *AuditLogImmutabilityTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	s.token = s.loginAsAdmin()
}

func (s *AuditLogImmutabilityTestSuite) TestLoginLogDirectDeleteCannotRemoveRecord() {
	log := models.UserLoginLog{
		Username:  "immutable-login",
		IP:        "127.0.0.1",
		OS:        "test",
		Browser:   "test",
		Status:    1,
		Message:   "success",
		LoginTime: time.Now(),
	}
	require.NoError(s.T(), facades.Orm().Query().Create(&log))

	response, err := s.Http(s.T()).WithToken(s.token).Delete(
		"/admin/user-login-log",
		strings.NewReader(`{"ids": [`+itoa(log.ID)+`]}`),
	)
	require.NoError(s.T(), err)
	response.AssertNotFound()

	s.assertLogRecordExists("user_login_log", log.ID)
	s.assertLogListContains("/admin/user-login-log/list?username=immutable-login")
}

func (s *AuditLogImmutabilityTestSuite) TestOperationLogDirectDeleteCannotRemoveRecord() {
	now := time.Now()
	log := models.UserOperationLog{
		Username:    "immutable-operation",
		Method:      "POST",
		Router:      "/admin/immutable",
		ServiceName: "immutable test",
		IP:          "127.0.0.1",
		Timestamps:  models.Timestamps{CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(s.T(), facades.Orm().Query().Create(&log))

	response, err := s.Http(s.T()).WithToken(s.token).Delete(
		"/admin/user-operation-log",
		strings.NewReader(`{"ids": [`+itoa(log.ID)+`]}`),
	)
	require.NoError(s.T(), err)
	response.AssertNotFound()

	s.assertLogRecordExists("user_operation_log", log.ID)
	s.assertLogListContains("/admin/user-operation-log/list?username=immutable-operation")
}

func (s *AuditLogImmutabilityTestSuite) assertLogRecordExists(table string, id uint64) {
	s.T().Helper()
	count, err := facades.Orm().Query().Table(table).Where("id", id).Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *AuditLogImmutabilityTestSuite) assertLogListContains(uri string) {
	s.T().Helper()
	response, err := s.Http(s.T()).WithToken(s.token).Get(uri)
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	data := body["data"].(map[string]any)
	require.Equal(s.T(), float64(1), data["total"])
}

func (s *AuditLogImmutabilityTestSuite) loginAsAdmin() string {
	response, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), response.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	return body.Data.AccessToken
}
