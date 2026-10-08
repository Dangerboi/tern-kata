package model

import "iter"

func (m Model) CurrentMode() Mode {
	if mode := m.EditorMode(); mode != "" { return mode }
	if m.FieldForm.Open { return ModeFieldForm }
	if m.HelpOpen {
		return ModeHelp
	}
	if m.CloseForm.Open { return ModeCloseForm }
	if m.Err != nil {
		return ModeError
	}
	switch m.Focus {
	case ColumnDetail:
		return ModeDetail
	case ColumnCollections:
		return ModeCollections
	}
	return ModeList
}

func LookupBindingForMode(chord string, mode Mode) (Binding, bool) {
	for _, binding := range Keymap {
		if binding.Chord == chord && binding.Mode == mode {
			return binding, true
		}
	}
	if mode == ModeEditor || mode == ModeDiscard || mode == ModeConflict { return Binding{}, false }
	for _, binding := range Keymap {
		if binding.Chord == chord && binding.Mode == "" {
			return binding, true
		}
	}
	return Binding{}, false
}

func BindingsForMode(mode Mode, footerOnly bool) iter.Seq[Binding] {
	return func(yield func(Binding) bool) {
		for _, binding := range Keymap {
			if binding.Mode != "" && binding.Mode != mode {
				continue
			}
			if footerOnly && !binding.ShowInFooter {
				continue
			}
			active, _ := LookupBindingForMode(binding.Chord, mode)
			if active != binding {
				continue
			}
			if !yield(binding) {
				return
			}
		}
	}
}
