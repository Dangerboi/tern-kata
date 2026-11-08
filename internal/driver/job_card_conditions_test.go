package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/view"
)

// conditionsCard is the card open on an adapter job whose options are the given.
func conditionsCard(t *testing.T, options map[string]any) string {
	t.Helper()
	job := promptJob("J1", "Nightly")
	job.Definition.Options = options
	s := step(t, jobsPane(t, 160, job), key("e")).State
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == "layer.job.conditions" || strings.HasPrefix(n.ID, "layer.job.conditions.") {
			text += wireText(n) + "\n"
		}
	}
	return text
}

func TestJobCardShowsTheConditionsOfAnAdapterJobReadOnly(t *testing.T) {
	text := conditionsCard(t, map[string]any{"conditions": map[string]any{
		"runs_per_day": float64(2), "runs_per_week": float64(5), "spend_per_day": 1.5, "spend_per_week": float64(10),
		"new_commits": map[string]any{"folders": []any{"docs", "internal/model"}},
	}})
	wantText(t, "conditions", text, "At most 2 runs per day", "At most 5 runs per week", "At most $1.5 spent per day", "At most $10 spent per week",
		"new commits since the last succeeded run in folders docs, internal/model", "A worker that does not know conditions ignores them.")
}

func TestJobCardShowsNoConditionsForAJobWithout(t *testing.T) {
	if text := conditionsCard(t, nil); text != "" {
		t.Errorf("a job without conditions shows %q", text)
	}
	if text := conditionsCard(t, map[string]any{"conditions": map[string]any{}}); text != "" {
		t.Errorf("a job with empty conditions shows %q", text)
	}
}

func TestJobCardShowsTheReadersErrorForConditionsThatDoNotParse(t *testing.T) {
	text := conditionsCard(t, map[string]any{"conditions": map[string]any{"runs_per_day": float64(0)}})
	wantText(t, "broken conditions", text, "not readable", "options.conditions.runs_per_day must be a whole number of at least 1")
	text = conditionsCard(t, map[string]any{"conditions": "often"})
	wantText(t, "conditions of the wrong type", text, "options.conditions must be an object")
}

func TestJobCardOfANewJobShowsNoConditions(t *testing.T) {
	for _, n := range wireNodes(t, view.Render(promptCard(t).Model, now)) {
		if strings.HasPrefix(n.ID, "layer.job.conditions") {
			t.Errorf("a new job's card shows %s", n.ID)
		}
	}
}
