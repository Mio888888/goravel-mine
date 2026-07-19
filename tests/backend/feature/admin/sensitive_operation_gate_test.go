package admin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
	contractshttp "github.com/goravel/framework/contracts/testing/http"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type SensitiveOperationGateSuite struct {
	suite.Suite
	tests.TestCase
	tenantToken   string
	platformToken string
}

type sensitiveGateScope struct {
	name, token, passwordPath, rolesPath, permissionsPath, mfaPath string
	tenant                                                         bool
	userID                                                         uint64
}

func TestSensitiveOperationGateSuite(t *testing.T) {
	suite.Run(t, new(SensitiveOperationGateSuite))
}

func (s *SensitiveOperationGateSuite) SetupTest() {
	s.RefreshDatabase()
	_ = facades.Cache().Flush()
	services.ResetEnterpriseSecurityControlForTest()
	services.ResetCasbinEnforcerCacheForTest()
	services.ResetTenantConnectionRegistryForTest()
	s.Seed(&seeders.TenantPlanSeeder{})
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	require.NoError(s.T(), (&seeders.PlatformAdminSeeder{}).Run())
	require.NoError(s.T(), (&seeders.PlatformMenuSeeder{}).Run())
	require.NoError(s.T(), (&seeders.PlatformCasbinSeeder{}).Run())
	s.tenantToken = s.loginTenant("admin")
	s.platformToken = s.loginPlatform("admin")
}

func (s *SensitiveOperationGateSuite) TestSensitiveOperationGateRejectsUnauthenticatedRBACAndMFAChanges() {
	for _, scope := range s.gateScopes() {
		s.Run(scope.name, func() {
			password := s.userPassword(scope)
			s.reject(s.put(scope, scope.passwordPath, `{"id":1}`))
			require.Equal(s.T(), password, s.userPassword(scope))

			roles, groups := s.userRoles(scope), s.userGroups(scope)
			s.reject(s.put(scope, scope.rolesPath, `{"role_codes":[]}`))
			require.Equal(s.T(), roles, s.userRoles(scope))
			require.Equal(s.T(), groups, s.userGroups(scope))

			permissions, policies := s.rolePermissions(scope), s.rolePolicies(scope)
			s.reject(s.put(scope, scope.permissionsPath, `{"permissions":[]}`))
			require.Equal(s.T(), permissions, s.rolePermissions(scope))
			require.Equal(s.T(), policies, s.rolePolicies(scope))

			s.enableMFA(scope)
			s.reject(s.post(scope, scope.mfaPath, `{}`))
			require.True(s.T(), s.mfaEnabled(scope))
		})
	}
}

func (s *SensitiveOperationGateSuite) TestSensitiveOperationGateRejectsProtectedStorageChangesWithoutEvidence() {
	createPayload := s.storagePayload("guarded-create", "https://create.example.test")
	s.reject(s.post(s.platformScope(), "/admin/platform/storage-config", createPayload))
	require.Equal(s.T(), int64(0), s.storageCount("guarded-create"))

	id := s.createStorage("guarded-existing", "https://existing.example.test")
	beforeUpdate := s.storage(id)
	s.reject(s.put(s.platformScope(), fmt.Sprintf("/admin/platform/storage-config/%d", id), s.storagePayload("guarded-existing", "https://changed.example.test")))
	require.Equal(s.T(), beforeUpdate, s.storage(id))

	beforeDelete := s.storage(id)
	s.reject(s.delete(s.platformScope(), "/admin/platform/storage-config", fmt.Sprintf(`{"ids":[%d]}`, id)))
	require.Equal(s.T(), beforeDelete, s.storage(id))
}

