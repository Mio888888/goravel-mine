package admin

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/golang-jwt/jwt/v5"
	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/russellhaering/goxmldsig"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests"
)

type SSOProviderTestSuite struct {
	suite.Suite
	tests.TestCase
	token    string
	rsaKey   *rsa.PrivateKey
	rsaCert  *x509.Certificate
	certPEM  string
	jwksJSON string
}

func TestSSOProviderTestSuite(t *testing.T) {
	suite.Run(t, new(SSOProviderTestSuite))
}

func (s *SSOProviderTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	s.token = s.loginAs("admin", "123456")
	s.rsaKey, s.rsaCert, s.certPEM = s.testCertificate()
	s.jwksJSON = s.testJWKS(s.rsaCert)
}

func (s *SSOProviderTestSuite) TestTenantSSOProviderSecretCreateRequiresBoundEvidence() {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(s.token).
		Post("/admin/sso-provider", strings.NewReader(`{
			"name": "guarded-secret-provider",
			"display_name": "Guarded Secret Provider",
			"scene": "admin",
			"type": "oidc",
			"enabled": true,
			"jwt_secret": "top-secret"
		}`))
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
}

func (s *SSOProviderTestSuite) TestTenantSSOProviderSensitiveMutationsRequireEvidence() {
	providerID := s.createProvider(`{
		"name":"guarded-provider","display_name":"Guarded Provider","scene":"admin","type":"oidc",
		"enabled":true,"issuer":"https://issuer.example.test","client_id":"client"
	}`)

	update, err := s.Http(s.T()).WithHeader("X-Tenant-Code", "default").WithToken(s.token).
		Put("/admin/sso-provider/"+itoa(providerID), strings.NewReader(`{
			"name":"guarded-provider","display_name":"Guarded Provider","scene":"admin","type":"oidc",
			"enabled":true,"issuer":"https://other.example.test","client_id":"client"
		}`))
	require.NoError(s.T(), err)
	update.AssertOk()
	updateBody, err := update.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), updateBody["code"])

	deleted, err := s.Http(s.T()).WithHeader("X-Tenant-Code", "default").WithToken(s.token).
		Delete("/admin/sso-provider", strings.NewReader(`{"ids":[`+itoa(providerID)+`]}`))
	require.NoError(s.T(), err)
	deleted.AssertOk()
	deleteBody, err := deleted.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), deleteBody["code"])

	tenant, err := services.NewTenantService().Resolve("default")
	require.NoError(s.T(), err)
	var provider models.SSOProvider
	require.NoError(s.T(), facades.Orm().Connection(services.TenantConnectionName(tenant)).Query().
		Table("sso_provider").Where("id", providerID).First(&provider))
	require.Equal(s.T(), "https://issuer.example.test", provider.Issuer)
}

