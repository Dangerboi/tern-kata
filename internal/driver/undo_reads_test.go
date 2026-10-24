package driver_test

import "testing"

// A read never clears the Undo history: opening each Lettered Page and the Attention Page, and reloading them, leaves `u` undoing.
func TestOpeningAndReloadingPagesLeavesTheUndoHistoryAlone(t *testing.T) {
	for _, tt := range []struct {
		name   string
		open   string
		reload string
	}{
		{"credentials", "shift+c", ""},
		{"federation", "shift+f", ""},
		{"daemons", "shift+d", ""},
		{"attention", "6", "ctrl+r"},
	} {
		s := undoable(t, 2)
		s = feed(t, s, key("escape")).State
		s = feed(t, s, key(tt.open)).State
		if tt.reload != "" {
			s = feed(t, s, key(tt.reload)).State
		}
		if got := len(s.Model.Undo.Entries); got != 2 {
			t.Errorf("%s: %d changes held after opening the Page, want 2 (boundary %q)", tt.name, got, s.Model.Undo.Boundary)
			continue
		}
		s = feed(t, s, key("1")).State
		out := pressU(t, s)
		if len(out.Cmds) != 1 {
			t.Errorf("%s: u after leaving the Page sent %#v on %v, notice %q", tt.name, out.Cmds, s.Model.Page, out.State.Model.Notice)
			continue
		}
		if got := undoCmd(t, out); got.Entry.Revision != 2 {
			t.Errorf("%s: u undoes revision %d, want 2", tt.name, got.Entry.Revision)
		}
	}
}
