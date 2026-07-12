package admin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/database/seeders"
	"goravel/tests"
)

type CaptchaTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestCaptchaTestSuite(t *testing.T) {
	suite.Run(t, new(CaptchaTestSuite))
}

func (s *CaptchaTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	_ = facades.Cache().Flush()
}

func (s *CaptchaTestSuite) TestCaptchaEndpointReturnsImageAndStoresAnswer() {
	body := s.captcha()
	data := body["data"].(map[string]any)
	key := data["key"].(string)
	base64Image := data["base64"].(string)

	require.NotEmpty(s.T(), key)
	require.True(s.T(), strings.HasPrefix(base64Image, "data:image/png;base64,"))
	require.NotEmpty(s.T(), facades.Cache().GetString("captcha:"+key))
}

func (s *CaptchaTestSuite) TestLoginRequiresServerCaptchaKey() {
	loginWithoutKey := s.login(`{
		"username": "admin",
		"password": "123456",
		"code": "local-canvas-code"
	}`)
	require.Equal(s.T(), float64(422), loginWithoutKey["code"])

	captcha := s.captcha()["data"].(map[string]any)
	key := captcha["key"].(string)
	answer := facades.Cache().GetString("captcha:" + key)
	require.NotEmpty(s.T(), answer)

	wrong := s.login(`{
		"username": "admin",
		"password": "123456",
		"captcha_key": "` + key + `",
		"code": "wrong"
	}`)
	require.Equal(s.T(), float64(422), wrong["code"])

	captcha = s.captcha()["data"].(map[string]any)
	key = captcha["key"].(string)
	answer = facades.Cache().GetString("captcha:" + key)
	ok := s.login(`{
		"username": "admin",
		"password": "123456",
		"captcha_key": "` + key + `",
		"code": "` + answer + `"
	}`)
	require.Equal(s.T(), float64(200), ok["code"])
}

func (s *CaptchaTestSuite) captcha() map[string]any {
	res, err := s.Http(s.T()).Get("/admin/passport/captcha")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	return body
}

func (s *CaptchaTestSuite) login(payload string) map[string]any {
	res, err := s.Http(s.T()).Post("/admin/passport/login", strings.NewReader(payload))
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}