func (s *SSOProviderTestSuite) TestTenantCanManageMultipleProvidersByScene() {
	s.createProvider(`{
		"name": "okta-admin",
		"display_name": "Okta Admin",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://issuer.example.test",
		"audience": "goravel-mine-admin",
		"jwt_secret": "admin-secret",
		"client_id": "admin-client",
		"client_secret": "admin-client-secret",
		"authorization_endpoint": "https://issuer.example.test/oauth2/v1/authorize",
		"token_endpoint": "https://issuer.example.test/oauth2/v1/token",
		"userinfo_endpoint": "https://issuer.example.test/oauth2/v1/userinfo",
		"jwks_uri": "https://issuer.example.test/oauth2/v1/keys",
		"scope": "openid profile email",
		"redirect_uri": "http://localhost:2888/login",
		"enable_pkce": true,
		"enable_nonce": true,
		"auto_create": true,
		"icon": "logos:okta",
		"button_color": "#2563eb",
		"display_order": 10,
		"role_mapping": {
			"claim": "groups",
			"default": ["SuperAdmin"],
			"mapping": {
				"admins": ["SuperAdmin"]
			}
		}
	}`)
	s.createProvider(`{
		"name": "github-mobile",
		"display_name": "GitHub Mobile",
		"scene": "mobile",
		"type": "oauth2",
		"enabled": true,
		"client_id": "mobile-client",
		"client_secret": "mobile-secret",
		"authorization_endpoint": "https://github.com/login/oauth/authorize",
		"token_endpoint": "https://github.com/login/oauth/access_token",
		"display_order": 20
	}`)
	s.createProvider(`{
		"name": "corp-saml",
		"display_name": "Corp SAML",
		"scene": "admin",
		"type": "saml",
		"enabled": false,
		"saml_entrypoint": "https://idp.example.test/saml/sso",
		"saml_entity_id": "goravel-mine",
		"saml_certificate": "-----BEGIN CERTIFICATE-----test-----END CERTIFICATE-----"
	}`)

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(s.token).
		Get("/admin/sso-provider/list?page=1&page_size=20")
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), body["code"])
	list := body["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), list, 3)

	brandingRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Get("/admin/passport/branding?scene=admin")
	require.NoError(s.T(), err)
	brandingRes.AssertOk()
	branding, err := brandingRes.Json()
	require.NoError(s.T(), err)
	providers := branding["data"].(map[string]any)["features"].(map[string]any)["sso"].(map[string]any)["providers"].([]any)
	require.Len(s.T(), providers, 1)
	require.Equal(s.T(), "okta-admin", providers[0].(map[string]any)["name"])
	require.Equal(s.T(), "Okta Admin", providers[0].(map[string]any)["display_name"])

	mobileRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Get("/admin/passport/branding?scene=mobile")
	require.NoError(s.T(), err)
	mobile, err := mobileRes.Json()
	require.NoError(s.T(), err)
	mobileProviders := mobile["data"].(map[string]any)["features"].(map[string]any)["sso"].(map[string]any)["providers"].([]any)
	require.Len(s.T(), mobileProviders, 1)
	require.Equal(s.T(), "github-mobile", mobileProviders[0].(map[string]any)["name"])
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginUsesProviderTableAndKeepsPasswordLoginEnabled() {
	s.createProvider(`{
		"name": "okta-admin",
		"display_name": "Okta Admin",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://idp.example.test",
		"audience": "mineadmin",
		"jwt_secret": "tenant-sso-secret",
		"auto_create": true,
		"role_mapping": {
			"default": ["SuperAdmin"]
		}
	}`)

	passwordRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "admin", "123456")),
		)
	require.NoError(s.T(), err)
	passwordRes.AssertOk()
	passwordBody, err := passwordRes.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(200), passwordBody["code"])

	idToken := s.oidcToken("tenant-sso-secret", map[string]any{
		"iss":   "https://idp.example.test",
		"aud":   "mineadmin",
		"sub":   "okta-user-1",
		"email": "sso-user@example.test",
		"name":  "SSO User",
	})
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta-admin",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)

	var email string
	err = facades.Orm().
		Query().
		Table(`"user"`).
		Where("username", "okta-user-1").
		Pluck("email", &email)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "sso-user@example.test", email)

	s.requireUserRole(s.userID("okta-user-1"), "SuperAdmin")
}

