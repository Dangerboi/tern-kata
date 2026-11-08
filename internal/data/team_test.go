package data

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// teamHost is one host's state folder, tern and kata daemon, with one Project, a parent Issue and children of it.
type teamHost struct {
	t         *testing.T
	kata      *kataServer
	dirs      worker.Dirs
	gateway   *Gateway
	workspace string
	project   model.Project
	parent    string
	children  []string

	mu    sync.Mutex
	runs  [][]string // every `tern split|new` command line, in order
	block int
}

var childTitles = []string{"Alpha rollout", "Bravo migration", "Charlie telemetry", "Delta billing"}

func newTeamHost(t *testing.T, children int) *teamHost {
	t.Helper()
	h := &teamHost{t: t, kata: startKata(t, "0.21.0"), block: 100}
	for _, name := range []string{"KATA_HOME", "KATA_SERVER", "KATA_AUTH_TOKEN_FILE", "KATA_ALLOW_INSECURE"} {
		t.Setenv(name, "")
	}
	h.gateway = newGateway(t, newLocator(h.kata.target()))
	h.dirs, _ = h.gateway.workerDirs()
	h.fakeTern(h.gateway)
	h.workspace, h.project = project(t, h.gateway, "demo")
	h.parent = createIssue(t, h.gateway, h.workspace, h.project, "Quarterly planning spec")
	for i := range children {
		h.children = append(h.children, h.child(childTitles[i]))
	}
	return h
}

func (h *teamHost) child(title string) string {
	created := as[model.IssueCreated](h.t, run(h.t, h.gateway, model.CreateIssueCmd{Workspace: h.workspace, Actor: "tester", ProjectID: h.project.ID, Title: title, Body: "body", ParentUID: h.parent}))
	if created.Err != nil {
		h.t.Fatal(created.Err)
	}
	return created.UID
}

// fakeTern answers tern as a window with no hidden session: every start gets the next block id.
func (h *teamHost) fakeTern(g *Gateway) {
	g.runTern = func(_ context.Context, args []string) ([]byte, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		switch args[0] {
		case "ls":
			return []byte(lsWithoutAgents), nil
		case "rename", "close":
			return nil, nil
		}
		h.runs = append(h.runs, args)
		h.block++
		return []byte(`{"session":3,"tab":9,"block":` + strconv.Itoa(h.block) + `}`), nil
	}
}

// another is a second Pane on the same host: its own gateway on the same state folder and daemon.
func (h *teamHost) another() *Gateway {
	g := newGateway(h.t, newLocator(h.kata.target()))
	g.workerDirs = h.gateway.workerDirs
	h.fakeTern(g)
	return g
}

func (h *teamHost) teamsFile() string  { return filepath.Join(h.dirs.State, TeamsFile) }
func (h *teamHost) agentsFile() string { return filepath.Join(h.dirs.State, AgentsFile) }

func (h *teamHost) startTeam(limit int) model.TeamStarted {
	h.t.Helper()
	return as[model.TeamStarted](h.t, run(h.t, h.gateway, model.StartTeam{Placement: model.AgentSplit, IssueUID: h.parent, IssueRef: "demo#parent", IssueTitle: "The parent",
		ProjectID: h.project.ID, ProjectName: "demo", Workspace: h.workspace, PaneBlock: "7", Actor: "dana", Limit: limit}))
}

func (h *teamHost) drive(g *Gateway, exited ...string) model.TeamsDriven {
	h.t.Helper()
	return as[model.TeamsDriven](h.t, run(h.t, g, model.DriveTeams{Workspace: h.workspace, PaneBlock: "7", Actor: "dana", BoundProjectID: h.project.ID, Exited: exited}))
}

func (h *teamHost) team() model.Team {
	h.t.Helper()
	teams, err := ReadTeams(h.teamsFile())
	if err != nil || len(teams) != 1 {
		h.t.Fatalf("teams.json %+v, %v", teams, err)
	}
	return teams[0]
}

