package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests"
)

type PassportTestSuite struct {
	suite.Suite
	tests.TestCase
}

type passportResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		AccessToken            string `json:"access_token"`
		RefreshToken           string `json:"refresh_token"`
		ExpireAt               int    `json:"expire_at"`
		MFARequired            bool   `json:"mfa_required"`
		MFAToken               string `json:"mfa_token"`
		PasswordChangeRequired bool   `json:"password_change_required"`
		PasswordChangeToken    string `json:"password_change_token"`
	} `json:"data"`
}

type passportErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []any  `json:"data"`
}

type userInfoResponse struct {
	Code int `json:"code"`
	Data struct {
		ID             uint64 `json:"id"`
		Username       string `json:"username"`
		Nickname       string `json:"nickname"`
		Signed         string `json:"signed"`
		BackendSetting any    `json:"backend_setting"`
		Departments    []any  `json:"departments"`
		Positions      []any  `json:"positions"`
		Roles          []struct {
			ID   uint64 `json:"id"`
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"roles"`
	} `json:"data"`
}

type menuResponse struct {
	Code int        `json:"code"`
	Data []menuItem `json:"data"`
}

type menuItem struct {
	ID        uint64         `json:"id"`
	Name      string         `json:"name"`
	Component string         `json:"component"`
	Meta      map[string]any `json:"meta"`
	Children  []menuItem     `json:"children"`
}

type rolesResponse struct {
	Code int `json:"code"`
	Data []struct {
		ID   uint64 `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"data"`
}

func TestPassportTestSuite(t *testing.T) {
	suite.Run(t, new(PassportTestSuite))
}

func (s *PassportTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	_ = facades.Cache().Flush()
}

func (s *PassportTestSuite) TestLoginReturnsMineAdminTokenShape() {
	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)

	require.NoError(s.T(), err)
	res.AssertOk()

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.Equal(s.T(), "成功", body.Message)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	require.NotEmpty(s.T(), body.Data.RefreshToken)
	require.Equal(s.T(), 3600, body.Data.ExpireAt)
}

func (s *PassportTestSuite) TestLoginRejectsWrongPasswordWithBusinessCode() {
	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "wrong")),
	)

	require.NoError(s.T(), err)
	res.AssertOk()

	var body passportErrorResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 422, body.Code)
	require.NotEmpty(s.T(), body.Message)
	require.Empty(s.T(), body.Data)
}

func (s *PassportTestSuite) TestTenantMFADisableRequiresSensitiveEvidence() {
	s.enableMFA()
	code := s.enableTenantAdminMFA()
	token := s.loginTokenWithMFA(code)

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/security/mfa/disable", strings.NewReader(`{"code":"`+code+`"}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.True(s.T(), services.NewMFAServiceForTenant(s.defaultTenant()).Enabled(1))
}

func (s *PassportTestSuite) TestPlatformMFADisableRequiresSensitiveEvidence() {
	s.Seed(&seeders.PlatformBootstrapSeeder{})
	s.enableMFA()
	code := s.enablePlatformAdminMFA()
	token := s.platformLoginTokenWithMFA(code)

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/platform/security/mfa/disable", strings.NewReader(`{"code":"`+code+`"}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
	require.True(s.T(), services.NewPlatformMFAService().Enabled(1))
}

func (s *PassportTestSuite) TestLoginRejectsDisabledUserWithBusinessCode() {
	_, err := facades.Orm().Query().Table(`"user"`).Where("id", 1).Update("status", 2)
	require.NoError(s.T(), err)

	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	res.AssertOk()

	var body passportErrorResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 423, body.Code)
	require.Empty(s.T(), body.Data)
}

func (s *PassportTestSuite) TestMFALoginRejectsUserDisabledAfterChallenge() {
	s.enableMFA()
	code := s.enableTenantAdminMFA()

	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	res.AssertOk()

	var login passportResponse
	require.NoError(s.T(), res.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.MFARequired)
	require.NotEmpty(s.T(), login.Data.MFAToken)

	_, err = facades.Orm().Query().Table(`"user"`).Where("id", 1).Update("status", 2)
	require.NoError(s.T(), err)

	mfaRes, err := s.Http(s.T()).Post("/admin/passport/mfa/login", strings.NewReader(`{
		"mfa_token": "`+login.Data.MFAToken+`",
		"mfa_code": "`+code+`"
	}`))
	require.NoError(s.T(), err)
	mfaRes.AssertOk()

	var body passportErrorResponse
	require.NoError(s.T(), mfaRes.Bind(&body))
	require.Equal(s.T(), 423, body.Code)
	require.Empty(s.T(), body.Data)
}

func (s *PassportTestSuite) TestMFALoginKeepsChallengeAfterInvalidCode() {
	s.enableMFA()
	code := s.enableTenantAdminMFA()

	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	res.AssertOk()

	var login passportResponse
	require.NoError(s.T(), res.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.MFARequired)
	require.NotEmpty(s.T(), login.Data.MFAToken)

	invalidRes, err := s.Http(s.T()).Post("/admin/passport/mfa/login", strings.NewReader(`{
		"mfa_token": "`+login.Data.MFAToken+`",
		"mfa_code": "`+wrongMFACode(code)+`"
	}`))
	require.NoError(s.T(), err)
	invalidRes.AssertOk()

	var invalid passportErrorResponse
	require.NoError(s.T(), invalidRes.Bind(&invalid))
	require.Equal(s.T(), 422, invalid.Code)

	validRes, err := s.Http(s.T()).Post("/admin/passport/mfa/login", strings.NewReader(`{
		"mfa_token": "`+login.Data.MFAToken+`",
		"mfa_code": "`+code+`"
	}`))
	require.NoError(s.T(), err)
	validRes.AssertOk()

	var valid passportResponse
	require.NoError(s.T(), validRes.Bind(&valid))
	require.Equal(s.T(), 200, valid.Code)
	require.NotEmpty(s.T(), valid.Data.AccessToken)
}

func (s *PassportTestSuite) TestPlatformMFALoginRejectsUserDisabledAfterChallenge() {
	s.enableMFA()
	s.Seed(&seeders.PlatformAdminSeeder{})
	code := s.enablePlatformAdminMFA()

	res, err := s.Http(s.T()).Post(
		"/admin/platform/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	res.AssertOk()

	var login passportResponse
	require.NoError(s.T(), res.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.MFARequired)
	require.NotEmpty(s.T(), login.Data.MFAToken)

	_, err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_user").
		Where("id", 1).
		Update("status", 2)
	require.NoError(s.T(), err)

	mfaRes, err := s.Http(s.T()).Post("/admin/platform/passport/mfa/login", strings.NewReader(`{
		"mfa_token": "`+login.Data.MFAToken+`",
		"mfa_code": "`+code+`"
	}`))
	require.NoError(s.T(), err)
	mfaRes.AssertOk()

	var body passportErrorResponse
	require.NoError(s.T(), mfaRes.Bind(&body))
	require.Equal(s.T(), 423, body.Code)
	require.Empty(s.T(), body.Data)
}

func (s *PassportTestSuite) TestCurrentUserInfoMenusAndRoles() {
	token := s.loginToken()

	infoRes, err := s.Http(s.T()).WithToken(token).Get("/admin/passport/getInfo")
	require.NoError(s.T(), err)
	infoRes.AssertOk()

	var info userInfoResponse
	require.NoError(s.T(), infoRes.Bind(&info))
	require.Equal(s.T(), 200, info.Code)
	require.Equal(s.T(), uint64(1), info.Data.ID)
	require.Equal(s.T(), "admin", info.Data.Username)
	require.NotNil(s.T(), info.Data.Departments)
	require.NotNil(s.T(), info.Data.Positions)
	require.Len(s.T(), info.Data.Roles, 1)
	require.Equal(s.T(), "SuperAdmin", info.Data.Roles[0].Code)

	menuRes, err := s.Http(s.T()).WithToken(token).Get("/admin/permission/menus")
	require.NoError(s.T(), err)
	menuRes.AssertOk()

	var menus menuResponse
	require.NoError(s.T(), menuRes.Bind(&menus))
	require.Equal(s.T(), 200, menus.Code)
	require.NotEmpty(s.T(), menus.Data)
	require.NotNil(s.T(), menus.Data[0].Children)
	require.Nil(s.T(), findMenuItem(menus.Data, "platform"))
	require.Nil(s.T(), findMenuItem(menus.Data, "platform:tenant"))
	position := findMenuItem(menus.Data, "permission:position")
	require.NotNil(s.T(), position)
	require.Equal(s.T(), "base/views/permission/department/index", position.Component)
	require.Equal(s.T(), "baseMenu.permission.positionList", position.Meta["i18n"])
	require.Equal(s.T(), float64(0), position.Meta["cache"])
	leader := findMenuItem(menus.Data, "permission:leader")
	require.NotNil(s.T(), leader)
	require.Equal(s.T(), "base/views/permission/department/index", leader.Component)
	require.Equal(s.T(), "baseMenu.permission.leaderList", leader.Meta["i18n"])
	require.Equal(s.T(), float64(0), leader.Meta["cache"])
	userLoginLog := findMenuItem(menus.Data, "log:userLogin")
	require.NotNil(s.T(), userLoginLog)
	require.Equal(s.T(), "base/views/log/userLogin", userLoginLog.Component)
	userOperationLog := findMenuItem(menus.Data, "log:userOperation")
	require.NotNil(s.T(), userOperationLog)
	require.Equal(s.T(), "base/views/log/userOperation", userOperationLog.Component)
	expectedI18n := map[string]string{
		"permission:role:getMenu":             "baseMenu.permission.getRolePermission",
		"permission:role:setMenu":             "baseMenu.permission.setRolePermission",
		"permission:department:update":        "baseMenu.permission.departmentSave",
		"permission:position:update":          "baseMenu.permission.positionSave",
		"permission:position:data_permission": "baseMenu.permission.positionDataScope",
		"log:userLogin":                       "baseMenu.log.userLoginLog",
		"log:userOperation":                   "baseMenu.log.operationLog",
	}
	for name, i18n := range expectedI18n {
		item := findMenuItem(menus.Data, name)
		require.NotNil(s.T(), item, name)
		require.Equal(s.T(), i18n, item.Meta["i18n"], name)
	}

	roleRes, err := s.Http(s.T()).WithToken(token).Get("/admin/permission/roles")
	require.NoError(s.T(), err)
	roleRes.AssertOk()

	var roles rolesResponse
	require.NoError(s.T(), roleRes.Bind(&roles))
	require.Equal(s.T(), 200, roles.Code)
	require.Len(s.T(), roles.Data, 1)
	require.Equal(s.T(), "SuperAdmin", roles.Data[0].Code)
}

func (s *PassportTestSuite) TestMenusNormalizeLegacyDepartmentActionComponents() {
	require.NoError(s.T(), execSQL(`
		UPDATE menu
		SET component = 'base/views/permission/department/setLeader'
		WHERE name = 'permission:leader'
	`))

	token := s.loginToken()
	menuRes, err := s.Http(s.T()).WithToken(token).Get("/admin/permission/menus")
	require.NoError(s.T(), err)
	menuRes.AssertOk()

	var menus menuResponse
	require.NoError(s.T(), menuRes.Bind(&menus))
	leader := findMenuItem(menus.Data, "permission:leader")
	require.NotNil(s.T(), leader)
	require.Equal(s.T(), "base/views/permission/department/index", leader.Component)
}

func (s *PassportTestSuite) TestCurrentUserInfoOmitsEmptyBackendSetting() {
	token := s.loginToken()

	infoRes, err := s.Http(s.T()).WithToken(token).Get("/admin/passport/getInfo")
	require.NoError(s.T(), err)
	infoRes.AssertOk()

	payload, err := infoRes.Json()
	require.NoError(s.T(), err)
	data := payload["data"].(map[string]any)
	_, exists := data["backend_setting"]
	require.False(s.T(), exists)
}

func (s *PassportTestSuite) TestRefreshReturnsNewAccessTokenFromRefreshToken() {
	login := s.login()

	res, err := s.Http(s.T()).WithToken(login.Data.RefreshToken).Post("/admin/passport/refresh", nil)
	require.NoError(s.T(), err)
	res.AssertOk()

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)
	require.NotEmpty(s.T(), body.Data.RefreshToken)
	require.Equal(s.T(), 3600, body.Data.ExpireAt)
	require.NotEqual(s.T(), login.Data.AccessToken, body.Data.AccessToken)

	reused, err := s.Http(s.T()).WithToken(login.Data.RefreshToken).Post("/admin/passport/refresh", nil)
	require.NoError(s.T(), err)
	reused.AssertOk()

	var reusedBody passportErrorResponse
	require.NoError(s.T(), reused.Bind(&reusedBody))
	require.Equal(s.T(), 401, reusedBody.Code)
}

func (s *PassportTestSuite) TestLogoutInvalidatesAccessToken() {
	token := s.loginToken()

	res, err := s.Http(s.T()).WithToken(token).Post("/admin/passport/logout", nil)
	require.NoError(s.T(), err)
	res.AssertOk()

	var logout passportErrorResponse
	require.NoError(s.T(), res.Bind(&logout))
	require.Equal(s.T(), 200, logout.Code)
	require.Empty(s.T(), logout.Data)

	infoRes, err := s.Http(s.T()).WithToken(token).Get("/admin/passport/getInfo")
	require.NoError(s.T(), err)
	infoRes.AssertOk()

	var info passportErrorResponse
	require.NoError(s.T(), infoRes.Bind(&info))
	require.Equal(s.T(), 401, info.Code)
}

func (s *PassportTestSuite) TestPermissionUpdateAndUserInfoAliasPersistProfile() {
	token := s.loginToken()

	updateRes, err := s.Http(s.T()).WithToken(token).Post("/admin/permission/update", strings.NewReader(`{
		"nickname": "Mine 管理员",
		"signed": "更新后的签名",
		"backend_setting": {"app": {"useLocale": "zh_CN"}}
	}`))
	require.NoError(s.T(), err)
	updateRes.AssertOk()

	var update passportErrorResponse
	require.NoError(s.T(), updateRes.Bind(&update))
	require.Equal(s.T(), 200, update.Code)
	require.Empty(s.T(), update.Data)

	aliasRes, err := s.Http(s.T()).WithToken(token).Put("/admin/user/info", strings.NewReader(`{
		"nickname": "Mine Alias",
		"signed": "别名更新"
	}`))
	require.NoError(s.T(), err)
	aliasRes.AssertOk()

	infoRes, err := s.Http(s.T()).WithToken(token).Get("/admin/passport/getInfo")
	require.NoError(s.T(), err)
	infoRes.AssertOk()

	var info userInfoResponse
	require.NoError(s.T(), infoRes.Bind(&info))
	require.Equal(s.T(), "Mine Alias", info.Data.Nickname)
	require.Equal(s.T(), "别名更新", info.Data.Signed)
	require.NotNil(s.T(), info.Data.BackendSetting)

	settingBytes, err := json.Marshal(info.Data.BackendSetting)
	require.NoError(s.T(), err)
	require.JSONEq(s.T(), `{"app":{"useLocale":"zh_CN"}}`, string(settingBytes))
}

func (s *PassportTestSuite) TestPermissionUpdateChangesCurrentPassword() {
	token := s.loginToken()

	updateRes, err := s.Http(s.T()).WithToken(token).Post("/admin/permission/update", strings.NewReader(`{
		"old_password": "123456",
		"new_password": "new-password-123",
		"new_password_confirmation": "new-password-123"
	}`))
	require.NoError(s.T(), err)
	updateRes.AssertOk()

	var update passportErrorResponse
	require.NoError(s.T(), updateRes.Bind(&update))
	require.Equal(s.T(), 200, update.Code)
	require.Empty(s.T(), update.Data)

	oldLoginRes, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	oldLogin, err := oldLoginRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), oldLogin["code"])

	newLoginRes, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "new-password-123")),
	)
	require.NoError(s.T(), err)
	var newLogin passportResponse
	require.NoError(s.T(), newLoginRes.Bind(&newLogin))
	require.Equal(s.T(), 200, newLogin.Code)
	require.NotEmpty(s.T(), newLogin.Data.AccessToken)
}

