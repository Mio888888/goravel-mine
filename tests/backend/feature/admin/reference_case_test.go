package admin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/facades"
	"goravel/app/moduleboot"
	"goravel/app/services"
	"goravel/database/seeders"
	"goravel/tests/backend/testcase"
)

type ReferenceCaseTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestReferenceCaseTestSuite(t *testing.T) {
	suite.Run(t, new(ReferenceCaseTestSuite))
}

func (s *ReferenceCaseTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.Seed(&seeders.PlatformAdminSeeder{})
	s.Seed(&seeders.PlatformMenuSeeder{})
	s.Seed(&seeders.PlatformCasbinSeeder{})
	s.Seed(&seeders.ReferenceCaseSeeder{})
}

func (s *ReferenceCaseTestSuite) TestReferenceCaseTableAndModuleContractExist() {
	require.True(s.T(), facades.Schema().HasTable("reference_case"))

	registry := moduleboot.Modules()
	require.NoError(s.T(), registry.ValidateRuntime())
	require.Contains(s.T(), registry.IDs(), "reference-case")
	require.Contains(s.T(), registry.OpenAPIFiles(), "docs/api-contract/openapi/reference-case.openapi.json")
	require.Contains(s.T(), registry.TestTemplates(), "tests/backend/feature/admin/reference_case_test.go")
	require.NotEmpty(s.T(), registry.Seeders())
}

func (s *ReferenceCaseTestSuite) TestReferenceCaseSeederAndLifecycleUpgradeRollback() {
	var seeded services.ReferenceCase
	require.NoError(s.T(), facades.Orm().Query().Table("reference_case").Where("code", "golden-case").First(&seeded))
	require.Equal(s.T(), "1.0.0", seeded.Version)

	require.NoError(s.T(), services.ApplyReferenceCaseUpgrade(s.T().Context()))
	require.True(s.T(), facades.Schema().HasColumn("reference_case", "upgrade_note"))
	var upgraded struct {
		Version     string `gorm:"column:version"`
		UpgradeNote string `gorm:"column:upgrade_note"`
	}
	require.NoError(s.T(), facades.Orm().Query().Table("reference_case").Where("code", "golden-case").First(&upgraded))
	require.Equal(s.T(), "1.1.0", upgraded.Version)
	require.Equal(s.T(), "reference lifecycle upgrade applied", upgraded.UpgradeNote)

	require.NoError(s.T(), services.RollbackReferenceCaseUpgrade(s.T().Context()))
	require.False(s.T(), facades.Schema().HasColumn("reference_case", "upgrade_note"))
	require.NoError(s.T(), facades.Orm().Query().Table("reference_case").Where("code", "golden-case").First(&seeded))
	require.Equal(s.T(), "1.0.0", seeded.Version)
}

