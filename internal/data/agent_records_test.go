package data

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// recordFixture is a daemon with one Issue and a Gateway that starts agents against it with a scripted tern. Its host is named by
// agent.toml.
type recordFixture struct {
	kata    *kataServer
	gateway *Gateway
	state   string
	work    string
	project model.Project
	uid     string
	live    []string
}

func newRecordFixture(t *testing.T, agentToml string) *recordFixture {
	t.Helper()
	kata := startKata(t, "0.21.0")
	t.Setenv("KATA_HOME", "") // a Pane on the default daemon: the agent's command line is the plain one
	gateway := newGateway(t, newLocator(kata.target()))
	work, project := project(t, gateway, "demo")
	uid := createIssue(t, gateway, work, project, "Agent at work")
	dirs, _ := gateway.workerDirs()
	writeAgentFile(t, filepath.Join(dirs.Config, AgentConfigFile), agentToml)
	f := &recordFixture{kata: kata, gateway: gateway, state: dirs.State, work: work, project: project, uid: uid}
	ternScript(gateway, map[string]func() ([]byte, error){
		"split":                   ternOut(`{"session": 1, "tab": 2, "block": 31}`),
		"close":                   ternOut(""),
		"ls":                      func() ([]byte, error) { return lsOf(f.live...), nil },
		"new session kata agents": ternOut(`{"session": 3, "tab": 9, "block": 31}`),
		"rename":                  ternOut(""),
	})
	f.live = []string{"7"}
	return f
}

func (f *recordFixture) start(t *testing.T, placement model.AgentPlacement) model.AgentStarted {
	t.Helper()
	got := as[model.AgentStarted](t, run(t, f.gateway, model.StartAgent{Placement: placement, IssueUID: f.uid, IssueRef: "demo#ab12", IssueTitle: "Agent at work",
		ProjectID: f.project.ID, ProjectName: "demo", Workspace: f.work, PaneBlock: "7", Actor: "dana"}))
	if got.Err != nil || got.Note != "" {
		t.Fatalf("start: %+v", got)
	}
	f.live = append(f.live, got.Entry.Block)
	return got
}

