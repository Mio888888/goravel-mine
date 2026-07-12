package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests"
)

type CacheTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestCacheTestSuite(t *testing.T) {
	suite.Run(t, new(CacheTestSuite))
}

func (s *CacheTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.TenantSeeder{})
	s.Seed(&seeders.AdminSeeder{})
	s.Seed(&seeders.MenuSeeder{})
	s.Seed(&seeders.DepartmentSeeder{})
	s.Seed(&seeders.CasbinSeeder{})
	_ = facades.Cache().Flush()
}

func (s *CacheTestSuite) TestCurrentUserInfoCacheInvalidatesOnProfileUpdate() {
	service := services.NewPassportServiceForTenant(s.defaultTenant())
	user := s.adminUser()

	first, err := service.FormatUserInfo(user)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "创始人", first.Nickname)

	_, err = facades.Orm().Query().Table(`"user"`).Where("id", 1).Update("nickname", "数据库直改")
	require.NoError(s.T(), err)

	cached, err := service.FormatUserInfo(s.adminUser())
	require.NoError(s.T(), err)
	require.Equal(s.T(), "创始人", cached.Nickname)

	require.NoError(s.T(), service.UpdateProfile(1, services.ProfileUpdate{Nickname: "缓存失效"}))

	updated, err := service.FormatUserInfo(s.adminUser())
	require.NoError(s.T(), err)
	require.Equal(s.T(), "缓存失效", updated.Nickname)
}

func (s *CacheTestSuite) TestCurrentUserInfoCacheInvalidatesOnRoleUpdate() {
	tenant := s.defaultTenant()
	passport := services.NewPassportServiceForTenant(tenant)
	permission := services.NewPermissionAdminServiceForTenant(tenant)

	first, err := passport.FormatUserInfo(s.adminUser())
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), first.Roles)
	require.Equal(s.T(), "超级管理员", first.Roles[0].Name)

	require.NoError(s.T(), permission.UpdateRole(1, services.RolePayload{
		Name:   "系统管理员",
		Code:   "SuperAdmin",
		Status: 1,
	}, 1))

	updated, err := passport.FormatUserInfo(s.adminUser())
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), updated.Roles)
	require.Equal(s.T(), "系统管理员", updated.Roles[0].Name)
}

func (s *CacheTestSuite) TestCurrentUserInfoCacheIsScopedByConnection() {
	tenant := s.defaultTenant()
	defaultService := services.NewPassportService()
	tenantService := services.NewPassportServiceForTenant(tenant)

	first, err := defaultService.FormatUserInfo(s.adminUser())
	require.NoError(s.T(), err)
	require.Equal(s.T(), "创始人", first.Nickname)

	_, err = facades.Orm().Query().Table(`"user"`).Where("id", 1).Update("nickname", "租户连接读取")
	require.NoError(s.T(), err)

	updated, err := tenantService.FormatUserInfo(s.adminUser())
	require.NoError(s.T(), err)
	require.Equal(s.T(), "租户连接读取", updated.Nickname)
}

func (s *CacheTestSuite) defaultTenant() services.Tenant {
	tenant, err := services.NewTenantService().Resolve("default")
	require.NoError(s.T(), err)
	return tenant
}

func (s *CacheTestSuite) adminUser() models.User {
	var user models.User
	require.NoError(s.T(), facades.Orm().Query().Where("id", 1).First(&user))
	return user
}
