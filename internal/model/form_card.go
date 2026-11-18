package model

import "errors"

// formCard is the state every form card opens with: Open, Saving while a write has not answered, the focused Field, the Caret
// in it and the Err of the last refusal or failure. A card state embeds it, so its fieldTable reaches focus, caret, error and
// the default busy condition (Saving) without closures of its own.
type formCard struct {
	Open, Saving bool
	Field        Field
	Caret        Caret
	Err          error
}

type cardShell interface{ card() *formCard }

func (c *formCard) card() *formCard { return c }

func (c *formCard) refuseStale(m Model, verb, thing string) bool {
	if !m.Stale {
		return false
	}
	c.Err = errors.New(Cannot(verb, thing, ErrDisconnected))
	return true
}

func (c *formCard) refuseSubmit(m Model, verb, thing string) bool {
	return !c.Open || c.Saving || c.refuseStale(m, verb, thing)
}
