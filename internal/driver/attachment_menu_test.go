package driver_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

func attachmentRef(name, mediaType string) map[string]any {
	return map[string]any{"name": name, "media_type": mediaType, "size": float64(5), "docbank_version": "6f1c2e8a-2222-4222-8222-000000000001"}
}

// attachmentsPane shows Issue "a" of Project "work" on its Attachments tab with a shot.png and a clip.mp4.
func attachmentsPane(t *testing.T) driver.State {
	t.Helper()
	issue := boardCard("a")
	issue.ProjectID = 7
	issue.Metadata = map[string]any{"attachment.A1": attachmentRef("shot.png", "image/png"), "attachment.A2": attachmentRef("clip.mp4", "video/mp4")}
	s := projectPane(t, 160, issue)
	s = step(t, s, model.DetailLoaded{UID: "a", Selected: &model.IssueDetailData{Issue: &issue}}).State
	return step(t, s, model.ActivityTabMsg{Tab: model.TabAttachments}).State
}

// attachmentRow is the drawn row of the Attachment with id and its context menu, read from the wire as Tern would.
func attachmentRow(t *testing.T, s driver.State, id string) (rowID string, menu []map[string]any) {
	t.Helper()
	key := model.AttachmentRowKey(id)
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if !strings.HasSuffix(n.ID, ".activity."+key) {
			continue
		}
		actions, _ := n.P["actions"].(map[string]any)
		items, _ := actions["menu"].([]any)
		for _, item := range items {
			menu = append(menu, item.(map[string]any))
		}
		held, _ := actions["hold"].([]any)
		if len(held) != len(items) {
			t.Fatalf("the long press lists %d items, the right-click menu %d", len(held), len(items))
		}
		return n.ID, menu
	}
	t.Fatalf("no row for attachment %s", id)
	return "", nil
}

func menuItem(t *testing.T, menu []map[string]any, label string) map[string]any {
	t.Helper()
	for _, item := range menu {
		if item["label"] == label {
			return item
		}
	}
	t.Fatalf("the menu has no %q: %v", label, menu)
	return nil
}

// pick steps the click of the menu item called label of the attachment's row.
func pick(t *testing.T, s driver.State, id, label string) driver.Output {
	t.Helper()
	rowID, menu := attachmentRow(t, s, id)
	itemID, _ := menuItem(t, menu, label)["id"].(string)
	return step(t, s, click(rowID, itemID))
}

func TestEachAttachmentMenuItemPressesItsKey(t *testing.T) {
	want := func(way model.AttachmentWay) func(*testing.T, driver.Output) {
		return func(t *testing.T, out driver.Output) {
			var open model.OpenAttachment
			for _, c := range out.Cmds {
				if o, ok := c.(model.OpenAttachment); ok {
					open = o
				}
			}
			if open.Attachment.ID != "A1" || open.UID != "a" || open.ProjectID != 7 || open.Way != way {
				t.Fatalf("cmds %#v, want A1 opened with way %v", out.Cmds, way)
			}
		}
	}
	for _, tc := range []struct {
		label string
		key   string
		check func(*testing.T, driver.Output)
	}{
		{"Open", "↵", want(model.WayOpen)},
		{"Open beside", "v", want(model.WayBeside)},
		{"Open with the system app", "o", want(model.WaySystem)},
		{"Save to Downloads", "s", want(model.WaySave)},
		{"Copy…", "y", want(model.WayCopy)},
		{"Unlink…", "z", func(t *testing.T, out driver.Output) {
			q, ok := out.State.Model.Question()
			if !ok || q.Mode != model.ModeUnlinkAttachment || q.Text != "Unlink shot.png from work#a? y/n" || !q.Danger || len(out.Cmds) != 0 {
				t.Fatalf("question %+v (%v), cmds %#v", q, ok, out.Cmds)
			}
		}},
	} {
		t.Run(tc.label, func(t *testing.T) {
			s := attachmentsPane(t)
			_, menu := attachmentRow(t, s, "A1")
			if keys, _ := menuItem(t, menu, tc.label)["keys"].([]any); len(keys) != 1 || keys[0] != tc.key {
				t.Fatalf("the item shows keys %v, want %s", keys, tc.key)
			}
			tc.check(t, pick(t, s, "A1", tc.label))
		})
	}
}