func (f *recordFixture) hostID(t *testing.T) string {
	t.Helper()
	id, err := ReadHostID(f.state)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// records are the Agent records on the Issue, as kata holds them.
func (f *recordFixture) records(t *testing.T) []model.AgentRecord {
	t.Helper()
	return model.AgentRecords(issueFixture{kata: f.kata}.issue(t, f.uid).Metadata)
}

// put writes a record by another host, or a stray of this one, straight to kata.
func (f *recordFixture) put(t *testing.T, r model.AgentRecord) {
	t.Helper()
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{model.AgentRecordKey(r.HostID, r.Block): r}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *recordFixture) prune(t *testing.T) []model.AgentEntry {
	t.Helper()
	got := as[model.AgentsPruned](t, run(t, f.gateway, model.PruneAgents{Workspace: f.work, PaneBlock: "7"}))
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	return got.Agents
}

func blocksOf(records []model.AgentRecord) []string {
	var blocks []string
	for _, r := range records {
		blocks = append(blocks, r.HostID+"/"+r.Block)
	}
	return blocks
}

func TestStartingAnAgentPublishesItsRecordOnItsIssue(t *testing.T) {
	f := newRecordFixture(t, "host = \"laptop\"\nmessage_from = [\"dana-laptop\"]\n")
	started := f.start(t, model.AgentSplit)
	hostID := f.hostID(t)
	want := model.AgentRecord{Address: "dana/omp-31", Host: "laptop", HostID: hostID, Block: "31", Name: "omp", Actor: "dana",
		StartedAt: started.Entry.StartedAt, MessageFrom: []string{"dana-laptop"}}
	got := f.records(t)
	if len(got) != 1 || got[0].Address != want.Address || got[0].Host != want.Host || got[0].HostID != want.HostID || got[0].Block != want.Block ||
		got[0].Name != want.Name || got[0].Actor != want.Actor || !got[0].StartedAt.Equal(want.StartedAt) || got[0].NoAddress != "" ||
		!slices.Equal(got[0].MessageFrom, want.MessageFrom) {
		t.Fatalf("records %+v, want %+v", got, want)
	}
	if key := model.AgentRecordKey(hostID, "31"); !strings.HasPrefix(key, "tern-kata.agent."+hostID+"-31") {
		t.Fatalf("key %q", key)
	}
	shown := issueFixture{kata: f.kata}.issue(t, f.uid)
	if _, held := shown.Metadata[model.AgentRecordKey(hostID, "31")]; !held {
		t.Fatal("the record is not under tern-kata.agent.<host id>-<block id>")
	}
}

func TestAnAgentThatIsNotOmpPublishesARecordThatSaysWhyItHasNoAddress(t *testing.T) {
	f := newRecordFixture(t, "command = [\"claude\"]\n")
	f.start(t, model.AgentSplit)
	got := f.records(t)
	if len(got) != 1 || got[0].Address != "" || got[0].NoAddress != model.AgentNotOmp || got[0].Name != "claude" || got[0].MessageFrom == nil {
		t.Fatalf("records %+v, want one without an address that says it is not omp", got)
	}
}

func TestTheHostNameIsTheMachinesWhenAgentTomlNamesNone(t *testing.T) {
	f := newRecordFixture(t, "")
	f.gateway.WithHost("")
	f.start(t, model.AgentSplit)
	if got := f.records(t); len(got) != 1 || got[0].Host != HostName() {
		t.Fatalf("records %+v, want the machine's host name %q", got, HostName())
	}
}

func TestTheHostIdIsMadeOnceAndKept(t *testing.T) {
	state := t.TempDir()
	first, err := ReadHostID(filepath.Join(state, "tern-kata"))
	if err != nil || len(first) < 16 || strings.ContainsAny(first, "-/ \n") {
		t.Fatalf("host id %q, %v; want a random id with no hyphen", first, err)
	}
	if again, err := ReadHostID(filepath.Join(state, "tern-kata")); err != nil || again != first {
		t.Fatalf("second read %q, %v; want %q", again, err, first)
	}
	if other, err := ReadHostID(t.TempDir()); err != nil || other == first {
		t.Fatalf("another state folder has %q, %v; want its own id", other, err)
	}
	if text, err := os.ReadFile(filepath.Join(state, "tern-kata", HostIDFile)); err != nil || strings.TrimSpace(string(text)) != first {
		t.Fatalf("host-id file %q, %v", text, err)
	}
}

func TestAnAgentsRecordGoesWhereItsEntryGoes(t *testing.T) {
	for name, c := range map[string]struct {
		placement model.AgentPlacement
		end       func(t *testing.T, f *recordFixture)
	}{
		"its exit": {model.AgentSplit, func(t *testing.T, f *recordFixture) {
			cleared := as[model.AgentAddressCleared](t, run(t, f.gateway, model.ClearAgentAddress{Workspace: f.work, ProjectID: f.project.ID, UID: f.uid, Actor: "dana", Block: "31", Address: "dana/omp-31"}))
			if cleared.Err != nil {
				t.Fatal(cleared.Err)
			}
		}},
		"its stop": {model.AgentSplit, func(t *testing.T, f *recordFixture) {
			if stopped := as[model.AgentStopped](t, run(t, f.gateway, model.StopAgent{Block: "31"})); stopped.Err != nil || stopped.Gone {
				t.Fatalf("stop %+v", stopped)
			}
		}},
		"a background agent ending": {model.AgentBackground, func(t *testing.T, f *recordFixture) {
			if closed := as[model.AgentClosed](t, run(t, f.gateway, model.CloseAgent{Block: "31"})); closed.Err != nil || closed.Kept {
				t.Fatalf("close %+v", closed)
			}
		}},
		"its block closing": {model.AgentSplit, func(t *testing.T, f *recordFixture) {
			f.live = []string{"7"}
			if kept := f.prune(t); len(kept) != 0 {
				t.Fatalf("prune kept %+v", kept)
			}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRecordFixture(t, "")
			f.start(t, c.placement)
			if len(f.records(t)) != 1 {
				t.Fatalf("records before: %+v", f.records(t))
			}
			c.end(t, f)
			if left := f.records(t); len(left) != 0 {
				t.Fatalf("records after %s: %+v", name, left)
			}
		})
	}
}

func TestAnAgentThatStaysMappedKeepsItsRecordThroughAReconcile(t *testing.T) {
	f := newRecordFixture(t, "")
	f.start(t, model.AgentSplit)
	if kept := f.prune(t); len(kept) != 1 || len(f.records(t)) != 1 {
		t.Fatalf("kept %+v, records %+v", kept, f.records(t))
	}
}

func TestAReconcileWritesTheRecordOfAnAgentThatHasNone(t *testing.T) {
	f := newRecordFixture(t, "host = \"laptop\"\n")
	seedAgent(t, worker.Dirs{Config: f.state, State: f.state}, model.AgentEntry{IssueUID: f.uid, IssueRef: "demo#ab12", ProjectID: f.project.ID, Block: "40", PaneBlock: "7", Teammate: "omp",
		Address: "dana/omp-40", MessageFrom: "dana-laptop,dana-phone", StartedAt: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)})
	f.live = []string{"7", "40"}
	f.prune(t)
	got := f.records(t)
	if len(got) != 1 || got[0].Block != "40" || got[0].HostID != f.hostID(t) || got[0].Host != "laptop" || got[0].Address != "dana/omp-40" ||
		got[0].Actor != "dana" || !slices.Equal(got[0].MessageFrom, []string{"dana-laptop", "dana-phone"}) {
		t.Fatalf("records %+v, want the entry's, written by the reconcile", got)
	}
}

