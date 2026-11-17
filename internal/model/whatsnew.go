package model

// ShowWhatsNew asks for the release notes of Version beside the Pane when this is the first Pane of that version on the machine:
// the sections of the CHANGELOG since the version shown last. A version without a section of its own (a dev build) shows nothing.
// PaneBlock is the Pane's block, which gets the focus back from the notes.
type ShowWhatsNew struct {
	Version, PaneBlock string
}

func (ShowWhatsNew) cmd() {}

// WhatsNewShown answers ShowWhatsNew. Err is why notes that were due could not be opened; the version counts as shown anyway.
type WhatsNewShown struct {
	Err error
}

func (WhatsNewShown) msg() {}

// whatsNewShown says only why the notes could not be opened: an open notes block, or nothing due, needs no word.
func whatsNewShown(m Model, msg WhatsNewShown) Model {
	if msg.Err != nil {
		m.Notice = "What's new in tern-kata " + m.Versions.Running + " was not opened: " + oneLine(msg.Err.Error())
	}
	return m
}
