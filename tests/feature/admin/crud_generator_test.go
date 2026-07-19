package admin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"goravel/app/console/commands/crudgen"
	"goravel/app/facades"
	"goravel/tests"
)

type CrudGeneratorTestSuite struct {
	suite.Suite
	tests.TestCase
}

func TestCrudGeneratorTestSuite(t *testing.T) {
	suite.Run(t, new(CrudGeneratorTestSuite))
}

func (s *CrudGeneratorTestSuite) SetupTest() {
	s.RefreshDatabase()
	s.createDemoTable()
}

func (s *CrudGeneratorTestSuite) TearDownTest() {
	_, _ = facades.Orm().Query().Exec("DROP TABLE IF EXISTS crud_demo_items")
}

func (s *CrudGeneratorTestSuite) TestGenerateCrudFilesFromPostgreSQLMetadata() {
	outputDir := s.T().TempDir()
	generator := crudgen.NewGenerator(facades.Orm(), outputDir)

	err := generator.Generate(crudgen.Options{
		Table:  "crud_demo_items",
		Module: "demo",
		Force:  false,
	})
	require.NoError(s.T(), err)

	model := s.read(outputDir, "app/models/crud_demo_item.go")
	require.Contains(s.T(), model, "type CrudDemoItem struct")
	require.Contains(s.T(), model, "func (CrudDemoItem) TableName() string")
	require.Contains(s.T(), model, "return \"crud_demo_items\"")
	require.Regexp(s.T(), "Name\\s+string\\s+`gorm:\"column:name\" json:\"name\"`", model)
	require.Regexp(s.T(), "Status\\s+int16\\s+`gorm:\"column:status\" json:\"status\"`", model)
	require.Regexp(s.T(), "Price\\s+float64\\s+`gorm:\"column:price\" json:\"price\"`", model)
	require.Regexp(s.T(), "Published\\s+bool\\s+`gorm:\"column:published\" json:\"published\"`", model)
	require.Regexp(s.T(), "CreatedAt\\s+time.Time\\s+`gorm:\"column:created_at\" json:\"created_at\"`", model)

	repository := s.read(outputDir, "app/repositories/demo/crud_demo_item_repository.go")
	require.Contains(s.T(), repository, "func (r *CrudDemoItemRepository) List")
	require.Contains(s.T(), repository, "query.Scopes(scopes.ContainsFoldIfPresent(\"name\", filters[\"name\"]))")
	require.Contains(s.T(), repository, "query.Scopes(scopes.EqualIfPresent(\"status\", filters[\"status\"]))")
	require.NotContains(s.T(), repository, "func applyStringFilter")
	require.Contains(s.T(), repository, "OrderByDesc(\"id\")")

	requestFile := s.read(outputDir, "app/http/request/demo/crud_demo_item_request.go")
	require.Contains(s.T(), requestFile, "type CrudDemoItemPayload struct")
	require.Regexp(s.T(), "Name\\s+string\\s+`json:\"name\"`", requestFile)
	require.Regexp(s.T(), "Status\\s+int16\\s+`json:\"status\"`", requestFile)

	controller := s.read(outputDir, "app/http/controllers/admin/demo/crud_demo_item_controller.go")
	require.Contains(s.T(), controller, "type CrudDemoItemController struct")
	require.Contains(s.T(), controller, "response.Success")
	require.Contains(s.T(), controller, "response.SuccessEmpty")

	routeSnippet := s.read(outputDir, "routes/demo_crud_demo_item_routes.go")
	require.Contains(s.T(), routeSnippet, "GET /admin/demo/crud-demo-item/list")
	require.Contains(s.T(), routeSnippet, "POST /admin/demo/crud-demo-item")
}

func (s *CrudGeneratorTestSuite) TestGenerateRefusesOverwriteWithoutForce() {
	outputDir := s.T().TempDir()
	modelPath := filepath.Join(outputDir, "app/models/crud_demo_item.go")
	require.NoError(s.T(), os.MkdirAll(filepath.Dir(modelPath), 0755))
	require.NoError(s.T(), os.WriteFile(modelPath, []byte("keep me"), 0644))

	generator := crudgen.NewGenerator(facades.Orm(), outputDir)
	err := generator.Generate(crudgen.Options{Table: "crud_demo_items", Module: "demo"})

	require.ErrorIs(s.T(), err, crudgen.ErrFileExists)
	content, readErr := os.ReadFile(modelPath)
	require.NoError(s.T(), readErr)
	require.Equal(s.T(), "keep me", string(content))
}

func (s *CrudGeneratorTestSuite) TestArtisanCommandGeneratesIntoCustomPath() {
	outputDir := s.T().TempDir()

	err := facades.Artisan().Call("--no-ansi make:crud crud_demo_items --module=demo --path=" + outputDir)

	require.NoError(s.T(), err)
	require.FileExists(s.T(), filepath.Join(outputDir, "app/models/crud_demo_item.go"))
	require.FileExists(s.T(), filepath.Join(outputDir, "app/http/controllers/admin/demo/crud_demo_item_controller.go"))
}

func (s *CrudGeneratorTestSuite) createDemoTable() {
	_, err := facades.Orm().Query().Exec("DROP TABLE IF EXISTS crud_demo_items")
	require.NoError(s.T(), err)
	_, err = facades.Orm().Query().Exec(`
CREATE TABLE crud_demo_items (
	id bigserial PRIMARY KEY,
	name varchar(80) NOT NULL,
	status smallint NOT NULL DEFAULT 1,
	price numeric(10,2) NOT NULL DEFAULT 0,
	published boolean NOT NULL DEFAULT false,
	remark text,
	created_by bigint NOT NULL DEFAULT 0,
	updated_by bigint NOT NULL DEFAULT 0,
	created_at timestamp NULL,
	updated_at timestamp NULL
)`)
	require.NoError(s.T(), err)
	_, err = facades.Orm().Query().Exec("COMMENT ON COLUMN crud_demo_items.name IS '名称'")
	require.NoError(s.T(), err)
	_, err = facades.Orm().Query().Exec("CREATE INDEX idx_crud_demo_items_status ON crud_demo_items(status)")
	require.NoError(s.T(), err)
}

func (s *CrudGeneratorTestSuite) read(root, path string) string {
	content, err := os.ReadFile(filepath.Join(root, path))
	require.NoError(s.T(), err)
	return string(content)
}
