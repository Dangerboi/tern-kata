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
		if input.ID == view.BodyID {
			return model.BodyEdited{Text: input.Text}
		}
		return view.DecodeChange(input.ID, input.Value)
	case *tern.Editing:
		if input.ID == view.BodyID {
			return model.BodyEditing{On: input.On}
		}
	case *tern.Comment:
		if input.ID == view.BodyID && input.Comment == "" {
			return model.SelectionComment{Quote: input.Quote, Note: input.Note}
		}
	case *tern.Toggle:
		return view.DecodeToggle(input.ID, input.Open)
	case *tern.Action:
		value, _ := input.Value.(string)
		return view.DecodeAction(input.ID, input.Act, value)
	}
	return nil
}
