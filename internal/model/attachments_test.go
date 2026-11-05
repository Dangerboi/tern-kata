package model_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func reference(name, mediaType string, size float64) map[string]any {
	return map[string]any{"name": name, "media_type": mediaType, "size": size, "docbank_version": "6f1c2e8a-2222-4222-8222-000000000001"}
}

// attached is Issue "a" in its detail with the metadata.
func attached(metadata map[string]any) model.Model {
	m := withDetail(model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}}},
		&model.IssueDetailData{Issue: &model.Issue{UID: "a", ProjectID: 3, Metadata: metadata}, Comments: []model.Comment{{UID: "c1"}}})
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

func TestAttachmentsReadOnlyKatasNativeReferences(t *testing.T) {
	list := model.Attachments(map[string]any{
		"attachment.B1":                reference("b.png", "image/png", 10),
		"attachment.A1":                reference("a.log", "text/plain", 2048),
		"attachment.NOSIZE":            map[string]any{"name": "x", "media_type": "text/plain", "docbank_version": "v"},
		"attachment.NEG":               reference("neg", "text/plain", -1),
		"attachment.SLASH":             reference("a/b", "text/plain", 1),
		"attachment.NOTYPE":            reference("t", "", 1),
		"attachment.NOVER":             map[string]any{"name": "v", "media_type": "text/plain", "size": float64(1), "docbank_version": ""},
		"attachment.STR":               "not a reference",
		"attachment.":                  reference("empty-id", "text/plain", 1),
		"attachment.a/b":               reference("badid", "text/plain", 1),
		"scheduled_on":                 "2026-10-20",
		"tern-kata.attachment.old.png": map[string]any{"host": "box", "path": "/b.png"},
	})
	want := []model.Attachment{{ID: "A1", Name: "a.log", MediaType: "text/plain", Version: "6f1c2e8a-2222-4222-8222-000000000001", Size: 2048}, {ID: "B1", Name: "b.png", MediaType: "image/png", Version: "6f1c2e8a-2222-4222-8222-000000000001", Size: 10}}
	if !slices.Equal(list, want) {
		t.Fatalf("got %+v, want %+v", list, want)
	}
}

func TestAttachmentLabelsAddAnIdSuffixOnlyToRepeatedNames(t *testing.T) {
	got := model.AttachmentLabels([]model.Attachment{{ID: "01ABCDEFGHJK", Name: "a"}, {ID: "01ZZZZZZ999999", Name: "a"}, {ID: "X", Name: "b"}})
	if !slices.Equal(got, []string{"a [EFGHJK]", "a [999999]", "b"}) {
		t.Fatalf("labels %q", got)
	}
}

func TestFormatSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1536: "1.5 KB", 5 << 20: "5.0 MB", 3 << 30: "3.0 GB", 300 << 30: "300.0 GB"} {
		if got := model.FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestAttachmentRowsAreSortedByNameAndIgnoreOtherMeta(t *testing.T) {
	m := attached(map[string]any{
		"attachment.B1": reference("b.png", "image/png", 10), "attachment.A1": reference("a.png", "image/png", 5),
		"attachment.BAD": "not a reference", "scheduled_on": "2026-10-20", "other.x": reference("x", "text/plain", 1),
	})
	if got := attachmentKeys(m); !slices.Equal(got, []string{model.AttachmentRowKey("A1"), model.AttachmentRowKey("B1")}) {
		t.Fatalf("rows %v", got)
	}
	if n := m.DetailRowCount(model.TabAttachments); n != 2 {
		t.Fatalf("count %d", n)
	}
	rows := m.DetailRows()
	if rows[0].Activity.File.Name != "a.png" || rows[1].Activity.File.Description() != "image/png, 10 B" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestAttachmentCursorStaysOnItsIdAcrossARefreshThatAddsOne(t *testing.T) {
	m := attached(map[string]any{"attachment.B": reference("b", "text/plain", 1)})
	m, _ = model.Update(m, model.KeyMsg{Chord: "j"})
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.ReadSnapshot)
	issue := model.Issue{UID: "a", Metadata: map[string]any{"attachment.A": reference("a", "text/plain", 1), "attachment.B": reference("b", "text/plain", 1)}}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: model.Snapshot{Cursor: 1, Issues: []model.Issue{issue}}, Selected: &model.IssueDetailData{Issue: &issue}})
	if got := cursorKey(m); got != model.AttachmentRowKey("B") {
		t.Fatalf("cursor on %q", got)
	}
}

