package driver_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/Dangerboi/tern-kata/internal/worker"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// workflowText is a workflow document with one step list, as the card holds it.
func workflowText(steps string) string {
	return `{"name":"Flow","definition":{"version":1,"steps":[` + steps + `]}}`
}

// typeWorkflow replaces the card's text.
func typeWorkflow(t *testing.T, s driver.State, text string) driver.Output {
	t.Helper()
	old := len(s.Model.Workflows.Card.Editor.Text)
	return step(t, s, &tern.Edit{ID: cardInput(t, s.Model), From: 0, To: old, Text: text, Cursor: len(text), Len: old})
}

func readingAsk(t *testing.T, out driver.Output) (model.ReadWorkflowText, bool) {
	t.Helper()
	var ask model.ReadWorkflowText
	found := false
	for _, cmd := range out.Cmds {
		if c, ok := cmd.(model.ReadWorkflowText); ok {
			ask, found = c, true
		}
	}
	return ask, found
}

// workersAnswer is the Daemon gateway's answer to an ask: the Worker's own check against a prompt budget, nil for none readable.
func workersAnswer(ask model.ReadWorkflowText, budget *int, why string, missing bool) model.WorkflowTextRead {
	check := worker.CheckWorkflow(ask.Definition, budget)
	reading := model.WorkerReading{Refusals: check.Refusals, BudgetUnchecked: check.BudgetUnchecked}
	if check.BudgetUnchecked {
		reading.BudgetWhy, reading.ConfigMissing = why, missing
	}
	return model.WorkflowTextRead{Request: ask.Request, Reading: reading}
}

func toneOfLine(t *testing.T, s driver.State, i int) string {
	t.Helper()
	nodes := wireNodes(t, view.Render(s.Model, now))
	for _, n := range nodes {
		if strings.HasSuffix(n.ID, fmt.Sprintf("worker-reading.line.%d", i)) {
			tone, _ := n.P["tone"].(string)
			return tone
		}
	}
	t.Fatalf("the card draws no reading line %d", i)
	return ""
}

func TestWorkflowCardSavesWhatTheWorkerWouldRefuseAndSaysWhy(t *testing.T) {
	two := 2
	for name, tc := range map[string]struct{ steps, why string }{
		"on_fail":                {`{"key":"a","kind":"prompt","prompt":"p","options":{"on_fail":{"step":"a"}}}`, "options.on_fail"},
		"park":                   {`{"key":"a","kind":"prompt","prompt":"p","options":{"on_failure":"park"}}`, `on_failure`},
		"profile on a command":   {`{"key":"a","kind":"command","command":"true","options":{"profile":"x"}}`, "names a profile"},
		"prompt budget exceeded": {`{"key":"a","kind":"prompt","prompt":"p","retries":2}`, "prompt_budget 3"},
	} {
		t.Run(name, func(t *testing.T) {
			s := step(t, workflowsPane(t, 160, reviewFlow()), key("n")).State
			typed := typeWorkflow(t, s, workflowText(tc.steps))
			ask, ok := readingAsk(t, typed)
			if !ok || ask.After <= 0 {
				t.Fatalf("typing sent %#v, want an ask after a pause", typed.Cmds)
			}
			answered := step(t, typed.State, workersAnswer(ask, &two, "", false)).State
			mustShow(t, answered, "note", "This machine's Worker will not run it", tc.why)
			if got := toneOfLine(t, answered, 0); got != "warning" {
				t.Errorf("the refusal is drawn in tone %q", got)
			}
			saved := step(t, answered, key("ctrl+o"))
			for _, cmd := range saved.Cmds {
				if _, ok := cmd.(model.SaveWorkflow); ok {
					return
				}
			}
			t.Fatalf("ctrl+o sent %#v, want the save", saved.Cmds)
		})
	}
}

func TestWorkflowCardSaysQuietlyThatTheWorkerWouldRunWhatItAccepts(t *testing.T) {
	five := 5
	s := step(t, workflowsPane(t, 160), key("n")).State
	typed := typeWorkflow(t, s, workflowText(`{"key":"a","kind":"prompt","prompt":"p","options":{"mystery":true}}`))
	ask, _ := readingAsk(t, typed)
	answered := step(t, typed.State, workersAnswer(ask, &five, "", false)).State
	mustShow(t, answered, "note", "This machine's Worker would run it.")
	if got := toneOfLine(t, answered, 0); got != "muted" {
		t.Errorf("the quiet note is drawn in tone %q", got)
	}
}

func TestWorkflowCardShowsOnlyKatasProblemForTextKataRefuses(t *testing.T) {
	s := step(t, workflowsPane(t, 160), key("n")).State
	typed := typeWorkflow(t, s, workflowText(`{"key":"a","kind":"shell"}`))
	if ask, ok := readingAsk(t, typed); ok {
		t.Fatalf("text kata refuses was sent to the Worker: %#v", ask)
	}
	mustShow(t, typed.State, "kata's problem while editing", "must be")
	saved := step(t, typed.State, key("ctrl+o")).State
	mustShow(t, saved, "kata's problem", "must be")
	if strings.Contains(screen(t, saved), "Worker") {
		t.Error("a Worker note is drawn under text kata refuses")
	}
}