func (h *teamHost) runCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.runs)
}

// runLines are the tern command lines that started agents, as text.
func (h *teamHost) runLines() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var lines []string
	for _, r := range h.runs {
		lines = append(lines, strings.Join(r, " "))
	}
	return lines
}

func childUIDs(t model.Team) []string {
	var out []string
	for _, c := range t.Children {
		out = append(out, c.UID)
	}
	return out
}

func (h *teamHost) close(uid string) {
	h.t.Helper()
	if done := as[model.CloseResultMsg](h.t, run(h.t, h.gateway, model.CloseIssueCmd{Workspace: h.workspace, UID: uid, Actor: "tester", ProjectID: h.project.ID, Quick: true, Key: "k-team-close-" + uid})); done.Err != nil {
		h.t.Fatal(done.Err)
	}
}

func (h *teamHost) setState(state model.TeamState) {
	h.t.Helper()
	if _, err := UpdateTeams(h.teamsFile(), func(l []model.Team) []model.Team {
		l[0].State = state
		return l
	}); err != nil {
		h.t.Fatal(err)
	}
}

func TestStartingATeamRunsTheCoordinatorAndRecordsTheTeam(t *testing.T) {
	h := newTeamHost(t, 3)
	got := h.startTeam(2)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	line := h.runLines()
	if len(line) != 1 || !strings.Contains(line[0], "--coordinating") || strings.Contains(line[0], "--coordinator ") || !strings.HasPrefix(line[0], "split 7 right") {
		t.Fatalf("coordinator ran %q", line)
	}
	team := h.team()
	if team.ParentUID != h.parent || team.ParentRef != "demo#parent" || team.Project != "demo" || team.ProjectID != h.project.ID || team.Limit != 2 || team.State != model.TeamRunning ||
		team.CoordinatorBlock != got.Entry.Block || team.CoordinatorAddress != "dana/omp-"+got.Entry.Block || len(team.Children) != 0 {
		t.Fatalf("team %+v", team)
	}
	// A second team on the Issue is refused and starts nothing.
	again := h.startTeam(2)
	if again.Err == nil || again.Err.Error() != model.TeamExistsReason || h.runCount() != 1 {
		t.Fatalf("second team: %+v, %d runs", again, h.runCount())
	}
}

func TestATeamIsRefusedWithItsReasonAndStartsNothing(t *testing.T) {
	for name, c := range map[string]struct {
		toml     string
		children int
		limit    int
		want     string
	}{
		"an agent that is not omp":     {"command = [\"claude\"]\n", 1, 2, "not omp"},
		"a name too long to be a mate": {"name = \"" + strings.Repeat("a", 50) + "\"\n", 1, 2, "cannot be a teammate"},
		"no open child":                {"", 0, 2, "no open child in the Project demo"},
		"a limit out of range":         {"", 1, 9, "1 to 8"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newTeamHost(t, c.children)
			writeAgentFile(t, filepath.Join(h.dirs.Config, "agent.toml"), c.toml)
			got := h.startTeam(c.limit)
			if got.Err == nil || !strings.Contains(got.Err.Error(), c.want) {
				t.Fatalf("got %+v, want an error with %q", got, c.want)
			}
			if h.runCount() != 0 {
				t.Errorf("a refused team ran %q", h.runLines())
			}
			if teams, _ := ReadTeams(h.teamsFile()); len(teams) != 0 {
				t.Errorf("a refused team left %+v", teams)
			}
		})
	}
}

