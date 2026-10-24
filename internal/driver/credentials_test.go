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

func askedCapabilities(cmds []model.Cmd) (model.LoadAuthCapabilities, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadAuthCapabilities); ok {
			return load, true
		}
	}
	return model.LoadAuthCapabilities{}, false
}

func askedCredentials(cmds []model.Cmd) (model.LoadCredentials, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadCredentials); ok {
			return load, true
		}
	}
	return model.LoadCredentials{}, false
}

// credentialsOpened presses C on s and returns the output.
func credentialsOpened(t *testing.T, s driver.State) driver.Output {
	t.Helper()
	out := step(t, s, key("shift+c"))
	if _, ok := askedCapabilities(out.Cmds); !ok || out.State.Model.Page != model.PageCredentials {
		t.Fatalf("C gave page %v and %#v, want the Credentials Page and a capability check", out.State.Model.Page, out.Cmds)
	}
	return out
}

// credentialsAudited is the Credentials Page after the check answered that the account may read the token audit.
func credentialsAudited(t *testing.T) (driver.State, model.LoadCredentials) {
	t.Helper()
	out := credentialsOpened(t, projectPane(t, 160, boardCard("a")))
	check, _ := askedCapabilities(out.Cmds)
	answered := step(t, out.State, model.AuthCapabilitiesLoaded{Request: check.Request, TokenAuditRead: true})
	load, ok := askedCredentials(answered.Cmds)
	if !ok || load.After != 0 {
		t.Fatalf("after the check: %#v, want an immediate token read", answered.Cmds)
	}
	return answered.State, load
}

func credentialToken(id int64, name, state string, created time.Time) model.Credential {
	return model.Credential{ID: id, Name: name, State: state, Actor: "alice", CreatedAt: created}
}

func TestCredentialsPageOpensFromEveryPageAndTheMenu(t *testing.T) {
	flows := workflowsPane(t, 160, nightlyFlow())
	for name, s := range map[string]driver.State{
		"Issues":    projectPane(t, 160, boardCard("a")),
		"Board":     boardPane(t, boardCard("a")),
		"Jobs":      jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false))),
		"Runs":      runsPane(t, 160, finishedRun("R1")),
		"Workflows": flows,
	} {
		out := credentialsOpened(t, s)
		if check, _ := askedCapabilities(out.Cmds); check.Workspace != "/repo" {
			t.Errorf("%s: the check asked for workspace %q", name, check.Workspace)
		}
		if !pageBarDrawn(t, out.State.Model, "C") || out.State.Model.CurrentMode() != model.ModeCredentials {
			t.Errorf("%s: no Page bar entry for C, or mode %q", name, out.State.Model.CurrentMode())
		}
	}

	menu := step(t, projectPane(t, 160, boardCard("a")), openMenu).State
	out := step(t, menu, model.PickerSelected{Key: "page.credentials"})
	if _, ok := askedCapabilities(out.Cmds); !ok || out.State.Model.Page != model.PageCredentials || out.State.Model.Picker.Open {
		t.Fatalf("the Menu's Credentials entry gave page %v, Menu open %v, %#v", out.State.Model.Page, out.State.Model.Picker.Open, out.Cmds)
	}
}

func TestMenuListsTheLetteredPageAndFiltersIt(t *testing.T) {
	menu := step(t, projectPane(t, 160, boardCard("a")), openMenu).State
	var titles []string
	for _, entry := range menu.Model.PickerEntries() {
		titles = append(titles, entry.Title)
	}
	if !strings.Contains(strings.Join(titles, ","), "Credentials") {
		t.Fatalf("Menu entries %v, want Credentials among them", titles)
	}
	typed := feed(t, menu, key("c"), key("r"), key("e"), key("d")).State
	entries := typed.Model.PickerEntries()
	if len(entries) != 1 || entries[0].Title != "Credentials" {
		t.Fatalf("filtered Menu %+v, want only Credentials", entries)
	}
}

