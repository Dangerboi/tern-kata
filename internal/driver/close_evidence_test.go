package driver_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func closeCmd(t *testing.T, out driver.Output) (model.CloseIssueCmd, bool) {
	t.Helper()
	for _, c := range out.Cmds {
		if cmd, ok := c.(model.CloseIssueCmd); ok {
			return cmd, true
		}
	}
	return model.CloseIssueCmd{}, false
}

func TestCloseCardSendsReviewedPathsAndExternalEvidence(t *testing.T) {
	message := "Implemented the requested change and verified the result."
	for _, tt := range []struct {
		name  string
		field model.Field
		value string
		want  model.CloseEvidence
	}{
		{"reviewed paths", model.CloseReviewed, " src/a.go , src/b.go ", model.CloseEvidence{Kind: model.EvidenceReviewed, Value: "src/a.go , src/b.go"}},
		{"external", model.CloseExternal, " alice ", model.CloseEvidence{Kind: model.EvidenceExternal, Value: "alice"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(t, browsing(t, 160), key("x")).State
			s = step(t, s, model.CloseFieldMsg{Field: model.CloseMessage, Value: message}).State
			s = step(t, s, model.CloseFieldMsg{Field: tt.field, Value: tt.value}).State
			out := step(t, s, key("ctrl+o"))
			cmd, ok := closeCmd(t, out)
			if !ok || cmd.Reason != "done" || len(cmd.Evidence) != 1 || cmd.Evidence[0] != tt.want || out.State.Model.CloseForm.Err != nil {
				t.Fatalf("sent %+v (sent %v), error %v, want evidence %+v", cmd, ok, out.State.Model.CloseForm.Err, tt.want)
			}
		})
	}
}

func TestCloseCardRefusesAnEmptyReviewedPathsOrExternalValue(t *testing.T) {
	message := "Implemented the requested change and verified the result."
	for _, tt := range []struct {
		name  string
		field model.Field
		value string
		want  string
	}{
		{"reviewed paths blank", model.CloseReviewed, "  ", "requires commit"},
		{"reviewed paths only commas", model.CloseReviewed, " , ,", "at least one path"},
		{"external blank", model.CloseExternal, "  ", "requires commit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := feed(t, browsing(t, 160), key("x")).State
			s = step(t, s, model.CloseFieldMsg{Field: model.CloseMessage, Value: message}).State
			s = step(t, s, model.CloseFieldMsg{Field: tt.field, Value: tt.value}).State
			out := step(t, s, key("ctrl+o"))
			if _, ok := closeCmd(t, out); ok || out.State.Model.CloseForm.Err == nil || !strings.Contains(out.State.Model.CloseForm.Err.Error(), tt.want) {
				t.Fatalf("sent %v, error %v, want a refusal naming %q", ok, out.State.Model.CloseForm.Err, tt.want)
			}
		})
	}
}
