package data

import (
	"os"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// A view saved through the gateway commands survives a new Pane's read, an edit keeps its place, a stale write is refused with
// the file as it is, and a delete removes it.
func TestGatewayViewCommandsRoundTripThroughViewsToml(t *testing.T) {
	f := newConfigFixture(t)
	first := as[model.ViewsLoaded](t, run(t, f.gateway, model.LoadViews{}))
	if first.Err != nil || len(first.Views) != 0 || first.Token == nil {
		t.Fatalf("first read = %+v", first)
	}
	view := model.SavedView{Name: "Mine", Base: model.Collection{View: "today"}, Filters: model.Filters{Owner: model.OwnerMe}}
	saved := as[model.ViewSaved](t, run(t, f.gateway, model.SaveView{View: view, Token: first.Token}))
	if saved.Err != nil || len(saved.Loaded.Views) != 1 || saved.Loaded.Views[0].Name != "Mine" {
		t.Fatalf("saved = %+v", saved)
	}
	again := as[model.ViewsLoaded](t, run(t, f.again(), model.LoadViews{}))
	if len(again.Views) != 1 || again.Views[0].Filters.Owner != model.OwnerMe || again.Views[0].Base.Title != "Today" {
		t.Fatalf("a new Pane read %+v", again)
	}
	stale := as[model.ViewSaved](t, run(t, f.gateway, model.SaveView{View: model.SavedView{Name: "Other", Base: model.Collection{View: "inbox"}}, Token: first.Token}))
	if !stale.Changed || stale.Err == nil || len(stale.Loaded.Views) != 1 {
		t.Fatalf("stale write = %+v", stale)
	}
	renamed := view
	renamed.Name = "Mine!"
	edit := as[model.ViewSaved](t, run(t, f.gateway, model.SaveView{Old: "Mine", View: renamed, Token: again.Token}))
	if edit.Err != nil || edit.Loaded.Views[0].Name != "Mine!" {
		t.Fatalf("edit = %+v", edit)
	}
	gone := as[model.ViewDeleted](t, run(t, f.gateway, model.DeleteView{Name: "Mine!", Token: edit.Loaded.Token}))
	if gone.Err != nil || len(gone.Loaded.Views) != 0 {
		t.Fatalf("delete = %+v", gone)
	}
}

func TestGatewayLoadViewsReportsAnUnreadableFileAndLeavesItAlone(t *testing.T) {
	f := newConfigFixture(t)
	path := f.write(t, "views.toml", "[[view\nname = ")
	got := as[model.ViewsLoaded](t, run(t, f.gateway, model.LoadViews{}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), path) || got.Token != nil {
		t.Fatalf("read = %+v", got)
	}
	if text, _ := os.ReadFile(path); string(text) != "[[view\nname = " {
		t.Fatalf("file changed: %q", text)
	}
}