func (s *PassportTestSuite) TestExpiredPasswordLoginAllowsTenantPasswordChange() {
	s.forceTenantPasswordExpired()

	loginRes, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	loginRes.AssertOk()

	var login passportResponse
	require.NoError(s.T(), loginRes.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.PasswordChangeRequired)
	require.NotEmpty(s.T(), login.Data.PasswordChangeToken)
	require.Empty(s.T(), login.Data.AccessToken)

	changeRes, err := s.Http(s.T()).Post("/admin/passport/password/change", strings.NewReader(`{
		"password_change_token": "`+login.Data.PasswordChangeToken+`",
		"old_password": "123456",
		"new_password": "changed-password-123",
		"new_password_confirmation": "changed-password-123"
	}`))
	require.NoError(s.T(), err)
	changeRes.AssertOk()

	var changed passportResponse
	require.NoError(s.T(), changeRes.Bind(&changed))
	require.Equal(s.T(), 200, changed.Code)
	require.NotEmpty(s.T(), changed.Data.AccessToken)
	require.NotEmpty(s.T(), changed.Data.RefreshToken)
	require.False(s.T(), changed.Data.PasswordChangeRequired)

	oldLoginRes, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	oldLogin, err := oldLoginRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), oldLogin["code"])
}

func (s *PassportTestSuite) TestExpiredPasswordChangeKeepsChallengeAfterValidationError() {
	s.forceTenantPasswordExpired()

	loginRes, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	loginRes.AssertOk()

	var login passportResponse
	require.NoError(s.T(), loginRes.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.PasswordChangeRequired)
	require.NotEmpty(s.T(), login.Data.PasswordChangeToken)

	invalidRes, err := s.Http(s.T()).Post("/admin/passport/password/change", strings.NewReader(`{
		"password_change_token": "`+login.Data.PasswordChangeToken+`",
		"old_password": "wrong-password",
		"new_password": "changed-after-error-123",
		"new_password_confirmation": "changed-after-error-123"
	}`))
	require.NoError(s.T(), err)
	invalidRes.AssertOk()

	var invalid passportErrorResponse
	require.NoError(s.T(), invalidRes.Bind(&invalid))
	require.Equal(s.T(), 422, invalid.Code)

	validRes, err := s.Http(s.T()).Post("/admin/passport/password/change", strings.NewReader(`{
		"password_change_token": "`+login.Data.PasswordChangeToken+`",
		"old_password": "123456",
		"new_password": "changed-after-error-123",
		"new_password_confirmation": "changed-after-error-123"
	}`))
	require.NoError(s.T(), err)
	validRes.AssertOk()

	var valid passportResponse
	require.NoError(s.T(), validRes.Bind(&valid))
	require.Equal(s.T(), 200, valid.Code)
	require.NotEmpty(s.T(), valid.Data.AccessToken)
	require.NotEmpty(s.T(), valid.Data.RefreshToken)
}

