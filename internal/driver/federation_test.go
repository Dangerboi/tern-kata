package driver_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func askedFederation(cmds []model.Cmd) (model.LoadFederation, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadFederation); ok {
			return load, true
		}
	}
	return model.LoadFederation{}, false
}

// federationOpened presses F on s and returns the output.
func federationOpened(t *testing.T, s driver.State) driver.Output {
	t.Helper()
	out := step(t, s, key("shift+f"))
	if _, ok := askedFederation(out.Cmds); !ok || out.State.Model.Page != model.PageFederation {
		t.Fatalf("F gave page %v and %#v, want the Federation Page and a status read", out.State.Model.Page, out.Cmds)
	}
	return out
}

// federationAnswer is a read's answer for a daemon called work at its endpoint, holding spokes.
func federationAnswer(load model.LoadFederation, spokes ...model.FederationSpoke) model.FederationLoaded {
	return model.FederationLoaded{Request: load.Request, Daemon: "work", Endpoint: "https://spoke.example:7474", InstanceUID: "INSTANCE-1",
		Auth: "token actor alice", Spokes: spokes}
}

func healthySpoke(name string) model.FederationSpoke {
	return model.FederationSpoke{ProjectName: name, HubURL: "https://hub.example:7474", HubProjectID: 12, HubProjectUID: "HUB-UID", BoundActor: "alice",
		Capabilities: "pull,push", PushEnabled: true, CredentialStatus: "valid", PendingPushCount: 3, LastSuccessfulSyncAt: now.Add(-5 * time.Minute),
		PullCursorEventID: 40, PushCursorEventID: 38, PendingPushHighWaterEventID: 41, PendingClaimCount: 1, LiveClaimCount: 2,
		LastPullSuccessAt: now.Add(-6 * time.Minute), LastPushSuccessAt: now.Add(-5 * time.Minute)}
}

func troubledSpoke(name string) model.FederationSpoke {
	return model.FederationSpoke{ProjectName: name, HubURL: "https://hub.example:7474", HubProjectID: 7, BoundActor: "bob", AllowInsecure: true,
		ActiveQuarantineCount: 1, ActiveQuarantines: []model.FederationQuarantine{{ID: 4, Direction: "pull", FirstEventID: 5, LastEventID: 9,
			CreatedAt: time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC), Error: "bad signature"}},
		ResetBlocker: "pending push", UnresolvedViolationCount: 2, RecentViolationCount: 3, HasLastError: true, LastError: "hub unreachable",
		LastErrorAt: time.Date(2026, 10, 8, 11, 0, 0, 0, time.UTC)}
}

// federationListed is the Federation Page after a read answered with spokes.
func federationListed(t *testing.T, spokes ...model.FederationSpoke) driver.State {
	t.Helper()
	out := federationOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedFederation(out.Cmds)
	return step(t, out.State, federationAnswer(load, spokes...)).State
}

func TestFederationPageOpensFromEveryPageAndTheMenu(t *testing.T) {
	flows := workflowsPane(t, 160, nightlyFlow())
	for name, s := range map[string]driver.State{
		"Issues":    projectPane(t, 160, boardCard("a")),
		"Board":     boardPane(t, boardCard("a")),
		"Jobs":      jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false))),
		"Runs":      runsPane(t, 160, finishedRun("R1")),
		"Workflows": flows,
	} {
		out := federationOpened(t, s)
		if load, _ := askedFederation(out.Cmds); load.Workspace != "/repo" {
			t.Errorf("%s: the read asked for workspace %q", name, load.Workspace)
		}
		if !pageBarDrawn(t, out.State.Model, "F") || out.State.Model.CurrentMode() != model.ModeFederation {
			t.Errorf("%s: no Page bar entry for F, or mode %q", name, out.State.Model.CurrentMode())
		}
	}

	menu := step(t, projectPane(t, 160, boardCard("a")), openMenu).State
	out := step(t, menu, model.PickerSelected{Key: "page.federation"})
	if _, ok := askedFederation(out.Cmds); !ok || out.State.Model.Page != model.PageFederation || out.State.Model.Picker.Open {
		t.Fatalf("the Menu's Federation entry gave page %v, Menu open %v, %#v", out.State.Model.Page, out.State.Model.Picker.Open, out.Cmds)
	}
	typed := feed(t, menu, key("f"), key("e"), key("d")).State.Model.PickerEntries()
	if len(typed) != 1 || typed[0].Title != "Federation" {
		t.Fatalf("filtered Menu %+v, want only Federation", typed)
	}
}

