package model_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/model"
)

var sidebarShown = []model.Column{model.ColumnCollections, model.ColumnList, model.ColumnDetail}
var sidebarHidden = []model.Column{model.ColumnList, model.ColumnDetail}

func projectCollection(m model.Model, name string) model.Collection {
	for _, c := range m.Collections {
		if c.IsProject() && c.Title == name {
			return c
		}
	}
	return model.Collection{}
}

func TestUnboundSidebarGoesWhenAProjectIsEntered(t *testing.T) {
	m := pickerStart(160, false)
	if got := m.VisibleColumns(); !reflect.DeepEqual(got, sidebarShown) {
		t.Fatalf("unbound start: columns %v", got)
	}

	// Entering a Project from the sidebar hides it, and the directory stays unbound.
	entered, _ := model.Update(m, model.CollectionSelected{Collection: projectCollection(m, "alpha")})
	if got := entered.VisibleColumns(); !reflect.DeepEqual(got, sidebarHidden) || entered.Bound {
		t.Fatalf("Project entered: columns %v bound %v", got, entered.Bound)
	}

	// A fixed Collection picked again, from the popup, brings it back.
	reopened, _ := pickerKeys(entered, "p", "home", "enter")
	if reopened.Collection.View != "inbox" || reopened.Picker.Open || !reflect.DeepEqual(reopened.VisibleColumns(), sidebarShown) {
		t.Fatalf("fixed Collection again: %+v columns %v", reopened.Collection, reopened.VisibleColumns())
	}
}

func TestUnboundSidebarGoesWhenAProjectIsPickedInThePopup(t *testing.T) {
	m, _ := pickerKeys(pickerStart(160, false), "p", "a", "l", "p", "enter")
	if m.Collection.Title != "alpha" || m.Picker.Open || !reflect.DeepEqual(m.VisibleColumns(), sidebarHidden) {
		t.Fatalf("picked alpha: %+v picker %v columns %v", m.Collection, m.Picker.Open, m.VisibleColumns())
	}
}

func TestUnboundSidebarGoesWhenAProjectIsCreatedAndEntered(t *testing.T) {
	saving, _ := pickerKeys(newProjectOpened(false), "enter")
	got, _ := model.Update(saving, model.ProjectSaved{Project: model.Project{ID: 9, UID: "repo-uid", Name: "my-repo"}})
	if !reflect.DeepEqual(got.VisibleColumns(), []model.Column{model.ColumnList, model.ColumnDetail}) {
		t.Fatalf("created: columns %v", got.VisibleColumns())
	}
}

func TestEscapeOpensTheMenuPopupWhileTheSidebarIsHidden(t *testing.T) {
	m := pickerStart(160, false)
	m, _ = model.Update(m, model.CollectionSelected{Collection: projectCollection(m, "alpha")})
	m, cmds := pickerKeys(m, "escape")
	if !m.Picker.Open || m.CurrentMode() != model.ModePicker || m.Focus != model.ColumnList || len(cmds) != 0 {
		t.Fatalf("escape: picker=%v mode=%s focus=%v cmds=%#v", m.Picker.Open, m.CurrentMode(), m.Focus, cmds)
	}
}