func (s *PassportTestSuite) TestExpiredPasswordLoginRequiresTenantMFABeforePasswordChange() {
	s.enableMFA()
	code := s.enableTenantAdminMFA()
	s.forceTenantPasswordExpired()

	loginRes, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	loginRes.AssertOk()

	var login passportResponse
	require.NoError(s.T(), loginRes.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.MFARequired)
	require.NotEmpty(s.T(), login.Data.MFAToken)
	require.False(s.T(), login.Data.PasswordChangeRequired)
	require.Empty(s.T(), login.Data.PasswordChangeToken)

	mfaRes, err := s.Http(s.T()).Post("/admin/passport/mfa/login", strings.NewReader(`{
		"mfa_token": "`+login.Data.MFAToken+`",
		"mfa_code": "`+code+`"
	}`))
	require.NoError(s.T(), err)
	mfaRes.AssertOk()

	var mfa passportResponse
	require.NoError(s.T(), mfaRes.Bind(&mfa))
	require.Equal(s.T(), 200, mfa.Code)
	require.True(s.T(), mfa.Data.PasswordChangeRequired)
	require.NotEmpty(s.T(), mfa.Data.PasswordChangeToken)
	require.Empty(s.T(), mfa.Data.AccessToken)

	changeRes, err := s.Http(s.T()).Post("/admin/passport/password/change", strings.NewReader(`{
		"password_change_token": "`+mfa.Data.PasswordChangeToken+`",
		"old_password": "123456",
		"new_password": "changed-mfa-password-123",
		"new_password_confirmation": "changed-mfa-password-123"
	}`))
	require.NoError(s.T(), err)
	changeRes.AssertOk()

	var changed passportResponse
	require.NoError(s.T(), changeRes.Bind(&changed))
	require.Equal(s.T(), 200, changed.Code)
	require.NotEmpty(s.T(), changed.Data.AccessToken)
	require.NotEmpty(s.T(), changed.Data.RefreshToken)
	require.False(s.T(), changed.Data.MFARequired)
	require.False(s.T(), changed.Data.PasswordChangeRequired)
}