func TestFederationPageStatesFromLoadingToTheList(t *testing.T) {
	out := federationOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedFederation(out.Cmds)
	wantText(t, "while the read is out", wireTextOf(t, out.State), "loading federation status...", "0 spoke federations")

	empty := step(t, out.State, federationAnswer(load)).State
	wantText(t, "an unfederated daemon", wireTextOf(t, empty), "no spoke federation enrollments", "0 spoke federations",
		"Federation for active daemon: work https://spoke.example:7474 instance INSTANCE-1 auth token actor alice")

	failed := step(t, out.State, model.FederationLoaded{Request: load.Request, Err: errors.New("daemon down\nretry later")}).State
	wantText(t, "a failed read", wireTextOf(t, failed), "failed to load federation: daemon down retry later")
	if strings.Contains(wireTextOf(t, failed), "no spoke federation enrollments") {
		t.Error("a failed read says there are no enrollments")
	}

	bare := step(t, out.State, model.FederationLoaded{Request: load.Request, Daemon: "local", Endpoint: "local", Auth: "no token"}).State
	wantText(t, "a daemon without an instance uid", wireTextOf(t, bare), "Federation for active daemon: local local instance - auth no token")
}

func TestFederationPageListsEachSpokeAndNamesTheSelectedOnesHub(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"), troubledSpoke("zeta"))
	wantText(t, "the list", wireTextOf(t, s), "2 spoke federations",
		"acme  Hub hub.example:7474  Actor alice", "Push push  Pending 3  Sync 5m ago  Badges -",
		"zeta  Hub hub.example:7474  Actor bob", "Push off  Pending 0  Sync error  Badges insecure,quarantine,reset,violations",
		"hub https://hub.example:7474/project 12 · actor alice · credential valid")
	if sel, total := s.Model.Position(); sel != 1 || total != 2 {
		t.Errorf("position %d of %d", sel, total)
	}
	moved := step(t, s, key("j")).State
	if moved.Model.Federation.Cursor != 1 || moved.Model.SelectedSpoke().ProjectName != "zeta" {
		t.Errorf("j left the cursor on %+v", moved.Model.SelectedSpoke())
	}
	wantText(t, "the footer of the second", wireTextOf(t, moved), "hub https://hub.example:7474/project 7 · actor bob · credential -")
}

func TestFederationSyncColumnSaysNeverErrorOrHowLongAgo(t *testing.T) {
	never := model.FederationSpoke{ProjectName: "n", HubURL: "https://h.example"}
	failing := model.FederationSpoke{ProjectName: "f", HubURL: "https://h.example", HasLastError: true, LastError: "x"}
	synced := healthySpoke("s")
	for _, c := range []struct {
		spoke model.FederationSpoke
		want  string
	}{{never, "never"}, {failing, "error"}, {synced, "5m ago"}} {
		if got := c.spoke.Sync(now); got != c.want {
			t.Errorf("%s: sync %q, want %q", c.spoke.ProjectName, got, c.want)
		}
	}
	if got := (model.FederationSpoke{HubURL: "not a url"}).HubHost(); got != "not a url" {
		t.Errorf("hub of a URL without a host: %q", got)
	}
	if got := (model.FederationSpoke{}).HubHost(); got != "-" {
		t.Errorf("hub of none: %q", got)
	}
}

