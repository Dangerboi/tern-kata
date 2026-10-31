package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// views runs `tern-kata views args...` as main does, as actor amy.
func (f subcommandFixture) views(t *testing.T, args ...string) (string, error) {
	t.Helper()
	plan, err := PlanLaunch(append([]string{"views", "--as", "amy"}, args...), func(string) (string, bool) { return "", false }, "test")
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	err = runSubcommand(context.Background(), plan, data.NewGateway(f.locate).WithHost(testHost), &out)
	return out.String(), err
}

func viewsFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tern-kata", "views.toml")
}

func writeViews(t *testing.T, text string) {
	t.Helper()
	path := viewsFile(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// issue creates an Issue in the fixture's Project.
func (f subcommandFixture) issue(t *testing.T, cmd model.CreateIssueCmd) {
	t.Helper()
	gateway := data.NewGateway(f.locate).WithHost(testHost)
	defer gateway.Close()
	loaded := send(t, gateway, model.ReadSnapshot{Workspace: f.repo, Query: model.CollectionQuery(model.InboxCollection(), model.Filters{})}).(model.SnapshotLoaded)
	cmd.Workspace, cmd.Actor, cmd.ProjectID = f.repo, "tester", loaded.Snapshot.Catalog[0].Project.ID
	if created := send(t, gateway, cmd).(model.IssueCreated); created.Err != nil {
		t.Fatal(created.Err)
	}
}

func TestViewsListsFixedCollectionsThenSavedViewsAndNotesAGoneProject(t *testing.T) {
	f := newSubcommandFixture(t)
	writeViews(t, `[[view]]
name = "Mine"
base = "today"
owner = "@me"
status = "open"

[[view]]
name = "Lost"
base = "project"
project_uid = "01GONEGONEGONEGONEGONEGONE"
project_name = "Vanished"
label = "bug"
`)
	out, err := f.views(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"inbox", "all-open   All Open", "logbook", "Saved views:", "Mine       Today  status=open owner=@me", "Lost       project Vanished (Project gone)  label=bug"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "logbook") > strings.Index(out, "Mine") {
		t.Errorf("saved views come before the fixed Collections:\n%s", out)
	}
}

func TestViewsListsARenamedBaseProjectByItsCurrentName(t *testing.T) {
	f := newSubcommandFixture(t)
	if out, err := f.views(t, "add", "Work", "--base", "project:demo"); err != nil {
		t.Fatalf("add: %q, %v", out, err)
	}
	text, _ := os.ReadFile(viewsFile(t))
	renamed := strings.Replace(string(text), `project_name = "demo"`, `project_name = "Old name"`, 1)
	if renamed == string(text) {
		t.Fatalf("views.toml has no project_name:\n%s", text)
	}
	writeViews(t, renamed)
	out, err := f.views(t)
	if err != nil || !strings.Contains(out, "project demo") || strings.Contains(out, "Old name") {
		t.Errorf("list: %q, %v; want the catalog's name", out, err)
	}
}

func TestViewsShowSaysWhyWhenTheBaseProjectIsGone(t *testing.T) {
	f := newSubcommandFixture(t)
	writeViews(t, "[[view]]\nname = \"Lost\"\nbase = \"project\"\nproject_uid = \"01GONEGONEGONEGONEGONEGONE\"\nproject_name = \"Vanished\"\n")
	out, err := f.views(t, "show", "lost")
	var failed subcommandError
	if !errors.As(err, &failed) || exitStatus(err) != 1 || !strings.Contains(err.Error(), "Lost") || !strings.Contains(err.Error(), "Vanished) is gone") {
		t.Fatalf("show of a view whose Project is gone: %q, %v; want exit 1 saying why", out, err)
	}
	if out != "" {
		t.Errorf("printed %q", out)
	}
}

func TestViewsListsTheFixedCollectionsAndReportsAnUnreadableFile(t *testing.T) {
	f := newSubcommandFixture(t)
	const garbage = "[[view\nname = \n"
	writeViews(t, garbage)
	out, err := f.views(t)
	var failed subcommandError
	if !errors.As(err, &failed) || !strings.Contains(err.Error(), "views.toml") {
		t.Fatalf("err = %v, want a refusal naming views.toml", err)
	}
	if !strings.Contains(out, "today      Today") {
		t.Errorf("the fixed Collections are not listed:\n%s", out)
	}
	if got, _ := os.ReadFile(viewsFile(t)); string(got) != garbage {
		t.Errorf("the unreadable file was changed to %q", got)
	}
	if _, err := f.views(t, "add", "x"); err == nil {
		t.Error("add wrote over an unreadable file")
	}
	if _, err := f.views(t, "show", "Mine"); err == nil || exitStatus(err) != 1 {
		t.Errorf("show of a name the unreadable file may hold: %v, want exit 1", err)
	}
	if out, err := f.views(t, "show", "all-open"); err != nil || !strings.Contains(out, "Screenshot") {
		t.Errorf("a fixed Collection shows without views.toml: %q, %v", out, err)
	}
}

func TestViewsShowPrintsAFixedCollectionGroupedAsThePane(t *testing.T) {
	f := newSubcommandFixture(t)
	today := time.Now().Format("2006-01-02")
	f.issue(t, model.CreateIssueCmd{Title: "Plan the week", ScheduledOn: today, Labels: []string{"plan"}})
	f.issue(t, model.CreateIssueCmd{Title: "Late report", ScheduledOn: "2020-01-02", DeadlineOn: "2020-01-05"})
	out, err := f.views(t, "show", "TODAY")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Overdue\n", "Today\n", "demo#", "open", "scheduled " + today, "Plan the week", "Late report", "scheduled 2020-01-02  deadline 2020-01-05"} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Screenshot") {
		t.Errorf("an unscheduled Issue is in Today:\n%s", out)
	}
	if strings.Index(out, "Overdue") > strings.Index(out, "Today\n") {
		t.Errorf("Overdue comes after Today:\n%s", out)
	}
	out, err = f.views(t, "show", "all-open")
	if err != nil || !strings.Contains(out, "demo\n") || !strings.Contains(out, "Screenshot") {
		t.Errorf("All Open is one group per Project: %q, %v", out, err)
	}
}

func TestViewsShowJSONHasOneObjectPerGroup(t *testing.T) {
	f := newSubcommandFixture(t)
	today := time.Now().Format("2006-01-02")
	prio := int64(1)
	f.issue(t, model.CreateIssueCmd{Title: "Plan the week", ScheduledOn: today, Labels: []string{"plan"}, Owner: "amy", Priority: &prio})
	out, err := f.views(t, "show", "today", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var groups []shownGroup
	if err := json.Unmarshal([]byte(out), &groups); err != nil || len(groups) != 1 {
		t.Fatalf("%v: %s", err, out)
	}
	g := groups[0]
	if g.Title != "Today" || len(g.Issues) != 1 {
		t.Fatalf("group %+v", g)
	}
	i := g.Issues[0]
	if !strings.HasPrefix(i.Ref, "demo#") || i.UID == "" || i.Title != "Plan the week" || i.Status != "open" || i.Priority == nil || *i.Priority != 1 ||
		i.Owner == nil || *i.Owner != "amy" || len(i.Labels) != 1 || i.ScheduledOn == nil || *i.ScheduledOn != today || i.DeadlineOn != nil || i.ClosedOn != nil {
		t.Errorf("issue %+v", i)
	}
	if !strings.Contains(out, `"deadline_on": null`) {
		t.Errorf("unset dates are not null:\n%s", out)
	}
	writeViews(t, "[[view]]\nname = \"Nobody\"\nbase = \"today\"\nowner = \"zed\"\n")
	if out, err = f.views(t, "show", "Nobody", "--json"); err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("an empty view is %q, %v; want []", out, err)
	}
}

func TestViewsShowASavedViewResolvesMeFromTheActor(t *testing.T) {
	f := newSubcommandFixture(t)
	f.issue(t, model.CreateIssueCmd{Title: "Mine to do", Owner: "amy", Labels: []string{"bug"}})
	f.issue(t, model.CreateIssueCmd{Title: "Theirs to do", Owner: "bob", Labels: []string{"bug"}})
	writeViews(t, "[[view]]\nname = \"My bugs\"\nbase = \"all-open\"\nowner = \"@me\"\nlabel = \"bug\"\n")
	out, err := f.views(t, "show", "my BUGS")
	if err != nil || !strings.Contains(out, "Mine to do") || strings.Contains(out, "Theirs to do") {
		t.Fatalf("@me as amy: %q, %v", out, err)
	}
	plan, err := PlanLaunch([]string{"views", "show", "My bugs", "--as", "bob"}, func(string) (string, bool) { return "", false }, "test")
	if err != nil {
		t.Fatal(err)
	}
	var bob bytes.Buffer
	if err := runSubcommand(context.Background(), plan, data.NewGateway(f.locate).WithHost(testHost), &bob); err != nil || !strings.Contains(bob.String(), "Theirs to do") || strings.Contains(bob.String(), "Mine to do") {
		t.Fatalf("@me as bob: %q, %v", bob.String(), err)
	}
	writeViews(t, "[[view]]\nname = \"Found\"\nbase = \"all-open\"\nsearch = \"Theirs\"\n")
	if out, err = f.views(t, "show", "Found"); err != nil || !strings.Contains(out, "Results\n") || !strings.Contains(out, "Theirs to do") || strings.Contains(out, "Mine to do") {
		t.Errorf("a search groups as Results: %q, %v", out, err)
	}
}

func TestViewsShowAnUnknownNameExitsTwoListingTheNames(t *testing.T) {
	f := newSubcommandFixture(t)
	writeViews(t, "[[view]]\nname = \"Mine\"\nbase = \"today\"\n")
	_, err := f.views(t, "show", "nope")
	if exitStatus(err) != 2 || !strings.Contains(err.Error(), `"nope"`) || !strings.Contains(err.Error(), "all-open") || !strings.Contains(err.Error(), "Mine") {
		t.Fatalf("err = %v (exit %d), want exit 2 listing the names", err, exitStatus(err))
	}
}

func TestViewsAddSavesAViewAndRemoveDeletesIt(t *testing.T) {
	f := newSubcommandFixture(t)
	out, err := f.views(t, "add", "Work bugs", "--base", "project:DEMO", "--status", "open", "--label", "bug", "--owner", "@me", "--search", "crash")
	if err != nil || strings.Count(out, "\n") != 1 || !strings.Contains(out, `Saved view "Work bugs"`) {
		t.Fatalf("add: %q, %v; want one line", out, err)
	}
	text, _ := os.ReadFile(viewsFile(t))
	for _, want := range []string{`name = "Work bugs"`, `base = "project"`, `project_name = "demo"`, `project_uid = "`, `status = "open"`, `owner = "@me"`, `search = "crash"`} {
		if !strings.Contains(string(text), want) {
			t.Errorf("views.toml lacks %q:\n%s", want, text)
		}
	}
	if out, err = f.views(t); err != nil || !strings.Contains(out, "Work bugs  project demo  status=open owner=@me label=bug search=\"crash\"") {
		t.Errorf("list: %q, %v", out, err)
	}
	if out, err = f.views(t, "add", "Plain", "--project", "demo", "--status", "all"); err != nil {
		t.Fatalf("add with --project and a default base: %q, %v", out, err)
	}
	if out, err = f.views(t, "show", "Plain"); err != nil || !strings.Contains(out, "Screenshot") {
		t.Errorf("show of a view filtered on a Project: %q, %v", out, err)
	}
	if out, err = f.views(t, "rm", "work BUGS"); err != nil || !strings.Contains(out, `Removed view "Work bugs"`) {
		t.Fatalf("rm: %q, %v", out, err)
	}
	if text, _ = os.ReadFile(viewsFile(t)); strings.Contains(string(text), "Work bugs") {
		t.Errorf("the view is still in views.toml:\n%s", text)
	}
}

func TestViewsAddRefusalsWriteNothing(t *testing.T) {
	f := newSubcommandFixture(t)
	const kept = "[[view]]\nname = \"Taken\"\nbase = \"today\"\n"
	writeViews(t, kept)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"add", "  "}, "Name is required"},
		{[]string{"add", strings.Repeat("x", 65)}, "Name is longer than 64 characters"},
		{[]string{"add", "today"}, `Name "today" is the fixed Collection Today`},
		{[]string{"add", "All Open"}, "is the fixed Collection All Open"},
		{[]string{"add", "taken"}, `a view named "Taken" exists; --replace saves over it`},
		{[]string{"add", "x", "--base", "nowhere"}, `Base "nowhere" is not a Collection`},
		{[]string{"add", "x", "--base", "project:ghost"}, `Project "ghost" is not a Project of this daemon; the Projects are demo`},
		{[]string{"add", "x", "--project", "ghost"}, `Project "ghost"`},
		{[]string{"add", "x", "--status", "done"}, `Status "done" must be open or closed, or empty for all`},
		{[]string{"add", "x", "--relationship", "friend"}, `Relationship "friend" must be one of`},
		{[]string{"add", "x", "--base", "project:demo", "--project", "demo", "--base", "today"}, "--base given twice"},
	} {
		out, err := f.views(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) || out != "" {
			t.Errorf("views %q: stdout %q, err %v; want %q", tc.args, out, err, tc.want)
		}
	}
	if got, _ := os.ReadFile(viewsFile(t)); string(got) != kept {
		t.Errorf("a refused add changed views.toml:\n%s", got)
	}
}

