package admin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type TenantPassportTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestTenantPassportTestSuite(t *testing.T) {
	suite.Run(t, new(TenantPassportTestSuite))
}

func (s *TenantPassportTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
}

func (s *TenantPassportTestSuite) TestLoginBindsAccessTokenToTenant() {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
		)
	require.NoError(s.T(), err)
	res.AssertOk()

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	require.NotEmpty(s.T(), body.Data.AccessToken)

	okRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(body.Data.AccessToken).
		Get("/admin/passport/getInfo")
	require.NoError(s.T(), err)
	okRes.AssertOk()
	okBody, err := okRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), okBody["code"])

	wrongRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "other").
		WithToken(body.Data.AccessToken).
		Get("/admin/passport/getInfo")
	require.NoError(s.T(), err)
	wrongRes.AssertOk()
	wrongBody, err := wrongRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(401), wrongBody["code"])
}

func (s *TenantPassportTestSuite) TestEntryShowsPlatformLoginForUnboundHost() {
	res, err := s.Http(s.T()).
		WithHeader("Host", "unbound.example.test").
		Get("/admin/passport/entry")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "platform", data["mode"])
	require.Equal(s.T(), true, data["available"])
	require.Empty(s.T(), data["message"])
	require.Nil(s.T(), data["tenant"])
}

func (s *TenantPassportTestSuite) TestEntryShowsTenantLoginForBoundHost() {
	s.bindDefaultTenantDomain("tenant.example.test", services.TenantStatusActive)

	res, err := s.Http(s.T()).
		WithHeader("Host", "tenant.example.test").
		Get("/admin/passport/entry")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "tenant", data["mode"])
	require.Equal(s.T(), true, data["available"])
	require.Empty(s.T(), data["message"])

	tenant := data["tenant"].(map[string]any)
	require.Equal(s.T(), "default", tenant["code"])
	require.Equal(s.T(), "默认租户", tenant["name"])
}

func (s *TenantPassportTestSuite) TestEntryUsesForwardedHostBeforeProxyHost() {
	s.bindDefaultTenantDomain("tenant.example.test", services.TenantStatusActive)
	s.trustForwardedHostProxies("192.0.2.1")

	res, err := s.Http(s.T()).
		WithHeader("Host", "127.0.0.1:3000").
		WithHeader("X-Forwarded-Host", "client.example.test, tenant.example.test:2888").
		Get("/admin/passport/entry")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "tenant", data["mode"])
	require.Equal(s.T(), true, data["available"])
}

func (s *TenantPassportTestSuite) TestEntryIgnoresUntrustedForwardedHost() {
	s.bindDefaultTenantDomain("tenant.example.test", services.TenantStatusActive)

	res, err := s.Http(s.T()).
		WithHeader("Host", "unbound.example.test").
		WithHeader("X-Forwarded-Host", "tenant.example.test").
		Get("/admin/passport/entry")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "platform", data["mode"])
	require.Equal(s.T(), true, data["available"])
}

func (s *TenantPassportTestSuite) TestEntryIgnoresForwardedHostFromUntrustedProxy() {
	s.bindDefaultTenantDomain("tenant.example.test", services.TenantStatusActive)
	s.trustForwardedHostProxies("127.0.0.1")

	res, err := s.Http(s.T()).
		WithHeader("Host", "unbound.example.test").
		WithHeader("X-Forwarded-Host", "tenant.example.test").
		Get("/admin/passport/entry")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "platform", data["mode"])
	require.Equal(s.T(), true, data["available"])
}

func (s *TenantPassportTestSuite) TestEntryDoesNotFallbackToPlatformWhenTenantLookupFails() {
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec("ALTER TABLE tenant RENAME COLUMN custom_domain TO custom_domain_lookup_failure")
	require.NoError(s.T(), err)
	s.T().Cleanup(func() {
		_, _ = facades.Orm().
			Connection(services.PlatformConnection()).
			Query().
			Exec("ALTER TABLE tenant RENAME COLUMN custom_domain_lookup_failure TO custom_domain")
	})

	res, err := s.Http(s.T()).
		WithHeader("Host", "tenant.example.test").
		Get("/admin/passport/entry")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "tenant", data["mode"])
	require.Equal(s.T(), false, data["available"])
	require.Equal(s.T(), "登录入口加载失败，请稍后重试", data["message"])
	require.Nil(s.T(), data["tenant"])
}

func (s *TenantPassportTestSuite) TestEntryShowsTenantDisabledMessageForSuspendedBoundHost() {
	s.bindDefaultTenantDomain("tenant.example.test", services.TenantStatusSuspended)

	res, err := s.Http(s.T()).
		WithHeader("Host", "tenant.example.test").
		Get("/admin/passport/entry")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])

	data := body["data"].(map[string]any)
	require.Equal(s.T(), "tenant", data["mode"])
	require.Equal(s.T(), false, data["available"])
	require.Equal(s.T(), "租户不存在或已停用", data["message"])

	tenant := data["tenant"].(map[string]any)
	require.Equal(s.T(), "default", tenant["code"])
	require.Equal(s.T(), float64(2), tenant["status"])
}

func (s *TenantPassportTestSuite) bindDefaultTenantDomain(host string, status int8) {
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("tenant").
		Where("code", "default").
		Update(map[string]any{
			"custom_domain": host,
			"status":        status,
		})
	require.NoError(s.T(), err)
}

func (s *TenantPassportTestSuite) trustForwardedHostProxies(value string) {
	original := facades.Config().GetString("tenant.trusted_forwarded_host_proxies")
	facades.Config().Add("tenant.trusted_forwarded_host_proxies", value)
	s.T().Cleanup(func() {
		facades.Config().Add("tenant.trusted_forwarded_host_proxies", original)
	})
}
