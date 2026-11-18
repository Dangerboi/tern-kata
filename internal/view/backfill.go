package view

import (
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func backfillLayer(m model.Model) tern.Element {
	b := m.Backfill
	if !b.Open {
		return nil
	}
	children := []tern.Element{toneText("warning", ui.ToneWarning, m.BackfillSentence())}
	// A single job is named in the sentence; a reopen names none there, so its jobs are always listed.
	if len(b.Jobs) > 1 || b.For == model.BackfillReopen {
		for i, j := range b.Jobs {
			name := ui.Text{Common: ui.Common{Key: "name", Grow: 1, Min: &ui.Bounds{W: ui.Ch(0)}}, Text: j.Job.Name}
			count := toneText("count", ui.ToneMuted, model.MissedWords(j.Missed))
			count.Shrink = new(float64)
			count.Wrap = ui.WrapNone
			children = append(children, ui.Row{Common: ui.Common{Key: fmt.Sprintf("job%d", i)}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(name, count)})
		}
	}
	create := cardButton("create", model.ModeBackfill, model.BackfillCreate, "Create all", false)
	latest := cardButton("latest", model.ModeBackfill, model.BackfillLatest, "Only the latest", true)
	cancel := exitButton("cancel", model.ModeBackfill, model.BackfillCancel, "Cancel")
	var status []tern.Element
	if b.Saving {
		pending := model.Progress(b.Verb(), "")
		create, latest, cancel = unavailable(create, pending), unavailable(latest, pending), unavailable(cancel, pending)
		status = append(status, toneText("pending", ui.TonePending, pending))
	}
	if b.Err != nil {
		status = append(status, toneText("error", ui.ToneError, b.Err.Error()))
	}
	children = append(children, status...)
	children = append(children, formFoot(latest, cancel, create))
	overlay := ui.Overlay{Common: ui.Common{Key: layerKey(model.CardBackfill)}, Modal: true, Size: ui.SizeLG, Head: ui.T(b.Head()), Icon: ui.IconHistory}
	if b.For == model.BackfillReopen {
		overlay.Meta = refMeta(m.RefName(b.Issue.ProjectName, b.Issue.ShortID))
	}
	return ui.Nodes(popup(overlay, children))
}
