package model

// FocusFieldMsg is the pointer putting focus on field Field of the open popup card, so the keys
// edit that field as they do after tab moves there.
type FocusFieldMsg struct{ Field int }

func (FocusFieldMsg) msg() {}

func focusField(m Model, field int) Model {
	if field < 0 {
		return m
	}
	switch {
	case m.Create.Open:
		if !m.Create.Saving && !m.Create.DiscardConfirm && field < 5 {
			m.Create.Field = field
		}
	case m.CloseForm.Open:
		if !m.CloseForm.Pending && field < len(m.CloseForm.Values) {
			m.CloseForm.Field = field
		}
	case m.FilterForm.Open:
		if field < 5 {
			m.FilterForm.Field = field
		}
	}
	return m
}
