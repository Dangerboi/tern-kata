package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// toIssues are the two ways to the Issues Page from another Page: its key and a click on its Page bar entry.
func toIssues(t *testing.T, s driver.State, how string) any {
	t.Helper()
	if how == "click" {
		return click(pageBarEntry(t, s.Model, "1"), view.KeyClick("1"))
	}
	return key("1")
}

var toIssuesHows = []string{"key", "click"}

// hiddenBefore is a Project Pane whose detail column is hidden (or not) before any other Page is shown.
func hiddenBefore(t *testing.T, hidden bool) driver.State {
	t.Helper()
	s := projectPane(t, 160, boardCard("a"), boardCard("b"))
	if hidden {
		s = feed(t, s, key("shift+l")).State
	}
	if s.Model.DetailHidden() != hidden {
		t.Fatalf("fixture: detail hidden %v, want %v", s.Model.DetailHidden(), hidden)
	}
	return s
}

func TestLeavingAPageWithItsOwnInspectorReturnsFocusToTheListAndTheDetailAsItWas(t *testing.T) {
	pages := map[string]func(*testing.T, driver.State) driver.State{
		"Jobs": func(t *testing.T, s driver.State) driver.State {
			s = answerJobs(t, step(t, s, key("3")), promptJob("J1", "Triage"))
			return step(t, s, key("enter")).State
		},
		"Runs": func(t *testing.T, s driver.State) driver.State {
			out := step(t, s, key("4"))
			s = answerRuns(t, out, "", finishedRun("R1"))
			return step(t, s, key("enter")).State
		},
	}
	modes := map[string]model.Mode{"Jobs": model.ModeJobInspector, "Runs": model.ModeRunInspector}
	for name, open := range pages {
		for _, hidden := range []bool{false, true} {
			for _, how := range toIssuesHows {
				inspecting := open(t, hiddenBefore(t, hidden))
				if got := inspecting.Model.CurrentMode(); got != modes[name] {
					t.Fatalf("%s: mode %q, want the inspector", name, got)
				}
				m := step(t, inspecting, toIssues(t, inspecting, how)).State.Model
				if m.Page != model.PageIssues || m.Focus != model.ColumnList || m.DetailHidden() != hidden {
					t.Errorf("%s, hidden before %v, %s: page %v focus %v hidden %v; want the Issues Page, the list focused and the detail as it was",
						name, hidden, how, m.Page, m.Focus, m.DetailHidden())
				}
			}
		}
	}
}

func TestLeavingAPageWhoseSecondColumnIsTheIssueDetailKeepsItFocusedAndOtherwiseGivesItBack(t *testing.T) {
	boardInLane := func(t *testing.T, s driver.State) driver.State {
		s = feed(t, s, key("2"), model.LanesLoaded{ProjectUID: "p"}).State
		return step(t, s, key("enter")).State // the Lane view, with the detail beside it
	}
	workflows := func(t *testing.T, s driver.State) driver.State {
		out := feed(t, s, key("5"))
		return step(t, out.State, model.WorkflowsLoaded{ProjectUID: "p", Workflows: []model.Workflow{nightlyFlow()}}).State
	}
	boardColumns := func(t *testing.T, s driver.State) driver.State {
		return feed(t, s, key("2"), model.LanesLoaded{ProjectUID: "p"}).State
	}
	for _, hidden := range []bool{false, true} {
		for _, how := range toIssuesHows {
			// The Lane view shows the detail and focus can move to it.
			lane := boardInLane(t, hiddenBefore(t, hidden))
			lane = step(t, lane, key("tab")).State
			if lane.Model.Focus != model.ColumnDetail || !lane.Model.LaneViewShown() {
				t.Fatalf("fixture: focus %v, lane view %v; want the Lane view with the detail focused", lane.Model.Focus, lane.Model.LaneViewShown())
			}
			m := step(t, lane, toIssues(t, lane, how)).State.Model
			if m.Page != model.PageIssues || m.Focus != model.ColumnDetail || m.DetailHidden() {
				t.Errorf("Board, hidden before %v, %s, detail focused: page %v focus %v hidden %v; want the detail kept shown and focused",
					hidden, how, m.Page, m.Focus, m.DetailHidden())
			}

			// Workflows: focus on the detail when the Page is left.
			flows := workflows(t, hiddenBefore(t, hidden))
			flows.Model.Focus = model.ColumnDetail
			shown := flows.Model.DetailHidden()
			m = step(t, flows, toIssues(t, flows, how)).State.Model
			if m.Page != model.PageIssues || m.Focus != model.ColumnDetail || m.DetailHidden() != shown {
				t.Errorf("Workflows, hidden before %v, %s, detail focused: page %v focus %v hidden %v; want focus and the column kept",
					hidden, how, m.Page, m.Focus, m.DetailHidden())
			}

			// Focus on the list or the Menu: the column goes back as it was.
			for name, from := range map[string]func(*testing.T, driver.State) driver.State{"Board": boardColumns, "Workflows": workflows} {
				for _, focus := range []model.Column{model.ColumnList, model.ColumnCollections} {
					s := from(t, hiddenBefore(t, hidden))
					s.Model.Focus = focus
					m := step(t, s, toIssues(t, s, how)).State.Model
					if m.Page != model.PageIssues || m.DetailHidden() != hidden {
						t.Errorf("%s, hidden before %v, %s, focus %v: page %v hidden %v; want the Issues Page and the detail as it was",
							name, hidden, how, focus, m.Page, m.DetailHidden())
					}
				}
			}
		}
	}
}
