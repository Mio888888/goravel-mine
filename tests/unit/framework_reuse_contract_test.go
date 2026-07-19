package unit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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

func TestApplicationUsesSharedSafeOutboundHTTP(t *testing.T) {
	legacyDefinitions := map[string][]string{
		"scheduledTaskSafeDialContext": nil,
		"scheduledTaskHostIPs":         nil,
		"ssoSafeDialContext":           nil,
		"ssoHostIPs":                   nil,
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
		require.Emptyf(t, paths, "%s must use the shared safe outbound HTTP transport", name)
	}
}

func TestAttachmentsUseFrameworkStorage(t *testing.T) {
	legacyDefinitions := map[string][]string{
		"copyUploadedFile":          nil,
		"deleteLocalAttachmentFile": nil,
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
		require.Emptyf(t, paths, "%s must use the framework Storage facade", name)
	}
}

func TestApplicationUsesSharedCronParser(t *testing.T) {
	allowedSuffix := filepath.ToSlash(filepath.Join("app", "support", "cronexpr", "cronexpr.go"))
	for _, root := range []string{
		filepath.Join("..", "..", "app"),
		filepath.Join("..", "..", "database"),
	} {
		forEachGoFile(t, root, func(path string, file *ast.File) {
			if strings.HasSuffix(filepath.ToSlash(path), allowedSuffix) {
				return
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "NewParser" {
					return true
				}
				identifier, ok := selector.X.(*ast.Ident)
				if ok && identifier.Name == "cron" {
					t.Errorf("%s must use app/support/cronexpr instead of configuring another parser", path)
				}
				return true
			})
		})
	}
}

func TestApplicationUsesSharedJWTTokenCodec(t *testing.T) {
	allowed := map[string]bool{
		filepath.ToSlash(filepath.Join("app", "services", "access", "auth", "jwt.go")): true,
		filepath.ToSlash(filepath.Join("app", "services", "application", "sso.go")):    true,
	}
	forEachServiceFile(t, func(path string, file *ast.File) {
		normalized := filepath.ToSlash(path)
		for suffix := range allowed {
			if strings.HasSuffix(normalized, suffix) {
				return
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "NewWithClaims" && selector.Sel.Name != "ParseWithClaims") {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && identifier.Name == "jwt" {
				t.Errorf("%s must use app/services/access/auth/jwt.go for application tokens", path)
			}
			return true
		})
	})
}

func TestApplicationUsesFrameworkProcessFacade(t *testing.T) {
	allowedSuffix := filepath.ToSlash(filepath.Join("app", "modulecatalog", "lifecycle_runtime.go"))
	forEachApplicationFile(t, func(path string, file *ast.File) {
		if strings.HasSuffix(filepath.ToSlash(path), allowedSuffix) {
			return
		}
		for _, imported := range file.Imports {
			if imported.Path.Value == `"os/exec"` {
				t.Errorf("%s must use facades.Process instead of os/exec", path)
			}
		}
	})
}

func TestApplicationDoesNotReintroduceQueueTaskLockStore(t *testing.T) {
	legacyDefinitions := map[string][]string{
		"QueueTaskLock":            nil,
		"QueueTaskLockStore":       nil,
		"MemoryQueueTaskLockStore": nil,
		"DBQueueTaskLockStore":     nil,
	}

	forEachApplicationFile(t, func(path string, file *ast.File) {
		for _, declaration := range file.Decls {
			typeDeclaration, ok := declaration.(*ast.GenDecl)
			if !ok || typeDeclaration.Tok != token.TYPE {
				continue
			}
			for _, spec := range typeDeclaration.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, tracked := legacyDefinitions[typeSpec.Name.Name]; tracked {
					legacyDefinitions[typeSpec.Name.Name] = append(legacyDefinitions[typeSpec.Name.Name], path)
				}
			}
		}
	})

	for name, paths := range legacyDefinitions {
		require.Emptyf(t, paths, "%s duplicates the framework Cache atomic lock", name)
	}
}

func TestQueueServicesStayGroupedByModule(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "app", "services", "queue_*.go"))
	require.NoError(t, err)
	require.Empty(t, matches, "queue service implementations must stay in app/services/runtime/queue")
}

func TestServicesRootOnlyContainsCompatibilityFacade(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "app", "services", "*.go"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "app/services root Go files must only expose the compatibility facade")
	require.Equal(t, "facade.go", filepath.Base(matches[0]))
}

func TestServicesUseCapabilityGroups(t *testing.T) {
	root := filepath.Join("..", "..", "app", "services")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)

	allowedGroups := map[string]bool{
		"access":      true,
		"application": true,
		"platform":    true,
		"runtime":     true,
		"tenancy":     true,
	}
	var unexpectedDirectories []string
	for _, entry := range entries {
		if entry.IsDir() && !allowedGroups[entry.Name()] {
			unexpectedDirectories = append(unexpectedDirectories, entry.Name())
		}
	}
	require.Empty(t, unexpectedDirectories, "service modules must stay in an explicit capability group")

	for _, group := range []string{"access", "platform", "runtime", "tenancy"} {
		groupEntries, readErr := os.ReadDir(filepath.Join(root, group))
		require.NoError(t, readErr)
		for _, entry := range groupEntries {
			require.Truef(t, entry.IsDir(), "%s/%s must be grouped in a module directory", group, entry.Name())
		}
	}
}

func TestApplicationServicesStayGroupedByDomain(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "app", "services", "application", "*.go"))
	require.NoError(t, err)

	allowed := map[string]bool{
		"audit.go":               true,
		"audit_test.go":          true,
		"auth.go":                true,
		"auth_test.go":           true,
		"infrastructure.go":      true,
		"infrastructure_test.go": true,
		"permission.go":          true,
		"permission_test.go":     true,
		"security.go":            true,
		"security_test.go":       true,
		"sso.go":                 true,
		"sso_test.go":            true,
		"tenant.go":              true,
		"tenant_test.go":         true,
	}
	var scattered []string
	for _, path := range matches {
		if !allowed[filepath.Base(path)] {
			scattered = append(scattered, path)
		}
	}
	require.Empty(t, scattered, "cross-module orchestration must stay grouped by domain")
	require.Len(t, matches, len(allowed), "application service domains must remain explicit")
}

func TestConsoleCommandsStayGroupedByModule(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "app", "console", "commands", "*.go"))
	require.NoError(t, err)

	allowed := map[string]bool{
		"commands.go":      true,
		"commands_test.go": true,
	}
	var scattered []string
	for _, path := range matches {
		if !allowed[filepath.Base(path)] {
			scattered = append(scattered, path)
		}
	}
	require.Empty(t, scattered, "console commands must stay in a functional subpackage")
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