func TestAReconcileRemovesARecordOfThisHostThatHasNoEntry(t *testing.T) {
	f := newRecordFixture(t, "")
	hostID := f.hostID(t)
	f.put(t, model.AgentRecord{Address: "dana/omp-99", Host: "gone", HostID: hostID, Block: "99", Name: "omp", Actor: "dana", MessageFrom: []string{}})
	f.prune(t)
	if left := f.records(t); len(left) != 0 {
		t.Fatalf("records %+v, want the stray gone", left)
	}
}

func TestAReconcileNeverRemovesARecordOfAnotherHostIdEvenWithTheSameHostName(t *testing.T) {
	f := newRecordFixture(t, "host = \"laptop\"\n")
	f.start(t, model.AgentSplit)
	f.put(t, model.AgentRecord{Address: "dana/omp-31", Host: "laptop", HostID: "0000000000000000", Block: "31", Name: "omp", Actor: "dana", MessageFrom: []string{}})
	f.put(t, model.AgentRecord{Address: "dana/omp-5", Host: "laptop", HostID: "1111111111111111", Block: "5", Name: "omp", Actor: "dana", MessageFrom: []string{}})
	f.live = []string{"7"}
	f.prune(t)
	want := []string{"0000000000000000/31", "1111111111111111/5"}
	if got := blocksOf(f.records(t)); !slices.Equal(got, want) {
		t.Fatalf("records %v, want only the other host ids' %v", got, want)
	}
}

