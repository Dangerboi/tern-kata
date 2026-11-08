package model

import "fmt"

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

func (c *formCard) refuseStale(m Model, doing string) bool {
	if !m.Stale {
		return false
	}
	c.Err = fmt.Errorf("Daemon disconnected; reconnect before %s.", doing)
	return true
}

func (c *formCard) refuseSubmit(m Model, doing string) bool {
	return !c.Open || c.Saving || c.refuseStale(m, doing)
}
