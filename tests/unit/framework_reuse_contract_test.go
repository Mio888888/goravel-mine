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

func TestApplicationUsesSharedORMScopes(t *testing.T) {
	legacyDefinitions := map[string][]string{
		"applyStringFilter": nil,
		"equalFilter":       nil,
		"adminEqualFilter":  nil,
	}

	forEachApplicationFile(t, func(path string, file *ast.File) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if _, tracked := legacyDefinitions[function.Name.Name]; tracked {
				legacyDefinitions[function.Name.Name] = append(legacyDefinitions[function.Name.Name], path)
			}
		}
	})

	for name, paths := range legacyDefinitions {
		require.Emptyf(t, paths, "%s must use shared ORM scopes instead", name)
	}
}

func TestServiceFiltersDoNotUseManualNonEmptyConditions(t *testing.T) {
	for _, root := range []string{
		filepath.Join("..", "..", "app", "services"),
		filepath.Join("..", "..", "app", "modulecatalog"),
	} {
		forEachGoFile(t, root, func(path string, file *ast.File) {
			ast.Inspect(file, func(node ast.Node) bool {
				condition, ok := node.(*ast.IfStmt)
				if !ok || !containsManualFilterCondition(condition.Cond) {
					return true
				}
				t.Errorf("%s must use app/scopes instead of a manual filters value check", path)
				return true
			})
		})
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
	forEachGoFile(t, root, inspect)
}

func forEachApplicationFile(t *testing.T, inspect func(string, *ast.File)) {
	t.Helper()
	root := filepath.Join("..", "..", "app")
	forEachGoFile(t, root, inspect)
}

func forEachGoFile(t *testing.T, root string, inspect func(string, *ast.File)) {
	t.Helper()
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

func containsManualFilterCondition(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || binary.Op != token.NEQ {
			return true
		}
		if (isEmptyString(binary.X) && containsFiltersIndex(binary.Y)) ||
			(isEmptyString(binary.Y) && containsFiltersIndex(binary.X)) {
			found = true
			return false
		}
		return true
	})
	return found
}

func isEmptyString(expression ast.Expr) bool {
	literal, ok := expression.(*ast.BasicLit)
	return ok && literal.Kind == token.STRING && literal.Value == `""`
}

func containsFiltersIndex(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		index, ok := node.(*ast.IndexExpr)
		if !ok {
			return true
		}
		identifier, ok := index.X.(*ast.Ident)
		if ok && identifier.Name == "filters" {
			found = true
			return false
		}
		return true
	})
	return found
}
