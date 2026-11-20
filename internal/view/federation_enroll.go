package view

import (
	"fmt"
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func FederationChoiceID(index int) string {
	return itemID(FederationListID, "choice"+strconv.Itoa(index))
}

// federationEnrollScreen is the screen of the enrollment, the browse or the recovery the Federation Page is on, after the Page
// head; false when the Page is on none of them. Each screen has a title in strong text, then the Project being enrolled, then its
// own rows or choices.
func federationEnrollScreen(m model.Model) ([]tern.Element, bool) {
	f := m.Federation
	title := func(text string) []tern.Element {
		out := ui.Nodes(ui.Text{Common: ui.Common{Key: "title"}, Spans: []ui.Span{{T: text, S: "strong"}}, Wrap: ui.WrapNone})
		if project := m.FederationSelectedProject(); project != "none" {
			out = append(out, keyedGrid("selected", propRow("Project", ui.Text{Common: ui.Common{Key: "v"}, Text: project})))
		}
		return out
	}
	failure := func() []tern.Element {
		if text := f.EnrollFailure(); text != "" {
			return ui.Nodes(toneText("error", ui.ToneError, text))
		}
		return nil
	}
	switch f.Screen {
	case model.FederationPickLocal:
		var labels []string
		for _, c := range m.FederationLocalChoices() {
			labels = append(labels, c.Label())
		}
		out := append(title("Pick the local Project"), choiceList(m, labels)...)
		return append(out, failure()...), true
	case model.FederationPickHub:
		out := title("Pick the hub daemon")
		var labels []string
		for _, h := range f.Hubs {
			labels = append(labels, h.Label())
		}
		switch {
		case f.HubsLoading:
			out = append(out, plainText("notice", ui.TonePending, model.Progress("Loading", "hub daemons")))
		case len(labels) == 0 && f.EnrollErr == nil:
			out = append(out, plainText("notice", ui.ToneMuted, "The catalog names no hub daemons."))
		default:
			out = append(out, choiceList(m, labels)...)
		}
		return append(out, failure()...), true
	case model.FederationPickHubProject:
		hub := f.Draft.Hub
		out := append(title("Pick the hub Project"), keyedGrid("hub", factPropRows([]model.FactRow{
			{Key: "Hub", Value: hub.Name},
			{Key: "Endpoint", Value: hub.Endpoint, Mono: true},
			{Key: "Auth", Value: authWords(f.Draft.HubAuth, hub.Auth)},
			{Key: "Insecure connection", Value: model.InsecureWords(hub.AllowInsecure)},
		})...))
		var labels []string
		for _, c := range m.FederationHubProjectChoices() {
			labels = append(labels, c.Label)
		}
		switch {
		case f.HubProjectsLoading:
			out = append(out, plainText("notice", ui.TonePending, model.Progress("Loading", "hub Projects")))
		case len(labels) == 0:
			out = append(out, plainText("notice", ui.ToneMuted, "The hub has no Projects."))
		default:
			out = append(out, choiceList(m, labels)...)
		}
		return append(out, failure()...), true
	case model.FederationBrowse:
		out := title("Browse hub Projects")
		for i, hub := range f.Browsed {
			out = append(out, ui.Text{Common: ui.Common{Key: "hub" + strconv.Itoa(i)}, Spans: []ui.Span{
				{T: hub.Hub.Name, S: "strong"}, {T: " · " + hub.Hub.Facts(hub.Auth), S: "muted"}}, Wrap: ui.WrapNone})
			if hub.Err != nil {
				out = append(out, toneText("failed"+strconv.Itoa(i), ui.ToneError, model.Cannot("read", "hub Projects", hub.Err)))
			}
		}
		out = append(out, plainText("mode", ui.ToneMuted, "Read only."))
		var labels []string
		for _, row := range f.BrowseRows() {
			labels = append(labels, f.Browsed[row.Hub].Hub.Name+" · "+row.Label())
		}
		switch {
		case f.Browsing:
			out = append(out, plainText("notice", ui.TonePending, model.Progress("Loading", "hub Projects")))
		case f.EnrollErr != nil:
			out = append(out, toneText("error", ui.ToneError, f.EnrollFailure()))
		case len(labels) == 0:
			out = append(out, plainText("notice", ui.ToneMuted, "No hub Projects."))
		default:
			out = append(out, choiceList(m, labels)...)
		}
		return out, true
	case model.FederationPreview:
		out := append(title("Enrollment preview"), factRows("", f.Draft.PreviewRows())...)
		if f.Draft.BlockedReason != "" {
			out = append(out, toneText("blocked", ui.ToneError, model.Refused("enroll", "", f.Draft.BlockedReason)))
		}
		out = append(out, failure()...)
		if f.Enrolling {
			out = append(out, plainText("running", ui.TonePending, model.Progress("Enrolling", "")))
		}
		return out, true
	case model.FederationRecovery:
		out := append(title("Enrollment recovery"), factRows("", f.Recovery.Rows())...)
		if f.Recovery.Reveal {
			out = append(out, factRows("reveal-", f.Recovery.RevealedRows())...)
			out = append(out, ui.Text{Common: ui.Common{Key: "reveal-command"}, Spans: []ui.Span{monoSpan(f.Recovery.Command.String(), "")}})
		}
		return out, true
	case model.FederationEnrollDone:
		return append(title("Enrollment result"), factRows("", f.Enrolled.ResultRows())...), true
	}
	return nil, false
}

// authWords is how the hub words its authentication: the read's wording when it changed it, else the catalog's.
func authWords(read, catalog string) string {
	if read != "" {
		return read
	}
	return catalog
}

func choiceList(m model.Model, labels []string) []tern.Element {
	cursor := m.FederationChoiceCursor()
	cards := make([]tern.Element, len(labels))
	for i, label := range labels {
		cards[i] = ui.Card{Common: ui.Common{Key: "choice" + strconv.Itoa(i)}, Selected: i == cursor,
			Children: ui.Nodes(ui.Text{Common: ui.Common{Key: "label"}, Text: label, Wrap: ui.WrapNone})}
	}
	return ui.Nodes(ui.List{CommonWithoutMax: scrollerCommon("federation"), Max: fillMax, Children: cards})
}

// federationAdoptLayer is the question that ends an adoption: it names both Projects, says what the adoption does to the local
// Project's history, and asks to type the local Project's name.
func federationAdoptLayer(m model.Model) tern.Element {
	f := m.Federation
	if !f.Adopting {
		return nil
	}
	name, hub := f.Draft.SpokeProjectName, f.Draft.HubProjectName
	input := ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Confirm name"}, Field: ui.Field{Text: f.AdoptInput, Cursor: caretAt(f.AdoptCaret, "", f.AdoptInput), Mode: "insert", Placeholder: ui.T("Project name")}}
	children := []tern.Element{
		toneText("question", ui.ToneWarning, fmt.Sprintf("Adopt %q into the hub Project %q?", name, hub)),
		toneText("consequence", ui.ToneMuted, "Its Issues move to the hub and the local event history becomes snapshots."),
	}
	children = append(children, labelledInput(inputLeaf, "Confirm name", fmt.Sprintf("Type the Project name, %q.", name), true, input)...)
	if f.AdoptErr != nil {
		children = append(children, toneText("error", ui.ToneError, model.Cannot("adopt", "", f.AdoptErr)))
	}
	children = append(children, confirmFoot(
		cardButton("adopt", model.ModeFederationAdopt, model.FederationAdoptConfirm, "Adopt", false),
		exitButton("cancel", model.ModeFederationAdopt, model.FederationAdoptCancel, "Cancel"), true))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardFederationAdopt)}, Head: ui.T("Adopt into hub"), Modal: true, Size: ui.SizeMD}, children))
}