func TestAnAgentWhoseAddressAnotherHostUsesPublishesNothingAndKeepsTheReason(t *testing.T) {
	f := newRecordFixture(t, "")
	f.put(t, model.AgentRecord{Address: "dana/omp-31", Host: "desktop", HostID: "0000000000000000", Block: "5", Name: "omp", Actor: "dana", MessageFrom: []string{}})
	started := f.start(t, model.AgentSplit)
	if started.Entry.AddressUsedBy != "desktop" {
		t.Fatalf("entry %+v, want the reason it published nothing", started.Entry)
	}
	if got := blocksOf(f.records(t)); !slices.Equal(got, []string{"0000000000000000/5"}) {
		t.Fatalf("records %v, want the other host's only", got)
	}
	stored, err := ReadAgents(filepath.Join(f.state, AgentsFile))
	if err != nil || len(stored) != 1 || stored[0].AddressUsedBy != "desktop" || stored[0].Address != "dana/omp-31" {
		t.Fatalf("agents.json %+v, %v; want the reason on the entry", stored, err)
	}
	// The reconcile does not publish it either while the other host holds the address, and does once the other record is gone.
	f.prune(t)
	if len(f.records(t)) != 1 {
		t.Fatalf("records after a reconcile: %+v", f.records(t))
	}
	_, err = f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{model.AgentRecordKey("0000000000000000", "5"): nil}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.prune(t)
	if got := f.records(t); len(got) != 1 || got[0].HostID != f.hostID(t) {
		t.Fatalf("records %+v, want this agent's once the address is free", got)
	}
	if stored, _ := ReadAgents(filepath.Join(f.state, AgentsFile)); len(stored) != 1 || stored[0].AddressUsedBy != "" {
		t.Fatalf("agents.json %+v, want the reason cleared", stored)
	}
}

