package model

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestUndoBoundaryListsEveryCmd lists each Cmd and whether emitting it clears the Undo history, so a new Cmd must be classified
// here. Cmds whose boundary depends on a field are listed with their zero value.
func TestUndoBoundaryListsEveryCmd(t *testing.T) {
	table := []struct {
		cmd    Cmd
		clears bool
	}{
		{AbandonLocalRun{}, false},
		{ArchiveJobCmd{}, true},
		{ArchiveWorkflow{}, true},
		{AttachFile{}, true},
		{BindProject{}, false},
		{BrowseFederationHubs{}, false},
		{ChangeRecurrenceCmd{}, true},
		{ClearAgentAttention{}, false},
		{ClearAttention{}, true},
		{CloseAgent{}, false},
		{CloseIssueCmd{}, false},
		{CreateIssueCmd{}, true},
		{CreateProject{}, false},
		{CreateRecurrenceCmd{}, true},
		{CreateTokenCmd{}, true},
		{DeleteIssueCmd{}, false},
		{DeleteView{}, false},
		{DetachFile{}, true},
		{EnrollFederation{}, true},
		{FocusAgent{}, false},
		{LeaveFederation{}, true},
		{LoadAttention{}, false},
		{LoadAuthCapabilities{}, false},
		{LoadCredentials{}, false},
		{LoadDaemons{}, false},
		{LoadDetail{}, false},
		{LoadFederation{}, false},
		{LoadFederationHubProjects{}, false},
		{LoadFederationHubs{}, false},
		{LoadFieldLabels{}, false},
		{LoadJobs{}, false},
		{LoadJobWorkflows{}, false},
		{LoadLanes{}, false},
		{LoadLocalRun{}, false},
		{LoadProject{}, false},
		{LoadRecurTarget{}, false},
		{LoadRuns{}, false},
		{LoadViews{}, false},
		{LoadWorker{}, false},
		{LoadWorkerPickers{}, false},
		{LoadWorkerSetup{}, false},
		{LoadWorkflows{}, false},
		{LookupDeleted{}, false},
		{LookupSearchID{}, false},
		{MoveIssueCmd{}, false},
		{MoveToProjectCmd{}, true},
		{OpenAgent{}, false},
		{OpenDraftEditor{}, false},
		{OpenIssue{}, false},
		{OpenRunSession{}, false},
		{PruneAgents{}, false},
		{RaiseAgentAttention{}, false},
		{ReadAgentIssue{}, false},
		{ReadSnapshot{}, false},
		{ReadWorkflowText{}, false},
		{RefreshAgents{}, false},
		{ReopenIssueCmd{}, false},
		{RestoreIssueCmd{}, true},
		{RestoreJobCmd{}, true},
		{RestoreWorkflow{}, true},
		{ResumeLocalRun{}, false},
		{RevealActivityTab{}, false},
		{RevokeTokenCmd{}, true},
		{RunJobNow{}, false},
		{SaveDraft{}, false},
		{SaveFilters{}, false},
		{SaveJobCmd{}, true},
		{SaveLanes{}, false},
		{SaveView{}, false},
		{SaveWorkerEdit{}, false},
		{SaveWorkerSetup{}, false},
		{SaveWorkflow{}, true},
		{Scroll{}, false},
		{SendAgentBack{}, false},
		{SetAgentOpen{}, false},
		{SetDatesCmd{}, false},
		{SetWorkerSupervision{}, false},
		{ShareLanes{}, false},
		{ShowRun{}, false},
		{StartAgent{}, false},
		{StopRecurrenceCmd{}, true},
		{StopWorker{}, false},
		{Subscribe{}, false},
		{SwitchDaemon{}, false},
		{UndoChange{}, false},
		{WakeAt{}, false},
		{WriteField{}, false},
	}
	listed := map[string]bool{}
	for _, row := range table {
		name := reflect.TypeOf(row.cmd).Name()
		listed[name] = true
		if _, clears := undoBoundary(row.cmd); clears != row.clears {
			t.Errorf("%s clears the Undo history = %v, want %v", name, clears, row.clears)
		}
	}
	declared := cmdTypeNames(t)
	for _, name := range declared {
		if !listed[name] {
			t.Errorf("Cmd %s is not in the table: decide whether it keeps or clears the Undo history", name)
		}
	}
	for name := range listed {
		if !slices.Contains(declared, name) {
			t.Errorf("table lists %s, which is not a Cmd", name)
		}
	}
}

// cmdTypeNames are the types of this package with a cmd method.
func cmdTypeNames(t *testing.T) []string {
	t.Helper()
	pkgs, err := parser.ParseDir(token.NewFileSet(), ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || fn.Name.Name != "cmd" || len(fn.Recv.List) != 1 {
					continue
				}
				if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
					names = append(names, id.Name)
				}
			}
		}
	}
	return names
}

func TestSharingTheLaneListKeepsTheUndoHistory(t *testing.T) {
	m := Model{}
	m.Undo = m.Undo.pushed(UndoEntry{Kind: UndoPriority, UID: "zzz"})
	m = m.noteWrites([]Cmd{ShareLanes{ProjectUID: "p", Lanes: []string{"Todo"}}})
	if len(m.Undo.Entries) != 1 || m.Undo.Boundary != "" {
		t.Fatalf("history = %+v, want the entry kept and no boundary", m.Undo)
	}
}