func TestPageBarDrawsAnLetteredPageOnlyWhileItIsShown(t *testing.T) {
	s := projectPane(t, 160, boardCard("a"))
	if pageBarDrawn(t, s.Model, "C") || len(s.Model.PageBar()) != 5 {
		t.Fatalf("the Page bar on the Issues Page has %d entries and draws C: %v", len(s.Model.PageBar()), pageBarDrawn(t, s.Model, "C"))
	}
	on := credentialsOpened(t, s).State
	if len(on.Model.PageBar()) != 6 || !pageBarDrawn(t, on.Model, "C") {
		t.Fatalf("the Credentials Page's Page bar has %d entries, draws C %v", len(on.Model.PageBar()), pageBarDrawn(t, on.Model, "C"))
	}
	back := step(t, on, key("escape")).State
	if pageBarDrawn(t, back.Model, "C") {
		t.Error("the Page bar still draws C after leaving the Credentials Page")
	}
}

func TestEscAndBackspaceReturnToThePageShownBefore(t *testing.T) {
	for _, leave := range []string{"escape", "backspace"} {
		for name, start := range map[string]struct {
			s    driver.State
			page model.Page
		}{
			"Issues": {projectPane(t, 160, boardCard("a")), model.PageIssues},
			"Board":  {boardPane(t, boardCard("a")), model.PageBoard},
			"Runs":   {runsPane(t, 160, finishedRun("R1")), model.PageRuns},
			"Jobs":   {jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false))), model.PageJobs},
		} {
			opened := credentialsOpened(t, start.s).State
			back := step(t, opened, key(leave)).State
			if back.Model.Page != start.page {
				t.Errorf("%s then %s: page %v, want %v", name, leave, back.Model.Page, start.page)
			}
		}
	}
	// A second Lettered Page would not lose the way back: the Menu's entry from the Credentials Page keeps it.
	board := boardPane(t, boardCard("a"))
	opened := credentialsOpened(t, board).State
	again := step(t, opened, key("shift+c")).State
	if back := step(t, again, key("backspace")).State; back.Model.Page != model.PageBoard {
		t.Errorf("C on the Credentials Page, then backspace: page %v, want the Board", back.Model.Page)
	}
}

func TestANumberedPageKeyLeavesTheCredentialsPage(t *testing.T) {
	opened := credentialsOpened(t, projectPane(t, 160, boardCard("a"))).State
	if got := step(t, opened, key("2")).State.Model.Page; got != model.PageBoard {
		t.Errorf("2 gave page %v, want the Board", got)
	}
}

func TestCredentialsPageStatesFromLoadingToTheList(t *testing.T) {
	out := credentialsOpened(t, projectPane(t, 160, boardCard("a")))
	wantText(t, "while the check is out", wireTextOf(t, out.State), "loading credential inventory...", "0 credentials")
	check, _ := askedCapabilities(out.Cmds)

	unavailable := step(t, out.State, model.AuthCapabilitiesLoaded{Request: check.Request}).State
	wantText(t, "without token audit read", wireTextOf(t, unavailable), "credential audit unavailable for this connection", "the active principal does not have token audit read capability")

	failedCheck := step(t, out.State, model.AuthCapabilitiesLoaded{Request: check.Request, Err: errors.New("boom")}).State
	wantText(t, "a failed check", wireTextOf(t, failedCheck), "failed to determine credential audit access: boom")

	audited, load := credentialsAuditedFrom(t, out)
	wantText(t, "while the list is out", wireTextOf(t, audited), "loading credential inventory...")

	empty := step(t, audited, model.CredentialsLoaded{Request: load.Request, ObservedAt: now}).State
	wantText(t, "an empty list", wireTextOf(t, empty), "no credentials", "0 credentials · observed at 2026-10-08 12:00:00Z")

	failed := step(t, audited, model.CredentialsLoaded{Request: load.Request, Err: errors.New("daemon down")}).State
	wantText(t, "a failed read", wireTextOf(t, failed), "failed to load credentials: daemon down")
}

