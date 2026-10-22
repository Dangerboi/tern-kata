package view

import (
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// backfillLayer is the Backfill card: what the step about to be taken would make the daemon create at once, job by job, and
// the three ways out.
func backfillLayer(m model.Model) tern.Element {
	b := m.Backfill
	if !b.Open {
		return nil
	}
	head, step, latest := "Resume job?", "Resuming", "Switch to latest and resume"
	switch b.For {
	case model.BackfillRestore:
		head, step, latest = "Restore job?", "Restoring", "Switch to latest and restore"
	case model.BackfillSave:
		head, step, latest = "Save and resume job?", "Resuming", "Switch to latest and save"
	case model.BackfillReopen:
		head, step, latest = "Reopen Issue?", "Reopening", "Switch to latest and reopen"
	}
	subject := "the job"
	if b.For == model.BackfillReopen {
		subject = "#" + b.Issue.ShortID + " " + b.Issue.Title
	} else if len(b.Jobs) == 1 {
		subject = fmt.Sprintf("job %q", b.Jobs[0].Job.Name)
	}
	children := []tern.Element{
		ui.Text{Common: ui.Common{Key: "warning", Tone: ui.ToneWarning}, Text: fmt.Sprintf("%s %s would create %s at once: its catch-up is every missed run.", step, subject, missedIssues(b.Total()))},
	}
	for i, j := range b.Jobs {
		name := ui.Text{Common: ui.Common{Key: "name", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Text: j.Job.Name}
		count := ui.Text{Common: ui.Common{Key: "count", Shrink: new(float64), Tone: ui.ToneInfo}, Text: model.MissedWords(j.Missed), Wrap: ui.WrapNone}
		children = append(children, ui.Row{Common: ui.Common{Key: fmt.Sprintf("job%d", i)}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(name, count)})
	}
	if b.Err != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: b.Err.Error()})
	}
	if b.Saving {
		return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "backfill"}, Modal: true, Size: ui.SizeLG, Head: ui.T(head),
			Children: append(children, ui.Text{Common: ui.Common{Key: "pending", Tone: ui.TonePending}, Text: "Switching to latest…"})})
	}
	children = append(children, buttonRow(
		cardButton("create", model.ModeBackfill, model.BackfillCreate, "Create them", false),
		cardButton("latest", model.ModeBackfill, model.BackfillLatest, latest, true),
		cardButton("cancel", model.ModeBackfill, model.BackfillCancel, "Cancel", false)))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: "backfill"}, Modal: true, Size: ui.SizeLG, Head: ui.T(head), Children: children})
}

// missedIssues is how many child Issues a count of missed runs makes: "1 child Issue", "3 child Issues".
func missedIssues(n int) string {
	switch {
	case n >= 10000:
		return "10000 or more child Issues"
	case n == 1:
		return "1 child Issue"
	}
	return fmt.Sprintf("%d child Issues", n)
}
