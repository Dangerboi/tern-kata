package view_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestNoticeToneFollowsItsKind(t *testing.T) {
	info := apply(browsing(), model.KeyMsg{Chord: "u"})
	node := render(t, info)["main.notice"]
	if got := text(node); got != "Nothing to undo." || node.P["tone"] != "neutral" {
		t.Errorf("info notice %q tone %v, want default text", got, node.P["tone"])
	}

	entry := model.UndoEntry{Kind: model.UndoPriority, UID: "alpha", ProjectID: 2, ShortID: "aaaa", InstanceUID: "i", Revision: 2}
	failed := apply(browsing(), model.FieldWritten{UID: "alpha", Record: model.WriteRecord{Entry: &entry}},
		model.StreamDropped{Generation: browsing().StreamGeneration, Err: errors.New("kata server not responding")},
		model.KeyMsg{Chord: "u"})
	node = render(t, failed)["main.notice"]
	if got := text(node); got != "Cannot undo: disconnected from kata." || node.P["tone"] != "error" {
		t.Errorf("failed notice %q tone %v, want the error tone", got, node.P["tone"])
	}
}

func TestStaleLineSaysWhatIsShownAndKeepsTheRawErrorOffTheLine(t *testing.T) {
	m := browsing()
	m.Now = time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)
	m = apply(m, model.StreamDropped{Generation: m.StreamGeneration, Err: errors.New("kata server not responding")})
	line := text(render(t, m)["main.stale"])
	if !strings.HasPrefix(line, "Disconnected from kata. Showing Issues as of ") || !strings.HasSuffix(line, "; changes are off until it reconnects.") {
		t.Errorf("stale line %q", line)
	}
	if strings.Contains(line, "not responding") || strings.Contains(line, "Read-only") {
		t.Errorf("stale line carries the raw error or the old wording: %q", line)
	}
}
