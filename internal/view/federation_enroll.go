package view

import (
	"fmt"
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// FederationChoiceID is the id of the row at index of the enrollment's choices or of the browse.
func FederationChoiceID(index int) string {
	return itemID(FederationListID, "choice"+strconv.Itoa(index))
}

// federationEnrollScreen draws the enrollment's screens and the browse, under the Page's heading and header: the choices as a
// list of cards, the preview, the recovery and the result as lines. It reports false for any other screen.
func federationEnrollScreen(m model.Model) ([]tern.Element, bool) {
	f := m.Federation
	title := func(text string) []tern.Element {
		return ui.Nodes(
			ui.Text{Common: ui.Common{Key: "selected", Tone: ui.ToneMuted}, Text: "selected project: " + m.FederationSelectedProject()},
			ui.Text{Common: ui.Common{Key: "title", Tone: ui.ToneMuted}, Spans: []ui.Span{{T: text, S: "strong"}}, Wrap: ui.WrapNone})
	}
	line := func(key, text string) tern.Element {
		return ui.Text{Common: ui.Common{Key: key}, Text: text, Wrap: ui.WrapNone}
	}
	failure := func() []tern.Element {
		if f.EnrollErr == nil {
			return nil
		}
		return ui.Nodes(ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.EnrollErr.Error()})
	}
	switch f.Screen {
	case model.FederationPickLocal:
		var labels []string
		for _, c := range m.FederationLocalChoices() {
			labels = append(labels, c.Label())
		}
		out := append(title("Select local spoke project"), choiceList(m, labels)...)
		return append(out, failure()...), true
	case model.FederationPickHub:
		out := title("Select hub daemon")
		if f.Draft.SpokeProjectName != "" {
			out = append(out, line("local", "local spoke project: "+f.Draft.SpokeProjectName))
		}
		if f.Draft.CreateReplica {
			out = append(out, line("local", "local spoke project: create after selecting hub project"))
		}
		var labels []string
		for _, h := range f.Hubs {
			labels = append(labels, h.Label())
		}
		switch {
		case f.HubsLoading:
			out = append(out, ui.Text{Common: ui.Common{Key: "notice", Tone: ui.TonePending}, Text: "loading hub daemons..."})
		case len(labels) == 0 && f.EnrollErr == nil:
			out = append(out, ui.Text{Common: ui.Common{Key: "notice", Tone: ui.ToneMuted}, Text: "no daemons in kata's catalog"})
		default:
			out = append(out, choiceList(m, labels)...)
		}
		return append(out, failure()...), true
	case model.FederationPickHubProject:
		hub := f.Draft.Hub
		auth := f.Draft.HubAuth
		if auth == "" {
			auth = hub.Auth
		}
		out := append(title("Select hub project"),
			line("hub", "hub daemon: "+hub.Name+" "+hub.Endpoint),
			line("auth", "hub auth: "+auth),
			line("insecure", "allow_insecure: "+strconv.FormatBool(hub.AllowInsecure)))
		var labels []string
		for _, c := range m.FederationHubProjectChoices() {
			labels = append(labels, c.Label)
		}
		switch {
		case f.HubProjectsLoading:
			out = append(out, ui.Text{Common: ui.Common{Key: "notice", Tone: ui.TonePending}, Text: "loading hub projects..."})
		case len(labels) == 0:
			out = append(out, ui.Text{Common: ui.Common{Key: "notice", Tone: ui.ToneMuted}, Text: "no hub projects"})
		default:
			out = append(out, choiceList(m, labels)...)
		}
		return append(out, failure()...), true
	case model.FederationBrowse:
		out := title("Browse catalog hub projects")
		for i, hub := range f.Browsed {
			out = append(out,
				line("hub"+strconv.Itoa(i), "catalog hub: "+hub.Hub.Name+" "+hub.Hub.Endpoint+" · auth "+hub.Auth+" · allow_insecure: "+strconv.FormatBool(hub.Hub.AllowInsecure)))
			if hub.Err != nil {
				out = append(out, ui.Text{Common: ui.Common{Key: "failed" + strconv.Itoa(i), Tone: ui.ToneError}, Text: "failed to load hub projects: " + hub.Err.Error()})
			}
		}
		out = append(out, line("mode", "mode: read-only"))
		var labels []string
		for _, row := range f.BrowseRows() {
			labels = append(labels, f.Browsed[row.Hub].Hub.Name+"  "+row.Label())
		}
		switch {
		case f.Browsing:
			out = append(out, ui.Text{Common: ui.Common{Key: "notice", Tone: ui.TonePending}, Text: "loading hub projects..."})
		case f.EnrollErr != nil:
			out = append(out, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: "failed to load hub projects: " + f.EnrollErr.Error()})
		case len(labels) == 0:
			out = append(out, ui.Text{Common: ui.Common{Key: "notice", Tone: ui.ToneMuted}, Text: "no hub projects"})
		default:
			out = append(out, choiceList(m, labels)...)
		}
		return out, true
	case model.FederationPreview:
		out := append(title("Enrollment Preview"), federationLines(f.Draft.PreviewLines(), -1)...)
		if f.Draft.BlockedReason != "" {
			out = append(out, ui.Text{Common: ui.Common{Key: "blocked", Tone: ui.ToneError}, Text: "Blocked: " + f.Draft.BlockedReason})
		}
		out = append(out, failure()...)
		if f.Enrolling {
			out = append(out, ui.Text{Common: ui.Common{Key: "running", Tone: ui.TonePending}, Text: "enrolling..."})
		}
		return out, true
	case model.FederationRecovery:
		lines, errLine := f.Recovery.Lines()
		out := append(title("Enrollment Recovery"), federationLines(lines, errLine)...)
		if f.Recovery.Reveal {
			revealed := f.Recovery.RevealedLines()
			out = append(out, ui.Text{Common: ui.Common{Key: "reveal-warning", Tone: ui.ToneError}, Text: revealed[0]})
			out = append(out, line("reveal-valid", revealed[1]), line("reveal-target", revealed[2]),
				ui.Text{Common: ui.Common{Key: "reveal-command"}, Text: revealed[3]})
		}
		return out, true
	case model.FederationEnrollDone:
		return append(title("Enrollment Result"), federationLines(f.Enrolled.ResultLines(), -1)...), true
	}
	return nil, false
}