func TestWorkflowCardIgnoresTheAnswerForTextTheUserHasSinceEdited(t *testing.T) {
	five := 5
	s := step(t, workflowsPane(t, 160), key("n")).State
	first := typeWorkflow(t, s, workflowText(`{"key":"a","kind":"prompt","prompt":"p","options":{"on_fail":{}}}`))
	older, _ := readingAsk(t, first)
	second := typeWorkflow(t, first.State, workflowText(`{"key":"a","kind":"prompt","prompt":"p"}`))
	newer, _ := readingAsk(t, second)
	if newer.Request == older.Request {
		t.Fatal("two asks share a request number")
	}
	stale := step(t, second.State, workersAnswer(older, &five, "", false)).State
	if strings.Contains(screen(t, stale), "Worker") {
		t.Error("the answer for the older text was drawn")
	}
	fresh := step(t, stale, workersAnswer(newer, &five, "", false)).State
	mustShow(t, fresh, "note", "would run it")

	// Text kata then refuses drops an answer still on its way.
	third := typeWorkflow(t, fresh, workflowText(`{"key":"a","kind":"prompt","prompt":"p"`))
	late := step(t, third.State, workersAnswer(newer, &five, "", false)).State
	if strings.Contains(screen(t, late), "Worker") {
		t.Error("an answer was drawn under text that no longer passes kata's rules")
	}
}

func TestWorkflowCardAsksAtOnceWhenItOpensOnAWorkflowAndWhenTheTextReturnsFromTheEditor(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	opened := step(t, s, key("e"))
	ask, ok := readingAsk(t, opened)
	if !ok || ask.After != 0 || len(ask.Definition.Steps) != 2 {
		t.Fatalf("opening the card on a workflow sent %#v", opened.Cmds)
	}
	edited := workflowText(`{"key":"a","kind":"command","command":"true","options":{"profile":"x"}}`)
	back := step(t, opened.State, model.DraftEditorClosed{UID: model.WorkflowDraftUID, Text: edited})
	ask, ok = readingAsk(t, back)
	if !ok || ask.After != 0 {
		t.Fatalf("the text returning from $EDITOR sent %#v, want an ask at once", back.Cmds)
	}
}

func TestWorkflowCardNamesWhatNeedsNoWorkerTomlAndWhyTheBudgetWasNotChecked(t *testing.T) {
	text := workflowText(`{"key":"a","kind":"command","command":"true","options":{"profile":"x"}}`)
	for name, tc := range map[string]struct {
		why     string
		missing bool
		tone    string
	}{
		"missing":    {"there is no worker.toml on this machine", true, "muted"},
		"unreadable": {"worker.toml could not be read: executor is required", false, "warning"},
	} {
		t.Run(name, func(t *testing.T) {
			s := step(t, workflowsPane(t, 160), key("n")).State
			typed := typeWorkflow(t, s, text)
			ask, _ := readingAsk(t, typed)
			answered := step(t, typed.State, workersAnswer(ask, nil, tc.why, tc.missing)).State
			mustShow(t, answered, "note", "names a profile", "The prompt budget could not be checked: "+tc.why)
			if got := toneOfLine(t, answered, 2); got != tc.tone {
				t.Errorf("the budget line is drawn in tone %q, want %q", got, tc.tone)
			}
		})
	}
}

func TestWorkflowsPageInspectorShowsTheWorkersReadingAndKeepsItThroughArchiveAndRestore(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow(), nightlyFlow())
	loaded := step(t, s, model.WorkflowsLoaded{ProjectUID: "p", Workflows: []model.Workflow{reviewFlow(), nightlyFlow()}, Readings: map[string]model.WorkerReading{
		"W1": {Refusals: []string{"options.on_fail (maker/checker loops) is not supported"}},
		"W2": {},
	}}).State
	opened := step(t, loaded, key("enter")).State
	mustShow(t, opened, "W1 reading", "This machine's Worker will not run it", "options.on_fail")

	flow := reviewFlow()
	flow.EventUID, flow.Archived = "E9", true
	archived := step(t, opened, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "W1", Workflow: &flow}).State
	flow.EventUID, flow.Archived = "E10", false
	restored := step(t, archived, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpRestore, UID: "W1", Workflow: &flow}).State
	restored = step(t, restored, model.WorkflowSelectMsg{UID: "W1"}).State
	mustShow(t, restored, "kept through archive and restore", "options.on_fail")

	saved := reviewFlow()
	saved.EventUID = "E11"
	after := step(t, restored, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Workflow: &saved, Reading: &model.WorkerReading{}}).State
	mustShow(t, after, "replaced by a save", "This machine's Worker would run it.")
	if strings.Contains(screen(t, after), "options.on_fail") {
		t.Error("the saved Workflow still shows the old reading")
	}
}