func TestAReconcileDoesNotPublishAnAgentThatExited(t *testing.T) {
	f := newRecordFixture(t, "")
	seedAgent(t, worker.Dirs{Config: f.state, State: f.state}, model.AgentEntry{IssueUID: f.uid, IssueRef: "demo#ab12", ProjectID: f.project.ID, Block: "40", PaneBlock: "7", Teammate: "omp", Address: "dana/omp-40"})
	if saved := as[model.AgentStatesSaved](t, run(t, f.gateway, model.SaveAgentStates{States: []model.AgentState{{Block: "40", State: model.AgentExited}}})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	f.live = []string{"7", "40"}
	f.prune(t)
	if got := f.records(t); len(got) != 0 {
		t.Fatalf("records %+v, want none for an agent that exited", got)
	}
}

func TestAgentTomlHostIsTopLevelOnly(t *testing.T) {
	cfg, err := ReadAgentConfig(writeAgentFile(t, filepath.Join(t.TempDir(), "agent.toml"), "host = \"laptop\"\n[projects.demo]\nname = \"x\"\n"))
	if err != nil || cfg.Host != "laptop" {
		t.Fatalf("host %q, %v", cfg.Host, err)
	}
	for _, text := range []string{"[projects.demo]\nhost = \"laptop\"\n", "host = \"\"\n", "host = 3\n"} {
		if _, err := ReadAgentConfig(writeAgentFile(t, filepath.Join(t.TempDir(), "agent.toml"), text)); err == nil || !strings.Contains(err.Error(), "host") {
			t.Errorf("%q: err %v, want a refusal naming host", text, err)
		}
	}
}

func TestForgettingARemoteRecordRemovesItAndTheMessageAtItsAddressOnly(t *testing.T) {
	f := newRecordFixture(t, "")
	f.start(t, model.AgentSplit)
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	gone := model.AgentRecord{Address: "dana/omp-8", Host: "desk", HostID: "00ff00ff", Block: "8", Name: "omp", Actor: "dana", StartedAt: at, MessageFrom: []string{}}
	other := model.AgentRecord{Address: "dana/omp-9", Host: "desk", HostID: "00ff00ff", Block: "9", Name: "omp", Actor: "dana", StartedAt: at.Add(time.Hour), MessageFrom: []string{}}
	f.put(t, gone)
	f.put(t, other)
	for _, address := range []string{gone.Address, other.Address} {
		_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
			PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
			Body:       &g.PatchIssueMetadataBody{Actor: new("dana"), Patch: map[string]any{model.NotificationKey(address): map[string]any{"from": "dana", "message": "hello"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	forgotten := as[model.AgentRecordForgotten](t, run(t, f.gateway, model.ForgetAgentRecord{Workspace: f.work, ProjectID: f.project.ID, UID: f.uid, Actor: "dana",
		HostID: gone.HostID, Block: gone.Block, Address: gone.Address, Host: "desk"}))
	if forgotten.Err != nil || forgotten.Host != "desk" {
		t.Fatalf("forget: %+v", forgotten)
	}
	metadata := issueFixture{kata: f.kata}.issue(t, f.uid).Metadata
	if _, held := metadata[model.NotificationKey(gone.Address)]; held {
		t.Errorf("the message at the forgotten address is still held")
	}
	if _, held := metadata[model.NotificationKey(other.Address)]; !held {
		t.Errorf("the message at another agent's address was cleared")
	}
	if got, want := slices.Sorted(slices.Values(blocksOf(f.records(t)))), slices.Sorted(slices.Values([]string{f.hostID(t) + "/31", "00ff00ff/9"})); !slices.Equal(got, want) {
		t.Errorf("records %v, want %v: only the forgotten one goes, this host's own stays", got, want)
	}
}

func TestRemoteAgentsAreTheRecordsOfOtherHostIdsOnAnIssueOrInTheProject(t *testing.T) {
	f := newRecordFixture(t, "")
	f.put(t, model.AgentRecord{Address: "dana/omp-9", Host: "desk", HostID: "feed0001", Block: "9", Name: "omp", Actor: "dana", MessageFrom: []string{"dana-laptop"}})
	f.put(t, model.AgentRecord{Address: "dana/omp-3", Host: "laptop", HostID: f.hostID(t), Block: "3", Name: "omp", Actor: "dana"})
	short := issueFixture{kata: f.kata}.issue(t, f.uid).ShortID
	ref := "demo#" + short
	for name, cmd := range map[string]model.LoadRemoteAgents{
		"an Issue by its ref":              {Workspace: f.work, Issue: ref},
		"an Issue by its short id":         {Workspace: f.work, Issue: "#" + short},
		"an Issue by its uid":              {Workspace: f.work, Issue: f.uid},
		"the workspace's Project":          {Workspace: f.work},
		"the Project of KATA_REF":          {Workspace: f.work, ScopeRef: ref},
		"the Project of KATA_REF by a uid": {Workspace: f.work, ScopeRef: f.uid},
	} {
		got := as[model.RemoteAgentsLoaded](t, run(t, f.gateway, cmd))
		if got.Err != nil || got.NotFound || len(got.Agents) != 1 {
			t.Errorf("%s: %+v, want the one record of another host id", name, got)
			continue
		}
		a := got.Agents[0]
		if a.Address != "dana/omp-9" || a.Host != "desk" || a.HostID != "feed0001" || a.IssueRef != ref || a.IssueUID != f.uid || a.ProjectID != f.project.ID || a.MessageFrom != "dana-laptop" || a.State != "" {
			t.Errorf("%s: agent %+v", name, a)
		}
	}
	if got := as[model.RemoteAgentsLoaded](t, run(t, f.gateway, model.LoadRemoteAgents{Workspace: f.work, Issue: "demo#zzzz"})); !got.NotFound || got.Err != nil {
		t.Errorf("an Issue that does not exist: %+v, want NotFound", got)
	}
}

func TestAReconcileKeepsTheRecordOfAnAgentAddedToTheMappingMeanwhile(t *testing.T) {
	f := newRecordFixture(t, "host = \"laptop\"\n")
	hostID := f.hostID(t)
	record := model.AgentRecord{Address: "dana/omp-55", Host: "laptop", HostID: hostID, Block: "55", Name: "omp", Actor: "dana", MessageFrom: []string{}}
	f.put(t, record)
	// The entry is in agents.json when the reconcile looks, though a Pane read the file before it was written.
	seedAgent(t, worker.Dirs{Config: f.state, State: f.state}, model.AgentEntry{IssueUID: f.uid, IssueRef: "demo#ab12", ProjectID: f.project.ID, Block: "55", PaneBlock: "7", Teammate: "omp",
		Address: "dana/omp-55", Actor: "dana", StartedAt: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)})
	if err := f.gateway.syncAgentRecords(context.Background(), f.work, agentDirs{config: f.state, state: f.state}, filepath.Join(f.state, AgentsFile)); err != nil {
		t.Fatal(err)
	}
	if got := blocksOf(f.records(t)); !slices.Equal(got, []string{hostID + "/55"}) {
		t.Fatalf("records %v, want the new agent's kept", got)
	}
}

func TestAReconcileReplacesAStaleRecordWithTheSameKey(t *testing.T) {
	f := newRecordFixture(t, "host = \"laptop\"\n")
	hostID := f.hostID(t)
	f.put(t, model.AgentRecord{Address: "dana/omp-40", Host: "laptop", HostID: hostID, Block: "40", Name: "old", Actor: "dana", MessageFrom: []string{}})
	seedAgent(t, worker.Dirs{Config: f.state, State: f.state}, model.AgentEntry{IssueUID: f.uid, IssueRef: "demo#ab12", ProjectID: f.project.ID, Block: "40", PaneBlock: "7", Teammate: "fresh",
		Address: "dana/fresh-40", Actor: "dana", StartedAt: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)})
	f.live = []string{"7", "40"}
	f.prune(t)
	got := f.records(t)
	if len(got) != 1 || got[0].Address != "dana/fresh-40" || got[0].Name != "fresh" {
		t.Fatalf("records %+v, want the stale one replaced", got)
	}
}

func TestAReconcileReadsOnlyTheShownProjectAndTheIssuesOfItsEntries(t *testing.T) {
	f := newRecordFixture(t, "")
	hostID := f.hostID(t)
	otherWork, otherProject := project(t, f.gateway, "elsewhere")
	elsewhere := createIssue(t, f.gateway, otherWork, otherProject, "Not shown")
	stray := model.AgentRecord{Address: "dana/omp-99", Host: "gone", HostID: hostID, Block: "99", Name: "omp", Actor: "dana", MessageFrom: []string{}}
	f.put(t, stray)
	unseen := stray
	unseen.Block, unseen.Address = "98", "dana/omp-98"
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: otherProject.ID, Ref: elsewhere},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{model.AgentRecordKey(hostID, "98"): unseen}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.prune(t)
	if left := f.records(t); len(left) != 0 {
		t.Fatalf("records %+v on the shown Project's Issue, want the stray gone", left)
	}
	if got := model.AgentRecords(issueFixture{kata: f.kata}.issue(t, elsewhere).Metadata); len(got) != 1 {
		t.Fatalf("records %+v on an Issue the Pane does not show, want it left alone", got)
	}
}

func TestAReconcileRemovesTheRecordOfAMappedAgentThatExited(t *testing.T) {
	f := newRecordFixture(t, "host = \"laptop\"\n")
	hostID := f.hostID(t)
	f.put(t, model.AgentRecord{Address: "", Host: "laptop", HostID: hostID, Block: "40", Name: "claude", Actor: "dana", NoAddress: model.AgentNotOmp, MessageFrom: []string{}})
	seedAgent(t, worker.Dirs{Config: f.state, State: f.state}, model.AgentEntry{IssueUID: f.uid, IssueRef: "demo#ab12", ProjectID: f.project.ID, Block: "40", PaneBlock: "7", Teammate: "claude",
		NoAddress: model.AgentNotOmp, StartedAt: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)})
	f.live = []string{"7", "40"}
	if saved := as[model.AgentStatesSaved](t, run(t, f.gateway, model.SaveAgentStates{States: []model.AgentState{{Block: "40", State: model.AgentExited}}})); saved.Err != nil {
		t.Fatal(saved.Err)
	}
	f.prune(t)
	if left := f.records(t); len(left) != 0 {
		t.Fatalf("records %+v, want the exited agent's gone", left)
	}
}

func TestAnAgentWithNoAddressHasItsRecordRemovedWhenItExits(t *testing.T) {
	f := newRecordFixture(t, "")
	f.put(t, model.AgentRecord{Host: "laptop", HostID: f.hostID(t), Block: "40", Name: "claude", Actor: "dana", NoAddress: model.AgentNotOmp, MessageFrom: []string{}})
	cleared := as[model.AgentAddressCleared](t, run(t, f.gateway, model.ClearAgentAddress{Workspace: f.work, ProjectID: f.project.ID, UID: f.uid, Actor: "dana", Block: "40"}))
	if cleared.Err != nil || cleared.Block != "40" {
		t.Fatalf("cleared %+v", cleared)
	}
	if left := f.records(t); len(left) != 0 {
		t.Fatalf("records %+v, want it gone", left)
	}
}

func TestAnEmptyHostIdFileIsMadeAgain(t *testing.T) {
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, HostIDFile), []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := ReadHostID(state)
	if err != nil || len(id) < 16 {
		t.Fatalf("host id %q, %v; want a new one", id, err)
	}
	if again, err := ReadHostID(state); err != nil || again != id {
		t.Fatalf("second read %q, %v; want %q kept", again, err, id)
	}
}

