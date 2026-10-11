package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

const newProjectWorkspace = "/home/dev/my-repo"

// newProjectStart is a loaded Pane in newProjectWorkspace: bound to Project
// smoke, or unbound on Inbox with the Menu sidebar.
func newProjectStart(bound bool) model.Model {
	loaded, snapshot := model.ProjectLoaded{}, model.SnapshotLoaded{Snapshot: g.UISnapshotResponseBody{
		Catalog:    pickerCatalog(),
		Collection: []model.Issue{{UID: "a", ShortID: "aaaa", Title: "First"}},
	}}
	if bound {
		loaded.Project = model.Project{ID: 7, UID: "smoke-uid", Name: "smoke"}
	}
	m, _ := loadFirst(model.Model{Cols: 160, Workspace: newProjectWorkspace, Actor: "alice"}, loaded, snapshot)
	return m
}

func TestSidebarEndsWithNewProject(t *testing.T) {
	for name, keys := range map[string][]string{
		"the last row":                          {"escape", "end", "enter"},
		"down reaches it from the last Project": {"escape", "end", "up", "down", "enter"},
		"it ends the list":                      {"escape", "end", "down", "down", "enter"},
	} {
		t.Run(name, func(t *testing.T) {
			m, cmds := pickerKeys(newProjectStart(false), keys...)
			if !m.ProjectForm.Open || m.CurrentMode() != model.ModeProjectForm || len(cmds) != 0 || m.Collection.View != "inbox" {
				t.Fatalf("form open=%v mode=%s cmds=%#v collection=%+v", m.ProjectForm.Open, m.CurrentMode(), cmds, m.Collection)
			}
		})
	}
}

func TestSidebarProjectAboveNewProjectStillSwitches(t *testing.T) {
	smoke := model.Collection{View: "project", ProjectUID: "smoke-uid", Title: "smoke"}
	m, cmds := pickerKeys(newProjectStart(false), "escape", "end", "up", "enter", "down", "down", "enter")
	if m.ProjectForm.Open || m.Collection != smoke || !readsCollection(cmds, m, "") {
		t.Fatalf("form open=%v collection=%+v cmds=%#v", m.ProjectForm.Open, m.Collection, cmds)
	}
}

func TestNewProjectFormStartsWithTheDirectoryName(t *testing.T) {
	for _, bound := range []bool{false, true} {
		m := newProjectOpened(bound)
		if m.ProjectForm.Name != "my-repo" || m.ProjectForm.Saving || m.ProjectForm.Err != nil || m.ProjectForm.Bind == bound {
			t.Fatalf("bound=%v: form = %+v", bound, m.ProjectForm)
		}
	}
}

func TestNewProjectFromTheSidebarCreatesAndBinds(t *testing.T) {
	m, cmds := pickerKeys(newProjectStart(false), "escape", "end", "enter", "enter")
	want := []model.Cmd{model.BindProject{Workspace: newProjectWorkspace, Actor: "alice", Name: "my-repo"}}
	if !reflect.DeepEqual(cmds, want) || !m.ProjectForm.Open || !m.ProjectForm.Saving {
		t.Fatalf("submit: cmds=%#v form=%+v", cmds, m.ProjectForm)
	}
}

// newProjectOpened has the form open: from the sidebar of an unbound
// directory, or, when bound, from the picker.
func newProjectOpened(bound bool) model.Model {
	if bound {
		m, _ := pickerKeys(newProjectStart(true), "p", "end", "enter")
		return m
	}
	m, _ := pickerKeys(newProjectStart(false), "escape", "end", "enter")
	return m
}

func TestNewProjectNameEditing(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
		want string
	}{
		{"typing appends", []string{"-", "x"}, "my-repo-x"},
		{"backspace erases a letter", []string{"backspace"}, "my-rep"},
		{"ctrl+u clears the name", []string{"ctrl+u", "a", "b"}, "ab"},
		{"space is typed", []string{"space", "x"}, "my-repo x"},
		{"global keys are typed", []string{"q", "?", "p", "j"}, "my-repoq?pj"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, cmds := pickerKeys(newProjectOpened(false), tt.keys...)
			if m.ProjectForm.Name != tt.want || !m.ProjectForm.Open || m.Quitting || m.HelpOpen || len(cmds) != 0 {
				t.Fatalf("name %q open=%v quitting=%v help=%v cmds=%#v, want %q", m.ProjectForm.Name, m.ProjectForm.Open, m.Quitting, m.HelpOpen, cmds, tt.want)
			}
		})
	}
	m, _ := model.Update(newProjectOpened(false), model.KeyMsg{Chord: "paste", Text: "-a\nb"})
	if m.ProjectForm.Name != "my-repo-a b" {
		t.Fatalf("pasted name = %q", m.ProjectForm.Name)
	}
}