func TestAWorkflowWithOptionKeysTheWorkerDoesNotKnowGetsNoRefusal(t *testing.T) {
	flow := reviewFlow()
	flow.Options = []byte(`{"mystery":true}`)
	flow.Steps[0].Options = []byte(`{"limits":{"turns":12},"other":1}`)
	five := 5
	check := worker.CheckWorkflow(flow.Definition(), &five)
	if len(check.Refusals) != 0 || check.BudgetUnchecked {
		t.Errorf("unknown option keys gave %+v", check)
	}
}

func TestWorkflowCardShowsKatasProblemWhileEditingAndClearsItWhenTheTextIsValid(t *testing.T) {
	five := 5
	s := step(t, workflowsPane(t, 160), key("n")).State
	broken := typeWorkflow(t, s, workflowText(`{"key":"a","kind":"shell"}`))
	mustShow(t, broken.State, "the problem before any save", "must be")
	if strings.Contains(screen(t, broken.State), "Worker") {
		t.Error("a Worker note is drawn under text kata refuses")
	}
	fixed := typeWorkflow(t, broken.State, workflowText(`{"key":"a","kind":"prompt","prompt":"p"}`))
	ask, ok := readingAsk(t, fixed)
	if !ok {
		t.Fatalf("text kata accepts sent no ask: %#v", fixed.Cmds)
	}
	if strings.Contains(screen(t, fixed.State), "must be") {
		t.Error("the problem is still drawn once the text is valid")
	}
	mustShow(t, step(t, fixed.State, workersAnswer(ask, &five, "", false)).State, "the Worker note", "would run it")
}

func TestWorkflowCardRefusesOptionsWrittenAsNullAndTakesOptionsLeftOut(t *testing.T) {
	for name, text := range map[string]string{
		"the workflow's": `{"name":"Flow","definition":{"version":1,"options":null,"steps":[{"key":"a","kind":"prompt","prompt":"p"}]}}`,
		"a step's":       workflowText(`{"key":"a","kind":"prompt","prompt":"p","options":null}`),
	} {
		t.Run(name, func(t *testing.T) {
			s := step(t, workflowsPane(t, 160), key("n")).State
			typed := typeWorkflow(t, s, text)
			mustShow(t, typed.State, "the problem while editing", `\"options\" must be an object.`)
			if _, ok := readingAsk(t, typed); ok {
				t.Error("text with options written as null was sent to the Worker")
			}
			if out := step(t, typed.State, key("ctrl+o")); len(out.Cmds) != 0 {
				t.Errorf("text with options written as null was saved: %#v", out.Cmds)
			}
		})
	}
	s := step(t, workflowsPane(t, 160), key("n")).State
	typed := typeWorkflow(t, s, workflowText(`{"key":"a","kind":"prompt","prompt":"p"}`))
	if strings.Contains(screen(t, typed.State), "options") {
		t.Error("options left out are refused")
	}
}

func TestWorkflowCardAsksAgainWhenTheEditorReturnsTheSameText(t *testing.T) {
	s := workflowsPane(t, 160, reviewFlow())
	opened := step(t, s, key("e"))
	if _, ok := readingAsk(t, opened); !ok {
		t.Fatalf("opening the card sent %#v", opened.Cmds)
	}
	back := step(t, opened.State, model.DraftEditorClosed{UID: model.WorkflowDraftUID, Text: reviewFlow().Text()})
	if ask, ok := readingAsk(t, back); !ok || ask.After != 0 {
		t.Fatalf("the unchanged text returning from $EDITOR sent %#v, want an ask at once", back.Cmds)
	}
}

func TestADiscardedDraftAfterAConflictLeavesTheCurrentVersionWithItsReading(t *testing.T) {
	refusal := model.WorkerReading{Refusals: []string{"options.on_fail (maker/checker loops) is not supported"}}
	newer := reviewFlow()
	newer.EventUID = "E2"
	s := workflowsPane(t, 160, reviewFlow())
	editing := step(t, s, key("e")).State
	edited := typeWorkflow(t, editing, strings.Replace(reviewFlow().Text(), "make test", "make check", 1)).State
	saving := step(t, edited, key("ctrl+o")).State
	conflict := step(t, saving, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpSave, UID: "W1", Current: &newer, Reading: &refusal}).State
	discarded := step(t, conflict, key("d")).State
	if discarded.Model.Workflows.Card.Open {
		t.Fatal("Discard Draft left the card open")
	}
	mustShow(t, step(t, discarded, key("enter")).State, "the current version's reading", "This machine's Worker will not run it", "options.on_fail")

	// A write refused while no card is saving keeps the reading of the version it found too.
	archived := step(t, s, model.WorkflowWritten{ProjectUID: "p", Op: model.WorkflowOpArchive, UID: "W1", Current: &newer, Reading: &refusal}).State
	mustShow(t, step(t, archived, key("enter")).State, "the reading after a refused archive", "options.on_fail")
}
