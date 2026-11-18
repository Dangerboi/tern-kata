package model

// ShowWhatsNew asks for the release notes of Version beside the Pane when this is the first Pane of that version on the machine:
// the sections of the CHANGELOG since the version shown last. A version without a section of its own (a dev build) shows nothing.
// PaneBlock is the Pane's block, which gets the focus back from the notes.
type ShowWhatsNew struct {
	Version, PaneBlock string
}

func (ShowWhatsNew) cmd() {}

// WhatsNewShown answers ShowWhatsNew for Version. Err is why notes that were due were not shown: notes that could not be written
// leave the version unrecorded, so the next Pane tries again; notes written but not opened count as shown.
type WhatsNewShown struct {
	Version string
	Err     error
}

func (WhatsNewShown) msg() {}

// whatsNewShown says only why the notes were not shown: open notes, or none due, need no word.
func whatsNewShown(m Model, msg WhatsNewShown) Model {
	if msg.Err != nil {
		m = m.fail(Cannot("show", "what's new in tern-kata "+msg.Version, msg.Err))
	}
	return m
}
