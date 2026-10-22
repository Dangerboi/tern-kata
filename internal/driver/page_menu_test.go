package driver_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// menuPages are the Pages that sit beside the Issues and Board Pages, each in a Project-bound Pane
// with one row to inspect.
func menuPages(t *testing.T) []struct {
	name      string
	list      driver.State
	inspector driver.State
	mode      model.Mode
	inspMode  model.Mode
} {
	t.Helper()
	jobs := jobsPane(t, 160, nativeJob("J1", "Weekly", jobParent("open", false)))
	runs := runsPane(t, 160, finishedRun("R1"))
	flows := workflowsPane(t, 160, nightlyFlow())
	return []struct {
		name      string
		list      driver.State
		inspector driver.State
		mode      model.Mode
		inspMode  model.Mode
	}{
		{"Jobs", jobs, step(t, jobs, key("enter")).State, model.ModeJobs, model.ModeJobInspector},
		{"Runs", runs, step(t, runs, key("enter")).State, model.ModeRuns, model.ModeRunInspector},
		{"Workflows", flows, step(t, flows, key("enter")).State, model.ModeWorkflows, model.ModeWorkflows},
	}
}

func TestEscOnAPageListOpensTheMenuAndInAnInspectorGoesBackToTheListFirst(t *testing.T) {
	for _, p := range menuPages(t) {
		if got := step(t, p.list, key("escape")).State.Model; !got.Picker.Open {
			t.Errorf("%s list: esc left the Menu closed", p.name)
		}
		if p.inspector.Model.CurrentMode() != p.inspMode {
			t.Fatalf("%s: enter left mode %v, want %v", p.name, p.inspector.Model.CurrentMode(), p.inspMode)
		}
		back := step(t, p.inspector, key("escape")).State.Model
		if back.Picker.Open || back.CurrentMode() != p.mode {
			t.Errorf("%s inspector: esc gave Menu open %v, mode %v; want the list", p.name, back.Picker.Open, back.CurrentMode())
		}
	}
}

func TestPAndIOnAPageOpenTheMenusProjectsAndTheInbox(t *testing.T) {
	for _, p := range menuPages(t) {
		for where, s := range map[string]driver.State{"list": p.list, "inspector": p.inspector} {
			if got := step(t, s, key("shift+p")).State.Model; !got.Picker.Open || !got.Picker.Projects {
				t.Errorf("%s %s: P gave Menu open %v, Projects level %v", p.name, where, got.Picker.Open, got.Picker.Projects)
			}
			if got := step(t, s, key("shift+i")).State.Model; got.Collection != model.InboxCollection() || got.Page != model.PageIssues {
				t.Errorf("%s %s: I gave Collection %+v on Page %v, want the Inbox", p.name, where, got.Collection, got.Page)
			}
		}
	}
}
