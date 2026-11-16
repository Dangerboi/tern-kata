package view

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// choiceControlKey is the key of the control inside a choice field's FormField, so the control's node id is the field's id and
// this key.
const choiceControlKey = "control"

// ControlID is the node id of the control of the choice field called name of card.
func ControlID(card model.Card, name model.Field) string {
	return itemID(FieldID(card, name), choiceControlKey)
}

// controlField is the field a control's node id belongs to, the inverse of ControlID: any single-segment field of a card's
// layer, since the field list of a card does not name every choice field (the Move card's switch).
func controlField(id string) (model.Field, bool) {
	fieldID, ok := strings.CutSuffix(id, itemID("", choiceControlKey))
	if !ok {
		return "", false
	}
	for _, card := range model.Cards() {
		if name, ok := strings.CutPrefix(fieldID, itemID(LayerID(card), "")); ok && name != "" && !strings.Contains(name, ".") {
			return model.Field(name), true
		}
	}
	return "", false
}

// CloseSelectTarget is the select node a CloseSelectMenu goes to (the control inside the field), "" when m draws no such
// select (a key closed the card with the menu), so no op names a node Tern no longer has.
func CloseSelectTarget(m model.Model, card model.Card, name model.Field) string {
	if !m.SelectDrawn(card, name) {
		return ""
	}
	return ControlID(card, name)
}

// The control never takes focus: the model owns the keys, and the select draws the menu, highlight and search text the model holds.
func choiceControl(m model.Model, card model.Card, name model.Field, title string, focused bool, hint, warning string) tern.Element {
	set, ok := m.CardChoices(card, name)
	if !ok {
		return nil
	}
	label := title
	tone := ui.ToneNeutral
	if focused {
		label, tone = "> "+title, ui.ToneAccent
	}
	field := ui.FormField{Common: ui.Common{Key: string(name), Tone: tone, Aria: title}, Label: ui.T(label)}
	if hint != "" {
		field.Hint = ui.T(hint)
	}
	if warning != "" {
		field.Warning = ui.Says(ui.T(warning))
	}
	common := ui.Common{Key: choiceControlKey, Aria: title}
	switch set.Control {
	case model.ControlTabs:
		items := make([]ui.Tab, len(set.Labels))
		for i, text := range set.Labels {
			items[i] = ui.Tab{ID: strconv.Itoa(i), Label: ui.T(text)}
		}
		field.Children = ui.Nodes(ui.Tabs{Common: common, Items: items, Active: choiceValue(set.Chosen), Variant: ui.TabsSegmented})
	case model.ControlSwitch:
		field.Children = ui.Nodes(ui.Switch{Common: common, On: set.On(), Variant: ui.SwitchSegment, Labels: set.SwitchLabels()})
	default:
		menu := m.OpenDropdown(card, name)
		sel := ui.Select{Common: common, Value: choiceValue(set.Chosen), Open: m.SelectShown(card, name), Search: set.Search()}
		sel.Options = make([]ui.SelectOption, 0, len(set.Labels))
		if sel.Search && menu.Query != "" {
			sel.Query = menu.Query
			total := len(set.Labels)
			sel.Total = &total
			for _, at := range set.Matches(menu.Query) {
				sel.Options = append(sel.Options, ui.SelectOption{Value: strconv.Itoa(at), Label: set.Labels[at]})
			}
		} else {
			for at, label := range set.Labels {
				sel.Options = append(sel.Options, ui.SelectOption{Value: strconv.Itoa(at), Label: label})
			}
		}
		// The highlight of a menu the pointer opened is Tern's until a key moves it: any change to the select right after the
		// click (as an active prop) lost the option click that followed it now and then.
		if menu.Open && (sel.Open || menu.Keyed) {
			sel.Active = choiceValue(menu.Active)
		}
		field.Children = ui.Nodes(sel)
	}
	return field
}

func choiceValue(index int) string {
	if index < 0 {
		return ""
	}
	return strconv.Itoa(index)
}

