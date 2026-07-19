package unit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServiceQueryHelpersHaveSingleDefinition(t *testing.T) {
	definitions := map[string][]string{
		"applyStringFilter": nil,
		"equalFilter":       nil,
	}

	forEachServiceFile(t, func(path string, file *ast.File) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if _, tracked := definitions[function.Name.Name]; tracked {
				definitions[function.Name.Name] = append(definitions[function.Name.Name], path)
			}
		}
	})

	for name, paths := range definitions {
		require.Equalf(t, []string{filepath.Join("..", "..", "app", "services", "query_filters.go")}, paths,
			"%s must be defined only in query_filters.go", name)
	}
}

func TestServicesUseORMConnectionFactory(t *testing.T) {
	forEachServiceFile(t, func(path string, file *ast.File) {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isFacadeORMConnection(call) {
				return true
			}
			t.Errorf("%s must use OrmForConnectionWithContext or OrmForConnection", path)
			return true
		})
	})
}

func forEachServiceFile(t *testing.T, inspect func(string, *ast.File)) {
	t.Helper()
	root := filepath.Join("..", "..", "app", "services")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		inspect(path, file)
		return nil
	})
	require.NoError(t, err)
}

func isFacadeORMConnection(call *ast.CallExpr) bool {
	connection, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || connection.Sel.Name != "Connection" {
		return false
	}
	return containsFacadeORMCall(connection.X)
}

func containsFacadeORMCall(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.CallExpr:
		if selector, ok := value.Fun.(*ast.SelectorExpr); ok {
			if selector.Sel.Name == "Orm" {
				facades, ok := selector.X.(*ast.Ident)
				return ok && facades.Name == "facades"
			}
			return containsFacadeORMCall(selector.X)
		}
	case *ast.SelectorExpr:
		return containsFacadeORMCall(value.X)
	}
	return false
}
