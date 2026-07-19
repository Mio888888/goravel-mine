package unit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeatureTestsDoNotRebindORM(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "feature", "**", "*_test.go"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, path := range files {
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, parseErr)
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isORMFreshCall(call) {
				return true
			}
			t.Errorf("%s must use TestCase.RefreshDatabase without facades.Orm().Fresh", path)
			return true
		})
	}
}

func isORMFreshCall(call *ast.CallExpr) bool {
	fresh, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || fresh.Sel.Name != "Fresh" {
		return false
	}
	ormCall, ok := fresh.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	orm, ok := ormCall.Fun.(*ast.SelectorExpr)
	if !ok || orm.Sel.Name != "Orm" {
		return false
	}
	facades, ok := orm.X.(*ast.Ident)
	return ok && facades.Name == "facades"
}
