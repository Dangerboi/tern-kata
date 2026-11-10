package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func TestClearButtonEmptiesTheFieldAndLeavesItOpen(t *testing.T) {
	for _, tt := range []struct {
		name string
		open func(t *testing.T) driver.State
		id   string
		mode model.Mode
		text func(m model.Model) string
	}{
		{
			"search line",
			func(t *testing.T) driver.State { return feed(t, browsing(t, 160), key("/"), key("a"), key("l")).State },
			view.SearchID, model.ModeSearch,
			func(m model.Model) string { return m.Filters.Search },
		},
		{
			"page search line",
			func(t *testing.T) driver.State {
				return feed(t, jobsPane(t, 160, promptJob("J1", "Triage")), key("/"), key("t"), key("r")).State
			},
			view.SearchID, model.ModePageSearch,
			func(m model.Model) string { return m.PageSearch.Text },
		},
		{
			"Menu filter",
			func(t *testing.T) driver.State { return feed(t, browsing(t, 160), openMenu, key("w"), key("o")).State },
			view.LayerID(model.CardPicker) + ".input", model.ModePicker,
			func(m model.Model) string { return m.Picker.Filter },
		},
		{
			"Move to Project filter",
			func(t *testing.T) driver.State {
				return feed(t, movePane(t, movable("a")), key("shift+m"), key("s"), key("h")).State
			},
			view.LayerID(model.CardMove) + ".input", model.ModeMoveProject,
			func(m model.Model) string { return m.MoveProject.Filter },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.open(t)
			if tt.text(s.Model) == "" || s.Model.CurrentMode() != tt.mode {
				t.Fatalf("before the click: text %q, mode %s; want typed text in %s", tt.text(s.Model), s.Model.CurrentMode(), tt.mode)
			}
			got := step(t, s, click(tt.id, "clear")).State.Model
			if text := tt.text(got); text != "" {
				t.Errorf("text %q after the click, want it emptied", text)
			}
			if got.CurrentMode() != tt.mode {
				t.Errorf("mode %s after the click, want %s still open", got.CurrentMode(), tt.mode)
			}
		})
	}
}