func TestNewProjectEscapeCancelsTheForm(t *testing.T) {
	base, _ := pickerKeys(newProjectStart(false), "escape", "end")
	got, cmds := pickerKeys(base, "enter", "x", "escape")
	if !reflect.DeepEqual(got, base) || len(cmds) != 0 {
		t.Fatalf("cancel left %+v, cmds %#v", got.ProjectForm, cmds)
	}
}

func TestNewProjectNeedsAName(t *testing.T) {
	for _, keys := range [][]string{{"ctrl+u"}, {"ctrl+u", "space", "space"}} {
		m, cmds := pickerKeys(newProjectOpened(false), append(keys, "enter")...)
		if len(cmds) != 0 || !m.ProjectForm.Open || m.ProjectForm.Saving || m.ProjectForm.Err == nil {
			t.Fatalf("empty name %q: cmds=%#v form=%+v", keys, cmds, m.ProjectForm)
		}
	}
}

func TestNewProjectSendsTheTrimmedNameAndKeepsWhatWasTyped(t *testing.T) {
	m, cmds := pickerKeys(newProjectOpened(false), "ctrl+u", "space", "f", "o", "o", "space", "enter")
	want := []model.Cmd{model.BindProject{Workspace: newProjectWorkspace, Actor: "alice", Name: "foo"}}
	if !reflect.DeepEqual(cmds, want) || m.ProjectForm.Name != " foo " {
		t.Fatalf("cmds=%#v name=%q", cmds, m.ProjectForm.Name)
	}
}

func TestNewProjectFormIgnoresKeysWhileSaving(t *testing.T) {
	saving, _ := pickerKeys(newProjectOpened(false), "enter")
	got, cmds := pickerKeys(saving, "x", "backspace", "ctrl+u", "escape", "enter")
	if !reflect.DeepEqual(got, saving) || len(cmds) != 0 {
		t.Fatalf("keys changed a saving form: %+v cmds %#v", got.ProjectForm, cmds)
	}
}

func TestNewProjectInterruptQuits(t *testing.T) {
	idle := newProjectOpened(false)
	saving, _ := pickerKeys(idle, "enter")
	for name, m := range map[string]model.Model{"idle": idle, "saving": saving} {
		got, cmds := pickerKeys(m, "ctrl+c")
		if !got.Quitting || len(cmds) != 0 {
			t.Fatalf("%s ctrl+c: quitting=%v cmds=%#v", name, got.Quitting, cmds)
		}
	}
}

func TestNewProjectRefusedWhileDisconnected(t *testing.T) {
	m := newProjectOpened(false)
	m.Stale = true
	m, cmds := pickerKeys(m, "enter")
	if len(cmds) != 0 || !m.ProjectForm.Open || m.ProjectForm.Saving || m.ProjectForm.Err == nil {
		t.Fatalf("disconnected submit: cmds=%#v form=%+v", cmds, m.ProjectForm)
	}
}

func TestNewProjectRejectionKeepsTheForm(t *testing.T) {
	const message = "project with this name was archived via `kata projects remove`\nrun \"kata projects restore my-repox\" or pick a different name"
	saving, _ := pickerKeys(newProjectOpened(false), "x", "enter")
	rejected, cmds := model.Update(saving, model.ProjectSaved{Err: errors.New(message)})
	form := rejected.ProjectForm
	if len(cmds) != 0 || !form.Open || form.Saving || form.Name != "my-repox" || form.Err == nil || form.Err.Error() != message {
		t.Fatalf("rejection: cmds=%#v form=%+v", cmds, form)
	}
	if rejected.Bound || rejected.Collection.View != "inbox" || rejected.CurrentMode() != model.ModeProjectForm {
		t.Fatalf("rejection moved on: bound=%v collection=%+v mode=%s", rejected.Bound, rejected.Collection, rejected.CurrentMode())
	}

	// Editing the name clears the message, and the corrected name goes out again.
	retried, cmds := pickerKeys(rejected, "backspace", "enter")
	if retried.ProjectForm.Err != nil || !retried.ProjectForm.Saving || !reflect.DeepEqual(cmds, []model.Cmd{model.BindProject{Workspace: newProjectWorkspace, Actor: "alice", Name: "my-repo"}}) {
		t.Fatalf("retry: cmds=%#v form=%+v", cmds, retried.ProjectForm)
	}
}

