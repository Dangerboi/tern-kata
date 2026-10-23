package driver_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func firstRead(t *testing.T, cmds []model.Cmd) model.ReadSnapshot {
	t.Helper()
	for _, cmd := range cmds {
		if read, ok := cmd.(model.ReadSnapshot); ok {
			return read
		}
	}
	t.Fatalf("no snapshot read in %#v", cmds)
	return model.ReadSnapshot{}
}

func filterLineText(t *testing.T, s driver.State) string {
	t.Helper()
	text, _ := nodeText(t, s, "main.filters")
	return text
}

func TestAPaneStartsWithTheProjectsSavedDefaultAndCClearsIt(t *testing.T) {
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	saved := model.Filters{Status: "open", Owner: "me"}
	m, cmds := model.Update(s.Model, model.ProjectLoaded{Project: model.Project{ID: 7, UID: "p", Name: "work"}, Default: saved})
	read := firstRead(t, cmds)
	if !slices.Equal(read.Query.Status, []string{"open"}) || !slices.Equal(read.Query.Owner, []string{"me"}) {
		t.Fatalf("first read query = %#v, want status open and owner me", read.Query)
	}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Issues: []model.Issue{alpha}}})
	s.Model = m
	if got := filterLineText(t, s); got != "status=open · owner=me (default)" {
		t.Fatalf("filter line = %q", got)
	}
	out := step(t, s, key("c"))
	read = firstRead(t, out.Cmds)
	if read.Query.Status != nil || read.Query.Owner != nil {
		t.Fatalf("read after c = %#v, want unfiltered", read.Query)
	}
	cleared := step(t, out.State, model.SnapshotLoaded{Request: read.Request, Query: read.Query, Snapshot: model.Snapshot{Issues: []model.Issue{alpha}}}).State
	if got, drawn := nodeText(t, cleared, "main.filters"); drawn && got != "" {
		t.Fatalf("filter line after c = %q, want none", got)
	}
}

func TestAnUnreadableDefaultStartsUnfilteredWithANotice(t *testing.T) {
	s := feed(t, driver.State{}, model.Start{Workspace: "/repo"}, resize(160)).State
	m, cmds := model.Update(s.Model, model.ProjectLoaded{Project: model.Project{UID: "p", Name: "work"}, DefaultErr: errors.New("filters.toml: bad")})
	if read := firstRead(t, cmds); read.Query.Status != nil || m.Filters != (model.Filters{}) || !strings.Contains(m.Notice, "filters.toml: bad") {
		t.Fatalf("query %#v filters %#v notice %q", read.Query, m.Filters, m.Notice)
	}
}

// filterCard is the Pane with the Filter card open and status set to open.
func filterCard(t *testing.T) driver.State {
	t.Helper()
	return feed(t, browsing(t, 160), key("f"), key("tab"), key("right")).State
}

func TestCtrlDInTheFilterCardSavesTheDefaultAndAppliesItOnSuccess(t *testing.T) {
	s := filterCard(t)
	out := step(t, s, key("ctrl+d"))
	want := []model.Cmd{model.SaveFilters{ProjectUID: "p", ProjectName: "work", Default: model.Filters{Status: "open"}}}
	if !slices.Equal(out.Cmds, want) {
		t.Fatalf("cmds %#v, want %#v", out.Cmds, want)
	}
	if !out.State.Model.FilterForm.Open || !out.State.Model.FilterForm.Saving || out.State.Model.Filters != (model.Filters{}) {
		t.Fatalf("while saving: open %v saving %v filters %#v", out.State.Model.FilterForm.Open, out.State.Model.FilterForm.Saving, out.State.Model.Filters)
	}
	if edited := step(t, out.State, key("x")).State.Model.FilterForm.Values; edited.Owner != "" {
		t.Errorf("a key edited the card while saving: %#v", edited)
	}
	done := step(t, out.State, model.FiltersSaved{ProjectUID: "p", Default: model.Filters{Status: "open"}})
	m := done.State.Model
	if m.FilterForm.Open || m.Filters.Status != "open" || m.Notice != "Saved as work's default filter" || !readsSnapshot(done) {
		t.Fatalf("after success: open %v filters %#v notice %q reads %v", m.FilterForm.Open, m.Filters, m.Notice, readsSnapshot(done))
	}
	if got := filterLineText(t, done.State); !strings.HasSuffix(got, "(default)") {
		t.Errorf("filter line %q lacks (default)", got)
	}
}

func TestAFailedDefaultSaveKeepsTheCardValuesAndError(t *testing.T) {
	out := step(t, filterCard(t), key("ctrl+d"))
	failed := step(t, out.State, model.FiltersSaved{ProjectUID: "p", Default: model.Filters{Status: "open"}, Err: errors.New("disk full")})
	m := failed.State.Model
	if !m.FilterForm.Open || m.FilterForm.Saving || m.FilterForm.Values.Status != "open" || m.Filters != (model.Filters{}) || readsSnapshot(failed) {
		t.Fatalf("after failure: open %v saving %v values %#v filters %#v", m.FilterForm.Open, m.FilterForm.Saving, m.FilterForm.Values, m.Filters)
	}
	if text, ok := nodeText(t, failed.State, "layer.filters.error"); !ok || !strings.Contains(text, "disk full") {
		t.Errorf("card error = %q, %v", text, ok)
	}
}

