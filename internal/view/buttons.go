package view

import (
	"strconv"
	"strings"
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
		tone = ui.ToneAccent
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

// The choices wrap in their own column, so a wrapped line never reads as another field. The chosen choice is accent and strong;
// the cursor "▸" marks the focused choice only.
func choiceField(name model.Field, tone ui.Tone, title, prompt string, labels []string, chosen func(int) bool, cursor int) tern.Element {
	choices := make([]tern.Element, 0, len(labels))
	for i, label := range labels {
		style := ""
		if chosen(i) {
			style = "accent strong"
		}
		if i == cursor {
			label = "▸ " + label
		}
		choices = append(choices, ui.Text{Common: ui.Common{Key: strconv.Itoa(i), Actions: &ui.Actions{Click: ChooseClick(name, i)}}, Spans: []ui.Span{{T: label, S: style}}, Wrap: ui.WrapNone})
	}
	row := ui.Row{Common: ui.Common{Key: "choices", Grow: 1}, Gap: ui.GapMD, Wrap: true, Children: choices}
	cells := ui.Nodes(row)
	if prompt != "" {
		cells = ui.Nodes(ui.Text{Common: ui.Common{Key: "prompt", Shrink: new(float64)}, Text: prompt, Wrap: ui.WrapNone}, row)
	}
	return ui.Row{Common: ui.Common{Key: string(name), Tone: tone, Aria: title}, Gap: ui.GapMD, Align: ui.AlignStart, Children: cells}
}

// cardKeys is the chord and the keycap of an action's main binding in mode: its first binding. An action with no binding in
// mode has no keys and no chord.
func cardKeys(mode model.Mode, action model.Action) (keys []string, chord string) {
	bindings := model.CardKeys(mode, action)
	if len(bindings) == 0 {
		return nil, ""
	}
	return []string{keycap(bindings[0].Chord, bindings[0].Key)}, bindings[0].Chord
}

// keycap is the keycap text of a binding, the chord named in words: "Ctrl+D", "Shift+N", "Enter", "Tab", "Esc", a letter as its
// capital, and arrows and other symbols as the binding labels them. n and Shift+N stay apart.
func keycap(chord, key string) string {
	if key == "" {
		key = chord
	}
	return KeyWords(key)
}

// Keycap is keycap for the tests, which compare what a button shows with the binding it presses.
func Keycap(chord, key string) string { return keycap(chord, key) }

// keyNames are the words for the keys a label names in lower case.
var keyNames = map[string]string{
	"escape": "Esc", "esc": "Esc", "enter": "Enter", "↵": "Enter", "⏎": "Enter", "tab": "Tab", "↹": "Tab", "⇥": "Tab", "space": "Space",
	"backspace": "Backspace", "delete": "Delete", "home": "Home", "end": "End", "pgup": "PgUp", "pgdn": "PgDn", "page_up": "PgUp",
	"page_down": "PgDn", "ctrl": "Ctrl", "alt": "Alt", "shift": "Shift", "meta": "Meta", "up": "↑", "down": "↓", "left": "←", "right": "→",
}

// KeyWords names a key label in words: "ctrl+d" is "Ctrl+D", "⇧N" is "Shift+N", "enter" is "Enter", "pgup/pgdn" is "PgUp/PgDn".
// A label of symbols (arrows, brackets) is returned as it is.
func KeyWords(label string) string {
	if utf8.RuneCountInString(label) < 2 {
		return strings.ToUpper(label)
	}
	if rest, ok := strings.CutPrefix(label, "⇧"); ok {
		return "Shift+" + KeyWords(rest)
	}
	if strings.Contains(label, "/") {
		parts := strings.Split(label, "/")
		for i, part := range parts {
			parts[i] = KeyWords(part)
		}
		return strings.Join(parts, "/")
	}
	if strings.Contains(label, "+") && !strings.HasSuffix(label, "+") {
		parts := strings.Split(label, "+")
		for i, part := range parts {
			parts[i] = KeyWords(part)
		}
		return strings.Join(parts, "+")
	}
	if name, ok := keyNames[label]; ok {
		return name
	}
	return model.Capitalize(label)
}

// cardButton is a card's Tern button for action in mode. Its click presses the action's first non-headless chord with the
// card's overlays closed and its one keycap is that chord's label, so a click and the key never disagree and a rebind moves
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

// menuButton opens the Menu. It never shows a keycap: the footer carries the esc hint, and esc only opens the Menu from some
// places, so a keycap on the button would say it where it is false.
func menuButton(model.Model) ui.Button {
	return ui.Button{Common: ui.Common{Key: "menu", Shrink: new(float64), Actions: &ui.Actions{Click: MenuClick}}, Text: "Menu"}
}

// exitButton is the safe way out of a card: a default button, never primary. Its words are "Cancel" on a form that holds unsaved
// input, "Close" on a read-only card and "Keep editing" on the discard question.
func exitButton(key string, mode model.Mode, action model.Action, words string) ui.Button {
	return cardButton(key, mode, action, words, false)
}

// formFoot is the foot of a form card: the primary button, the other actions, then the safe exit last.
func formFoot(primary ui.Button, exit ui.Button, others ...ui.Button) tern.Element {
	buttons := append([]ui.Button{primary}, others...)
	return buttonRow(buttonElements(append(buttons, exit))...)
}

// confirmFoot is the foot of a question: the confirm button, in the danger variant when the answer destroys something, then the
// safe exit last. The exit is never primary.
func confirmFoot(confirm ui.Button, exit ui.Button, danger bool) tern.Element {
	if danger {
		confirm = destructive(confirm)
	}
	return buttonRow(confirm, exit)
}

func buttonElements(buttons []ui.Button) []tern.Element {
	elements := make([]tern.Element, len(buttons))
	for i, b := range buttons {
		elements[i] = b
	}
	return elements
}

// footKey is the key of a card's button row while the card is built. Tern keys the footer slot itself, so the buttons' ids are
// the overlay's id, ".foot" and the button's key.
const footKey = "foot"

// buttonRow is the row of buttons that ends a card. It is placed anywhere among a card's children; popup moves it to the footer.
func buttonRow(buttons ...tern.Element) tern.Element {
	return ui.Row{Common: ui.Common{Key: footKey}, Gap: ui.GapMD, Justify: ui.JustifyEnd, Wrap: true, Children: buttons}
}

// footStatusKeys are the card lines that must stay visible beside the buttons: an error, a pending line, a saving or sending
// line and a question. popup moves them into the footer above the buttons, so a long card body scrolls under them.
var footStatusKeys = map[string]bool{"error": true, "pending": true, "saving": true, "sending": true, "question": true, "discard": true}

// cardMaxHeight is the share of the layer a card may take; the body scrolls under the fixed head and foot beyond it.
const cardMaxHeight = 0.9

func childKey(child tern.Element) string {
	key, _ := child.Node().Props["key"].(string)
	return key
}

// popup builds the card's overlay: the button row and the status lines (see footStatusKeys) go to the foot, every other child to
// the body. A status line whose key a button already uses stays in the body.
func popup(overlay ui.Overlay, children []tern.Element) ui.Overlay {
	if overlay.Max == nil {
		overlay.Max = &ui.Bounds{H: ui.Frac(cardMaxHeight)}
	}
	if overlay.Foot != nil {
		overlay.Children = children
		return overlay
	}
	var foot ui.Row
	hasFoot := false
	for _, child := range children {
		if row, ok := child.(ui.Row); ok && row.Key == footKey {
			foot, hasFoot = row, true
		}
	}
	taken := map[string]bool{}
	for _, button := range foot.Children {
		taken[childKey(button)] = true
	}
	body := make([]tern.Element, 0, len(children))
	var statuses []tern.Element
	for _, child := range children {
		if row, ok := child.(ui.Row); ok && row.Key == footKey {
			continue
		}
		if key := childKey(child); footStatusKeys[key] && !taken[key] {
			if text, ok := child.(ui.Text); ok {
				text.Basis, text.Grow = ui.BasisFrac(1), 1
				child = text
			}
			statuses = append(statuses, child)
			continue
		}
		body = append(body, child)
	}
	// A card that holds nothing but such lines keeps them in its body: moved to the foot they would leave an empty band under
	// the head.
	if len(body) == 0 {
		body, statuses = statuses, nil
	}
	if len(statuses) > 0 {
		if !hasFoot {
			foot = ui.Row{Common: ui.Common{Key: footKey}, Gap: ui.GapMD, Justify: ui.JustifyEnd, Wrap: true}
		}
		foot.Children = append(statuses, foot.Children...)
		hasFoot = true
	}
	if hasFoot {
		overlay.Foot = foot
	}
	overlay.Children = body
	return overlay
}

// discardButtons is the question that ends a Draft card asking to discard it: the destructive confirm in its own verb, and
// the way back to editing, which is never primary.
func discardButtons() tern.Element {
	return confirmFoot(cardButton("discard", model.ModeDiscard, model.DiscardDraft, "Discard Draft", false), exitButton("keep", model.ModeDiscard, model.KeepDraft, "Keep editing"), true)
}

func refMeta(ref string) ui.Rich {
	return ui.Spans(ui.Span{T: ref, S: "mono"})
}