// The choices wrap in their own column, so a wrapped line never reads as another field.
func choiceField(name model.Field, tone ui.Tone, title, prompt string, labels []string, chosen func(int) bool, cursor int) tern.Element {
	choices := make([]tern.Element, 0, len(labels))
	for i, label := range labels {
		style := ""
		if chosen(i) {
			label, style = "[ "+label+" ]", "accent strong"
		}
		if i == cursor {
			label = "▸" + label
		}
		choices = append(choices, ui.Text{Common: ui.Common{Key: strconv.Itoa(i), Actions: &ui.Actions{Click: ChooseClick(name, i)}}, Spans: []ui.Span{{T: label, S: style}}, Wrap: ui.WrapNone})
	}
	return ui.Row{Common: ui.Common{Key: string(name), Tone: tone, Aria: title}, Gap: ui.GapMD, Align: ui.AlignStart, Children: ui.Nodes(
		ui.Text{Common: ui.Common{Key: "prompt", Shrink: new(float64)}, Text: prompt, Wrap: ui.WrapNone},
		ui.Row{Common: ui.Common{Key: "choices", Grow: 1}, Gap: ui.GapMD, Wrap: true, Children: choices})}
}

// An action with no binding in mode has no keys and no chord.
func cardKeys(mode model.Mode, action model.Action) (keys []string, chord string) {
	bindings := model.CardKeys(mode, action)
	if len(bindings) > 0 {
		chord = bindings[0].Chord
	}
	return keycaps(bindings), chord
}

func keycaps(bindings []model.Binding) []string {
	keys := make([]string, len(bindings))
	for i, b := range bindings {
		keys[i] = keycap(b.Chord, b.Key)
	}
	return keys
}

// keycap is the keycap text of a binding's key label. Tern draws letter keycaps in capitals, so a shifted letter is "⇧" and the
// capital, matching the plain letter's keycap.
func keycap(chord, key string) string {
	if letter, ok := strings.CutPrefix(chord, "shift+"); ok && utf8.RuneCountInString(letter) == 1 && unicode.IsLetter([]rune(letter)[0]) {
		return "⇧" + strings.ToUpper(letter)
	}
	return key
}

// cardButton is a card's Tern button for action in mode. Its click presses the action's first non-headless chord with the card's overlays closed
// and its keycaps are every non-headless key label of the action, so a click and the key never disagree and a rebind moves
// both. An action with no binding in its mode gets no chord, which the driver's card test fails.
func cardButton(key string, mode model.Mode, action model.Action, words string, primary bool) ui.Button {
	keys, chord := cardKeys(mode, action)
	button := ui.Button{Common: ui.Common{Key: key, Actions: &ui.Actions{Click: ButtonClick(chord)}}, Text: words, Keys: keys}
	if primary {
		button.Variant = ui.ButtonPrimary
	}
	return button
}

func destructive(button ui.Button) ui.Button {
	button.Variant = ui.ButtonDanger
	return button
}

// An empty reason leaves the button enabled.
func unavailable(button ui.Button, reason string) ui.Button {
	if reason != "" {
		button.Disabled = ui.Because(reason)
		if button.Variant == ui.ButtonPrimary {
			button.Variant = ui.ButtonDefault
		}
	}
	return button
}

func menuButton(m model.Model) ui.Button {
	return ui.Button{Common: ui.Common{Key: "menu", Shrink: new(float64), Actions: &ui.Actions{Click: MenuClick}}, Text: "Menu", Keys: keycaps(m.MenuKeys())}
}

// footKey is the key of a card's button row while the card is built. Tern keys the footer slot itself, so the buttons' ids are
// the overlay's id, ".foot" and the button's key.
const footKey = "foot"

// buttonRow is the row of buttons that ends a card. It is placed anywhere among a card's children; popup moves it to the footer.
func buttonRow(buttons ...tern.Element) tern.Element {
	return ui.Row{Common: ui.Common{Key: footKey}, Gap: ui.GapMD, Justify: ui.JustifyEnd, Wrap: true, Children: buttons}
}

func popup(overlay ui.Overlay, children []tern.Element) ui.Overlay {
	body := make([]tern.Element, 0, len(children))
	for _, child := range children {
		if row, ok := child.(ui.Row); ok && row.Key == footKey {
			overlay.Foot = row
			continue
		}
		body = append(body, child)
	}
	overlay.Children = body
	return overlay
}

func discardButtons() tern.Element {
	return buttonRow(destructive(cardButton("discard", model.ModeDiscard, model.DiscardDraft, "Discard", false)), cardButton("keep", model.ModeDiscard, model.KeepDraft, "Keep editing", true))
}

func refMeta(ref string) ui.Rich {
	return ui.Spans(ui.Span{T: ref, S: "mono"})
}