func TestALateDefaultSaveResultAfterCancelChangesNothing(t *testing.T) {
	out := step(t, filterCard(t), key("ctrl+d"))
	cancelled := step(t, out.State, key("escape")).State
	for _, msg := range []model.FiltersSaved{
		{ProjectUID: "p", Default: model.Filters{Status: "open"}},
		{ProjectUID: "p", Default: model.Filters{Status: "open"}, Err: errors.New("late")},
	} {
		late := step(t, cancelled, msg)
		if late.State.Model.Filters != (model.Filters{}) || late.State.Model.FilterForm.Open || late.State.Model.Notice != "" || len(late.Cmds) != 0 {
			t.Errorf("%+v changed the Pane: filters %#v notice %q cmds %#v", msg, late.State.Model.Filters, late.State.Model.Notice, late.Cmds)
		}
	}
	other := step(t, out.State, model.FiltersSaved{ProjectUID: "elsewhere", Default: model.Filters{Status: "open"}})
	if !other.State.Model.FilterForm.Open || !other.State.Model.FilterForm.Saving {
		t.Error("another Project's result answered the card")
	}
}

func TestSavingAnEmptyCardRemovesTheDefault(t *testing.T) {
	s := feed(t, browsing(t, 160), key("f")).State
	out := step(t, s, key("ctrl+d"))
	want := []model.Cmd{model.SaveFilters{ProjectUID: "p", ProjectName: "work"}}
	if !slices.Equal(out.Cmds, want) {
		t.Fatalf("cmds %#v, want %#v", out.Cmds, want)
	}
	done := step(t, out.State, model.FiltersSaved{ProjectUID: "p"})
	if done.State.Model.FilterForm.Open || done.State.Model.Notice != "Removed work's default filter" {
		t.Fatalf("open %v notice %q", done.State.Model.FilterForm.Open, done.State.Model.Notice)
	}
}

func TestSearchTextIsNeverPartOfTheSavedDefault(t *testing.T) {
	s := feed(t, browsing(t, 160), key("/"), key("a")).State
	s.Model.Search = model.SearchState{}
	out := feed(t, s, key("f"), key("ctrl+d"))
	save, _ := out.Cmds[0].(model.SaveFilters)
	if save.Default.Search != "" || save.Default != (model.Filters{}) {
		t.Fatalf("save default %#v", save.Default)
	}
}

func TestCtrlDWithNoBoundProjectSaysWhyAndSavesNothing(t *testing.T) {
	s := loadedWith(t, 160, model.Project{}, model.Snapshot{Issues: []model.Issue{alpha}})
	out := feed(t, s, key("f"), key("ctrl+d"))
	if len(out.Cmds) != 0 || !out.State.Model.FilterForm.Open || out.State.Model.FilterForm.Saving {
		t.Fatalf("cmds %#v open %v", out.Cmds, out.State.Model.FilterForm.Open)
	}
	if text, ok := nodeText(t, out.State, "layer.filters.error"); !ok || !strings.Contains(text, "Project") {
		t.Errorf("card error = %q, %v", text, ok)
	}
}

func TestTheFilterCardListsSaveAsDefaultInFooterAndHelp(t *testing.T) {
	s := feed(t, browsing(t, 160), key("f")).State
	footer := ""
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		footer += wireText(n)
	}
	if !strings.Contains(footer, "save as default") {
		t.Error("the footer lacks save as default")
	}
	help := ""
	for _, n := range wireNodes(t, view.Render(feed(t, browsing(t, 160), key("?")).State.Model, now)) {
		help += wireText(n)
	}
	if !strings.Contains(help, "save as default filter") {
		t.Error("the help sheet lacks save as default filter")
	}
}

func groupTitles(s driver.State) []string {
	var titles []string
	for _, g := range s.Model.Groups {
		titles = append(titles, g.Title)
	}
	return titles
}

func TestFieldFiltersKeepTheCollectionsHeadingsAndSearchShowsResults(t *testing.T) {
	overdue, today := dated("a", "2026-10-01", ""), dated("b", "2026-10-08", "")
	s := projectPane(t, 160, overdue, today)
	s = answered(t, s, model.CollectionSelected{Collection: model.FixedCollections()[1]}, overdue, today).State
	if got := groupTitles(s); !slices.Equal(got, []string{"Overdue", "Today"}) {
		t.Fatalf("Today unfiltered groups %v", got)
	}
	s = answered(t, s, key("s"), overdue, today).State
	if s.Model.Filters.Status != "open" {
		t.Fatalf("status filter %q", s.Model.Filters.Status)
	}
	if got := groupTitles(s); !slices.Equal(got, []string{"Overdue", "Today"}) {
		t.Errorf("Today with a status filter groups %v, want its own headings", got)
	}
	s = answered(t, feed(t, s, key("/")).State, key("a"), overdue, today).State
	if got := groupTitles(s); !slices.Equal(got, []string{"Results"}) {
		t.Errorf("Today with search groups %v, want Results", got)
	}
}