func TestViewsAddReplaceSavesOverTheNameKeepingItsPlace(t *testing.T) {
	f := newSubcommandFixture(t)
	writeViews(t, "[[view]]\nname = \"First\"\nbase = \"today\"\n\n[[view]]\nname = \"Second\"\nbase = \"inbox\"\n")
	out, err := f.views(t, "add", "first", "--base", "Upcoming", "--replace")
	if err != nil || !strings.Contains(out, `Replaced view "first"`) {
		t.Fatalf("replace: %q, %v", out, err)
	}
	text, _ := os.ReadFile(viewsFile(t))
	if !strings.Contains(string(text), `name = "first"`) || strings.Index(string(text), "first") > strings.Index(string(text), "Second") || strings.Contains(string(text), `"today"`) {
		t.Errorf("views.toml after replace:\n%s", text)
	}
	if out, err = f.views(t, "add", "Third", "--replace"); err != nil || !strings.Contains(out, `Saved view "Third"`) {
		t.Errorf("--replace with a new name adds it: %q, %v", out, err)
	}
}

func TestViewsAddRefusesAViewsFileChangedMeanwhile(t *testing.T) {
	f := newSubcommandFixture(t)
	writeViews(t, "[[view]]\nname = \"Kept\"\nbase = \"today\"\n")
	plan, err := PlanLaunch([]string{"views", "add", "Mine", "--project", "demo"}, func(string) (string, bool) { return "", false }, "test")
	if err != nil {
		t.Fatal(err)
	}
	gateway := data.NewGateway(f.locate).WithHost(testHost)
	defer gateway.Close()
	changed := "[[view]]\nname = \"Kept\"\nbase = \"today\"\n\n[[view]]\nname = \"Sneaked\"\nbase = \"inbox\"\n"
	send := func(cmd model.Cmd) model.Msg {
		writeViews(t, changed) // another writer lands between the read of views.toml and the write
		return send(t, gateway, cmd)
	}
	var out bytes.Buffer
	err = runViews(plan, f.repo, gateway.ViewsStore, send, &out)
	if !errors.Is(err, data.ErrViewsChanged) || exitStatus(err) != 1 || out.Len() != 0 {
		t.Fatalf("err = %v (exit %d), out %q; want ErrViewsChanged and exit 1", err, exitStatus(err), out.String())
	}
	if got, _ := os.ReadFile(viewsFile(t)); string(got) != changed {
		t.Errorf("the add overwrote the other change:\n%s", got)
	}
}

func TestViewsRemoveRefusesAFixedCollectionAndAnUnknownName(t *testing.T) {
	f := newSubcommandFixture(t)
	const kept = "[[view]]\nname = \"Mine\"\nbase = \"today\"\n"
	writeViews(t, kept)
	for _, tc := range []struct{ name, want string }{
		{"today", `"today" is the fixed Collection Today, not a saved view`},
		{"All Open", "fixed Collection All Open"},
		{"nope", `no saved view named "nope"; the views are`},
	} {
		out, err := f.views(t, "rm", tc.name)
		if err == nil || exitStatus(err) != 1 || !strings.Contains(err.Error(), tc.want) || out != "" {
			t.Errorf("rm %q: stdout %q, err %v (exit %d); want %q", tc.name, out, err, exitStatus(err), tc.want)
		}
	}
	if got, _ := os.ReadFile(viewsFile(t)); string(got) != kept {
		t.Errorf("a refused rm changed views.toml:\n%s", got)
	}
}