func TestATeamStartsTheLimitAsBackgroundChildrenAndTheNextWhenOneExits(t *testing.T) {
	h := newTeamHost(t, 3)
	coordinator := h.startTeam(2).Entry
	driven := h.drive(h.gateway)
	if driven.Err != nil || len(driven.Failures) != 0 || len(driven.Started) != 2 {
		t.Fatalf("drive: %+v", driven)
	}
	lines := h.runLines()
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "new session kata agents") || !strings.Contains(line, "--coordinator "+coordinator.Address+" ") || strings.Contains(line, "--coordinating") {
			t.Errorf("child ran %q", line)
		}
	}
	// kata ready lists the most recently updated first, and the queue follows that order, so which two start is kata's to say.
	left := slices.Clone(h.children)
	for _, e := range driven.Started {
		if !e.Background || !slices.Contains(left, e.IssueUID) {
			t.Errorf("started %+v", e)
		}
		left = slices.DeleteFunc(left, func(uid string) bool { return uid == e.IssueUID })
	}
	team := h.team()
	if !slices.Equal(childUIDs(team), []string{driven.Started[0].IssueUID, driven.Started[1].IssueUID}) || team.Children[0].Block != driven.Started[0].Block {
		t.Fatalf("team %+v", team)
	}
	// Nothing more starts while both run, and a drive again starts nothing.
	if again := h.drive(h.gateway); len(again.Started) != 0 || h.runCount() != 3 {
		t.Fatalf("a full team started %+v", again.Started)
	}
	// One exits: the third child starts, and an exited child is never started again.
	third := h.drive(h.gateway, driven.Started[0].Block)
	if len(third.Started) != 1 || third.Started[0].IssueUID != left[0] {
		t.Fatalf("after an exit: %+v", third.Started)
	}
	if more := h.drive(h.gateway, driven.Started[0].Block, driven.Started[1].Block, third.Started[0].Block); len(more.Started) != 0 || h.runCount() != 4 {
		t.Fatalf("a child already run started again: %+v", more.Started)
	}
}

func TestAChildCreatedLaterJoinsTheQueueAndSkippedChildrenAreNotStarted(t *testing.T) {
	h := newTeamHost(t, 2)
	h.startTeam(1)
	// A child with a running agent, and one somebody owns, are not started.
	seedAgent(t, h.dirs, model.AgentEntry{IssueUID: h.children[0], IssueRef: "demo#c1", Block: "900"})
	owned := as[model.IssueCreated](t, run(t, h.gateway, model.CreateIssueCmd{Workspace: h.workspace, Actor: "tester", ProjectID: h.project.ID, Title: "Zulu ownership transfer", Body: "b", Owner: "someone", ParentUID: h.parent}))
	if owned.Err != nil {
		t.Fatal(owned.Err)
	}
	stranger := createIssue(t, h.gateway, h.workspace, h.project, "Yankee unrelated chore")
	first := h.drive(h.gateway)
	if len(first.Started) != 1 || first.Started[0].IssueUID != h.children[1] {
		t.Fatalf("started %+v, want only the second child", first.Started)
	}
	later := h.child("Xray late arrival")
	h.close(h.children[1])
	next := h.drive(h.gateway, first.Started[0].Block)
	if len(next.Started) != 1 || next.Started[0].IssueUID != later {
		t.Fatalf("started %+v, want the child created later (not %s, not %s)", next.Started, owned.UID, stranger)
	}
}

func TestTwoPanesDrivingOneTeamStartEachChildOnce(t *testing.T) {
	h := newTeamHost(t, 3)
	h.startTeam(2)
	panes := []*Gateway{h.gateway, h.another(), h.another()}
	var wg sync.WaitGroup
	for _, g := range panes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.drive(g)
		}()
	}
	wg.Wait()
	team := h.team()
	if len(team.Children) != 2 || h.runCount() != 3 {
		t.Fatalf("team %+v after %d runs, want two children and the coordinator started once", team, h.runCount())
	}
	seen := map[string]bool{}
	for _, c := range team.Children {
		if seen[c.UID] || c.Block == "" {
			t.Fatalf("children %+v", team.Children)
		}
		seen[c.UID] = true
	}
	entries, _ := ReadAgents(h.agentsFile())
	if len(entries) != 3 {
		t.Fatalf("agents.json holds %d entries, want the coordinator and two children", len(entries))
	}
}