func TestEnterShowsASpokesDetailAndEscReturnsToTheList(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"), troubledSpoke("zeta"))
	detail := step(t, s, key("enter")).State
	if detail.Model.Federation.Screen != model.FederationDetail {
		t.Fatal("Enter shows no detail")
	}
	text := wireTextOf(t, detail)
	wantText(t, "the first spoke's detail", text, "acme", "hub URL: https://hub.example:7474", "hub project ID: 12", "hub project UID: HUB-UID", "actor: alice",
		"capabilities: pull,push", "push enabled: true", "credential: valid", "allow_insecure: false",
		"pull cursor: 40", "push cursor: 38", "pending push: 3", "pending push high water: 41", "pending claims: 1", "live claims: 2",
		"quarantine count: 0", "reset blocker: -", "claim violations: 0 unresolved, 0 recent", "last pull success: 2026-10-08 11:54:00Z",
		"last push success: 2026-10-08 11:55:00Z", "last sync success: 2026-10-08 11:55:00Z", "last error: -",
		"Federation for active daemon: work")
	if strings.Contains(text, "spoke federations") {
		t.Error("the detail still shows the list's summary")
	}
	if moved := step(t, detail, key("j")).State; moved.Model.Federation.Cursor != 0 {
		t.Error("j moved the cursor inside a detail")
	}
	if got := step(t, detail, key("enter")).State; got.Model.Federation.Screen != model.FederationDetail {
		t.Error("Enter inside a detail left it")
	}

	list := step(t, detail, key("escape")).State
	if list.Model.Federation.Screen != model.FederationList || list.Model.Page != model.PageFederation {
		t.Fatalf("esc from a detail gave screen %v on page %v, want the list", list.Model.Federation.Screen, list.Model.Page)
	}
	if back := step(t, list, key("escape")).State; back.Model.Page != model.PageIssues {
		t.Errorf("esc from the list gave page %v, want the Issues Page", back.Model.Page)
	}

	second := step(t, step(t, s, key("j")).State, key("enter")).State
	wantText(t, "the second spoke's detail", wireTextOf(t, second), "zeta", "allow_insecure: true", "quarantine count: 1",
		"quarantine #4: pull events 5-9 at 2026-10-07 08:00:00Z: bad signature", "reset blocker: pending push",
		"claim violations: 2 unresolved, 3 recent", "last error: 2026-10-08 11:00:00Z hub unreachable", "last pull success: -", "capabilities: ")
	if back := step(t, step(t, second, key("backspace")).State, key("backspace")).State; back.Model.Page != model.PageIssues {
		t.Errorf("two backspaces gave page %v, want the Issues Page", back.Model.Page)
	}
}

func TestEnterOnAnEmptyListOpensNoDetail(t *testing.T) {
	s := federationListed(t)
	if got := step(t, s, key("enter")).State; got.Model.Federation.Screen != model.FederationList {
		t.Error("Enter on an empty list opened a detail")
	}
}

func TestEscAndBackspaceReturnFromTheFederationPageToThePageShownBefore(t *testing.T) {
	for _, leave := range []string{"escape", "backspace"} {
		for name, start := range map[string]struct {
			s    driver.State
			page model.Page
		}{
			"Issues": {projectPane(t, 160, boardCard("a")), model.PageIssues},
			"Board":  {boardPane(t, boardCard("a")), model.PageBoard},
			"Runs":   {runsPane(t, 160, finishedRun("R1")), model.PageRuns},
		} {
			back := step(t, federationOpened(t, start.s).State, key(leave)).State
			if back.Model.Page != start.page || pageBarDrawn(t, back.Model, "F") {
				t.Errorf("%s then %s: page %v, want %v with no F entry", name, leave, back.Model.Page, start.page)
			}
		}
	}
	// From the Credentials Page, F replaces it and the way back stays the Page before it.
	board := credentialsOpened(t, boardPane(t, boardCard("a"))).State
	onFederation := step(t, board, key("shift+f")).State
	if onFederation.Model.Page != model.PageFederation {
		t.Fatalf("F on the Credentials Page gave page %v", onFederation.Model.Page)
	}
	if back := step(t, onFederation, key("escape")).State; back.Model.Page != model.PageBoard {
		t.Errorf("esc after Credentials then Federation: page %v, want the Board", back.Model.Page)
	}
}

func TestRRefreshesTheStatusFromTheListAndFromADetail(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"))
	out := step(t, s, key("r"))
	read, ok := askedFederation(out.Cmds)
	if !ok || read.Workspace != "/repo" {
		t.Fatalf("r asked %#v, want a status read", out.Cmds)
	}
	wantText(t, "while refreshing", wireTextOf(t, out.State), "loading federation status...")
	refreshed := step(t, out.State, federationAnswer(read, healthySpoke("acme"), healthySpoke("beta"))).State
	wantText(t, "after the refresh", wireTextOf(t, refreshed), "2 spoke federations", "beta  Hub")

	detail := step(t, s, key("enter")).State
	again := step(t, detail, key("r"))
	if _, ok := askedFederation(again.Cmds); !ok || again.State.Model.Federation.Screen != model.FederationDetail {
		t.Fatalf("r in a detail asked %#v on screen %v, want a read that keeps the detail", again.Cmds, again.State.Model.Federation.Screen)
	}
}