func (s *ReferenceCaseTestSuite) TestPlatformAdminCanCRUDReferenceCase() {
	token := s.loginAsPlatformAdmin()

	create := s.postJSON(token, "/admin/platform/reference-case", `{
		"code": "crud-case",
		"title": "CRUD Reference Case",
		"status": 1,
		"version": "1.0.0",
		"payload": {"scenario": "upgrade"},
		"remark": "baseline"
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	created := create["data"].(map[string]any)
	require.Equal(s.T(), "crud-case", created["code"])
	require.Equal(s.T(), "CRUD Reference Case", created["title"])

	list := s.getJSON(token, "/admin/platform/reference-case/list?code=crud-case")
	require.Equal(s.T(), float64(200), list["code"])
	rows := list["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), rows, 1)
	require.Equal(s.T(), "crud-case", rows[0].(map[string]any)["code"])

	id := uint64(created["id"].(float64))
	update := s.putJSON(token, "/admin/platform/reference-case/"+itoa(id), `{
		"code": "crud-case",
		"title": "CRUD Reference Case v2",
		"status": 2,
		"version": "1.0.1",
		"payload": {"scenario": "rollback"},
		"remark": "updated"
	}`)
	require.Equal(s.T(), float64(200), update["code"])
	require.Equal(s.T(), "CRUD Reference Case v2", update["data"].(map[string]any)["title"])

	deleteRes := s.deleteJSON(token, "/admin/platform/reference-case", `[`+itoa(id)+`]`)
	require.Equal(s.T(), float64(200), deleteRes["code"])

	empty := s.getJSON(token, "/admin/platform/reference-case/list?code=crud-case")
	require.Equal(s.T(), float64(200), empty["code"])
	require.Empty(s.T(), empty["data"].(map[string]any)["list"].([]any))
}

func (s *ReferenceCaseTestSuite) TestReferenceCaseUpdateRollsBackWhenPayloadCannotPersist() {
	token := s.loginAsPlatformAdmin()
	create := s.postJSON(token, "/admin/platform/reference-case", `{
		"code": "rollback-case",
		"title": "Rollback Case",
		"status": 1,
		"version": "1.0.0",
		"payload": {"scenario": "original"},
		"remark": "before"
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	_, err := facades.Orm().Query().Exec("ALTER TABLE reference_case ADD CONSTRAINT reference_case_payload_object CHECK (jsonb_typeof(payload) = 'object')")
	require.NoError(s.T(), err)

	update := s.putJSON(token, "/admin/platform/reference-case/"+itoa(id), `{
		"code": "rollback-case",
		"title": "Rollback Case Mutated",
		"status": 2,
		"version": "2.0.0",
		"payload": null,
		"remark": "after"
	}`)
	require.NotEqual(s.T(), float64(200), update["code"])

	list := s.getJSON(token, "/admin/platform/reference-case/list?code=rollback-case")
	rows := list["data"].(map[string]any)["list"].([]any)
	require.Len(s.T(), rows, 1)
	row := rows[0].(map[string]any)
	require.Equal(s.T(), "Rollback Case", row["title"])
	require.Equal(s.T(), float64(1), row["status"])
	require.Equal(s.T(), "1.0.0", row["version"])
	require.Equal(s.T(), "before", row["remark"])
}

func (s *ReferenceCaseTestSuite) TestReferenceCaseUpdateRejectsNoRowsAffected() {
	token := s.loginAsPlatformAdmin()
	create := s.postJSON(token, "/admin/platform/reference-case", `{
		"code": "blocked-update",
		"title": "Blocked Update",
		"status": 1,
		"version": "1.0.0",
		"payload": {"scenario": "original"}
	}`)
	require.Equal(s.T(), float64(200), create["code"])
	id := uint64(create["data"].(map[string]any)["id"].(float64))

	_, err := facades.Orm().Query().Exec(`
		CREATE OR REPLACE FUNCTION skip_reference_case_update() RETURNS trigger AS $$
		BEGIN
			RETURN NULL;
		END;
		$$ LANGUAGE plpgsql
	`)
	require.NoError(s.T(), err)
	_, err = facades.Orm().Query().Exec(`
		CREATE TRIGGER reference_case_skip_update
		BEFORE UPDATE ON reference_case
		FOR EACH ROW EXECUTE FUNCTION skip_reference_case_update()
	`)
	require.NoError(s.T(), err)

	update := s.putJSON(token, "/admin/platform/reference-case/"+itoa(id), `{
		"code": "blocked-update",
		"title": "Blocked Update v2",
		"status": 2,
		"version": "2.0.0",
		"payload": {"scenario": "changed"}
	}`)

	require.Equal(s.T(), float64(422), update["code"])
	require.Contains(s.T(), update["message"], "参考案例不存在")
}

func (s *ReferenceCaseTestSuite) TestReferenceCaseRequiresDedicatedPermission() {
	token := s.loginAsReadonlyPlatformUser()

	res := s.postJSON(token, "/admin/platform/reference-case", `{
		"code": "denied",
		"title": "Denied",
		"status": 1,
		"version": "1.0.0"
	}`)

	require.Equal(s.T(), float64(403), res["code"])
}

func (s *ReferenceCaseTestSuite) loginAsPlatformAdmin() string {
	result, err := services.NewPlatformPassportService().Login("admin", "123456")
	require.NoError(s.T(), err)
	return result.AccessToken
}

func (s *ReferenceCaseTestSuite) loginAsReadonlyPlatformUser() string {
	err := facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_user").Create(map[string]any{
		"id":              50,
		"username":        "reference_reader",
		"password":        "$2a$10$/mc6xDxW3q3aJfzZBVBXT.a9GEWkm5p2griG8xDcNjKJL9OhLlToe",
		"user_type":       "900",
		"nickname":        "参考模块只读",
		"email":           "reference-reader@example.test",
		"phone":           "16800000050",
		"signed":          "",
		"dashboard":       "platform:referenceCase",
		"status":          1,
		"backend_setting": "{}",
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_role").Create(map[string]any{
		"id":     50,
		"name":   "参考模块只读",
		"code":   "ReferenceReader",
		"status": 1,
		"sort":   50,
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_user_belongs_role").Create(map[string]any{
		"user_id": 50,
		"role_id": 50,
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_casbin_rule").Create(map[string]any{
		"ptype": "g",
		"v0":    "user:50",
		"v1":    "role:ReferenceReader",
	})
	require.NoError(s.T(), err)
	err = facades.Orm().Connection(services.PlatformConnection()).Query().Table("platform_casbin_rule").Create(map[string]any{
		"ptype": "p",
		"v0":    "role:ReferenceReader",
		"v1":    "platform:referenceCase:list",
		"v2":    "*",
	})
	require.NoError(s.T(), err)

	result, err := services.NewPlatformPassportService().Login("reference_reader", "123456")
	require.NoError(s.T(), err)
	return result.AccessToken
}

func (s *ReferenceCaseTestSuite) getJSON(token string, url string) map[string]any {
	res, err := s.Http(s.T()).WithToken(token).Get(url)
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *ReferenceCaseTestSuite) postJSON(token string, url string, payload string) map[string]any {
	res, err := s.Http(s.T()).WithToken(token).Post(url, strings.NewReader(payload))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *ReferenceCaseTestSuite) putJSON(token string, url string, payload string) map[string]any {
	res, err := s.Http(s.T()).WithToken(token).Put(url, strings.NewReader(payload))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}

func (s *ReferenceCaseTestSuite) deleteJSON(token string, url string, payload string) map[string]any {
	res, err := s.Http(s.T()).WithToken(token).Delete(url, strings.NewReader(payload))
	require.NoError(s.T(), err)
	res.AssertOk()
	body, err := res.Json()
	require.NoError(s.T(), err)
	return body
}
