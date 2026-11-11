package model

import "time"

// Toast is a Notice with one button. Chord is the key the button presses in Mode, the mode the toast was raised in, on the Issue UID
// when one is set; Ref, when set, is an Issue the button opens instead. Undoes, when set, is the deleted Issue whose delete the
// button undoes: it presses Chord only while that delete is the newest Undo entry.
type Toast struct {
	Seq    int
	Text   string
	Label  string
	Chord  string
	UID    string
	Mode   Mode
	Ref    string
	Undoes string
}

// ToastLife is how long Tern keeps a toast with a button up; the model drops its toast after as long.
const ToastLife = 6 * time.Second

// WaitToast is the pause of ToastLife after toast Seq was raised. It reads nothing; its answer ToastExpired takes the toast away.
type WaitToast struct{ Seq int }

func (WaitToast) cmd() {}

type ToastExpired struct{ Seq int }

func (ToastExpired) msg() {}

// raiseToast shows t as the model's toast, under a Seq no earlier toast had, and returns the Cmd that ends it. The toast keeps the
// mode the Pane is in now, so its button does what its key does here, whatever mode the Pane has moved to by the tap.
func (m Model) raiseToast(t Toast) (Model, Cmd) {
	m.ToastSeq++
	t.Seq, t.Mode = m.ToastSeq, m.CurrentMode()
	m.Toast = &t
	return m, WaitToast{Seq: t.Seq}
}

// toastExpired drops the toast the answer is for; a newer toast stays.
func toastExpired(m Model, msg ToastExpired) Model {
	if m.Toast != nil && m.Toast.Seq == msg.Seq {
		m.Toast = nil
	}
	return m
}

// ToastPressMsg is a tap on the button of toast Seq.
type ToastPressMsg struct{ Seq int }

func (ToastPressMsg) msg() {}

// pressToast opens the toast's Ref, or runs its Chord in its Mode on the Issue it names, once: the toast goes with the tap. A toast
// that has gone, whose Issue no longer shows, or whose delete is no longer the newest Undo entry, does nothing.
func pressToast(m Model, seq int) (Model, []Cmd) {
	t := m.Toast
	if t == nil || t.Seq != seq {
		return m, nil
	}
	m.Toast = nil
	switch {
	case t.Ref != "":
		return openIssue(m, t.Ref)
	case t.Undoes != "" && !newestUndoDeletes(m, t.Undoes):
		return m, nil
	}
	return runChordOn(m, t.UID, t.Chord, t.Mode)
}