func (s *PassportTestSuite) TestExpiredPasswordLoginAllowsPlatformPasswordChange() {
	s.forcePlatformPasswordExpired()

	loginRes, err := s.Http(s.T()).Post(
		"/admin/platform/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	loginRes.AssertOk()

	var login passportResponse
	require.NoError(s.T(), loginRes.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.PasswordChangeRequired)
	require.NotEmpty(s.T(), login.Data.PasswordChangeToken)
	require.Empty(s.T(), login.Data.AccessToken)

	changeRes, err := s.Http(s.T()).Post("/admin/platform/passport/password/change", strings.NewReader(`{
		"password_change_token": "`+login.Data.PasswordChangeToken+`",
		"old_password": "123456",
		"new_password": "platform-password-123",
		"new_password_confirmation": "platform-password-123"
	}`))
	require.NoError(s.T(), err)
	changeRes.AssertOk()

	var changed passportResponse
	require.NoError(s.T(), changeRes.Bind(&changed))
	require.Equal(s.T(), 200, changed.Code)
	require.NotEmpty(s.T(), changed.Data.AccessToken)
	require.NotEmpty(s.T(), changed.Data.RefreshToken)
	require.False(s.T(), changed.Data.PasswordChangeRequired)
}

func (s *PassportTestSuite) TestExpiredPasswordLoginRequiresPlatformMFABeforePasswordChange() {
	s.enableMFA()
	s.Seed(&seeders.PlatformAdminSeeder{})
	code := s.enablePlatformAdminMFA()
	s.forcePlatformPasswordExpired()

	loginRes, err := s.Http(s.T()).Post(
		"/admin/platform/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)
	loginRes.AssertOk()

	var login passportResponse
	require.NoError(s.T(), loginRes.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.MFARequired)
	require.NotEmpty(s.T(), login.Data.MFAToken)
	require.False(s.T(), login.Data.PasswordChangeRequired)
	require.Empty(s.T(), login.Data.PasswordChangeToken)

	mfaRes, err := s.Http(s.T()).Post("/admin/platform/passport/mfa/login", strings.NewReader(`{
		"mfa_token": "`+login.Data.MFAToken+`",
		"mfa_code": "`+code+`"
	}`))
	require.NoError(s.T(), err)
	mfaRes.AssertOk()

	var mfa passportResponse
	require.NoError(s.T(), mfaRes.Bind(&mfa))
	require.Equal(s.T(), 200, mfa.Code)
	require.True(s.T(), mfa.Data.PasswordChangeRequired)
	require.NotEmpty(s.T(), mfa.Data.PasswordChangeToken)
	require.Empty(s.T(), mfa.Data.AccessToken)

	changeRes, err := s.Http(s.T()).Post("/admin/platform/passport/password/change", strings.NewReader(`{
		"password_change_token": "`+mfa.Data.PasswordChangeToken+`",
		"old_password": "123456",
		"new_password": "changed-platform-mfa-password-123",
		"new_password_confirmation": "changed-platform-mfa-password-123"
	}`))
	require.NoError(s.T(), err)
	changeRes.AssertOk()

	var changed passportResponse
	require.NoError(s.T(), changeRes.Bind(&changed))
	require.Equal(s.T(), 200, changed.Code)
	require.NotEmpty(s.T(), changed.Data.AccessToken)
	require.NotEmpty(s.T(), changed.Data.RefreshToken)
	require.False(s.T(), changed.Data.MFARequired)
	require.False(s.T(), changed.Data.PasswordChangeRequired)
}

func (s *PassportTestSuite) loginToken() string {
	return s.login().Data.AccessToken
}

func (s *PassportTestSuite) loginTokenWithMFA(code string) string {
	loginRes, err := s.Http(s.T()).Post("/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")))
	require.NoError(s.T(), err)
	var login passportResponse
	require.NoError(s.T(), loginRes.Bind(&login))
	require.Equal(s.T(), 200, login.Code)
	require.True(s.T(), login.Data.MFARequired)
	require.NotEmpty(s.T(), login.Data.MFAToken)
	res, err := s.Http(s.T()).Post("/admin/passport/mfa/login", strings.NewReader(`{
		"mfa_token":"`+login.Data.MFAToken+`","mfa_code":"`+code+`"
	}`))
	require.NoError(s.T(), err)
	var authenticated passportResponse
	require.NoError(s.T(), res.Bind(&authenticated))
	require.Equal(s.T(), 200, authenticated.Code)
	return authenticated.Data.AccessToken
}