func TestNewProjectResultWithoutASavingFormIsIgnored(t *testing.T) {
	for name, m := range map[string]model.Model{"no form": newProjectStart(false), "form not saving": newProjectOpened(false)} {
		for _, msg := range []model.ProjectSaved{{Project: model.Project{UID: "late"}}, {Err: errors.New("late")}} {
			got, cmds := model.Update(m, msg)
			if !reflect.DeepEqual(got, m) || len(cmds) != 0 {
				t.Fatalf("%s: a stray result changed %+v, cmds %#v", name, got.ProjectForm, cmds)
			}
		}
	}
}

func TestNewProjectBindOpensTheProjectFirst(t *testing.T) {
	created := model.Project{ID: 9, UID: "repo-uid", Name: "my-repo"}
	collection := model.Collection{View: "project", ProjectUID: "repo-uid", Title: "my-repo"}
	saving, _ := pickerKeys(newProjectOpened(false), "enter")
	got, cmds := model.Update(saving, model.ProjectSaved{Project: created})

	if got.ProjectForm.Open || !got.Bound || got.Project.UID != "repo-uid" || got.Collection != collection || got.Focus != model.ColumnList {
		t.Fatalf("after bind: form=%+v bound=%v project=%q collection=%+v focus=%v", got.ProjectForm, got.Bound, got.Project.UID, got.Collection, got.Focus)
	}
	if !readsCollection(cmds, got, "") {
		t.Fatalf("after bind requested %#v, want a read of %+v", cmds, collection)
	}
	if want := []model.Column{model.ColumnList, model.ColumnDetail}; !reflect.DeepEqual(got.VisibleColumns(), want) {
		t.Fatalf("columns = %v: the sidebar should be gone", got.VisibleColumns())
	}
	if got.Collection.Title != "my-repo" || !got.Loading {
		t.Fatalf("heading %q loading=%v", got.Collection.Title, got.Loading)
	}

	// The snapshot that arrives lists the Project, which is now the pinned entry.
	snapshot := g.UISnapshotResponseBody{Catalog: append(pickerCatalog(), g.UIProject{Project: created})}
	loaded, _ := model.Update(got, model.SnapshotLoaded{Request: got.RefreshRequest, Query: got.SnapshotQuery(), Snapshot: snapshot})
	if loaded.Loading || !reflect.DeepEqual(pickerTitles(loaded)[:2], []string{"my-repo", "Inbox"}) {
		t.Fatalf("loaded: loading=%v entries=%q", loaded.Loading, pickerTitles(loaded))
	}
}

func TestPickerNewProjectOpensTheFormAndEscReturnsToThePicker(t *testing.T) {
	for _, tt := range []struct {
		name   string
		keys   []string
		filter string
	}{
		{"last entry", []string{"p", "end", "enter"}, ""},
		{"found by its title", []string{"p", "n", "e", "w", "enter"}, "new"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opened, cmds := pickerKeys(newProjectStart(true), tt.keys...)
			if !opened.ProjectForm.Open || opened.ProjectForm.Bind || opened.CurrentMode() != model.ModeProjectForm || len(cmds) != 0 {
				t.Fatalf("form=%+v mode=%s cmds=%#v", opened.ProjectForm, opened.CurrentMode(), cmds)
			}
			back, cmds := pickerKeys(opened, "escape")
			if back.CurrentMode() != model.ModePicker || back.Picker.Filter != tt.filter || back.Collection != opened.Collection || len(cmds) != 0 {
				t.Fatalf("esc: mode=%s picker=%+v cmds=%#v", back.CurrentMode(), back.Picker, cmds)
			}
			// The picker is back as it was: the cursor still rests on New Project.
			entries := back.PickerEntries()
			if entries[back.Picker.Cursor].Title != "New Project" {
				t.Fatalf("cursor on %q", entries[back.Picker.Cursor].Title)
			}
		})
	}
}