// A pick acts on the row it was drawn on, not on the one the cursor was on.
func TestAnAttachmentMenuPickMovesTheCursorToItsRow(t *testing.T) {
	s := attachmentsPane(t)
	out := pick(t, s, "A2", "Save to Downloads")
	save, ok := out.Cmds[0].(model.OpenAttachment)
	if len(out.Cmds) != 1 || !ok || save.Attachment.ID != "A2" || save.Way != model.WaySave {
		t.Fatalf("cmds %#v, want a save of A2", out.Cmds)
	}
}

func TestOpenBesideIsRefusedForATypeTernDoesNotDraw(t *testing.T) {
	s := attachmentsPane(t)
	_, menu := attachmentRow(t, s, "A2")
	reason, _ := menuItem(t, menu, "Open beside")["disabled"].(string)
	if reason == "" {
		t.Fatalf("Open beside of a video is on offer: %v", menuItem(t, menu, "Open beside"))
	}
	if _, menu := attachmentRow(t, s, "A1"); menuItem(t, menu, "Open beside")["disabled"] != nil {
		t.Error("Open beside of an image is refused")
	}
	s = step(t, s, model.DetailItemMsg{Tab: model.TabAttachments, Key: model.AttachmentRowKey("A2")}).State
	out := step(t, s, key("v"))
	if len(out.Cmds) != 0 || out.State.Model.Notice != reason {
		t.Fatalf("v on a video: cmds %#v, notice %q, want the refusal %q", out.Cmds, out.State.Model.Notice, reason)
	}
}

func unlinkAsked(t *testing.T) driver.State {
	t.Helper()
	return pick(t, attachmentsPane(t), "A1", "Unlink…").State
}

func TestTheUnlinkQuestionsYUnlinksAndNKeeps(t *testing.T) {
	out := step(t, unlinkAsked(t), key("y"))
	want := model.UnlinkAttachment{Workspace: "/repo", UID: "a", ProjectID: 7, ID: "A1", Name: "shot.png", FromQuestion: true}
	if len(out.Cmds) != 1 || out.Cmds[0] != model.Cmd(want) {
		t.Fatalf("y sent %#v, want %#v", out.Cmds, want)
	}
	if _, asking := out.State.Model.Question(); asking {
		t.Error("the question stays after y")
	}
	for _, chord := range []string{"n", "escape"} {
		kept := step(t, unlinkAsked(t), key(chord))
		if len(kept.Cmds) != 0 {
			t.Errorf("%s sent %#v", chord, kept.Cmds)
		}
		if _, asking := kept.State.Model.Question(); asking {
			t.Errorf("the question stays after %s", chord)
		}
	}
}

func TestTheUnlinkQuestionsButtonsAnswerItToo(t *testing.T) {
	s := unlinkAsked(t)
	buttons := map[string]string{}
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.K == "button" && strings.Contains(n.ID, "."+string(model.ModeUnlinkAttachment)+".") {
			act, _ := n.P["actions"].(map[string]any)
			buttons[n.ID[strings.LastIndex(n.ID, ".")+1:]], _ = act["click"].(string)
		}
	}
	if buttons["yes"] == "" || buttons["no"] == "" {
		t.Fatalf("buttons %v, want Unlink and Keep", buttons)
	}
	if out := step(t, s, click("", buttons["yes"])); len(out.Cmds) != 1 {
		t.Errorf("Unlink sent %#v", out.Cmds)
	}
	if out := step(t, s, click("", buttons["no"])); len(out.Cmds) != 0 || out.State.Model.AskUnlink != nil {
		t.Errorf("Keep sent %#v, ask %v", out.Cmds, out.State.Model.AskUnlink)
	}
}

