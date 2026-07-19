package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type PlatformBootstrapTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestPlatformBootstrapTestSuite(t *testing.T) {
	suite.Run(t, new(PlatformBootstrapTestSuite))
}

func (s *PlatformBootstrapTestSuite) SetupTest() {
	s.RefreshDatabase()
}

func (s *PlatformBootstrapTestSuite) TestLocalDefaultsRestoreMissingPlatformAdmin() {
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec("DELETE FROM platform_user")
	require.NoError(s.T(), err)

	err = services.NewPlatformBootstrapService().EnsureLocalDefaults()
	require.NoError(s.T(), err)

	var count int64
	count, err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_user").
		Where("username", "admin").
		Where("user_type", "900").
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)

	_, err = services.NewPlatformPassportService().Login("admin", "123456")
	require.NoError(s.T(), err)
}

func (s *PlatformBootstrapTestSuite) TestLocalDefaultsSyncMenusWhenPlatformAdminExists() {
	s.Seed(&seeders.PlatformAdminSeeder{})
	s.seedLegacyPlatformMenu()
	s.Seed(&seeders.PlatformCasbinSeeder{})

	err := services.NewPlatformBootstrapService().EnsureLocalDefaults()
	require.NoError(s.T(), err)
	err = services.NewPlatformBootstrapService().EnsureLocalDefaults()
	require.NoError(s.T(), err)

	s.assertPlatformMenuParent("platform:tenantManage", 0)
	s.assertPlatformMenuParent("platform:system", 0)
	s.assertPlatformMenuParent("platform:tenant", 42)
	s.assertPlatformMenuParent("platform:tenantPlan", 42)
	s.assertPlatformMenuParent("platform:dictionary", 43)
	s.assertPlatformMenuParent("platform:scheduledTask", 43)

	var count int64
	count, err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_role_belongs_menu").
		Where("role_id", 1).
		WhereIn("menu_id", []any{uint64(42), uint64(43)}).
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(2), count)

	count, err = facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_casbin_rule").
		Where("ptype", "g").
		Where("v0", "user:1").
		Where("v1", "role:PlatformSuperAdmin").
		Count()
	require.NoError(s.T(), err)
	require.Equal(s.T(), int64(1), count)
}

func (s *PlatformBootstrapTestSuite) TestPlatformLoginRestoresMissingLocalAdmin() {
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec("DELETE FROM platform_user")
	require.NoError(s.T(), err)

	_, err = services.NewPlatformPassportService().Login("admin", "123456")
	require.NoError(s.T(), err)
}

func (s *PlatformBootstrapTestSuite) seedLegacyPlatformMenu() {
	items := []struct {
		id       uint64
		parentID uint64
		name     string
		path     string
		title    string
	}{
		{1, 0, "platform", "/platform", "平台管理"},
		{2, 1, "platform:tenant", "/platform/tenant", "租户管理"},
		{31, 1, "platform:tenantPlan", "/platform/tenant-plan", "套餐管理"},
		{36, 1, "platform:dictionary", "/platform/dictionary", "默认字典"},
	}
	for _, item := range items {
		meta := models.JSONMap{"title": item.title, "type": "M"}
		encoded, err := json.Marshal(meta)
		require.NoError(s.T(), err)
		_, err = facades.Orm().
			Connection(services.PlatformConnection()).
			Query().
			Exec(`
				INSERT INTO platform_menu (
					id, parent_id, name, meta, path, component, redirect, status, sort,
					created_by, updated_by, created_at, updated_at, remark
				)
				VALUES (?, ?, ?, ?::jsonb, ?, '', '', 1, 10, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, '')
			`, item.id, item.parentID, item.name, string(encoded), item.path)
		require.NoError(s.T(), err)
	}
}

func (s *PlatformBootstrapTestSuite) assertPlatformMenuParent(name string, parentID uint64) {
	var menu models.Menu
	err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Table("platform_menu").
		Where("name", name).
		First(&menu)
	require.NoError(s.T(), err)
	require.Equal(s.T(), parentID, menu.ParentID)
}

func (s *PlatformBootstrapTestSuite) TestPlatformLoginRestoresAdminWithoutDictionaryTables() {
	for _, table := range []string{"platform_dict_item", "platform_dict_type"} {
		_, err := facades.Orm().
			Connection(services.PlatformConnection()).
			Query().
			Exec("DROP TABLE IF EXISTS " + table)
		require.NoError(s.T(), err)
	}
	_, err := facades.Orm().
		Connection(services.PlatformConnection()).
		Query().
		Exec("DELETE FROM platform_user")
	require.NoError(s.T(), err)

	_, err = services.NewPlatformPassportService().Login("admin", "123456")
	require.NoError(s.T(), err)
}
