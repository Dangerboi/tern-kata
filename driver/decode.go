package driver

import (
	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// decode turns one Tern input into the message the model understands, nil for
// an input the Pane does not use.
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
	case *tern.Action:
		return view.DecodeAction(input.ID, input.Act, input.Value)
	case *tern.Resize:
		return model.ResizeMsg{Cols: input.Cols}
	}
	return nil
}