func (s *SSOProviderTestSuite) TestTenantSSOManagedUserCannotLoginWithSentinelPassword() {
	s.createProvider(`{
		"name": "okta-managed",
		"display_name": "Okta Managed",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://managed-idp.example.test",
		"audience": "mineadmin",
		"jwt_secret": "managed-secret",
		"auto_create": true
	}`)
	idToken := s.oidcToken("managed-secret", map[string]any{
		"iss":   "https://managed-idp.example.test",
		"aud":   "mineadmin",
		"sub":   "managed-user-1",
		"email": "managed-user@example.test",
		"name":  "Managed User",
	})
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta-managed",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 200)

	passwordRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), "managed-user-1", "__sso_managed__")),
		)
	require.NoError(s.T(), err)
	passwordRes.AssertOk()
	passwordBody, err := passwordRes.Json()
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), float64(200), passwordBody["code"])
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginAutoCreateSupportsLongSubject() {
	s.createProvider(`{
		"name": "okta-long-sub",
		"display_name": "Okta Long Subject",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://long-sub-idp.example.test",
		"audience": "mineadmin",
		"jwt_secret": "long-sub-secret",
		"auto_create": true
	}`)
	subject := "00u1234567890abcdef-long-subject-value"
	idToken := s.oidcToken("long-sub-secret", map[string]any{
		"iss":                "https://long-sub-idp.example.test",
		"aud":                "mineadmin",
		"sub":                subject,
		"preferred_username": "Long Subject User",
		"email":              "long-subject-user@example.test",
		"name":               "Long Subject User",
	})
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta-long-sub",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 200)

	var binding struct {
		UserID    uint64 `gorm:"column:user_id"`
		SSOUserID string `gorm:"column:sso_user_id"`
	}
	require.NoError(s.T(), facades.Orm().Query().
		Table("sso_user_binding").
		Where("sso_user_id", subject).
		First(&binding))

	var username string
	require.NoError(s.T(), facades.Orm().Query().
		Table(`"user"`).
		Where("id", binding.UserID).
		Pluck("username", &username))
	require.NotEmpty(s.T(), username)
	require.NotEqual(s.T(), subject, username)
	require.LessOrEqual(s.T(), len(username), 20)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginCreatesBindingAndLogsAttempts() {
	providerID := s.createProvider(`{
		"name": "okta-binding",
		"display_name": "Okta Binding",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://binding-idp.example.test",
		"audience": "mineadmin",
		"jwt_secret": "binding-secret",
		"auto_create": true
	}`)
	idToken := s.oidcToken("binding-secret", map[string]any{
		"iss":     "https://binding-idp.example.test",
		"aud":     "mineadmin",
		"sub":     "binding-user-1",
		"email":   "binding-user@example.test",
		"name":    "Binding User",
		"picture": "https://example.test/avatar.png",
	})

	for i := 0; i < 2; i++ {
		res, err := s.Http(s.T()).
			WithHeader("X-Tenant-Code", "default").
			Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
				"provider": "okta-binding",
				"nonce": "test-nonce",
				"id_token": %q
			}`, idToken)))
		require.NoError(s.T(), err)
		res.AssertOk()
		s.assertResponseCode(res, 200)
	}

	userID := s.userID("binding-user-1")
	var binding struct {
		ID         uint64 `gorm:"column:id"`
		UserID     uint64 `gorm:"column:user_id"`
		ProviderID uint64 `gorm:"column:provider_id"`
		SSOUserID  string `gorm:"column:sso_user_id"`
		SSOEmail   string `gorm:"column:sso_email"`
		SSOAvatar  string `gorm:"column:sso_avatar"`
		LoginCount int    `gorm:"column:login_count"`
	}
	require.NoError(s.T(), facades.Orm().Query().
		Table("sso_user_binding").
		Where("provider_id", providerID).
		Where("sso_user_id", "binding-user-1").
		First(&binding))
	require.Equal(s.T(), userID, binding.UserID)
	require.Equal(s.T(), "binding-user@example.test", binding.SSOEmail)
	require.Equal(s.T(), "https://example.test/avatar.png", binding.SSOAvatar)
	require.Equal(s.T(), 2, binding.LoginCount)

	list := s.getJSONWithToken("/admin/sso-user-binding/list?sso_email=binding-user@example.test")
	require.Equal(s.T(), float64(200), list["code"])
	bindingRows := list["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), bindingRows, 1)
	require.Equal(s.T(), "Okta Binding", bindingRows[0].(map[string]any)["provider_display_name"])

	logs := s.getJSONWithToken("/admin/sso-login-log/list?provider_id=" + itoa(providerID))
	require.Equal(s.T(), float64(200), logs["code"])
	logRows := logs["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), logRows, 2)
	require.Equal(s.T(), float64(1), logRows[0].(map[string]any)["status"])
	require.Equal(s.T(), "binding-user-1", logRows[0].(map[string]any)["sso_user_id"])

	stats := s.getJSONWithToken("/admin/sso-login-log/stats?provider_id=" + itoa(providerID))
	require.Equal(s.T(), float64(200), stats["code"])
	require.Equal(s.T(), float64(2), stats["data"].(map[string]any)["total"])
	require.Equal(s.T(), float64(2), stats["data"].(map[string]any)["success_count"])
	require.Equal(s.T(), float64(100), stats["data"].(map[string]any)["success_rate"])

	statsByProviderName := s.getJSONWithToken("/admin/sso-login-log/stats?provider_name=okta-binding")
	require.Equal(s.T(), float64(200), statsByProviderName["code"])
	require.Equal(s.T(), float64(2), statsByProviderName["data"].(map[string]any)["total"])

	statsByUsername := s.getJSONWithToken("/admin/sso-login-log/stats?username=binding-user-1")
	require.Equal(s.T(), float64(200), statsByUsername["code"])
	require.Equal(s.T(), float64(2), statsByUsername["data"].(map[string]any)["total"])

	s.assertOK(s.Http(s.T()).WithHeader("X-Tenant-Code", "default").WithToken(s.token).Delete("/admin/sso-user-binding/"+itoa(binding.ID), nil))
	count, err := facades.Orm().Query().Table("sso_user_binding").Where("id", binding.ID).Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginUsesExistingBindingBeforeUsernameLookup() {
	providerID := s.createProvider(`{
		"name": "okta-existing-binding",
		"display_name": "Okta Existing Binding",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://existing-binding.example.test",
		"audience": "mineadmin",
		"jwt_secret": "existing-binding-secret",
		"auto_create": false
	}`)
	adminID := s.userID("admin")
	s.createSSOBinding(adminID, providerID, "external-admin-1")
	idToken := s.oidcToken("existing-binding-secret", map[string]any{
		"iss":   "https://existing-binding.example.test",
		"aud":   "mineadmin",
		"sub":   "external-admin-1",
		"email": "admin-external@example.test",
		"name":  "External Admin",
	})

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta-existing-binding",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 200)

	count, err := facades.Orm().Query().Table(`"user"`).Where("username", "external-admin-1").Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), count)

	var loginCount int
	require.NoError(s.T(), facades.Orm().Query().
		Table("sso_user_binding").
		Where("provider_id", providerID).
		Where("sso_user_id", "external-admin-1").
		Pluck("login_count", &loginCount))
	require.Equal(s.T(), 2, loginCount)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginReturnsBindingLookupErrors() {
	s.createProvider(`{
		"name": "okta-binding-error",
		"display_name": "Okta Binding Error",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://binding-error.example.test",
		"audience": "mineadmin",
		"jwt_secret": "binding-error-secret",
		"auto_create": false
	}`)
	idToken := s.oidcToken("binding-error-secret", map[string]any{
		"iss": "https://binding-error.example.test",
		"aud": "mineadmin",
		"sub": "binding-error-user",
	})
	_, err := facades.Orm().Query().Exec("ALTER TABLE sso_user_binding RENAME TO sso_user_binding_broken")
	require.NoError(s.T(), err)
	defer func() {
		_, err := facades.Orm().Query().Exec("ALTER TABLE sso_user_binding_broken RENAME TO sso_user_binding")
		require.NoError(s.T(), err)
	}()

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta-binding-error",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 500)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginDoesNotWriteSuccessLoginLogWhenBindingUpdateFails() {
	providerID := s.createProvider(`{
		"name": "okta-binding-update-error",
		"display_name": "Okta Binding Update Error",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://binding-update-error.example.test",
		"audience": "mineadmin",
		"jwt_secret": "binding-update-error-secret",
		"auto_create": false
	}`)
	userID := s.userID("admin")
	s.createSSOBinding(userID, providerID, "binding-update-error-user")
	s.corruptSSOBindingLoginCount(providerID, "binding-update-error-user")

	idToken := s.oidcToken("binding-update-error-secret", map[string]any{
		"iss": "https://binding-update-error.example.test",
		"aud": "mineadmin",
		"sub": "binding-update-error-user",
	})
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta-binding-update-error",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 500)

	successLoginLogs, err := facades.Orm().Query().
		Table("user_login_log").
		Where("username", "admin").
		Where("status", 1).
		Where("message", "SSO 登录成功").
		Count()
	require.NoError(s.T(), err)
	require.Zero(s.T(), successLoginLogs)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginReturnsUpsertBindingLookupErrors() {
	providerID := s.createProvider(`{
		"name": "okta-upsert-binding-error",
		"display_name": "Okta Upsert Binding Error",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://upsert-binding-error.example.test",
		"audience": "mineadmin",
		"jwt_secret": "upsert-binding-error-secret",
		"auto_create": false
	}`)
	userID := s.userID("admin")
	s.createSSOBinding(userID, providerID, "upsert-binding-error-user")
	_, err := facades.Orm().Query().Exec("ALTER TABLE sso_user_binding DROP CONSTRAINT sso_user_binding_provider_id_sso_user_id_unique")
	require.NoError(s.T(), err)
	s.corruptSSOBindingLoginCount(providerID, "upsert-binding-error-user")

	idToken := s.oidcToken("upsert-binding-error-secret", map[string]any{
		"iss": "https://upsert-binding-error.example.test",
		"aud": "mineadmin",
		"sub": "upsert-binding-error-user",
	})
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta-upsert-binding-error",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 500)

	bindingCount, err := facades.Orm().Query().
		Table("sso_user_binding").
		Where("provider_id", providerID).
		Where("sso_user_id", "upsert-binding-error-user").
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), bindingCount)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginLogsProviderFailures() {
	providerID := s.createProvider(`{
		"name": "bad-token-oidc",
		"display_name": "Bad Token OIDC",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://bad-token.example.test",
		"audience": "mineadmin",
		"jwt_secret": "expected-secret",
		"auto_create": true
	}`)
	badToken := s.oidcToken("wrong-secret", map[string]any{
		"iss": "https://bad-token.example.test",
		"aud": "mineadmin",
		"sub": "bad-user-1",
	})

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "bad-token-oidc",
			"nonce": "test-nonce",
			"id_token": %q
		}`, badToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 422)

	logs := s.getJSONWithToken("/admin/sso-login-log/list?provider_id=" + itoa(providerID) + "&status=2")
	require.Equal(s.T(), float64(200), logs["code"])
	rows := logs["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), rows, 1)
	require.Equal(s.T(), "SSO Token 无效", rows[0].(map[string]any)["failure_reason"])
	require.Equal(s.T(), "Bad Token OIDC", rows[0].(map[string]any)["provider_display_name"])
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginAppliesRoleAndDataPermissionMappings() {
	s.createRole("Manager", "经理")
	s.createProvider(`{
		"name": "mapped-oidc",
		"display_name": "Mapped OIDC",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://mapped-idp.example.test",
		"audience": "mineadmin",
		"jwt_secret": "mapped-secret",
		"auto_create": true,
		"role_mapping": {
			"claim": "groups",
			"default": ["SuperAdmin"],
			"mapping": {
				"managers": {
					"condition": "{{level}} >= 5 && (department == 'sales' || department == 'growth')",
					"roles": ["Manager"]
				}
			}
		},
		"data_permission_mapping": {
			"claim": "department",
			"default": "SELF",
			"mapping": {
				"sales": {
					"condition": "level >= 5 || email contains '@example.test'",
					"policy_type": "CUSTOM_DEPT",
					"value": [1]
				}
			}
		}
	}`)

	idToken := s.oidcToken("mapped-secret", map[string]any{
		"iss":        "https://mapped-idp.example.test",
		"aud":        "mineadmin",
		"sub":        "mapped-user-1",
		"email":      "mapped-user@example.test",
		"name":       "Mapped User",
		"groups":     []string{"managers"},
		"department": "sales",
		"level":      7,
	})
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "mapped-oidc",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 200)

	userID := s.userID("mapped-user-1")
	s.requireUserRole(userID, "Manager")
	s.requireUserPolicy(userID, "CUSTOM_DEPT", `[1]`)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginAppliesDefaultMappingsWhenNoClaimRuleMatches() {
	s.createProvider(`{
		"name": "default-mapped-oidc",
		"display_name": "Default Mapped OIDC",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://default-idp.example.test",
		"audience": "mineadmin",
		"jwt_secret": "default-secret",
		"auto_create": true,
		"role_mapping": {
			"claim": "groups",
			"default": ["SuperAdmin"],
			"mapping": {
				"managers": ["MissingRole"]
			}
		},
		"data_permission_mapping": {
			"claim": "department",
			"default": "SELF",
			"mapping": {
				"sales": "ALL"
			}
		}
	}`)

	idToken := s.oidcToken("default-secret", map[string]any{
		"iss":        "https://default-idp.example.test",
		"aud":        "mineadmin",
		"sub":        "default-map-1",
		"groups":     []string{"staff"},
		"department": "engineering",
	})
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "default-mapped-oidc",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	res.AssertOk()
	s.assertResponseCode(res, 200)

	userID := s.userID("default-map-1")
	s.requireUserRole(userID, "SuperAdmin")
	s.requireUserPolicy(userID, "SELF", `[]`)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginRejectsUnverifiedSubjectPayload() {
	s.createProvider(`{
		"name": "corp-saml",
		"display_name": "Corp SAML",
		"scene": "admin",
		"type": "saml",
		"enabled": true,
		"auto_create": true
	}`)

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(`{
			"provider": "corp-saml",
			"scene": "admin",
			"subject": "admin",
			"email": "attacker@example.test"
		}`))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(422), body["code"], "response body: %#v", body)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginRequiresSceneWhenProviderNameIsShared() {
	s.createProvider(`{
		"name": "okta",
		"display_name": "Okta Admin",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://admin-idp.example.test",
		"audience": "admin-client",
		"jwt_secret": "admin-secret",
		"auto_create": true
	}`)
	s.createProvider(`{
		"name": "okta",
		"display_name": "Okta Portal",
		"scene": "portal",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://portal-idp.example.test",
		"audience": "portal-client",
		"jwt_secret": "portal-secret",
		"auto_create": true
	}`)
	portalToken := s.oidcToken("portal-secret", map[string]any{
		"iss":   "https://portal-idp.example.test",
		"aud":   "portal-client",
		"sub":   "portal-user-1",
		"email": "portal@example.test",
	})

	missingScene, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta",
			"nonce": "test-nonce",
			"id_token": %q
		}`, portalToken)))
	require.NoError(s.T(), err)
	missingSceneBody, err := missingScene.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(401), missingSceneBody["code"], "response body: %#v", missingSceneBody)

	ok, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "okta",
			"scene": "portal",
			"nonce": "test-nonce",
			"id_token": %q
		}`, portalToken)))
	require.NoError(s.T(), err)
	okBody, err := ok.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), okBody["code"], "response body: %#v", okBody)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginSupportsOIDCJWKSRS256Token() {
	s.createProvider(fmt.Sprintf(`{
		"name": "enterprise-oidc",
		"display_name": "Enterprise OIDC",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"issuer": "https://oidc.example.test",
		"audience": "mineadmin",
		"jwks_json": %q,
		"auto_create": true
	}`, s.jwksJSON))
	idToken := s.oidcRSToken(map[string]any{
		"iss":   "https://oidc.example.test",
		"aud":   "mineadmin",
		"sub":   "rs-user-1",
		"email": "rs-user@example.test",
	})

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "enterprise-oidc",
			"scene": "admin",
			"nonce": "test-nonce",
			"id_token": %q
		}`, idToken)))
	require.NoError(s.T(), err)
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginSupportsOAuth2AuthorizationCodeUserinfo() {
	defer services.AllowLoopbackSSOEndpointsForTesting()()
	var sawCodeVerifier bool
	var expectedCodeChallenge string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			require.NoError(s.T(), r.ParseForm())
			require.Equal(s.T(), "authorization_code", r.Form.Get("grant_type"))
			require.Equal(s.T(), "oauth-code", r.Form.Get("code"))
			require.Equal(s.T(), "oauth-client", r.Form.Get("client_id"))
			verifierDigest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			sawCodeVerifier = r.Form.Get("code_verifier") != "" &&
				base64.RawURLEncoding.EncodeToString(verifierDigest[:]) == expectedCodeChallenge
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer"}`))
		case "/userinfo":
			require.Equal(s.T(), "Bearer access-1", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sub":"oauth-user-1","email":"oauth-user@example.test","name":"OAuth User"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()
	s.createProvider(fmt.Sprintf(`{
		"name": "enterprise-oauth2",
		"display_name": "Enterprise OAuth2",
		"scene": "admin",
		"type": "oauth2",
		"enabled": true,
		"authorization_endpoint": %q,
		"token_endpoint": %q,
		"userinfo_endpoint": %q,
		"client_id": "oauth-client",
		"redirect_uri": "http://localhost:2888/login",
		"enable_pkce": true,
		"auto_create": true
	}`, idp.URL+"/authorize", idp.URL+"/token", idp.URL+"/userinfo"))

	authorize, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/authorize", strings.NewReader(`{
			"provider": "enterprise-oauth2",
			"scene": "admin"
		}`))
	require.NoError(s.T(), err)
	transactionID, state, authorizationURL := s.ssoAuthorizationData(authorize)
	parsedAuthorizationURL, err := url.Parse(authorizationURL)
	require.NoError(s.T(), err)
	expectedCodeChallenge = parsedAuthorizationURL.Query().Get("code_challenge")
	require.NotEmpty(s.T(), expectedCodeChallenge)

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/callback", strings.NewReader(fmt.Sprintf(`{
			"transaction_id": %q,
			"code": "oauth-code",
			"state": %q
		}`, transactionID, state)))
	require.NoError(s.T(), err)
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
	require.True(s.T(), sawCodeVerifier)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginUsesOIDCDiscoveryDocument() {
	defer services.AllowLoopbackSSOEndpointsForTesting()()
	var expectedNonce string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"issuer": %q,
				"authorization_endpoint": %q,
				"token_endpoint": %q,
				"jwks_uri": %q
			}`, idpIssuer(r), idpIssuer(r)+"/authorize", idpIssuer(r)+"/token", idpIssuer(r)+"/jwks")))
		case "/token":
			require.NoError(s.T(), r.ParseForm())
			require.Equal(s.T(), "discovery-code", r.Form.Get("code"))
			idToken := s.oidcRSToken(map[string]any{
				"iss":   idpIssuer(r),
				"sub":   "discovery-user-1",
				"email": "discovery@example.test",
				"name":  "Discovery User",
				"nonce": expectedNonce,
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id_token":%q,"token_type":"Bearer"}`, idToken)))
		case "/jwks":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(s.jwksJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()
	s.createProvider(fmt.Sprintf(`{
		"name": "discovery-oidc",
		"display_name": "Discovery OIDC",
		"scene": "admin",
		"type": "oidc",
		"enabled": true,
		"discovery_url": %q,
		"client_id": "discovery-client",
		"redirect_uri": "http://localhost:2888/login",
		"auto_create": true
	}`, idp.URL+"/.well-known/openid-configuration"))

	brandingRes, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Get("/admin/passport/branding?scene=admin")
	require.NoError(s.T(), err)
	branding, err := brandingRes.Json()
	require.NoError(s.T(), err)
	providers := branding["data"].(map[string]any)["features"].(map[string]any)["sso"].(map[string]any)["providers"].([]any)
	require.Equal(s.T(), idp.URL+"/authorize", providers[0].(map[string]any)["authorization_endpoint"])

	authorize, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/authorize", strings.NewReader(`{
			"provider": "discovery-oidc",
			"scene": "admin"
		}`))
	require.NoError(s.T(), err)
	transactionID, state, authorizationURL := s.ssoAuthorizationData(authorize)
	parsedAuthorizationURL, err := url.Parse(authorizationURL)
	require.NoError(s.T(), err)
	expectedNonce = parsedAuthorizationURL.Query().Get("nonce")
	require.NotEmpty(s.T(), expectedNonce)
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/callback", strings.NewReader(fmt.Sprintf(`{
			"transaction_id": %q,
			"code": "discovery-code",
			"state": %q
		}`, transactionID, state)))
	require.NoError(s.T(), err)
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
}

func (s *SSOProviderTestSuite) TestTenantSSOLoginSupportsSignedSAMLAssertion() {
	s.createProvider(fmt.Sprintf(`{
		"name": "corp-saml",
		"display_name": "Corp SAML",
		"scene": "admin",
		"type": "saml",
		"enabled": true,
		"issuer": "https://saml-idp.example.test",
		"saml_entity_id": "mineadmin",
		"saml_certificate": %q,
		"auto_create": true
	}`, s.certPEM))

	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post("/admin/passport/sso/login", strings.NewReader(fmt.Sprintf(`{
			"provider": "corp-saml",
			"scene": "admin",
			"saml_response": %q
		}`, s.signedSAMLAssertion("saml-user-1", "saml-user@example.test", "SAML User"))))
	require.NoError(s.T(), err)
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
}

func idpIssuer(r *http.Request) string {
	return "http://" + r.Host
}

func (s *SSOProviderTestSuite) TestTenantSSOProviderUpdateRejectsMissingID() {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(s.token).
		Put("/admin/sso-provider/999999", strings.NewReader(`{
			"name": "missing",
			"display_name": "Missing",
			"scene": "admin",
			"type": "oidc",
			"enabled": true
		}`))
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.NotEqual(s.T(), float64(200), body["code"])
}

func (s *SSOProviderTestSuite) TestTenantSSOProviderRejectsDuplicateNameScene() {
	s.createProvider(`{
		"name": "okta-admin",
		"display_name": "Okta Admin",
		"scene": "admin",
		"type": "oidc",
		"enabled": true
	}`)
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(s.token).
		Post("/admin/sso-provider", strings.NewReader(`{
			"name": "okta-admin",
			"display_name": "Okta Admin Copy",
			"scene": "admin",
			"type": "oidc",
			"enabled": true
		}`))
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equal(s.T(), float64(422), body["code"])
}

func (s *SSOProviderTestSuite) createProvider(payload string) uint64 {
	var input services.SSOProviderPayload
	err := json.Unmarshal([]byte(payload), &input)
	require.NoError(s.T(), err)
	tenant, err := services.NewTenantService().Resolve("default")
	require.NoError(s.T(), err)
	provider, err := services.NewSSOProviderServiceForTenant(tenant).WithContext(s.T().Context()).Create(input, 1)
	require.NoError(s.T(), err)
	return provider.ID
}

func (s *SSOProviderTestSuite) ssoAuthorizationData(response contractshttp.Response) (string, string, string) {
	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
	data, ok := body["data"].(map[string]any)
	require.True(s.T(), ok)
	transactionID, _ := data["transaction_id"].(string)
	state, _ := data["state"].(string)
	authorizationURL, _ := data["authorization_url"].(string)
	require.NotEmpty(s.T(), transactionID)
	require.NotEmpty(s.T(), state)
	require.NotEmpty(s.T(), authorizationURL)
	return transactionID, state, authorizationURL
}

func (s *SSOProviderTestSuite) getJSONWithToken(uri string) map[string]any {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(s.token).
		Get(uri)
	require.NoError(s.T(), err)
	res.AssertOk()

	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *SSOProviderTestSuite) assertOK(response contractshttp.Response, err error) {
	require.NoError(s.T(), err)
	response.AssertOk()

	body, err := response.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
}

func (s *SSOProviderTestSuite) assertResponseCode(res contractshttp.Response, expected float64) {
	body, err := res.Json()
	require.NoError(s.T(), err)
	require.Equalf(s.T(), expected, body["code"], "response body: %#v", body)
}

func (s *SSOProviderTestSuite) createRole(code, name string) uint64 {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		WithToken(s.token).
		Post("/admin/role", strings.NewReader(fmt.Sprintf(`{
			"name": %q,
			"code": %q,
			"status": 1
		}`, name, code)))
	require.NoError(s.T(), err)
	res.AssertOk()

	var id uint64
	require.NoError(s.T(), facades.Orm().Query().Table("role").Where("code", code).Pluck("id", &id))
	require.NotZero(s.T(), id)
	return id
}

func (s *SSOProviderTestSuite) userID(username string) uint64 {
	var id uint64
	require.NoError(s.T(), facades.Orm().Query().Table(`"user"`).Where("username", username).Pluck("id", &id))
	require.NotZero(s.T(), id)
	return id
}

func (s *SSOProviderTestSuite) createSSOBinding(userID, providerID uint64, ssoUserID string) {
	now := time.Now()
	err := facades.Orm().Query().Create(&models.SSOUserBinding{
		UserID:       userID,
		ProviderID:   providerID,
		SSOUserID:    ssoUserID,
		SSOEmail:     "bound@example.test",
		SSOUsername:  "Bound User",
		FirstLoginAt: now,
		LastLoginAt:  now,
		LoginCount:   1,
		Timestamps:   models.Timestamps{CreatedAt: now, UpdatedAt: now},
	})
	require.NoError(s.T(), err)
}

func (s *SSOProviderTestSuite) corruptSSOBindingLoginCount(providerID uint64, ssoUserID string) {
	_, err := facades.Orm().Query().Exec("ALTER TABLE sso_user_binding ALTER COLUMN login_count TYPE text USING login_count::text")
	require.NoError(s.T(), err)
	_, err = facades.Orm().Query().
		Table("sso_user_binding").
		Where("provider_id", providerID).
		Where("sso_user_id", ssoUserID).
		Update(map[string]any{"login_count": "broken"})
	require.NoError(s.T(), err)
}

func (s *SSOProviderTestSuite) requireUserRole(userID uint64, roleCode string) {
	count, err := facades.Orm().
		Query().
		Table("user_belongs_role").
		Join("JOIN role ON role.id = user_belongs_role.role_id").
		Where("user_belongs_role.user_id", userID).
		Where("role.code", roleCode).
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *SSOProviderTestSuite) requireUserPolicy(userID uint64, policyType string, value string) {
	var row struct {
		PolicyType string `gorm:"column:policy_type"`
		Value      string `gorm:"column:value"`
	}
	err := facades.Orm().
		Query().
		Table("data_permission_policy").
		Select("policy_type", "value::text AS value").
		Where("user_id", userID).
		WhereNull("deleted_at").
		First(&row)
	require.NoError(s.T(), err)
	require.Equal(s.T(), policyType, row.PolicyType)
	require.JSONEq(s.T(), value, row.Value)
}

func (s *SSOProviderTestSuite) oidcToken(secret string, claims map[string]any) string {
	now := time.Now()
	tokenClaims := jwt.MapClaims{
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"nonce": "test-nonce",
	}
	for key, value := range claims {
		tokenClaims[key] = value
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims).SignedString([]byte(secret))
	require.NoError(s.T(), err)
	return token
}

func (s *SSOProviderTestSuite) oidcRSToken(claims map[string]any) string {
	now := time.Now()
	tokenClaims := jwt.MapClaims{
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"nonce": "test-nonce",
	}
	for key, value := range claims {
		tokenClaims[key] = value
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, tokenClaims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(s.rsaKey)
	require.NoError(s.T(), err)
	return signed
}

func (s *SSOProviderTestSuite) testCertificate() (*rsa.PrivateKey, *x509.Certificate, string) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(s.T(), err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-idp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(s.T(), err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(s.T(), err)
	return key, cert, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func (s *SSOProviderTestSuite) testJWKS(cert *x509.Certificate) string {
	jwk := map[string]any{
		"kty": "RSA",
		"use": "sig",
		"kid": "test-key",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(cert.PublicKey.(*rsa.PublicKey).N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(cert.PublicKey.(*rsa.PublicKey).E)).Bytes()),
	}
	raw, err := json.Marshal(map[string]any{"keys": []any{jwk}})
	require.NoError(s.T(), err)
	return string(raw)
}

func (s *SSOProviderTestSuite) signedSAMLAssertion(subject, email, name string) string {
	doc := etree.NewDocument()
	assertion := doc.CreateElement("Assertion")
	assertion.CreateAttr("ID", "assertion-1")
	assertion.CreateAttr("IssueInstant", time.Now().UTC().Format(time.RFC3339))
	issuer := assertion.CreateElement("Issuer")
	issuer.SetText("https://saml-idp.example.test")
	subjectEl := assertion.CreateElement("Subject")
	nameID := subjectEl.CreateElement("NameID")
	nameID.SetText(subject)
	conditions := assertion.CreateElement("Conditions")
	conditions.CreateAttr("NotBefore", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	conditions.CreateAttr("NotOnOrAfter", time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	audienceRestriction := conditions.CreateElement("AudienceRestriction")
	audience := audienceRestriction.CreateElement("Audience")
	audience.SetText("mineadmin")
	attributes := assertion.CreateElement("AttributeStatement")
	s.samlAttribute(attributes, "email", email)
	s.samlAttribute(attributes, "name", name)

	ctx, err := dsig.NewSigningContext(s.rsaKey, [][]byte{s.rsaCert.Raw})
	require.NoError(s.T(), err)
	ctx.IdAttribute = "ID"
	signed, err := ctx.SignEnveloped(assertion)
	require.NoError(s.T(), err)
	wrapped := etree.NewDocument()
	wrapped.SetRoot(signed)
	raw, err := wrapped.WriteToString()
	require.NoError(s.T(), err)
	return raw
}

func (s *SSOProviderTestSuite) samlAttribute(parent *etree.Element, name, value string) {
	attr := parent.CreateElement("Attribute")
	attr.CreateAttr("Name", name)
	attrValue := attr.CreateElement("AttributeValue")
	attrValue.SetText(value)
}

func (s *SSOProviderTestSuite) loginAs(username, password string) string {
	res, err := s.Http(s.T()).
		WithHeader("X-Tenant-Code", "default").
		Post(
			"/admin/passport/login",
			strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, password)),
		)
	require.NoError(s.T(), err)

	var body passportResponse
	require.NoError(s.T(), res.Bind(&body))
	require.Equal(s.T(), 200, body.Code)
	return body.Data.AccessToken
}