func TestAttachmentRowKeysAreUniqueAndDotFree(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range []string{"tabs", "empty", "a", "a-row", "A_1"} {
		key := model.AttachmentRowKey(id)
		if strings.Contains(key, ".") || seen[key] {
			t.Fatalf("key %q for %q", key, id)
		}
		seen[key] = true
	}
}

func TestEnterOnAnAttachmentRowDownloadsAndOpensIt(t *testing.T) {
	m := attached(map[string]any{"attachment.A": reference("a.png", "image/png", 5)})
	next, cmds := model.Update(m, model.KeyMsg{Chord: "enter"})
	want := model.OpenAttachment{Workspace: "/repo", UID: "a", ProjectID: 3, Attachment: model.Attachment{ID: "A", Name: "a.png", MediaType: "image/png", Version: "6f1c2e8a-2222-4222-8222-000000000001", Size: 5}}
	if len(cmds) != 1 || cmds[0] != model.Cmd(want) || next.SelectedUID != "a" || next.ActiveTab() != model.TabAttachments || next.Focus != model.ColumnDetail {
		t.Fatalf("enter: cmds %#v, want %#v; selected %q tab %v", cmds, want, next.SelectedUID, next.ActiveTab())
	}
}

func TestAnOpenedAttachmentIsSaidAsANoticeAndAFailureToo(t *testing.T) {
	m := attached(map[string]any{"attachment.A": reference("a.png", "image/png", 5)})
	m, _ = model.Update(m, model.AttachmentOpened{UID: "a", Name: "a.png", Path: "/cache/a.png"})
	if m.Notice != "Opened a.png (/cache/a.png)" {
		t.Fatalf("notice %q", m.Notice)
	}
	m, _ = model.Update(m, model.AttachmentOpened{UID: "a", Name: "a.png", Err: errors.New("kata has no Docbank")})
	if !strings.HasPrefix(m.Notice, "Cannot open a.png: ") || !strings.Contains(m.Notice, "no Docbank") {
		t.Fatalf("notice %q", m.Notice)
	}
}

func TestTheAttachmentsTabSaysWhyWhenTheDaemonHasNoDocbank(t *testing.T) {
	m := withDetail(model.Model{Workspace: "/repo", Cols: 160, Focus: model.ColumnDetail, SelectedUID: "a", Issues: []model.Issue{{UID: "a"}}},
		&model.IssueDetailData{Issue: &model.Issue{UID: "a"}, AttachmentsOff: true})
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabAttachments})
	if got := m.EmptyDetailText(); got != model.DocbankOffMessage {
		t.Fatalf("empty text %q", got)
	}
	m, _ = model.Update(m, model.ActivityTabMsg{Tab: model.TabLinks})
	if got := m.EmptyDetailText(); got != "No links." {
		t.Fatalf("Links empty text %q", got)
	}
}

func TestAttachmentRowsFollowLiveRefreshes(t *testing.T) {
	m := attached(map[string]any{"attachment.A": reference("a", "text/plain", 1), "attachment.B": reference("b", "text/plain", 1)})
	m, cmds := model.Update(m, model.StreamEvent{ID: 1, Type: "issue.updated"})
	refresh := cmds[0].(model.ReadSnapshot)
	issue := model.Issue{UID: "a", Metadata: map[string]any{"attachment.B": reference("b", "text/plain", 1), "attachment.C": reference("c", "text/plain", 1)}}
	m, _ = model.Update(m, model.SnapshotLoaded{Request: refresh.Request, Query: refresh.Query, Snapshot: model.Snapshot{Cursor: 1, Issues: []model.Issue{issue}}, Selected: &model.IssueDetailData{Issue: &issue}})
	if got := attachmentKeys(m); !slices.Equal(got, []string{model.AttachmentRowKey("B"), model.AttachmentRowKey("C")}) {
		t.Fatalf("rows after the refresh %v", got)
	}
}