func (s *SensitiveOperationGateSuite) TestSensitiveOperationGateConsumesServerBoundPlatformRoleEvidenceOnce() {
	admin := services.NewPlatformPermissionAdminService().WithContext(s.T().Context())
	targetID := s.createPlatformUser(admin, "gate-target")
	require.NoError(s.T(), admin.CreateRole(services.RolePayload{Name: "Gate Target", Code: "GateTarget", Status: 1}, 1))
	approverToken := s.loginPlatformApprover(admin)

	selector := s.rbacSelector(targetID, []string{"GateTarget"})
	approvalBody := s.success(s.post(s.platformScope(), "/admin/platform/security/approvals", fmt.Sprintf(`{"policy_key":"user.roles.sync","resource":%q,"reason":"feature gate","before":["client-before"],"after":["client-after"]}`, selector)))
	approvalID := approvalBody["data"].(map[string]any)["approval_id"].(string)
	approval := s.approval(approvalID)
	require.Equal(s.T(), "user.roles.sync", approval.PolicyKey)
	require.NotEmpty(s.T(), approval.BindingDigest)
	before, after := s.approvalSnapshots(approval)
	require.Contains(s.T(), before, `"values":[]`)
	require.Contains(s.T(), after, "GateTarget")
	require.NotContains(s.T(), before+after, "client-")

	s.success(s.putWithToken(approverToken, "/admin/platform/security/approvals/"+approvalID+"/approve", `{}`))
	reauth := s.success(s.post(s.platformScope(), "/admin/platform/security/reauth-token", fmt.Sprintf(`{"password":"123456","operation":"user.roles.sync","resource":%q}`, selector)))
	token := reauth["data"].(map[string]any)["reauth_token"].(string)
	payload := fmt.Sprintf(`{"role_codes":["GateTarget"],"reauth_token":%q,"approval_id":%q}`, token, approvalID)
	s.success(s.put(s.platformScope(), fmt.Sprintf("/admin/platform/user/%d/roles", targetID), payload))
	require.Equal(s.T(), []string{"GateTarget"}, s.userRoles(s.platformScopeFor(targetID)))
	require.Equal(s.T(), []string{"role:GateTarget"}, s.userGroups(s.platformScopeFor(targetID)))
	require.False(s.T(), s.approval(approvalID).UsedAt.IsZero())

	s.reject(s.put(s.platformScope(), fmt.Sprintf("/admin/platform/user/%d/roles", targetID), payload))
	require.Equal(s.T(), []string{"GateTarget"}, s.userRoles(s.platformScopeFor(targetID)))
	require.Equal(s.T(), []string{"role:GateTarget"}, s.userGroups(s.platformScopeFor(targetID)))
}

func (s *SensitiveOperationGateSuite) gateScopes() []sensitiveGateScope {
	return []sensitiveGateScope{
		{name: "tenant", token: s.tenantToken, tenant: true, userID: 1, passwordPath: "/admin/user/password", rolesPath: "/admin/user/1/roles", permissionsPath: "/admin/role/1/permissions", mfaPath: "/admin/security/mfa/disable"},
		{name: "platform", token: s.platformToken, userID: 1, passwordPath: "/admin/platform/user/password", rolesPath: "/admin/platform/user/1/roles", permissionsPath: "/admin/platform/role/1/permissions", mfaPath: "/admin/platform/security/mfa/disable"},
	}
}

func (s *SensitiveOperationGateSuite) platformScope() sensitiveGateScope {
	return sensitiveGateScope{name: "platform", token: s.platformToken, userID: 1}
}

func (s *SensitiveOperationGateSuite) platformScopeFor(userID uint64) sensitiveGateScope {
	scope := s.platformScope()
	scope.userID = userID
	scope.rolesPath = fmt.Sprintf("/admin/platform/user/%d/roles", userID)
	return scope
}

func (s *SensitiveOperationGateSuite) request(scope sensitiveGateScope) contractshttp.Request {
	request := s.Http(s.T()).WithToken(scope.token)
	if scope.tenant {
		return request.WithHeader("X-Tenant-Code", "default")
	}
	return request
}

func (s *SensitiveOperationGateSuite) put(scope sensitiveGateScope, path, body string) (contractshttp.Response, error) {
	return s.request(scope).Put(path, strings.NewReader(body))
}

func (s *SensitiveOperationGateSuite) putWithToken(token, path, body string) (contractshttp.Response, error) {
	return s.Http(s.T()).WithToken(token).Put(path, strings.NewReader(body))
}

func (s *SensitiveOperationGateSuite) post(scope sensitiveGateScope, path, body string) (contractshttp.Response, error) {
	return s.request(scope).Post(path, strings.NewReader(body))
}

func (s *SensitiveOperationGateSuite) delete(scope sensitiveGateScope, path, body string) (contractshttp.Response, error) {
	return s.request(scope).Delete(path, strings.NewReader(body))
}

