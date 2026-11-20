package model_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// workerwidget.luau keeps a Lua twin of HoldWords; a hold code in one and not the other, or with other words, shows a different
// reason in the worker widget than in the Pane.
func TestLuauHoldWordsMatchHoldWords(t *testing.T) {
	src, err := os.ReadFile("../../workerwidget.luau")
	if err != nil {
		t.Fatal(err)
	}
	table := regexp.MustCompile(`(?s)local HOLD_WORDS[^=]*=\s*\{(.*?)\n\}`).FindSubmatch(src)
	if table == nil {
		t.Fatal("workerwidget.luau has no HOLD_WORDS table")
	}
	lua := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+)\s*=\s*"((?:[^"\\]|\\.)*)"`).FindAllSubmatch(table[1], -1) {
		words, err := strconv.Unquote(`"` + string(m[2]) + `"`)
		if err != nil {
			t.Fatalf("HOLD_WORDS %s: %v", m[1], err)
		}
		lua[string(m[1])] = words
	}

	for _, code := range holdCodes(t) {
		words, ok := lua[code]
		if !ok {
			t.Errorf("HOLD_WORDS in workerwidget.luau lacks %q", code)
			continue
		}
		if want := model.HoldWords(code); words != want {
			t.Errorf("HOLD_WORDS[%q] = %q, HoldWords says %q", code, words, want)
		}
		delete(lua, code)
	}
	for code := range lua {
		t.Errorf("HOLD_WORDS in workerwidget.luau has %q, which HoldWords does not know", code)
	}
}

// holdCodes are the case strings of HoldWords' switch in words.go.
func holdCodes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "words.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "HoldWords" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if clause, ok := n.(*ast.CaseClause); ok {
				for _, e := range clause.List {
					if lit, ok := e.(*ast.BasicLit); ok {
						code, _ := strconv.Unquote(lit.Value)
						codes = append(codes, code)
					}
				}
			}
			return true
		})
	}
	if len(codes) == 0 {
		t.Fatal("found no hold codes in HoldWords")
	}
	return codes
}