func (s *PassportTestSuite) platformLoginTokenWithMFA(code string) string {
	loginRes, err := s.Http(s.T()).Post("/admin/platform/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")))
	require.NoError(s.T(), err)
	var login passportResponse
	require.NoError(s.T(), loginRes.Bind(&login))
	mfaRes, err := s.Http(s.T()).Post("/admin/platform/passport/mfa/login", strings.NewReader(`{
		"mfa_token":"`+login.Data.MFAToken+`","mfa_code":"`+code+`"
	}`))
	require.NoError(s.T(), err)
	var authenticated passportResponse
	require.NoError(s.T(), mfaRes.Bind(&authenticated))
	require.Equal(s.T(), 200, authenticated.Code)
	return authenticated.Data.AccessToken
}

func (s *PassportTestSuite) defaultTenant() services.Tenant {
	tenant, err := services.NewTenantService().Resolve("default")
	require.NoError(s.T(), err)
	return tenant
}

func (s *PassportTestSuite) login() passportResponse {
	res, err := s.Http(s.T()).Post(
		"/admin/passport/login",
		strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
	)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.NotEmpty(s.T(), body.Data.AccessToken)

	return body
}

func (s *PassportTestSuite) enableMFA() {
	original := facades.Config().GetBool("security.mfa.totp_enabled", false)
	facades.Config().Add("security.mfa.totp_enabled", true)
	s.T().Cleanup(func() {
		facades.Config().Add("security.mfa.totp_enabled", original)
	})
}