// federationNameLayer is the card asking for a spoke's name: for the enrollment being previewed, or for an enrolled spoke, where
// it runs on the hub and an empty name clears the spoke's.
func federationNameLayer(m model.Model) tern.Element {
	n := m.Federation.Name
	if !n.Open {
		return nil
	}
	head, hint := "Name spoke", "A letter, then up to 31 lowercase letters, digits or hyphens. Empty names none."
	if n.Rename {
		head, hint = "Rename spoke", "A letter, then up to 31 lowercase letters, digits or hyphens. Empty clears it."
	}
	input := ui.Input{Common: ui.Common{Key: inputLeaf, Tone: ui.ToneAccent, Aria: "Spoke name"}, Field: ui.Field{Text: n.Input, Cursor: caretAt(n.Caret, "", n.Input), Mode: "insert", Readonly: n.Saving}}
	children := labelledInput(inputLeaf, "Spoke name", hint, true, input)
	if n.Saving {
		children = append(children, toneText("saving", ui.TonePending, model.Progress("Saving", "")))
	}
	if failure := n.Failure(); failure != "" {
		children = append(children, toneText("error", ui.ToneError, failure))
	}
	children = append(children, formFoot(
		cardButton("save", model.ModeFederationName, model.FederationNameSave, "Save", true),
		exitButton("cancel", model.ModeFederationName, model.FederationNameCancel, "Cancel")))
	return ui.Nodes(popup(ui.Overlay{Common: ui.Common{Key: layerKey(model.CardFederationName)}, Head: ui.T(head), Modal: true, Size: ui.SizeMD}, children))
}