func credentialsAuditedFrom(t *testing.T, out driver.Output) (driver.State, model.LoadCredentials) {
	t.Helper()
	check, _ := askedCapabilities(out.Cmds)
	answered := step(t, out.State, model.AuthCapabilitiesLoaded{Request: check.Request, TokenAuditRead: true})
	load, ok := askedCredentials(answered.Cmds)
	if !ok {
		t.Fatalf("no token read asked for: %#v", answered.Cmds)
	}
	return answered.State, load
}

func TestCredentialsPageListsEachTokenWithKataTuisFieldsNewestFirst(t *testing.T) {
	audited, load := credentialsAudited(t)
	created := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	revoked := created.Add(48 * time.Hour)
	scoped := model.Credential{ID: 9, State: "revoked", Actor: "bot", CreatedAt: created.Add(time.Hour), ExpiresAt: created.AddDate(0, 1, 0),
		LastUsedAt: created.Add(2 * time.Hour), RevokedAt: revoked, ScopeKind: "issue_subtree", ProjectUID: "P-UID", RootIssueUID: "ROOT-UID"}
	s := step(t, audited, model.CredentialsLoaded{Request: load.Request, ObservedAt: now, Tokens: []model.Credential{
		credentialToken(3, "laptop", "live", created), scoped,
	}}).State
	text := wireTextOf(t, s)
	wantText(t, "token cards", text,
		"2 credentials · observed at 2026-10-08 12:00:00Z",
		"state live  ID 3  name laptop  actor alice", "scope global  project —  root —",
		"created 2026-09-01 08:30:00Z  expires —", "last observed use — ", "revoked —",
		"state revoked  ID 9  name —  actor bot", "scope issue_subtree  project P-UID  root ROOT-UID",
		"created 2026-09-01 09:30:00Z  expires 2026-10-01 08:30:00Z", "last observed use 2026-09-01 10:30:00Z  revoked 2026-09-03 08:30:00Z")
	if strings.Index(text, "ID 9") > strings.Index(text, "ID 3") {
		t.Error("the newer token is not listed first")
	}
	if sel, total := s.Model.Position(); sel != 1 || total != 2 {
		t.Errorf("position %d of %d", sel, total)
	}
	if moved := step(t, s, key("j")).State; moved.Model.Credentials.Cursor != 1 || moved.Model.SelectedCredential().ID != 3 {
		t.Errorf("j left the cursor on %+v", moved.Model.SelectedCredential())
	}
}

func TestCredentialsPageRefreshesEveryThirtySecondsWhileShown(t *testing.T) {
	audited, load := credentialsAudited(t)
	first := step(t, audited, model.CredentialsLoaded{Request: load.Request, ObservedAt: now, Tokens: []model.Credential{credentialToken(1, "a", "live", now)}})
	next, ok := askedCredentials(first.Cmds)
	if !ok || next.After != 30*time.Second || next.Request == load.Request || next.Workspace != "/repo" {
		t.Fatalf("after a read: %#v, want one more read after 30 seconds", first.Cmds)
	}
	if wantText(t, "while waiting", wireTextOf(t, first.State), "1 credentials"); strings.Contains(wireTextOf(t, first.State), "loading") {
		t.Error("the Page shows loading while it only waits for the next refresh")
	}

	later := now.Add(30 * time.Second)
	second := step(t, first.State, model.CredentialsLoaded{Request: next.Request, ObservedAt: later, Tokens: []model.Credential{credentialToken(1, "a", "live", now), credentialToken(2, "b", "live", later)}})
	wantText(t, "after the refresh", wireTextOf(t, second.State), "2 credentials · observed at 2026-10-08 12:00:30Z")
	if again, ok := askedCredentials(second.Cmds); !ok || again.After != 30*time.Second || again.Request == next.Request {
		t.Fatalf("after a refresh: %#v, want the next one", second.Cmds)
	}
}