func (s *PassportTestSuite) enableTenantAdminMFA() string {
	secret, code := s.mfaSecretAndCode()
	require.NoError(s.T(), execSQL(`
		INSERT INTO user_mfa (user_id, secret, enabled, recovery_codes, confirmed_at, created_at, updated_at)
		VALUES (1, ?, true, '[]'::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET
			secret = EXCLUDED.secret,
			enabled = true,
			recovery_codes = EXCLUDED.recovery_codes,
			confirmed_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`, secret))
	return code
}

func (s *PassportTestSuite) enablePlatformAdminMFA() string {
	secret, code := s.mfaSecretAndCode()
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec(`
			INSERT INTO platform_user_mfa (user_id, secret, enabled, recovery_codes, confirmed_at, created_at, updated_at)
			VALUES (1, ?, true, '[]'::jsonb, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id) DO UPDATE SET
				secret = EXCLUDED.secret,
				enabled = true,
				recovery_codes = EXCLUDED.recovery_codes,
				confirmed_at = CURRENT_TIMESTAMP,
				updated_at = CURRENT_TIMESTAMP
		`, secret)
	require.NoError(s.T(), err)
	return code
}

func (s *PassportTestSuite) mfaSecretAndCode() (string, string) {
	secret, err := services.GenerateTOTPSecret()
	require.NoError(s.T(), err)
	raw, err := services.DecodeTOTPSecret(secret)
	require.NoError(s.T(), err)
	code, err := services.GenerateTOTPCode(raw, time.Now(), 30, 6)
	require.NoError(s.T(), err)
	return secret, code
}

