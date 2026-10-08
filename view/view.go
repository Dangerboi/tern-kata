package view

import (
	"fmt"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

const IssueListID = "main.issues"

func IssueID(uid string) string { return IssueListID + "." + uid }

func Render(m model.Model) tern.View {
	main := ui.Nodes(ui.Text{Common: ui.Common{Key: "heading"}, Text: m.Project.Name + " / Open Issues"})
	switch {
	case m.Err != nil:
		main = failureView(m)
	case m.Loading:
		main = ui.Nodes(ui.Text{Common: ui.Common{Key: "loading", Tone: ui.TonePending}, Text: "Loading Project's open Issues…"})
	default:
		items := make([]tern.Element, 0, len(m.Issues))
		for _, issue := range m.Issues {
			items = append(items, ui.Item{
				Common: ui.Common{Key: issue.UID},
				Label:  ui.T(issue.ShortID + "  " + issue.Title),
			})
		}
		selected := ""
		if m.SelectedUID != "" {
			selected = IssueID(m.SelectedUID)
		}
		main = append(main, ui.List{
			CommonWithoutMax: ui.CommonWithoutMax{Key: "issues"},
			Selected:         selected,
			Empty:            ui.T("No open Issues in this Project. Create one with kata create."),
			Children:         items,
		})
	}
	if m.Stale && m.Err == nil && !m.Loading {
		main = append(main, staleNotice(m))
	}
	return tern.View{Main: main, Dock: footer(m)}
}

func footer(m model.Model) tern.Element {
	segments := make([]tern.Element, 0, len(model.Keymap)+1)
	for _, binding := range model.Keymap {
		if binding.Action == model.RetryAction && m.Err == nil {
			continue
		}
		segments = append(segments, ui.Seg{Common: ui.Common{Key: binding.Chord}, Text: binding.Key + " " + binding.Help})
	}
	selected := 0
	for i, issue := range m.Issues {
		if issue.UID == m.SelectedUID {
			selected = i + 1
			break
		}
	}
	segments = append(segments, ui.Seg{Common: ui.Common{Key: "position"}, Side: ui.SideRight, Text: fmt.Sprintf("%d / %d", selected, len(m.Issues))})
	return ui.Nodes(ui.Status{Common: ui.Common{Key: "hints"}, Children: segments})
}