func TestAPausedTeamStartsNothingAndNeverEnds(t *testing.T) {
	h := newTeamHost(t, 2)
	h.startTeam(2)
	h.setState(model.TeamPaused)
	if got := h.drive(h.gateway); len(got.Started) != 0 || h.runCount() != 1 {
		t.Fatalf("a paused team started %+v", got.Started)
	}
	for _, c := range h.children {
		h.close(c)
	}
	h.drive(h.gateway)
	if team := h.team(); team.State != model.TeamPaused {
		t.Fatalf("a paused team ended or moved: %+v", team)
	}
	if held := h.slotOnParent("dana/omp-" + h.team().CoordinatorBlock); held != "" {
		t.Fatalf("a paused team wrote %s", held)
	}
}

func TestACoordinatorThatExitedStopsNewStartsAndTheRecordGoesWithTheLastChild(t *testing.T) {
	h := newTeamHost(t, 3)
	coordinator := h.startTeam(1).Entry
	child := h.drive(h.gateway).Started[0]
	// The coordinator exits: no new child starts although the limit would allow it once the child is done.
	got := h.drive(h.gateway, coordinator.Block)
	if len(got.Started) != 0 || h.team().State != model.TeamStopping {
		t.Fatalf("after the coordinator exited: %+v, team %+v", got.Started, h.team())
	}
	if h.drive(h.gateway, coordinator.Block); h.team().State != model.TeamStopping {
		t.Fatalf("the record went while a child runs: %+v", h.team())
	}
	got = h.drive(h.gateway, coordinator.Block, child.Block)
	if len(got.Started) != 0 || len(got.Teams) != 0 {
		t.Fatalf("with the last child exited: %+v", got)
	}
	if teams, err := ReadTeams(h.teamsFile()); err != nil || len(teams) != 0 {
		t.Fatalf("teams.json %+v, %v", teams, err)
	}
	// A coordinator whose entry is gone (its block closed) stops the team as well.
	h2 := newTeamHost(t, 1)
	h2.startTeam(1)
	if _, err := UpdateAgents(h2.agentsFile(), func([]model.AgentEntry) []model.AgentEntry { return nil }); err != nil {
		t.Fatal(err)
	}
	h2.drive(h2.gateway)
	if teams, _ := ReadTeams(h2.teamsFile()); len(teams) != 0 {
		t.Fatalf("a coordinator that closed left %+v", teams)
	}
}

// slotOnParent is the raw message held at address on the parent Issue, "" when none.
func (h *teamHost) slotOnParent(address string) string {
	h.t.Helper()
	var raw string
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := h.gateway.call(ctx, h.workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		var err error
		raw, _, err = c.readSlot(ctx, h.project.ID, h.parent, model.NotificationKey(address))
		return nil, err
	}); err != nil {
		h.t.Fatal(err)
	}
	return raw
}