func TestAnUnlinkTheQuestionSentIsReportedAndReadsTheIssueAgain(t *testing.T) {
	s := step(t, unlinkAsked(t), key("y")).State
	out := step(t, s, model.AttachmentUnlinked{UID: "a", Name: "shot.png", FromQuestion: true})
	if !strings.HasPrefix(out.State.Model.Notice, "Unlinked shot.png from a;") || !hasCmd[model.ReadSnapshot](out.Cmds) {
		t.Fatalf("notice %q, cmds %#v", out.State.Model.Notice, out.Cmds)
	}
	failed := step(t, s, model.AttachmentUnlinked{UID: "a", Name: "shot.png", FromQuestion: true, Err: errors.New("kata said no")})
	if !strings.Contains(failed.State.Model.Notice, "Cannot unlink shot.png: kata said no") {
		t.Fatalf("notice %q", failed.State.Model.Notice)
	}
}

func TestTheCopyCardHoldsThePathAndTheKataCommandEachWithACopyButton(t *testing.T) {
	s := attachmentsPane(t)
	s = step(t, s, model.AttachmentOpened{UID: "a", AttachmentID: "A1", Name: "shot.png", Path: "/cache/shot.png", Way: model.WayCopy}).State
	if s.Model.TopCard() != model.CardAttachmentCopy {
		t.Fatalf("top card %v", s.Model.TopCard().Name())
	}
	layer := view.LayerID(model.CardAttachmentCopy)
	texts := map[string]string{}
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.K != "input" || !strings.HasPrefix(n.ID, layer+".") {
			continue
		}
		text, _ := n.P["text"].(string)
		buttons, _ := n.P["buttons"].([]any)
		if len(buttons) != 1 || buttons[0].(map[string]any)["act"] != "copy" || n.P["readonly"] != true {
			t.Errorf("%s: buttons %v readonly %v, want one copy button on a read-only field", n.ID, buttons, n.P["readonly"])
		}
		texts[strings.TrimPrefix(n.ID, layer+".")] = text
	}
	if texts["path"] != "/cache/shot.png" || texts["command"] != "kata docbank attachments get work#a A1" || len(texts) != 2 {
		t.Fatalf("fields %v", texts)
	}
	if closed := step(t, s, key("escape")).State.Model; closed.AttachmentCopy.Open {
		t.Error("esc leaves the card open")
	}
}

func TestASavedAttachmentIsToastedWithItsPath(t *testing.T) {
	s := attachmentsPane(t)
	out := step(t, s, model.AttachmentOpened{UID: "a", AttachmentID: "A1", Name: "shot.png", Path: "/home/me/Downloads/shot (1).png", Way: model.WaySave})
	if toast := out.State.Model.Toast; toast == nil || toast.Text != "Saved /home/me/Downloads/shot (1).png" || toast.Label != "" {
		t.Fatalf("toast %+v", toast)
	}
}

func TestTwoUnlinksInFlightAreEachReportedAgainstTheirOwnRequest(t *testing.T) {
	// z+y unlinks shot.png, then U unlinks clip.mp4 before the first answer comes back.
	s := step(t, unlinkAsked(t), key("y")).State
	s = step(t, s, key("shift+u")).State
	if !s.Model.FieldForm.Open {
		t.Fatal("U did not open its card")
	}
	out := step(t, s, key("enter"))
	if len(out.Cmds) != 1 || out.Cmds[0].(model.UnlinkAttachment).Name != "clip.mp4" || out.Cmds[0].(model.UnlinkAttachment).FromQuestion {
		t.Fatalf("U sent %#v", out.Cmds)
	}
	s = out.State

	// The question's answer reports its own failure and leaves the saving U form alone.
	failed := step(t, s, model.AttachmentUnlinked{UID: "a", Name: "shot.png", FromQuestion: true, Err: errors.New("kata said no")})
	f := failed.State.Model.FieldForm
	if !f.Open || !f.Saving || f.Err != nil || failed.State.Model.Notice != "Cannot unlink shot.png: kata said no" {
		t.Fatalf("form %+v, notice %q", f, failed.State.Model.Notice)
	}

	// Only the U path's answer closes the form.
	done := step(t, failed.State, model.AttachmentUnlinked{UID: "a", Name: "clip.mp4"})
	if done.State.Model.FieldForm.Open || !strings.HasPrefix(done.State.Model.Notice, "Unlinked clip.mp4 from a;") {
		t.Fatalf("form %+v, notice %q", done.State.Model.FieldForm, done.State.Model.Notice)
	}
}
