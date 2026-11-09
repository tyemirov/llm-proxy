package tests_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPersistenceBoundaryOwnsDatabaseMechanisms(t *testing.T) {
	directory := filepath.Join(operationalRepositoryRoot(t), "internal", "proxy")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		switch name {
		case "managed_store_connections.go", "managed_store_constraints.go", "managed_store_transactions.go", "managed_store_recovery.go":
			continue
		}
		t.Run(name, func(t *testing.T) {
			positions := token.NewFileSet()
			file, err := parser.ParseFile(positions, filepath.Join(directory, name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			imports := make(map[string]string)
			for _, imported := range file.Imports {
				path, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				alias := filepath.Base(path)
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				imports[alias] = path
				if path == "github.com/glebarez/sqlite" || strings.HasPrefix(path, "gorm.io/driver/") || path == "modernc.org/sqlite" {
					t.Errorf("%s: database drivers belong in the persistence implementation", positions.Position(imported.Pos()))
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch expression := node.(type) {
				case *ast.CompositeLit:
					if persistenceImportedName(expression.Type, imports, "gorm.io/gorm/clause", "Locking") {
						t.Errorf("%s: locking clauses belong in the persistence implementation", positions.Position(expression.Pos()))
					}
				case *ast.CallExpr:
					if function, ok := expression.Fun.(*ast.Ident); ok && function.Name == "lockManagedRows" {
						t.Errorf("%s: use a named persistence operation instead of the lock primitive", positions.Position(expression.Pos()))
					}
					method, ok := expression.Fun.(*ast.SelectorExpr)
					if ok && (method.Sel.Name == "Exec" || method.Sel.Name == "Raw") {
						t.Errorf("%s: raw SQL entry points belong in the persistence implementation", positions.Position(expression.Pos()))
					}
					if !ok || method.Sel.Name != "UpdateColumn" || len(expression.Args) != 2 {
						break
					}
					column, columnOK := persistenceStringLiteral(expression.Args[0])
					value, valueOK := expression.Args[1].(*ast.CallExpr)
					if !columnOK || !valueOK || len(value.Args) != 1 || !persistenceImportedName(value.Fun, imports, "gorm.io/gorm", "Expr") {
						break
					}
					statement, statementOK := persistenceStringLiteral(value.Args[0])
					if statementOK && strings.TrimSpace(statement) == column {
						t.Errorf("%s: no-op locking updates belong in the persistence implementation", positions.Position(expression.Pos()))
					}
				case *ast.BasicLit:
					value, ok := persistenceStringLiteral(expression)
					if !ok {
						break
					}
					value = strings.ToLower(value)
					for _, mechanism := range []string{"pragma", "sqlite_master", "sqlite_schema", "sqlite_busy", "begin immediate", "begin exclusive"} {
						if strings.Contains(value, mechanism) {
							t.Errorf("%s: database-specific %q belongs in the persistence implementation", positions.Position(expression.Pos()), mechanism)
							break
						}
					}
				}
				return true
			})
		})
	}
}

func persistenceImportedName(expression ast.Expr, imports map[string]string, importPath, name string) bool {
	switch expression := expression.(type) {
	case *ast.SelectorExpr:
		qualifier, ok := expression.X.(*ast.Ident)
		return ok && imports[qualifier.Name] == importPath && expression.Sel.Name == name
	case *ast.Ident:
		return imports["."] == importPath && expression.Name == name
	default:
		return false
	}
}

func persistenceStringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}