func TestCredentialsPageKeepsItsListWhenARefreshFails(t *testing.T) {
	audited, load := credentialsAudited(t)
	held := step(t, audited, model.CredentialsLoaded{Request: load.Request, ObservedAt: now, Tokens: []model.Credential{credentialToken(1, "a", "live", now)}})
	next, _ := askedCredentials(held.Cmds)
	failed := step(t, held.State, model.CredentialsLoaded{Request: next.Request, Err: errors.New("timeout")})
	wantText(t, "failed refresh", wireTextOf(t, failed.State), "state live  ID 1", "refresh failed: timeout")
	if _, ok := askedCredentials(failed.Cmds); !ok {
		t.Error("a failed refresh stops the refreshing")
	}
}

func TestLeavingTheCredentialsPageDropsAnswersStillOut(t *testing.T) {
	audited, load := credentialsAudited(t)
	left := step(t, audited, key("escape")).State
	late := step(t, left, model.CredentialsLoaded{Request: load.Request, ObservedAt: now, Tokens: []model.Credential{credentialToken(1, "a", "live", now)}})
	if _, ok := askedCredentials(late.Cmds); ok || late.State.Model.Page != model.PageIssues {
		t.Errorf("an answer after leaving asked %#v on page %v", late.Cmds, late.State.Model.Page)
	}
	reopened := credentialsOpened(t, late.State).State
	wantText(t, "reopened", wireTextOf(t, reopened), "loading credential inventory...")
}

func TestRRefreshesTheListAtOnceAndRetriesAFailedCheck(t *testing.T) {
	audited, load := credentialsAudited(t)
	held := step(t, audited, model.CredentialsLoaded{Request: load.Request, ObservedAt: now, Tokens: []model.Credential{credentialToken(1, "a", "live", now)}}).State
	out := step(t, held, key("r"))
	read, ok := askedCredentials(out.Cmds)
	if !ok || read.After != 0 {
		t.Fatalf("r asked %#v, want a read at once", out.Cmds)
	}
	// The refresh the key replaced is dropped when it answers.
	stale := step(t, out.State, model.CredentialsLoaded{Request: load.Request + 1, ObservedAt: now.Add(time.Hour)})
	wantText(t, "after a stale answer", wireTextOf(t, stale.State), "observed at 2026-10-08 12:00:00Z")

	opened := credentialsOpened(t, projectPane(t, 160, boardCard("a")))
	check, _ := askedCapabilities(opened.Cmds)
	failed := step(t, opened.State, model.AuthCapabilitiesLoaded{Request: check.Request, Err: errors.New("boom")}).State
	retry := step(t, failed, key("r"))
	again, ok := askedCapabilities(retry.Cmds)
	if !ok || again.Request == check.Request {
		t.Fatalf("r after a failed check asked %#v, want the check again", retry.Cmds)
	}
	wantText(t, "while retrying", wireTextOf(t, retry.State), "loading credential inventory...")

	unavailable := step(t, opened.State, model.AuthCapabilitiesLoaded{Request: check.Request}).State
	if got := step(t, unavailable, key("r")); len(got.Cmds) != 0 {
		t.Errorf("r on a connection without audit rights asked %#v", got.Cmds)
	}
}

func TestCredentialsPageKeysKeepThePageKeysAndTheMenu(t *testing.T) {
	audited, _ := credentialsAudited(t)
	if got := step(t, audited, key("shift+p")).State.Model; !got.Picker.Open && !got.SidebarProjects {
		t.Error("P opens no Menu Projects level")
	}
	if got := step(t, audited, key("shift+i")).State.Model; got.Page != model.PageIssues || got.Collection != model.InboxCollection() {
		t.Errorf("I gave page %v, Collection %+v", got.Page, got.Collection)
	}
	if got := step(t, audited, key("?")).State.Model; !got.HelpOpen {
		t.Error("? opens no help")
	}
	if got := step(t, audited, click(pageBarEntry(t, audited.Model, "C"), view.KeyClick("shift+c"))); got.State.Model.Page != model.PageCredentials {
		t.Errorf("clicking the shown entry left page %v", got.State.Model.Page)
	}
}