// choiceList is the rows of a choice screen as cards in the Page's list, the selected one marked.
func choiceList(m model.Model, labels []string) []tern.Element {
	cursor := m.FederationChoiceCursor()
	cards := make([]tern.Element, len(labels))
	for i, label := range labels {
		cards[i] = ui.Card{Common: ui.Common{Key: "choice" + strconv.Itoa(i)}, Selected: i == cursor,
			Children: ui.Nodes(ui.Text{Common: ui.Common{Key: "label"}, Text: label, Wrap: ui.WrapNone})}
	}
	return ui.Nodes(ui.List{CommonWithoutMax: scrollerCommon("federation"), Max: scrollMax(m, 2*(m.RowPx+6+8)), Children: cards})
}

// federationAdoptLayer is the card asking to type the local Project's name before an adoption.
func federationAdoptLayer(m model.Model) tern.Element {
	f := m.Federation
	if !f.Adopting {
		return nil
	}
	name, hub := f.Draft.SpokeProjectName, f.Draft.HubProjectName
	children := []tern.Element{
		ui.Text{Common: ui.Common{Key: "question"}, Text: fmt.Sprintf("federate local project %q INTO hub project %q?", name, hub)},
		ui.Text{Common: ui.Common{Key: "consequence", Tone: ui.ToneMuted}, Text: fmt.Sprintf("%q's issues will be adopted into the hub project and its local event history rewritten as snapshots", name)},
		ui.Text{Common: ui.Common{Key: "prompt"}, Text: fmt.Sprintf("type %q to confirm:", name)},
		ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Local Project name"}, Field: ui.Field{Text: f.AdoptInput, Cursor: caretAt(f.AdoptCaret, "", f.AdoptInput), Mode: "insert", Placeholder: "Project name"}},
	}
	if f.AdoptErr != nil {
		children = append(children, ui.Text{Common: ui.Common{Key: "error", Tone: ui.ToneError}, Text: f.AdoptErr.Error()})
	}
	children = append(children, buttonRow(
		cardButton("adopt", model.ModeFederationAdopt, model.FederationAdoptConfirm, "Adopt", true),
		cardButton("back", model.ModeFederationAdopt, model.FederationAdoptCancel, "Back", false)))
	return ui.Nodes(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardFederationAdopt)}, Head: ui.T("Confirm Adoption"), Modal: true, Size: ui.SizeMD, Children: children})
}
