package model

import (
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
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
		{ClearAgentAddress{}, false},
		{ForgetAgentRecord{}, false},
		{LoadRemoteAgents{}, false},
		{ClearAttention{}, true},
		{CloseAgent{}, false},
		{StopAgent{}, false},
		{InterruptAgent{}, false},
		{CloseIssueCmd{}, false},
		{CreateIssueCmd{}, true},
		{CreateProject{}, false},
		{CreateRecurrenceCmd{}, true},
		{CreateTokenCmd{}, true},
		{DeleteIssueCmd{}, false},
		{DeleteView{}, false},
		{UnlinkAttachment{}, true},
		{EnrollFederation{}, true},
		{FocusAgent{}, false},
		{LeaveFederation{}, true},
		{LoadAttention{}, false},
		{LoadAuthCapabilities{}, false},
		{LoadCredentials{}, false},
		{LoadDaemons{}, false},
		{LoadConfigFiles{}, false},
		{LoadEmbeddingsStatus{}, false},
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
		{LoadTeamKids{}, false},
		{ResolveComment{}, false},
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
		{MigrateAttachments{}, true},
		{OpenAgent{}, false},
		{OpenAttachment{}, false},
		{OpenDraftEditor{}, false},
		{OpenIssue{}, false},
		{OpenRunSession{}, false},
		{PruneAgents{}, false},
		{RaiseAgentAttention{}, false},
		{ReadAgentIssue{}, false},
		{ReadSweepComments{}, false},
		{ReadAgentSessions{}, false},
		{WaitAgentSessions{}, false},
		{HandOffAgent{}, false},
		{ReadSnapshot{}, false},
		{ReadWorkflowText{}, false},
		{RefreshAgents{}, false},
		{ReopenIssueCmd{}, false},
		{RestoreIssueCmd{}, true},
		{RestoreJobCmd{}, true},
		{RestoreWorkflow{}, true},
		{ResumeJob{}, false},
		{ResumeLocalRun{}, false},
		{RevealActivityTab{}, false},
		{RevokeTokenCmd{}, true},
		{RunJobNow{}, false},
		{SaveAgentConfig{}, false},
		{SwitchDocbank{}, false},
		{SaveAgentStates{}, false},
		{SaveDraft{}, false},
		{SaveFilters{}, false},
		{LoadSeen{}, false},
		{SaveSeen{}, false},
		{SaveJobCmd{}, true},
		{SaveLanes{}, false},
		{SaveView{}, false},
		{SaveWorkerEdit{}, false},
		{SaveWorkerSetup{}, false},
		{SaveWorkflow{}, true},
		{Scroll{}, false},
		{SendAgentBack{}, false},
		{SendAgentMessage{}, true},
		{SetAgentOpen{}, false},
		{SetDatesCmd{}, false},
		{SetFederationSpokeName{}, true},
		{SetWorkerSupervision{}, false},
		{SearchByMeaning{}, false},
		{ShareLanes{}, false},
		{ShowRun{}, false},
		{StartAgent{}, false},
		{StopRecurrenceCmd{}, true},
		{StartTeam{}, false},
		{DriveTeams{}, false},
		{SetTeamPaused{}, false},
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
	// The source importer reads go/build.Default and offers no way to pass a context; without a C compiler cgo files would fail to type-check,
	// and a -trimpath test binary has no GOROOT of its own to find the standard library in.
	cgo, goroot := build.Default.CgoEnabled, build.Default.GOROOT
	build.Default.CgoEnabled = false
	if goroot == "" {
		out, err := exec.Command("go", "env", "GOROOT").Output()
		if err != nil {
			t.Fatal(err)
		}
		build.Default.GOROOT = strings.TrimSpace(string(out))
	}
	t.Cleanup(func() { build.Default.CgoEnabled, build.Default.GOROOT = cgo, goroot })
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
