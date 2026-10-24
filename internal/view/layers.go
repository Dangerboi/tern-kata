package view

import (
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// layerDraws is what draws each popup card's layer. The model's card table decides which cards exist, their order and their
// names; this is the one place the view says how each is drawn, and a card missing here draws nothing.
var layerDraws = map[model.Card]func(model.Model, time.Time) tern.Element{
	model.CardCreate:     func(m model.Model, _ time.Time) tern.Element { return createLayer(m) },
	model.CardDraft:      func(m model.Model, _ time.Time) tern.Element { return editorLayer(m) },
	model.CardWorkflow:   func(m model.Model, _ time.Time) tern.Element { return workflowLayer(m) },
	model.CardField:      func(m model.Model, _ time.Time) tern.Element { return fieldLayer(m) },
	model.CardClose:      func(m model.Model, _ time.Time) tern.Element { return closeLayer(m) },
	model.CardDates:      func(m model.Model, _ time.Time) tern.Element { return datesLayer(m) },
	model.CardToken:      func(m model.Model, _ time.Time) tern.Element { return tokenLayer(m) },
	model.CardRevoke:     func(m model.Model, _ time.Time) tern.Element { return revokeLayer(m) },
	model.CardMove:       func(m model.Model, _ time.Time) tern.Element { return moveLayer(m) },
	model.CardDelete:     func(m model.Model, _ time.Time) tern.Element { return deleteLayer(m) },
	model.CardFederationAdopt: func(m model.Model, _ time.Time) tern.Element { return federationAdoptLayer(m) },
	model.CardRecurrence: func(m model.Model, _ time.Time) tern.Element { return recurrenceLayer(m) },
	model.CardJob:        func(m model.Model, now time.Time) tern.Element { return jobLayer(m, now) },
	model.CardBackfill:   func(m model.Model, _ time.Time) tern.Element { return backfillLayer(m) },
	model.CardProject:    func(m model.Model, _ time.Time) tern.Element { return projectLayer(m) },
	model.CardLanes:      func(m model.Model, _ time.Time) tern.Element { return lanesLayer(m) },
	model.CardWorker:     func(m model.Model, _ time.Time) tern.Element { return workerLayer(m) },
	model.CardFilter:     func(m model.Model, _ time.Time) tern.Element { return filterLayer(m) },
	model.CardPicker:     func(m model.Model, _ time.Time) tern.Element { return pickerLayer(m) },
}

// topLayer is the layer of the card on top, nil when none is.
func topLayer(m model.Model, now time.Time) tern.Element {
	if draw := layerDraws[m.TopCard()]; draw != nil {
		return draw(m, now)
	}
	return nil
}

// inputLeaf is the last part of the node id of the one input of a card without fields.
const inputLeaf = "input"

// LayerID is the id of card's layer: "layer." and the card's name in the model's card table.
func LayerID(card model.Card) string { return "layer." + card.Name() }

// layerKey is the key of card's overlay under the layer, which makes LayerID the overlay's node id.
func layerKey(card model.Card) string { return card.Name() }

// FieldID is the node that holds card's field called name, under the card's layer; a card without fields has one input, named "".
func FieldID(card model.Card, name model.Field) string {
	if name == "" {
		name = inputLeaf
	}
	return LayerID(card) + "." + string(name)
}

// FieldFromID is the card and field whose node has id, found by walking the model's card table; the field is "" for the one
// input of a card without fields. A card with no input at all, as Backfill, has no node to name.
func FieldFromID(id string) (model.Card, model.Field, bool) {
	for _, card := range model.Cards() {
		fields, _ := model.CardFields(card)
		for _, field := range fields {
			if id == FieldID(card, field.Name) {
				return card, field.Name, true
			}
		}
		if model.CardInput(card) && id == FieldID(card, "") {
			return card, "", true
		}
	}
	return model.NoCard, "", false
}

// BodyTarget is the body a Tern editor node edits: the card's own body field or input, none for any other node.
func BodyTarget(id string) model.EditTarget {
	card, field, ok := FieldFromID(id)
	if !ok {
		return model.NoEditTarget
	}
	if target, body := model.CardBody(card); field == body {
		return target
	}
	return model.NoEditTarget
}