func TestAStaleFederationAnswerIsDropped(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"))
	first := step(t, s, key("r"))
	older, _ := askedFederation(first.Cmds)
	second := step(t, first.State, key("r"))
	newer, _ := askedFederation(second.Cmds)
	if newer.Request == older.Request {
		t.Fatal("two refreshes share a request number")
	}
	late := step(t, second.State, federationAnswer(older, healthySpoke("stale")))
	wantText(t, "after the older answer", wireTextOf(t, late.State), "loading federation status...")
	if strings.Contains(wireTextOf(t, late.State), "stale") {
		t.Error("an answer to an older read was drawn")
	}
	current := step(t, late.State, federationAnswer(newer, healthySpoke("fresh")))
	wantText(t, "after the current answer", wireTextOf(t, current.State), "fresh  Hub")
}

func TestAFailedRefreshShowsKatasErrorInPlaceOfTheListUntilTheNextRead(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"))
	out := step(t, s, key("r"))
	read, _ := askedFederation(out.Cmds)
	failed := step(t, out.State, model.FederationLoaded{Request: read.Request, Err: errors.New("timeout")}).State
	if text := wireTextOf(t, failed); !strings.Contains(text, "failed to load federation: timeout") || strings.Contains(text, "acme") {
		t.Errorf("a failed refresh drew %s", text)
	}
	retry := step(t, failed, key("r"))
	again, _ := askedFederation(retry.Cmds)
	wantText(t, "after the answer", wireTextOf(t, step(t, retry.State, federationAnswer(again, healthySpoke("acme"))).State), "acme  Hub")
}

func TestFederationCursorStaysOnARowThatRemainsAfterARefresh(t *testing.T) {
	s := federationListed(t, healthySpoke("a"), healthySpoke("b"), healthySpoke("c"))
	s = step(t, s, key("shift+g")).State
	if s.Model.Federation.Cursor != 2 {
		t.Fatalf("G left the cursor on %d", s.Model.Federation.Cursor)
	}
	out := step(t, s, key("r"))
	read, _ := askedFederation(out.Cmds)
	shrunk := step(t, out.State, federationAnswer(read, healthySpoke("a"))).State
	if shrunk.Model.Federation.Cursor != 0 || shrunk.Model.SelectedSpoke().ProjectName != "a" {
		t.Errorf("after the list shrank the cursor is on %d", shrunk.Model.Federation.Cursor)
	}
}

func TestLeavingTheFederationPageDropsAnswersStillOutAndReadsAgainOnReturn(t *testing.T) {
	out := federationOpened(t, projectPane(t, 160, boardCard("a")))
	load, _ := askedFederation(out.Cmds)
	left := step(t, out.State, key("escape")).State
	late := step(t, left, federationAnswer(load, healthySpoke("acme")))
	if _, ok := askedFederation(late.Cmds); ok || late.State.Model.Page != model.PageIssues {
		t.Errorf("an answer after leaving asked %#v on page %v", late.Cmds, late.State.Model.Page)
	}
	reopened := federationOpened(t, late.State).State
	wantText(t, "reopened", wireTextOf(t, reopened), "loading federation status...")
}

func TestFederationPageKeepsThePageKeysAndTheMenu(t *testing.T) {
	s := federationListed(t, healthySpoke("acme"))
	if got := step(t, s, key("shift+p")).State.Model; !got.Picker.Open && !got.SidebarProjects {
		t.Error("P opens no Menu Projects level")
	}
	if got := step(t, s, key("shift+i")).State.Model; got.Page != model.PageIssues || got.Collection != model.InboxCollection() {
		t.Errorf("I gave page %v, Collection %+v", got.Page, got.Collection)
	}
	if got := step(t, s, key("?")).State.Model; !got.HelpOpen {
		t.Error("? opens no help")
	}
	if got := step(t, s, key("2")).State.Model.Page; got != model.PageBoard {
		t.Errorf("2 gave page %v, want the Board", got)
	}
	if got := step(t, s, click(pageBarEntry(t, s.Model, "F"), view.KeyClick("shift+f"))); got.State.Model.Page != model.PageFederation {
		t.Errorf("clicking the shown entry left page %v", got.State.Model.Page)
	}
}
