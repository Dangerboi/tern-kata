package driver

import (
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func decode(input tern.Input) model.Msg {
	switch input := input.(type) {
	case tern.Key:
		text := ""
		if input.Name == "paste" || (!input.Ctrl && !input.Alt && !input.Meta) {
			text = input.Text
		}
		return model.KeyMsg{Chord: input.String(), Text: text}
	case *tern.Edit:
		target := view.BodyTarget(input.ID)
		if target == model.NoEditTarget {
			return nil
		}
		return model.EditMsg{Target: target, From: input.From, To: input.To, Text: input.Text, Cursor: input.Cursor, Len: input.Len}
	case *tern.Undo:
		target := view.BodyTarget(input.ID)
		if target == model.NoEditTarget {
			return nil
		}
		return model.UndoMsg{Target: target}
	case *tern.Focus:
		return view.DecodeFocus(input.ID)
	case *tern.Select:
		return view.DecodeSelect(input.ID, input.Item)
	case *tern.Change:
		if view.IsBodyDoc(input.ID) {
			return model.BodyEdited{UID: view.BodyDocUID(input.ID), Text: input.Text}
		}
		return view.DecodeChange(input.ID, input.Value)
	case *tern.Editing:
		if view.IsBodyDoc(input.ID) {
			return model.BodyEditing{UID: view.BodyDocUID(input.ID), On: input.On}
		}
	case *tern.Comment:
		if view.IsBodyDoc(input.ID) && input.Comment == "" {
			return model.SelectionComment{UID: view.BodyDocUID(input.ID), Quote: input.Quote, Note: input.Note}
		}
	case *tern.Toggle:
		return view.DecodeToggle(input.ID, input.Open)
	case *tern.Action:
		value, _ := input.Value.(string)
		return view.DecodeAction(input.ID, input.Act, value)
	}
	return nil
}
