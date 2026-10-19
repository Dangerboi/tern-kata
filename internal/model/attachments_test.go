package model_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func pointer(host, path string) map[string]any { return map[string]any{"host": host, "path": path} }

// attached is Issue "a" in its detail with the metadata, on a machine called "box".
func attached(metadata map[string]any) model.Model {
	m := withDetail(model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Host: "box", Issues: []model.Issue{{UID: "a"}}},
		&model.IssueDetailData{Issue: &model.Issue{UID: "a", Metadata: metadata}, Comments: []model.Comment{{UID: "c1"}}})
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabAttachments})
	return m
}

func attachmentKeys(m model.Model) []string {
	var keys []string
	for _, row := range m.DetailRows() {
		keys = append(keys, row.Key)
	}
	return keys
}

func TestAttachmentsIsTheFourthTabAndCycles(t *testing.T) {
	if model.TabAttachments != model.TabLinks+1 || model.TabCount != model.TabAttachments+1 || model.TabAttachments.String() != "Attachments" {
		t.Fatalf("tabs: attachments %d, count %d", model.TabAttachments, model.TabCount)
	}
	m := attached(nil)
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabLinks})
	m, _ = model.Update(m, model.KeyMsg{Chord: "tab"})
	if m.ActiveTab() != model.TabAttachments {
		t.Fatalf("tab after Links: %v", m.ActiveTab())
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "tab"})
	if m.ActiveTab() != model.TabComments {
		t.Fatalf("tab after Attachments: %v", m.ActiveTab())
	}
	m, _ = model.Update(m, model.KeyMsg{Chord: "shift+tab"})
	if m.ActiveTab() != model.TabAttachments {
		t.Fatalf("shift+tab from Comments: %v", m.ActiveTab())
	}
}

func TestAttachmentRowsAreSortedByNameAndIgnoreOtherMeta(t *testing.T) {
	m := attached(map[string]any{
		"attachment.b.png": pointer("box", "/b.png"), "attachment.a.png": pointer("elsewhere", "/a.png"),
		"attachment.bad": "not a pointer", "scheduled_on": "2026-10-20", "other.x": pointer("box", "/x"),
	})
	if got := attachmentKeys(m); !slices.Equal(got, []string{model.AttachmentRowKey("a.png"), model.AttachmentRowKey("b.png")}) {
		t.Fatalf("rows %v", got)
	}
	if n := m.DetailRowCount(model.TabAttachments); n != 2 {
		t.Fatalf("count %d", n)
	}
	rows := m.DetailRows()
	if rows[0].Activity.URL != "" || rows[0].Activity.File.Host != "elsewhere" || rows[1].Activity.URL != "file:///b.png" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestAttachmentFileURLEscapesAndHandlesWindowsPaths(t *testing.T) {
	m := attached(map[string]any{"attachment.w": pointer("BOX", `C:\a b\x.png`), "attachment.u": pointer("box", "/a b/x.png")})
	rows := m.DetailRows()
	if rows[0].Activity.URL != "file:///a%20b/x.png" || rows[1].Activity.URL != "file:///C:/a%20b/x.png" {
		t.Fatalf("urls %q %q", rows[0].Activity.URL, rows[1].Activity.URL)
	}
}

func TestAttachmentFileURLKeepsABackslashInAUnixName(t *testing.T) {
	m := attached(map[string]any{"attachment.a": pointer("box", `/a/b\c.png`)})
	rows := m.DetailRows()
	if rows[0].Activity.URL != "file:///a/b%5Cc.png" {
		t.Fatalf("url %q", rows[0].Activity.URL)
	}
}

func TestAttachmentCursorStaysOnItsNameAcrossARefreshThatAddsOne(t *testing.T) {
	m := attached(map[string]any{"attachment.b": pointer("box", "/b")})
	m, _ = model.Update(m, model.KeyMsg{Chord: "j"})
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.ReadSnapshot)
	issue := model.Issue{UID: "a", Metadata: map[string]any{"attachment.a": pointer("box", "/a"), "attachment.b": pointer("box", "/b")}}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: model.Snapshot{Cursor: 1, Issues: []model.Issue{issue}}, Selected: &model.IssueDetailData{Issue: &issue}})
	if got := cursorKey(m); got != model.AttachmentRowKey("b") {
		t.Fatalf("cursor on %q", got)
	}
}

func TestAttachmentRowKeysAreUniqueAndDotFree(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range []string{"tabs", "empty", "a", "a.row", "a-row", ""} {
		key := model.AttachmentRowKey(name)
		if strings.Contains(key, ".") || seen[key] {
			t.Fatalf("key %q for %q", key, name)
		}
		seen[key] = true
	}
}

func TestEnterOnAnAttachmentRowDoesNothing(t *testing.T) {
	m := attached(map[string]any{"attachment.a": pointer("box", "/a")})
	next, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	if len(cmds) != 0 || next.SelectedUID != "a" || next.ActiveTab() != model.TabAttachments || cursorKey(next) != model.AttachmentRowKey("a") || next.Focus != model.ColumnDetail {
		t.Fatalf("enter changed state: cmds %v selected %q tab %v", cmds, next.SelectedUID, next.ActiveTab())
	}
}

func TestAttachmentRowsFollowLiveRefreshes(t *testing.T) {
	m := attached(map[string]any{"attachment.a": pointer("box", "/a"), "attachment.b": pointer("box", "/b")})
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.ReadSnapshot)
	issue := model.Issue{UID: "a", Metadata: map[string]any{"attachment.b": pointer("box", "/b"), "attachment.c": pointer("box", "/c")}}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: model.Snapshot{Cursor: 1, Issues: []model.Issue{issue}}, Selected: &model.IssueDetailData{Issue: &issue}})
	if got := attachmentKeys(m); !slices.Equal(got, []string{model.AttachmentRowKey("b"), model.AttachmentRowKey("c")}) {
		t.Fatalf("rows after the refresh %v", got)
	}
}
