package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoticeIsOnlyAssignedInNoticeGo keeps every Notice going through say, fail or clearNotice, so each carries its kind.
func TestNoticeIsOnlyAssignedInNoticeGo(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if name == "notice.go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "Notice" {
					t.Errorf("%s: assigns .Notice directly; use say, fail or clearNotice", fset.Position(assign.Pos()))
				}
			}
			return true
		})
	}
}
