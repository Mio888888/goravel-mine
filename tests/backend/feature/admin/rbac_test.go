package admin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"

	"goravel/app/facades"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type RBACTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestRBACTestSuite(t *testing.T) {
	suite.Run(t, new(RBACTestSuite))
}

func (s *RBACTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
}

func (s *RBACTestSuite) TestOrdinaryUserRequiresMatchingCasbinPolicy() {
	s.createOrdinaryUserWithPolicy("permission:user:index")
	token := s.loginAs("auditor", "123456")

	allowedRes, err := s.Http(s.T()).WithToken(token).Get("/admin/user/list")
	require.NoError(s.T(), err)
	allowedRes.AssertOk()
	allowed, err := allowedRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), allowed["code"])

	deniedRes, err := s.Http(s.T()).WithToken(token).Get("/admin/role/list")
	require.NoError(s.T(), err)
	deniedRes.AssertOk()
	denied, err := deniedRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(403), denied["code"])
}

func (s *RBACTestSuite) TestSuperAdminBypassesCasbinPolicy() {
	token := s.loginAs("admin", "123456")

	res, err := s.Http(s.T()).WithToken(token).Get("/admin/role/list")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
}

func (s *RBACTestSuite) createOrdinaryUserWithPolicy(permission string) {
	password, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	require.NoError(s.T(), err)

	require.NoError(s.T(), execSQL(`
		INSERT INTO "user" (
			id, username, password, user_type, nickname, status, backend_setting,
			created_by, updated_by, created_at, updated_at, remark
		)
		VALUES (2, 'auditor', ?, '100', '审计员', 1, '{}'::jsonb, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
	`, string(password)))
	require.NoError(s.T(), execSQL(`
		INSERT INTO role (id, name, code, status, sort, created_by, updated_by, created_at, updated_at, remark)
		VALUES (2, '审计角色', 'Auditor', 1, 10, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
	`))
	require.NoError(s.T(), execSQL(`
		INSERT INTO user_belongs_role (user_id, role_id, created_at, updated_at)
		VALUES (2, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`))
	require.NoError(s.T(), execSQL(`
		INSERT INTO casbin_rule (ptype, v0, v1, created_at, updated_at)
		VALUES ('g', 'user:2', 'role:Auditor', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`))
	require.NoError(s.T(), execSQL(`
		INSERT INTO casbin_rule (ptype, v0, v1, v2, created_at, updated_at)
		VALUES ('p', 'role:Auditor', ?, '*', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, permission))
}

func (s *RBACTestSuite) loginAs(username, password string) string {
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

func execSQL(sql string, values ...any) error {
	_, err := facades.Orm().Query().Exec(sql, values...)

	return err
}