func wrongMFACode(code string) string {
	if code == "000000" {
		return "000001"
	}
	return "000000"
}

func (s *PassportTestSuite) forceTenantPasswordExpired() {
	s.setPasswordMaxAge(1)
	require.NoError(s.T(), execSQL(`
		INSERT INTO user_password_history (user_id, password, created_at, updated_at)
		SELECT id, password, CURRENT_TIMESTAMP - INTERVAL '48 hours', CURRENT_TIMESTAMP - INTERVAL '48 hours'
		FROM "user"
		WHERE id = 1
	`))
}

func (s *PassportTestSuite) forcePlatformPasswordExpired() {
	s.setPasswordMaxAge(1)
	require.NoError(s.T(), services.NewPlatformBootstrapService().EnsureLocalDefaults())
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec(`
			INSERT INTO platform_user_password_history (user_id, password, created_at, updated_at)
			SELECT id, password, CURRENT_TIMESTAMP - INTERVAL '48 hours', CURRENT_TIMESTAMP - INTERVAL '48 hours'
			FROM platform_user
			WHERE id = 1
		`)
	require.NoError(s.T(), err)
}

func (s *PassportTestSuite) setPasswordMaxAge(days int) {
	original := facades.Config().GetInt("security.password.max_age_days", 0)
	facades.Config().Add("security.password.max_age_days", days)
	s.T().Cleanup(func() {
		facades.Config().Add("security.password.max_age_days", original)
	})
}

func findMenuItem(menus []menuItem, name string) *menuItem {
	for i := range menus {
		if menus[i].Name == name {
			return &menus[i]
		}
		if child := findMenuItem(menus[i].Children, name); child != nil {
			return child
		}
	}

	return nil
}
