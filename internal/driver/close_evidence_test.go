package driver_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
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

// attachedAlpha is the Pane on Issue "alpha", which has the attachments "a.txt" (id A) and "b.log" (id B).
func attachedAlpha(t *testing.T) driver.State {
	t.Helper()
	ref := func(name string) map[string]any {
		return map[string]any{"name": name, "media_type": "text/plain", "size": 1.0, "docbank_version": "6f1c2e8a-2222-4222-8222-000000000001"}
	}
	issue := alpha
	issue.Metadata = map[string]any{"attachment.A": ref("a.txt"), "attachment.B": ref("b.log")}
	project := model.Project{ID: 7, UID: "p", Name: "work"}
	snapshot := model.Snapshot{Issues: []model.Issue{issue, beta}, Catalog: []model.CatalogProject{{Project: project}}}
	return loadedWith(t, 160, project, snapshot, model.DetailLoaded{UID: "alpha", Selected: &model.IssueDetailData{Issue: &issue}})
}

func drawsPicker(t *testing.T, s driver.State) bool {
	t.Helper()
	for _, n := range drawnNodes(t, s) {
		if n.ID == view.FieldID(model.CardClose, model.CloseAttachments) {
			return true
		}
	}
	return false
}

func TestCloseCardDrawsTheAttachmentPickerAndSendsTheCitedAttachments(t *testing.T) {
	s := feed(t, attachedAlpha(t), key("x")).State
	if !drawsPicker(t, s) {
		t.Fatal("the Close card of an Issue with attachments draws no picker")
	}
	s = step(t, s, model.CloseFieldMsg{Field: model.CloseMessage, Value: "Implemented the requested change and verified the result."}).State
	s = step(t, s, model.FocusFieldMsg{Field: model.CloseAttachments}).State
	s = step(t, s, key("right")).State
	s = step(t, s, key("space")).State
	out := step(t, s, key("ctrl+o"))
	cmd, ok := closeCmd(t, out)
	want := []model.CloseEvidence{{Kind: model.EvidenceAttachment, Value: "B"}}
	if !ok || cmd.Reason != "done" || !reflect.DeepEqual(cmd.Evidence, want) || out.State.Model.CloseForm.Err != nil {
		t.Fatalf("sent %+v (sent %v), error %v, want evidence %v", cmd, ok, out.State.Model.CloseForm.Err, want)
	}
}

func TestCloseCardDrawsNoAttachmentPickerWithoutAttachmentsOrForAnotherReason(t *testing.T) {
	if s := feed(t, browsing(t, 160), key("x")).State; drawsPicker(t, s) {
		t.Fatal("an Issue with no attachments is drawn a picker")
	}
	s := feed(t, attachedAlpha(t), key("x")).State
	s = step(t, s, model.CloseFieldMsg{Field: model.CloseReason, Value: "duplicate"}).State
	if drawsPicker(t, s) {
		t.Fatal("a duplicate close is drawn the picker")
	}
	s = step(t, s, model.CloseFieldMsg{Field: model.CloseReason, Value: "done"}).State
	if !drawsPicker(t, s) {
		t.Fatal("a done close lost the picker")
	}
}

func TestTheCloseCardOfAnUndoOffersTheAttachmentPicker(t *testing.T) {
	s := attachedAlpha(t)
	entry := undoEntry(2)
	entry.Kind = model.UndoReopen
	s = feed(t, s, model.FieldWritten{UID: "alpha", Record: model.WriteRecord{Entry: &entry}}).State
	out := pressU(t, s)
	cmd := undoCmd(t, out)
	s = feed(t, out.State, model.ChangeUndone{EntryID: cmd.Entry.ID, Result: model.UndoNeedsEvidence, Ref: "aaaa"}).State
	if !s.Model.CloseForm.Open || !drawsPicker(t, s) {
		t.Fatalf("the undo's Close card %+v draws no picker", s.Model.CloseForm)
	}
	s = feed(t, s,
		model.CloseFieldMsg{Field: model.CloseMessage, Value: "Finished again after the reopen, so closing it as done once more."},
		model.ChooseMsg{Field: model.CloseAttachments, Index: 0},
	).State
	sent := undoCmd(t, feed(t, s, key("ctrl+o")))
	if sent.Close == nil || !reflect.DeepEqual(sent.Close.Evidence, []model.CloseEvidence{{Kind: model.EvidenceAttachment, Value: "A"}}) {
		t.Fatalf("the card sent %+v", sent)
	}
}
