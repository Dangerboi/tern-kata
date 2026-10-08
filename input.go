package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/Dangerboi/tern-kata/model"
	"github.com/Dangerboi/tern-kata/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func startInputReader(ctx context.Context, session *tern.Session, messages chan<- model.Msg, inputErrors chan<- error) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			input, err := session.Next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					inputErrors <- err
				}
				return
			}
			var msg model.Msg
			switch input := input.(type) {
			case tern.Key:
				text := ""
				if input.Name == "paste" || (!input.Ctrl && !input.Alt && !input.Meta) {
					text = input.Text
				}
				msg = model.KeyMsg{Chord: input.String(), Text: text}
			case *tern.Edit:
				msg = model.EditMsg{ID: input.ID, From: input.From, To: input.To, Text: input.Text, Cursor: input.Cursor, Len: input.Len}
			case *tern.Undo:
				msg = model.UndoMsg{ID: input.ID}
			case *tern.Focus:
				if field, ok := view.FilterFieldIndex(input.ID); ok {
					msg = model.FilterFieldMsg{Field: field}
				}
				if field, ok := view.CreateFieldIndex(input.ID); ok {
					msg = model.CreateFieldMsg{Field: field}
				}
			case *tern.Select:
				if input.ID == view.IssueListID {
					msg = model.SelectMsg{UID: strings.TrimPrefix(input.Item, view.IssueListID+".")}
				}
				if input.ID == view.CollectionsID {
					msg = model.CollectionPointerSelected{Key: strings.TrimPrefix(input.Item, view.CollectionsID+".")}
				}
				if input.ID == view.PickerListID {
					msg = model.PickerSelected{Key: strings.TrimPrefix(input.Item, view.PickerListID+".")}
				}
			case *tern.Action:
				msg = detailAction(input)
				if strings.HasPrefix(input.ID, "layer.close.") {
					switch input.Act {
					case "close-save":
						msg = model.CloseSubmitMsg{}
					case "close-quick":
						msg = model.CloseSubmitMsg{Quick: true}
					case "close-cancel":
						msg = model.KeyMsg{Chord: "escape"}
					}
				}
				if input.ID == view.RetryID && input.Act == string(model.RetryAction) {
					msg = model.Retry{}
				}
				if strings.HasPrefix(input.ID, "layer.filters.actions.") {
					msg = model.FilterFormActionMsg{Action: model.Action(input.Act)}
				}
			case *tern.Resize:
				msg = model.ResizeMsg{Cols: input.Cols}
			case *tern.ErrorEvent:
				// SDK queues raw focus/reveal ops even when a later Render replaces their nodes.
				if id, ok := strings.CutPrefix(input.Msg, "unknown id "); ok && view.IsTransientTarget(id) {
					continue
				}
				inputErrors <- fmt.Errorf("Tern surface: %s", input.Msg)
				return
			}
			if msg != nil {
				select {
				case messages <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}
