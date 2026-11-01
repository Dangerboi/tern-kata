package model

import (
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
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

// cmdTypeNames are the named types of this package that implement Cmd, found by type-checking the package's non-test files.
func cmdTypeNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		if name := e.Name(); strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, f)
		}
	}
	// The source importer reads go/build.Default and offers no way to pass a context; without a C compiler cgo files would fail to type-check.
	cgo := build.Default.CgoEnabled
	build.Default.CgoEnabled = false
	t.Cleanup(func() { build.Default.CgoEnabled = cgo })
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("model", fset, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmd, ok := pkg.Scope().Lookup("Cmd").Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatal("Cmd is not an interface")
	}
	var names []string
	for _, name := range pkg.Scope().Names() {
		tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || tn.Name() == "Cmd" || types.IsInterface(tn.Type()) {
			continue
		}
		if types.Implements(tn.Type(), cmd) || types.Implements(types.NewPointer(tn.Type()), cmd) {
			names = append(names, name)
		}
	}
	return names
}