func TestPickerNewProjectOnlyCreates(t *testing.T) {
	created := model.Project{ID: 9, UID: "extra-uid", Name: "extra"}
	collection := model.Collection{View: "project", ProjectUID: "extra-uid", Title: "extra"}
	m, cmds := pickerKeys(newProjectOpened(true), "ctrl+u", "e", "x", "t", "r", "a", "enter")
	if want := []model.Cmd{model.CreateProject{Workspace: newProjectWorkspace, Actor: "alice", Name: "extra"}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("submit requested %#v, want %#v", cmds, want)
	}
	got, cmds := model.Update(m, model.ProjectSaved{Project: created})
	if got.ProjectForm.Open || got.Picker.Open || got.CurrentMode() != model.ModeList || got.Focus != model.ColumnList {
		t.Fatalf("after create: form=%+v picker=%+v mode=%s", got.ProjectForm, got.Picker, got.CurrentMode())
	}
	if !got.Bound || got.Project.UID != "smoke-uid" || got.Collection != collection {
		t.Fatalf("after create: bound=%v project=%q collection=%+v; the directory should stay bound to smoke", got.Bound, got.Project.UID, got.Collection)
	}
	if !readsCollection(cmds, got, "") {
		t.Fatalf("after create requested %#v, want a read of %+v", cmds, collection)
	}
	if got.Collection.Title != "extra" || pickerTitles(got)[0] != "smoke" {
		t.Fatalf("heading %q, picker pins %q", got.Collection.Title, pickerTitles(got)[0])
	}

	// An Issue created before the snapshot arrives belongs to the new Project.
	drafting, _ := pickerKeys(got, "n")
	if drafting.Create.ProjectID != 9 {
		t.Fatalf("Issue draft targets Project %d, want 9", drafting.Create.ProjectID)
	}
}

func TestNewProjectPointerEqualsKeyboard(t *testing.T) {
	unbound := newProjectStart(false)
	wantSidebar, wantCmds := pickerKeys(unbound, "escape", "end", "enter")
	gotSidebar, gotCmds := model.Update(unbound, model.CollectionPointerSelected{Key: "new-project"})
	if !wantSidebar.ProjectForm.Open || !reflect.DeepEqual(gotSidebar, wantSidebar) || !reflect.DeepEqual(gotCmds, wantCmds) {
		t.Fatalf("sidebar click differs from the keyboard: %+v vs %+v", gotSidebar.ProjectForm, wantSidebar.ProjectForm)
	}

	picker, _ := pickerKeys(newProjectStart(true), "p")
	wantPicker, wantCmds := pickerKeys(picker, "end", "enter")
	gotPicker, gotCmds := model.Update(picker, model.PickerSelected{Key: "new-project"})
	if !wantPicker.ProjectForm.Open || !reflect.DeepEqual(gotPicker, wantPicker) || !reflect.DeepEqual(gotCmds, wantCmds) {
		t.Fatalf("picker click differs from the keyboard: %+v vs %+v", gotPicker.ProjectForm, wantPicker.ProjectForm)
	}
}

func TestPickerNewProjectBindsAnUnboundDirectory(t *testing.T) {
	m, _ := pickerKeys(newProjectStart(false), "p", "end", "enter")
	if !m.ProjectForm.Open || !m.ProjectForm.Bind {
		t.Fatalf("form = %+v", m.ProjectForm)
	}
}

func TestNewProjectFormAdvertisesItsKeysFromTheKeymap(t *testing.T) {
	hints := map[string]model.Action{}
	for binding := range model.BindingsForMode(model.ModeProjectForm, true) {
		if binding.Mode != model.ModeProjectForm {
			t.Fatalf("the form advertises an inactive global binding: %+v", binding)
		}
		hints[binding.Chord] = binding.Action
	}
	if hints["enter"] != model.ProjectSubmit || hints["escape"] != model.ProjectCancel {
		t.Fatalf("form hints = %v", hints)
	}
}

func TestNewProjectRejectionInTheBoundPickerKeepsTheForm(t *testing.T) {
	saving, _ := pickerKeys(newProjectOpened(true), "enter")
	rejected, cmds := model.Update(saving, model.ProjectSaved{Err: errors.New("kata is unreachable")})
	if len(cmds) != 0 || !rejected.ProjectForm.Open || rejected.ProjectForm.Saving || rejected.ProjectForm.Err == nil || rejected.CurrentMode() != model.ModeProjectForm {
		t.Fatalf("rejection: cmds=%#v form=%+v mode=%s", cmds, rejected.ProjectForm, rejected.CurrentMode())
	}

	// The retry only creates, as the first try did.
	_, cmds = pickerKeys(rejected, "enter")
	if want := []model.Cmd{model.CreateProject{Workspace: newProjectWorkspace, Actor: "alice", Name: "my-repo"}}; !reflect.DeepEqual(cmds, want) {
		t.Fatalf("retry requested %#v, want %#v", cmds, want)
	}

	// Leaving returns to the picker, and the next form starts clean.
	back, _ := pickerKeys(rejected, "x", "escape")
	reopened, _ := pickerKeys(back, "enter")
	if back.CurrentMode() != model.ModePicker || reopened.ProjectForm.Name != "my-repo" || reopened.ProjectForm.Err != nil {
		t.Fatalf("after leaving: mode=%s reopened=%+v", back.CurrentMode(), reopened.ProjectForm)
	}
}
