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

// TestFailedNoticesAreBuiltWithCannot keeps a tooltip reason (a sentence a button shows) from being the Notice itself: fail takes a
// text built with Cannot or Refused, never a bare reason.
func TestFailedNoticesAreBuiltWithCannot(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "fail" {
				return true
			}
			if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == "reason" {
				t.Errorf("%s: fail takes a bare reason; build the Notice with Refused", fset.Position(call.Pos()))
			}
			return true
		})
	}
}

// TestAgentRefusalNoticesStartWithCannot walks the refusal constructors: every Notice an agent action's refusal can raise starts
// with "Cannot ".
func TestAgentRefusalNoticesStartWithCannot(t *testing.T) {
	notices := map[string]string{
		"sweep with no agent":     NoAgentsToSweepRefusal,
		"broadcast with no agent": NoAgentsToBroadcastRefusal,
		"message on no request":   Refused("message", "the agent", NoAgentRaisedRefusal),
		"close on a closed Issue": Refused("close", "#1", ClosedAlreadyRefusal),
		"start on a closed Issue": Refused("start", "an agent on #1", ClosedIssueRefusal),
		"remote agent":            Refused("stop", "the agent", RemoteAgentRefusal("desk")),
	}
	for _, action := range []Action{AgentCardGo, AgentCardBackground, AgentCardInterrupt, AgentCardStop, AgentCardForget, AgentCardMessage,
		AgentCardQuickReply, AgentCardSplit, AgentCardTab, AgentCardNewBackground, AgentCardTeam, AgentCardPause} {
		notices["card "+string(action)] = Model{}.agentCardNotice(action, noAgentReason)
	}
	for name, notice := range notices {
		if !strings.HasPrefix(notice, "Cannot ") || !strings.HasSuffix(notice, ".") {
			t.Errorf("%s: notice %q does not read \"Cannot <verb> <thing>: <reason>.\"", name, notice)
		}
	}
	if got := Refused("start", "an agent on #12", ClosedIssueRefusal); got != "Cannot start an agent on #12: the Issue is closed." {
		t.Errorf("closed Issue notice %q", got)
	}
}