func TestTheEndMessageIsWrittenOnceAndTheRecordGoes(t *testing.T) {
	h := newTeamHost(t, 2)
	coordinator := h.startTeam(2).Entry
	started := h.drive(h.gateway).Started
	for _, c := range h.children[:1] {
		h.close(c)
	}
	blocks := []string{started[0].Block, started[1].Block}
	// One child still runs: no message.
	h.drive(h.gateway, blocks[0])
	if held := h.slotOnParent(coordinator.Address); held != "" || h.team().State != model.TeamRunning {
		t.Fatalf("a team with a child running ended: %q, %+v", held, h.team())
	}
	// The other exits with its Issue open: nothing is left to start, so the message names both.
	h.drive(h.gateway, blocks...)
	raw := h.slotOnParent(coordinator.Address)
	slot, err := model.ReadNotificationSlot(model.NotificationKey(coordinator.Address), raw)
	if err != nil {
		t.Fatalf("slot %q: %v", raw, err)
	}
	if slot.From != "dana" || slot.Teammate != "" || !strings.Contains(slot.Message, "Children closed (1): ") || !strings.Contains(slot.Message, "Children still open (1): ") ||
		!strings.Contains(slot.Message, "Alpha") || !strings.Contains(slot.Message, "Bravo") {
		t.Fatalf("end message %+v", slot)
	}
	if teams, _ := ReadTeams(h.teamsFile()); len(teams) != 0 {
		t.Fatalf("the record stayed after the message: %+v", teams)
	}
	// The coordinator clears the message; nothing writes it again.
	other := h.another()
	if _, err := other.call(context.Background(), h.workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		_, err := c.clearSlot(ctx, h.project.ID, h.parent, "dana", model.NotificationKey(coordinator.Address), raw)
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	h.drive(other, blocks...)
	if held := h.slotOnParent(coordinator.Address); held != "" {
		t.Fatalf("the end message was written twice: %s", held)
	}
}

func TestAnUnreadCoordinatorMessageIsNeverReplacedAndTheEndMessageFollowsOnceItClears(t *testing.T) {
	h := newTeamHost(t, 1)
	coordinator := h.startTeam(1).Entry
	child := h.drive(h.gateway).Started[0]
	h.close(h.children[0])
	// The coordinator holds a message nobody read.
	ctx := context.Background()
	if _, err := h.gateway.call(ctx, h.workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		sent, err := c.SendAgentMessage(ctx, model.SendAgentMessage{ProjectID: h.project.ID, UID: h.parent, Actor: "dana", Address: coordinator.Address, Message: "a child asks"})
		if sent.Held != "" {
			t.Fatalf("held %s", sent.Held)
		}
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	held := h.slotOnParent(coordinator.Address)
	h.drive(h.gateway, child.Block)
	if now := h.slotOnParent(coordinator.Address); now != held {
		t.Fatalf("the unread message was replaced: %s", now)
	}
	if team := h.team(); team.State != model.TeamEnding {
		t.Fatalf("team %+v, want ending", team)
	}
	h.drive(h.gateway, child.Block)
	if now := h.slotOnParent(coordinator.Address); now != held || h.team().State != model.TeamEnding {
		t.Fatalf("a later drive replaced the unread message or moved the team: %s, %+v", now, h.team())
	}
	// The coordinator clears it: the end message follows and the record goes.
	if _, err := h.gateway.call(ctx, h.workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		_, err := c.clearSlot(ctx, h.project.ID, h.parent, "dana", model.NotificationKey(coordinator.Address), held)
		return nil, err
	}); err != nil {
		t.Fatal(err)
	}
	h.drive(h.gateway, child.Block)
	slot, err := model.ReadNotificationSlot(model.NotificationKey(coordinator.Address), h.slotOnParent(coordinator.Address))
	if err != nil || !strings.Contains(slot.Message, "Children closed (1)") {
		t.Fatalf("end message %+v, %v", slot, err)
	}
	if teams, _ := ReadTeams(h.teamsFile()); len(teams) != 0 {
		t.Fatalf("the record stayed: %+v", teams)
	}
}

func TestATeamsJSONRoundTripsAndEveryChangeStartsFromTheFileAsItIs(t *testing.T) {
	path := filepath.Join(t.TempDir(), TeamsFile)
	if teams, err := ReadTeams(path); err != nil || teams != nil {
		t.Fatalf("a missing file read %+v, %v", teams, err)
	}
	want := model.Team{ParentUID: "P", ParentRef: "demo#p", ProjectID: 3, Project: "demo", CoordinatorBlock: "5", CoordinatorAddress: "dana/omp-5", Limit: 2, State: model.TeamPaused,
		Children: []model.TeamChild{{UID: "C", Ref: "demo#c", Block: "6", StartedAt: time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)}}}
	if _, err := UpdateTeams(path, func(l []model.Team) []model.Team { return append(l, want) }); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, key := range []string{`"parent_uid"`, `"parent_ref"`, `"project_id"`, `"coordinator_block"`, `"coordinator_address"`, `"limit"`, `"state": "paused"`, `"children"`, `"block": "6"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("teams.json lacks %s:\n%s", key, raw)
		}
	}
	got, err := ReadTeams(path)
	if err != nil || len(got) != 1 || got[0].State != want.State || got[0].Children[0] != want.Children[0] {
		t.Fatalf("read %+v, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTeams(path); err == nil {
		t.Error("a broken teams.json was read")
	}
}

func TestAgentCommandLinePassesTheTeamFlagsOnlyWithAnAddress(t *testing.T) {
	has := func(line []string, flag string) bool { return slices.Contains(line, flag) }
	child := AgentCommandLine("me", "demo#ab12", AgentMessaging{AddressPrefix: "dana/omp", Coordinator: "dana/omp-7"}, AgentRoute{}, []string{"omp"}, "/p.md")
	if i := slices.Index(child, "--coordinator"); i < 0 || child[i+1] != "dana/omp-7" || has(child, "--coordinating") {
		t.Errorf("child %q", child)
	}
	coordinator := AgentCommandLine("me", "demo#ab12", AgentMessaging{AddressPrefix: "dana/omp", Coordinating: true}, AgentRoute{}, []string{"omp"}, "/p.md")
	if !has(coordinator, "--coordinating") || has(coordinator, "--coordinator") {
		t.Errorf("coordinator %q", coordinator)
	}
	bare := AgentCommandLine("me", "demo#ab12", AgentMessaging{Coordinator: "dana/omp-7", Coordinating: true}, AgentRoute{}, []string{"omp"}, "/p.md")
	if has(bare, "--coordinator") || has(bare, "--coordinating") {
		t.Errorf("without an address: %q", bare)
	}
}

func (h *teamHost) pause(paused bool) model.TeamPausedSet {
	h.t.Helper()
	return as[model.TeamPausedSet](h.t, run(h.t, h.gateway, model.SetTeamPaused{ParentUID: h.parent, ParentRef: "demo#parent", Paused: paused}))
}

func TestPausingWritesPausedAndResumingLetsTheQueueGoOn(t *testing.T) {
	h := newTeamHost(t, 2)
	h.startTeam(2)
	if got := h.pause(true); got.Err != nil || len(got.Teams) != 1 || got.Teams[0].State != model.TeamPaused || h.team().State != model.TeamPaused {
		t.Fatalf("pause: %+v, file %+v", got, h.team())
	}
	if got := h.drive(h.gateway); len(got.Started) != 0 || h.runCount() != 1 {
		t.Fatalf("a paused team started %+v", got.Started)
	}
	if got := h.pause(true); got.Err != nil || h.team().State != model.TeamPaused {
		t.Fatalf("pausing twice: %+v", got)
	}
	if got := h.pause(false); got.Err != nil || h.team().State != model.TeamRunning {
		t.Fatalf("resume: %+v, file %+v", got, h.team())
	}
	if got := h.drive(h.gateway); len(got.Started) != 2 {
		t.Fatalf("a resumed team started %d children, want 2", len(got.Started))
	}
}

func TestPausingRefusesAStoppingEndingOrMissingTeamAndLeavesTheFileAsItIs(t *testing.T) {
	h := newTeamHost(t, 1)
	h.startTeam(1)
	for state, reason := range map[model.TeamState]string{model.TeamStopping: model.TeamStoppingReason, model.TeamEnding: model.TeamEndingReason} {
		h.setState(state)
		if got := h.pause(true); got.Err == nil || got.Err.Error() != reason || h.team().State != state {
			t.Errorf("%s: %+v, file %+v", state, got, h.team())
		}
	}
	if got := as[model.TeamPausedSet](t, run(t, h.gateway, model.SetTeamPaused{ParentUID: "nobody", ParentRef: "demo#x", Paused: true})); got.Err == nil || got.Err.Error() != model.TeamNoneReason {
		t.Errorf("a missing team: %+v", got)
	}
}

func TestAFailingChildStartIsNotRetriedBeforeItsClaimExpires(t *testing.T) {
	h := newTeamHost(t, 1)
	h.startTeam(2)
	ran := h.runCount()
	h.gateway.runTern = func(_ context.Context, args []string) ([]byte, error) {
		if args[0] == "ls" {
			return []byte(lsWithoutAgents), nil
		}
		h.mu.Lock()
		h.runs = append(h.runs, args)
		h.mu.Unlock()
		return []byte("tern: refused\n"), os.ErrInvalid
	}
	first := h.drive(h.gateway)
	if len(first.Failures) != 1 || len(first.Started) != 0 || h.runCount() != ran+1 {
		t.Fatalf("first drive: %+v after %d runs", first, h.runCount())
	}
	if again := h.drive(h.gateway); len(again.Failures) != 0 || h.runCount() != ran+1 {
		t.Fatalf("a failed start was retried: %+v after %d runs", again, h.runCount())
	}
	if team := h.team(); len(team.Children) != 1 || team.Children[0].Block != "" {
		t.Fatalf("team %+v, want the claim kept", team)
	}
}

func TestAnExpiredClaimWhoseAgentRunsTakesItsBlockAndIsNotStartedAgain(t *testing.T) {
	h := newTeamHost(t, 1)
	h.startTeam(2)
	ran := h.runCount()
	claimed := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	if _, err := UpdateTeams(h.teamsFile(), func(l []model.Team) []model.Team {
		l[0].Children = []model.TeamChild{{UID: h.children[0], Ref: "demo#c1", StartedAt: claimed}}
		return l
	}); err != nil {
		t.Fatal(err)
	}
	seedAgent(t, h.dirs, model.AgentEntry{IssueUID: h.children[0], IssueRef: "demo#c1", Block: "900", StartedAt: claimed.Add(time.Minute), Coordinator: h.team().CoordinatorAddress})
	got := h.drive(h.gateway)
	team := h.team()
	if len(got.Started) != 0 || h.runCount() != ran || len(team.Children) != 1 || team.Children[0].Block != "900" {
		t.Fatalf("started %+v after %d runs, team %+v", got.Started, h.runCount(), team)
	}
}

func TestAPaneOfAnotherActorDoesNotDriveATeam(t *testing.T) {
	h := newTeamHost(t, 1)
	h.startTeam(1)
	if team := h.team(); team.Actor != "dana" {
		t.Fatalf("team %+v, want the starting Pane's actor", team)
	}
	other := as[model.TeamsDriven](t, run(t, h.gateway, model.DriveTeams{Workspace: h.workspace, PaneBlock: "7", Actor: "erin", BoundProjectID: h.project.ID}))
	if len(other.Started) != 0 || h.runCount() != 1 {
		t.Fatalf("another actor started %+v", other.Started)
	}
	if own := h.drive(h.gateway); len(own.Started) != 1 {
		t.Fatalf("the team's actor started %+v", own.Started)
	}
	// A record written before the actor was kept is driven by any Pane.
	if _, err := UpdateTeams(h.teamsFile(), func(l []model.Team) []model.Team { l[0].Actor = ""; return l }); err != nil {
		t.Fatal(err)
	}
	h.close(h.children[0])
	if legacy := as[model.TeamsDriven](t, run(t, h.gateway, model.DriveTeams{Workspace: h.workspace, PaneBlock: "7", Actor: "erin", BoundProjectID: h.project.ID})); legacy.Err != nil {
		t.Fatal(legacy.Err)
	}
}

func TestAChildWithAnAgentOnAnotherHostIsNotStarted(t *testing.T) {
	h := newTeamHost(t, 2)
	h.startTeam(2)
	hostID, err := ReadHostID(h.dirs.State)
	if err != nil {
		t.Fatal(err)
	}
	record := model.AgentRecord{Address: "dana/omp-9", Host: "desk", HostID: "feed0001", Block: "9", Name: "omp", Actor: "dana", MessageFrom: []string{}}
	own := model.AgentRecord{Address: "dana/omp-3", Host: "laptop", HostID: hostID, Block: "3", Name: "omp", Actor: "dana", MessageFrom: []string{}}
	for uid, r := range map[string]model.AgentRecord{h.children[0]: record, h.children[1]: own} {
		_, err = h.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
			PathParams: &g.PatchIssueMetadataPath{ProjectID: h.project.ID, Ref: uid},
			Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{model.AgentRecordKey(r.HostID, r.Block): r}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got := h.drive(h.gateway)
	if len(got.Started) != 1 || got.Started[0].IssueUID != h.children[1] {
		t.Fatalf("started %+v, want only the child whose agent is not on another host (a record of this host id does not hold a child)", got.Started)
	}
	kids := as[model.TeamKidsLoaded](t, run(t, h.gateway, model.LoadTeamKids{Workspace: h.workspace, ProjectID: h.project.ID, ParentUID: h.parent}))
	if kids.Err != nil {
		t.Fatal(kids.Err)
	}
	rows := model.TeamRows(h.team(), nil, nil, kids.Kids, time.Now())
	found := false
	for _, r := range rows {
		if r.UID == h.children[0] {
			found = r.State == model.TeamChildOnHost("desk")
		}
	}
	if !found {
		t.Fatalf("rows %+v, want the first child as having an agent on desk", rows)
	}
}

func TestAnExpiredClaimDoesNotTakeAnAgentAPersonStartedByHand(t *testing.T) {
	h := newTeamHost(t, 2)
	h.startTeam(2)
	claimed := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	if _, err := UpdateTeams(h.teamsFile(), func(l []model.Team) []model.Team {
		l[0].Children = []model.TeamChild{{UID: h.children[0], Ref: "demo#c1", StartedAt: claimed}}
		return l
	}); err != nil {
		t.Fatal(err)
	}
	seedAgent(t, h.dirs, model.AgentEntry{IssueUID: h.children[0], IssueRef: "demo#c1", Block: "901", StartedAt: claimed.Add(time.Minute)})
	h.drive(h.gateway)
	for _, c := range h.team().Children {
		if c.Block == "901" {
			t.Fatalf("team %+v adopted an agent no team started", h.team())
		}
	}
}

func TestAPaneOfAnotherActorRetiresATeamWhoseCoordinatorExited(t *testing.T) {
	h := newTeamHost(t, 1)
	coordinator := h.startTeam(1).Entry
	got := as[model.TeamsDriven](t, run(t, h.gateway, model.DriveTeams{Workspace: h.workspace, PaneBlock: "7", Actor: "erin", BoundProjectID: h.project.ID, Exited: []string{coordinator.Block}}))
	if got.Err != nil || len(got.Started) != 0 {
		t.Fatalf("driven %+v", got)
	}
	if teams, err := ReadTeams(h.teamsFile()); err != nil || len(teams) != 0 {
		t.Fatalf("teams.json %+v, %v: a Pane of another actor must retire the team", teams, err)
	}
}

func TestATeamWithAChildHeldByAnotherHostDoesNotEnd(t *testing.T) {
	h := newTeamHost(t, 1)
	h.startTeam(2)
	record := model.AgentRecord{Address: "dana/omp-9", Host: "desk", HostID: "feed0001", Block: "9", Name: "omp", Actor: "dana", MessageFrom: []string{}}
	if _, err := h.kata.target().Client.PatchIssueMetadataWithResponse(context.Background(), &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: h.project.ID, Ref: h.children[0]},
		Body:       &g.PatchIssueMetadataBody{Actor: new("tester"), Patch: map[string]any{model.AgentRecordKey(record.HostID, record.Block): record}},
	}); err != nil {
		t.Fatal(err)
	}
	h.drive(h.gateway)
	if team := h.team(); team.State != model.TeamRunning {
		t.Fatalf("team %+v, want it running while a child waits on another host's agent", team)
	}
}