func (s *SensitiveOperationGateSuite) reject(res contractshttp.Response, err error) {
	body := s.response(res, err)
	require.Equal(s.T(), float64(422), body["code"])
	require.Equal(s.T(), services.ErrReAuthRequired.Error(), body["message"])
}

func (s *SensitiveOperationGateSuite) success(res contractshttp.Response, err error) map[string]any {
	body := s.response(res, err)
	require.Equalf(s.T(), float64(200), body["code"], "response body: %#v", body)
	return body
}

func (s *SensitiveOperationGateSuite) response(res contractshttp.Response, err error) map[string]any {
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *SensitiveOperationGateSuite) query(scope sensitiveGateScope) contractsorm.Query {
	if scope.tenant {
		return facades.Orm().Query()
	}
	return facades.Orm().Connection(services.PlatformConnection()).Query()
}

func (s *SensitiveOperationGateSuite) userPassword(scope sensitiveGateScope) string {
	var password string
	table := "platform_user"
	if scope.tenant {
		table = `"user"`
	}
	require.NoError(s.T(), s.query(scope).Table(table).Where("id", 1).Pluck("password", &password))
	return password
}

func (s *SensitiveOperationGateSuite) userRoles(scope sensitiveGateScope) []string {
	roleTable, relationTable := "platform_role", "platform_user_belongs_role"
	if scope.tenant {
		roleTable, relationTable = "role", "user_belongs_role"
	}
	roles := make([]string, 0)
	require.NoError(s.T(), s.query(scope).Table(roleTable).Select(roleTable+".code").Join("JOIN "+relationTable+" r ON r.role_id = "+roleTable+".id").Where("r.user_id", scope.userID).OrderBy(roleTable+".code").Pluck(roleTable+".code", &roles))
	return roles
}

func (s *SensitiveOperationGateSuite) userGroups(scope sensitiveGateScope) []string {
	table := "platform_casbin_rule"
	if scope.tenant {
		table = "casbin_rule"
	}
	groups := make([]string, 0)
	require.NoError(s.T(), s.query(scope).Table(table).Where("ptype", "g").Where("v0", fmt.Sprintf("user:%d", scope.userID)).OrderBy("v1").Pluck("v1", &groups))
	return groups
}

func (s *SensitiveOperationGateSuite) rolePermissions(scope sensitiveGateScope) []string {
	menuTable, relationTable := "platform_menu", "platform_role_belongs_menu"
	if scope.tenant {
		menuTable, relationTable = "menu", "role_belongs_menu"
	}
	permissions := make([]string, 0)
	require.NoError(s.T(), s.query(scope).Table(menuTable).Select(menuTable+".name").Join("JOIN "+relationTable+" r ON r.menu_id = "+menuTable+".id").Where("r.role_id", 1).OrderBy(menuTable+".name").Pluck(menuTable+".name", &permissions))
	return permissions
}

func (s *SensitiveOperationGateSuite) rolePolicies(scope sensitiveGateScope) []string {
	table, role := "platform_casbin_rule", "role:PlatformSuperAdmin"
	if scope.tenant {
		table, role = "casbin_rule", "role:SuperAdmin"
	}
	policies := make([]string, 0)
	require.NoError(s.T(), s.query(scope).Table(table).Where("ptype", "p").Where("v0", role).OrderBy("v1").Pluck("v1", &policies))
	return policies
}

func (s *SensitiveOperationGateSuite) enableMFA(scope sensitiveGateScope) {
	table := "platform_user_mfa"
	if scope.tenant {
		table = "user_mfa"
	}
	_, err := s.query(scope).Exec(`
		INSERT INTO `+table+` (user_id, secret, enabled, recovery_codes, created_at, updated_at)
		VALUES (?, ?, ?, ?::jsonb, ?, ?)
	`, scope.userID, "fixture", true, `[]`, time.Now(), time.Now())
	require.NoError(s.T(), err)
}

func (s *SensitiveOperationGateSuite) mfaEnabled(scope sensitiveGateScope) bool {
	table := "platform_user_mfa"
	if scope.tenant {
		table = "user_mfa"
	}
	var enabled bool
	require.NoError(s.T(), s.query(scope).Table(table).Where("user_id", scope.userID).Pluck("enabled", &enabled))
	return enabled
}