func TestClearingAnExitedAgentOnAnIssueKataNoLongerHasClearsOnceWithNoError(t *testing.T) {
	f := newRecordFixture(t, "")
	cleared := as[model.AgentAddressCleared](t, run(t, f.gateway, model.ClearAgentAddress{Workspace: f.work, ProjectID: f.project.ID, UID: "01ZZZZZZZZZZZZZZZZZZZZZZZZ", Actor: "dana", Block: "31", Address: "dana/omp-31"}))
	if cleared.Err != nil || cleared.Block != "31" {
		t.Fatalf("cleared %+v, want no error for an Issue kata no longer has", cleared)
	}
}

func TestPanesFindingAnEmptyHostIDFileAllEndWithOneID(t *testing.T) {
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, HostIDFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const panes = 8
	ids := make([]string, panes)
	var wg sync.WaitGroup
	for i := range panes {
		wg.Go(func() {
			id, err := ReadHostID(state)
			if err != nil {
				t.Error(err)
			}
			ids[i] = id
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("ids %v, want one id for every Pane", ids)
		}
	}
	if again, _ := ReadHostID(state); again != ids[0] {
		t.Fatalf("file holds %q, Panes got %q", again, ids[0])
	}
}

func TestListingRecordsToleratesAProjectTheDaemonDoesNotKnow(t *testing.T) {
	f := newRecordFixture(t, "")
	unknown := t.TempDir()
	if err := os.WriteFile(filepath.Join(unknown, ".kata.toml"), []byte("version = 1\n\n[project]\nname = \"not-on-this-daemon\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := model.AgentEntry{IssueUID: f.uid, IssueRef: "demo#ab12", ProjectID: f.project.ID, Block: "31"}
	var issues map[string]issueRecordsListed
	err := f.gateway.withDaemon(context.Background(), unknown, func(c *Client) error {
		var err error
		issues, err = c.listIssueRecords(context.Background(), unknown, []model.AgentEntry{entry})
		return err
	})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if _, ok := issues[f.uid]; !ok {
		t.Fatalf("issues %v, want the entry's Issue listed", issues)
	}
}

func TestAnAddressAnotherHostTookAfterTheListingIsNotPublishedAgain(t *testing.T) {
	f := newRecordFixture(t, "")
	host, err := f.gateway.hostOf(agentDirs{config: f.state, state: f.state})
	if err != nil {
		t.Fatal(err)
	}
	entry := model.AgentEntry{IssueUID: f.uid, IssueRef: "demo#ab12", ProjectID: f.project.ID, Block: "31", Teammate: "omp", Address: "dana/omp-31", Actor: "dana"}
	var usedBy string
	err = f.gateway.withDaemon(context.Background(), f.work, func(c *Client) error {
		issues, err := c.listIssueRecords(context.Background(), f.work, []model.AgentEntry{entry})
		if err != nil {
			return err
		}
		// The other host publishes after the listing was read.
		f.put(t, model.AgentRecord{Address: "dana/omp-31", Host: "desktop", HostID: "0000000000000000", Block: "5", Name: "omp", Actor: "dana", MessageFrom: []string{}})
		var skip bool
		usedBy, skip, err = c.publishListed(context.Background(), host, entry, issues[f.uid])
		if skip {
			t.Error("skipped an Issue kata has")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if usedBy != "desktop" {
		t.Fatalf("used by %q, want desktop", usedBy)
	}
	if got := blocksOf(f.records(t)); !slices.Equal(got, []string{"0000000000000000/5"}) {
		t.Fatalf("records %v, want the other host's only", got)
	}
}

func TestAReconcileSkipsAnIssueKataNoLongerHas(t *testing.T) {
	f := newRecordFixture(t, "")
	seedAgent(t, worker.Dirs{Config: f.state, State: f.state}, model.AgentEntry{IssueUID: "01ZZZZZZZZZZZZZZZZZZZZZZZZ", IssueRef: "demo#gone", ProjectID: f.project.ID, Block: "40", PaneBlock: "7", Teammate: "omp", Address: "dana/omp-40", Actor: "dana"})
	host, err := f.gateway.hostOf(agentDirs{config: f.state, state: f.state})
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := ReadAgents(filepath.Join(f.state, AgentsFile))
	err = f.gateway.withDaemon(context.Background(), f.work, func(c *Client) error {
		issues, err := c.listIssueRecords(context.Background(), f.work, entries)
		if err != nil {
			return err
		}
		if !issues["01ZZZZZZZZZZZZZZZZZZZZZZZZ"].missing {
			t.Errorf("issues %+v, want the gone Issue marked missing", issues)
		}
		if _, skip, err := c.publishListed(context.Background(), host, entries[0], issues["01ZZZZZZZZZZZZZZZZZZZZZZZZ"]); !skip || err != nil {
			t.Errorf("skip %v, err %v; want the gone Issue skipped with no publish", skip, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.gateway.syncAgentRecords(context.Background(), f.work, agentDirs{config: f.state, state: f.state}, filepath.Join(f.state, AgentsFile)); err != nil {
		t.Fatalf("reconcile: %v, want no error for a gone Issue", err)
	}
}

func TestPruningAnAgentWhoseBlockClosedClearsWhatIsLeftAtItsAddress(t *testing.T) {
	f := newRecordFixture(t, "")
	f.start(t, model.AgentSplit)
	issue := issueFixture{kata: f.kata}
	key := model.NotificationKey("dana/omp-31")
	sent := as[model.AgentMessageSent](t, run(t, f.gateway, model.SendAgentMessage{Workspace: f.work, ProjectID: f.project.ID, UID: f.uid, Actor: "dana", Address: "dana/omp-31", Message: "hello"}))
	if sent.Err != nil || sent.Held != "" {
		t.Fatalf("sent %+v", sent)
	}
	if _, held := issue.issue(t, f.uid).Metadata[key]; !held {
		t.Fatal("the message was not written")
	}
	f.live = []string{"7"}
	if kept := f.prune(t); len(kept) != 0 {
		t.Fatalf("prune kept %+v", kept)
	}
	if _, held := issue.issue(t, f.uid).Metadata[key]; held {
		t.Fatal("the message is still at the address of an agent that is gone")
	}
}

func TestAReconcileClearsTheMessageAtTheAddressOfARecordWithNoEntry(t *testing.T) {
	f := newRecordFixture(t, "host = \"laptop\"\n")
	record := model.AgentRecord{Address: "dana/omp-55", Host: "laptop", HostID: f.hostID(t), Block: "55", Name: "omp", Actor: "dana", MessageFrom: []string{}}
	f.put(t, record)
	_, err := f.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: f.project.ID, Ref: f.uid},
		Body:       &g.PatchIssueMetadataBody{Actor: new("dana"), Patch: map[string]any{model.NotificationKey(record.Address): map[string]any{"from": "dana", "message": "hello"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.gateway.syncAgentRecords(context.Background(), f.work, agentDirs{config: f.state, state: f.state}, filepath.Join(f.state, AgentsFile)); err != nil {
		t.Fatal(err)
	}
	metadata := issueFixture{kata: f.kata}.issue(t, f.uid).Metadata
	if _, held := metadata[model.NotificationKey(record.Address)]; held {
		t.Errorf("the message at the address of the dropped record is still held")
	}
	if got := f.records(t); len(got) != 0 {
		t.Errorf("records %v, want the entry-less one dropped", got)
	}
}