func (s *SensitiveOperationGateSuite) storagePayload(name, endpoint string) string {
	return fmt.Sprintf(`{"name":%q,"provider":"minio","driver":"s3_compatible","bucket":"gate-bucket","endpoint":%q,"access_key":"gate-ak","secret_key":"gate-sk","status":1}`, name, endpoint)
}

func (s *SensitiveOperationGateSuite) createStorage(name, endpoint string) uint64 {
	config, err := services.NewStorageConfigService().WithContext(s.T().Context()).Create(services.StorageConfigPayload{Name: name, Provider: "minio", Driver: "s3_compatible", Bucket: "gate-bucket", Endpoint: endpoint, AccessKey: "gate-ak", SecretKey: "gate-sk", Status: 1}, 1)
	require.NoError(s.T(), err)
	return config.ID
}

func (s *SensitiveOperationGateSuite) storage(id uint64) models.StorageConfig {
	var config models.StorageConfig
	require.NoError(s.T(), s.query(s.platformScope()).Table("storage_config").Where("id", id).First(&config))
	return config
}

func (s *SensitiveOperationGateSuite) storageCount(name string) int64 {
	count, err := s.query(s.platformScope()).Table("storage_config").Where("name", name).Count()
	require.NoError(s.T(), err)
	return count
}

func (s *SensitiveOperationGateSuite) createPlatformUser(admin *services.PlatformPermissionAdminService, username string) uint64 {
	require.NoError(s.T(), admin.CreateUser(services.UserPayload{Username: username, Password: "123456", Nickname: username, Email: username + "@example.test", Phone: "16800000009", Dashboard: "platform:tenant", Status: 1}, 1))
	var id uint64
	require.NoError(s.T(), s.query(s.platformScope()).Table("platform_user").Where("username", username).Pluck("id", &id))
	return id
}

func (s *SensitiveOperationGateSuite) loginPlatformApprover(admin *services.PlatformPermissionAdminService) string {
	approverID := s.createPlatformUser(admin, "gate-approver")
	require.NoError(s.T(), admin.SyncUserRoles(approverID, []string{"PlatformSuperAdmin"}))
	return s.loginPlatform("gate-approver")
}

func (s *SensitiveOperationGateSuite) rbacSelector(userID uint64, roles []string) string {
	encoded, err := json.Marshal(roles)
	require.NoError(s.T(), err)
	return fmt.Sprintf("rbac:user:%d:roles:%s", userID, base64.RawURLEncoding.EncodeToString(encoded))
}

func (s *SensitiveOperationGateSuite) approval(id string) struct {
	PolicyKey, BindingDigest, BeforeSnapshot, AfterSnapshot string
	UsedAt                                                  time.Time
} {
	var approval struct {
		PolicyKey, BindingDigest, BeforeSnapshot, AfterSnapshot string
		UsedAt                                                  time.Time
	}
	require.NoError(s.T(), s.query(s.platformScope()).Table("enterprise_security_approval").Where("approval_id", id).First(&approval))
	return approval
}

func (s *SensitiveOperationGateSuite) approvalSnapshots(approval struct {
	PolicyKey, BindingDigest, BeforeSnapshot, AfterSnapshot string
	UsedAt                                                  time.Time
}) (string, string) {
	var before, after []string
	require.NoError(s.T(), json.Unmarshal([]byte(approval.BeforeSnapshot), &before))
	require.NoError(s.T(), json.Unmarshal([]byte(approval.AfterSnapshot), &after))
	return strings.Join(before, "\n"), strings.Join(after, "\n")
}

func (s *SensitiveOperationGateSuite) loginTenant(username string) string {
	res, err := s.Http(s.T()).WithHeader("X-Tenant-Code", "default").Post("/admin/passport/login", strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, "123456")))
	body := s.success(res, err)
	return body["data"].(map[string]any)["access_token"].(string)
}

func (s *SensitiveOperationGateSuite) loginPlatform(username string) string {
	res, err := s.Http(s.T()).Post("/admin/platform/passport/login", strings.NewReader(loginJSONWithCaptcha(s.T(), s.Http(s.T()), username, "123456")))
	body := s.success(res, err)
	return body["data"].(map[string]any)["access_token"].(string)
}
