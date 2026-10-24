package driver_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
)

// attentionRequest is a request for dana on an Issue of a Project, read through the same slot reader the gateway uses.
func attentionRequest(t *testing.T, projectID int64, project, short, title, value string, minutesAgo int) model.AttentionRequest {
	t.Helper()
	slot, err := model.ReadNotificationSlot(model.NotificationKey("dana"), value)
	if err != nil {
		t.Fatal(err)
	}
	return model.AttentionRequest{
		IssueUID: project + "-" + short, ProjectID: projectID, ProjectName: project, ShortID: short, Title: title,
		Updated: now.Add(-time.Duration(minutesAgo) * time.Minute), Slot: slot,
	}
}

// attentionRequests are three requests newest first, the second of them in another Project than the Pane's.
func attentionRequests(t *testing.T) []model.AttentionRequest {
	return []model.AttentionRequest{
		attentionRequest(t, 7, "work", "aa11", "Deploy the fix", `{"from":"bob","message":"Please deploy today\nsecond line of the message"}`, 5),
		attentionRequest(t, 8, "other", "bb22", "Review the schema", `{"from":"pip","teammate":"scout","message":"Is this right?","re":"01JABCDEFGHJKMNPQRSTVWXYZ0"}`, 20),
		attentionRequest(t, 7, "work", "cc33", "Release notes", `{"from":"worker","message":"Run held: needs the owner","kind":"reply","broadcast":true}`, 60),
	}
}

// attentionPane is the Pane of a user named dana on a Project Collection of "work", the catalog also listing "other".
func attentionPane(t *testing.T) driver.State {
	t.Helper()
	s := projectPane(t, 160, boardCard("work-aa11"))
	s.Model.Actor = "dana"
	s.Model.Snapshot.Catalog = append(s.Model.Snapshot.Catalog, model.CatalogProject{Project: model.Project{ID: 8, UID: "p2", Name: "other"}})
	return s
}

func askedAttention(cmds []model.Cmd) (model.LoadAttention, bool) {
	for _, cmd := range cmds {
		if load, ok := cmd.(model.LoadAttention); ok {
			return load, true
		}
	}
	return model.LoadAttention{}, false
}

func answerAttention(t *testing.T, out driver.Output, requests []model.AttentionRequest, skipped int) driver.State {
	t.Helper()
	load, ok := askedAttention(out.Cmds)
	if !ok {
		t.Fatalf("no attention read asked for: %#v", out.Cmds)
	}
	return step(t, out.State, model.AttentionLoaded{Request: load.Request, Requests: requests, Skipped: skipped}).State
}

// attentionPageWith is the Pane on the Attention Page with the read answered.
func attentionPageWith(t *testing.T, requests []model.AttentionRequest, skipped int) driver.State {
	t.Helper()
	return answerAttention(t, step(t, attentionPane(t), key("6")), requests, skipped)
}

func attentionCardText(t *testing.T, s driver.State, uid string) string {
	t.Helper()
	id := view.AttentionID(uid)
	var text string
	for _, n := range wireNodes(t, view.Render(s.Model, now)) {
		if n.ID == id || strings.HasPrefix(n.ID, id+".") {
			text += wireText(n)
		}
	}
	if text == "" {
		t.Fatalf("the Attention Page draws no card for %s", uid)
	}
	return text
}

func TestAttentionPageKeyReadsTheInboxAddressAndThePageBarShowsThePage(t *testing.T) {
	s := attentionPane(t)
	out := step(t, s, key("6"))
	load, ok := askedAttention(out.Cmds)
	if !ok || load.Address != "dana" || load.Workspace != "/repo" || load.Request == 0 {
		t.Fatalf("cmds %#v, want a read of dana's requests", out.Cmds)
	}
	m := out.State.Model
	if m.Page != model.PageAttention || m.CurrentMode() != model.ModeAttention || !pageBarDrawn(t, m, "6") {
		t.Fatalf("page %v mode %q, want the Attention Page with its Page bar entry", m.Page, m.CurrentMode())
	}
	if len(m.VisibleColumns()) != 1 || m.VisibleColumns()[0] != model.ColumnList {
		t.Errorf("columns %v, want the one full-width list", m.VisibleColumns())
	}
	if _, ok := askedAttention(step(t, s, click(pageBarEntry(t, m, "6"), view.KeyClick("6"))).Cmds); !ok {
		t.Error("clicking the Attention entry asks for no requests")
	}
	if back := step(t, out.State, key("1")); back.State.Model.Page != model.PageIssues {
		t.Errorf("1 leaves the Attention Page on page %v", back.State.Model.Page)
	}
}

func TestAttentionPageReadsTheInboxUserInsteadOfTheActor(t *testing.T) {
	s := attentionPane(t)
	s.Model.InboxUser = "coordinator/pip"
	load, ok := askedAttention(step(t, s, key("6")).Cmds)
	if !ok || load.Address != "coordinator/pip" {
		t.Fatalf("read %+v, want coordinator/pip's", load)
	}
}

func TestAttentionPageListsRequestsNewestFirstWithTheirSenderTagHandleAndFirstLine(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)

	wantText(t, "work request", attentionCardText(t, s, "work-aa11"), "work#aa11", "Deploy the fix", "bob", "request", "Please deploy today")
	if strings.Contains(attentionCardText(t, s, "other-bb22"), "second line") {
		t.Error("a row that is not selected draws more than the message's first line")
	}
	wantText(t, "other Project's request", attentionCardText(t, s, "other-bb22"), "other#bb22", "Review the schema", "pip/scout", "request", "c:vwxyz0", "Is this right?")
	wantText(t, "reply notice", attentionCardText(t, s, "work-cc33"), "work#cc33", "worker", "reply", "broadcast", "Run held: needs the owner")

	// The drawn order is the order kata's read gave.
	text := wireTextOf(t, s)
	first, second, third := strings.Index(text, "work#aa11"), strings.Index(text, "other#bb22"), strings.Index(text, "work#cc33")
	if !(first >= 0 && first < second && second < third) {
		t.Errorf("drawn order at %d, %d, %d; want newest first", first, second, third)
	}
	if got := s.Model.Attention.Selected; got != "work-aa11" {
		t.Errorf("selected %q, want the first request", got)
	}
}

func TestAttentionPageSelectedRowShowsTheWholeMessageAndTheHeaderCountsRequestsAndSkipped(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 2)
	wantText(t, "selected row", attentionCardText(t, s, "work-aa11"), "Please deploy today", "second line of the message")
	wantText(t, "page", wireTextOf(t, s), "for dana · 3 requests · 2 skipped")

	one := attentionPageWith(t, attentionRequests(t)[:1], 0)
	wantText(t, "page", wireTextOf(t, one), "for dana · 1 request")
	if strings.Contains(wireTextOf(t, one), "skipped") {
		t.Error("the header names skipped slots when none were skipped")
	}
}

func TestAttentionPageMovesTheSelectionWithTheArrowKeys(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	s = feed(t, s, key("down"), key("down"), key("down")).State
	if got := s.Model.Attention.Selected; got != "work-cc33" {
		t.Fatalf("selected %q after moving down past the end, want the last request", got)
	}
	s = step(t, s, key("up")).State
	if got := s.Model.Attention.Selected; got != "other-bb22" {
		t.Errorf("selected %q after up", got)
	}
	if selected, total := s.Model.Position(); selected != 2 || total != 3 {
		t.Errorf("position %d of %d, want 2 of 3", selected, total)
	}
	wantText(t, "selected row", attentionCardText(t, s, "other-bb22"), "Is this right?")
}

func TestAttentionPageSaysWhenNoRequestWaits(t *testing.T) {
	s := attentionPageWith(t, nil, 0)
	wantText(t, "empty Page", wireTextOf(t, s), "No requests for dana", "for dana · 0 requests")
}

func TestAttentionPageFailedReadShowsKatasErrorAndKeepsTheRows(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	out := step(t, s, key("ctrl+r"))
	load, ok := askedAttention(out.Cmds)
	if !ok {
		t.Fatal("ctrl+r asks for no read")
	}
	failed := step(t, out.State, model.AttentionLoaded{Request: load.Request, Err: errors.New("the daemon hiccuped")}).State
	text := wireTextOf(t, failed)
	wantText(t, "failed Page", text, "the daemon hiccuped", "work#aa11", "other#bb22", "work#cc33", "for dana · 3 requests")

	// A later good read ends the failure.
	again := step(t, failed, key("ctrl+r"))
	recovered := answerAttention(t, again, attentionRequests(t)[:1], 0)
	if strings.Contains(wireTextOf(t, recovered), "the daemon hiccuped") {
		t.Error("the error line stays after a good read")
	}

	// With nothing read yet, the Page does not claim there is nothing for dana.
	first := step(t, step(t, attentionPane(t), key("6")).State, model.AttentionLoaded{Request: 1, Err: errors.New("no daemon")}).State
	wantText(t, "first read failed", wireTextOf(t, first), "no daemon")
	if strings.Contains(wireTextOf(t, first), "No requests for dana") {
		t.Error("a failed first read is drawn as an empty inbox")
	}
}

func TestAttentionPageRefusedAddressShowsTheReasonAndReadsNothing(t *testing.T) {
	for name, set := range map[string]func(*model.Model){
		"a blank actor":             func(m *model.Model) { m.Actor = "  " },
		"an inbox user with a tab":  func(m *model.Model) { m.InboxUser = "da\tna" },
		"an inbox user over 128 B": func(m *model.Model) { m.InboxUser = strings.Repeat("a", 129) },
	} {
		s := attentionPane(t)
		set(&s.Model)
		out := step(t, s, key("6"))
		if cmds := out.Cmds; len(cmds) != 0 {
			if _, asked := askedAttention(cmds); asked {
				t.Errorf("%s: a read was asked for", name)
			}
		}
		wantText(t, name, wireTextOf(t, out.State), "refused")
		if strings.Contains(wireTextOf(t, out.State), "No requests for") || strings.Contains(wireTextOf(t, out.State), "requests ·") {
			t.Errorf("%s: the Page draws a header or an empty inbox", name)
		}
		if _, asked := askedAttention(step(t, out.State, key("ctrl+r")).Cmds); asked {
			t.Errorf("%s: ctrl+r asked for a read", name)
		}
	}
}

func TestAttentionPageKeepsTheSelectionByIssueAcrossReads(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	s = step(t, s, key("down")).State // other-bb22

	// A newer request arrives on top: the same Issue stays selected at its new row.
	newest := attentionRequest(t, 7, "work", "dd44", "Fresh", `{"from":"erin","message":"new"}`, 1)
	s = answerAttention(t, step(t, s, key("ctrl+r")), append([]model.AttentionRequest{newest}, attentionRequests(t)...), 0)
	if got := s.Model.Attention.Selected; got != "other-bb22" {
		t.Fatalf("selected %q after a read with a new first row, want other-bb22", got)
	}
	if selected, _ := s.Model.Position(); selected != 3 {
		t.Errorf("selected row %d, want 3", selected)
	}

	// When the selected request is gone, the row now at its place is selected.
	s = answerAttention(t, step(t, s, key("ctrl+r")), []model.AttentionRequest{newest, attentionRequests(t)[0], attentionRequests(t)[2]}, 0)
	if got := s.Model.Attention.Selected; got != "work-cc33" {
		t.Errorf("selected %q after its request cleared, want the one now at its row", got)
	}
}

func TestAttentionPageDropsTheAnswerToAnOlderRead(t *testing.T) {
	s := attentionPane(t)
	older := step(t, s, key("6"))
	newer := step(t, older.State, key("ctrl+r"))
	stale, _ := askedAttention(older.Cmds)
	fresh, _ := askedAttention(newer.Cmds)
	if stale.Request == fresh.Request {
		t.Fatal("two reads share a request number")
	}
	got := step(t, newer.State, model.AttentionLoaded{Request: stale.Request, Requests: attentionRequests(t)}).State
	if len(got.Model.Attention.Requests) != 0 {
		t.Fatalf("the answer to an older read was applied: %d requests", len(got.Model.Attention.Requests))
	}
	got = step(t, got, model.AttentionLoaded{Request: fresh.Request, Requests: attentionRequests(t)[:1]}).State
	if len(got.Model.Attention.Requests) != 1 {
		t.Fatalf("the answer to the latest read was dropped")
	}
}

func TestEnterOnARequestOpensItsIssueOnTheIssuesPageWithTheDetailFocused(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	out := step(t, s, key("enter"))
	m := out.State.Model
	if m.Page != model.PageIssues || m.SelectedUID != "work-aa11" || m.Focus != model.ColumnDetail || m.DetailHidden() {
		t.Fatalf("page %v selected %q focus %v hidden %v; want the Issues Page on the request's Issue with its detail focused", m.Page, m.SelectedUID, m.Focus, m.DetailHidden())
	}
}

func TestEnterOnARequestOfAnotherProjectOpensItsIssueThere(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	s = step(t, s, key("down")).State
	out := step(t, s, key("enter"))
	m := out.State.Model
	if m.Page != model.PageIssues || m.SelectedUID != "other-bb22" || m.Focus != model.ColumnDetail {
		t.Fatalf("page %v selected %q focus %v; want the other Project's Issue with its detail focused", m.Page, m.SelectedUID, m.Focus)
	}
	if m.Collection != model.ProjectCollection("p2", "other") {
		t.Errorf("collection %+v, want the Project the request is in", m.Collection)
	}
	if len(out.Cmds) == 0 {
		t.Error("no read asked for the other Project's Issues")
	}
}

func TestEnterOnAnEmptyAttentionPageDoesNothing(t *testing.T) {
	s := attentionPageWith(t, nil, 0)
	if m := step(t, s, key("enter")).State.Model; m.Page != model.PageAttention {
		t.Fatalf("enter left the Attention Page for page %v", m.Page)
	}
}

func TestSixReturnsToTheAttentionPageAsItWas(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 2)
	s = feed(t, s, key("down"), key("1")).State
	if s.Model.Page != model.PageIssues {
		t.Fatalf("page %v, want the Issues Page", s.Model.Page)
	}
	// Neither a change of Project Collection nor the Page's absence empties it.
	s.Model.Collection = model.ProjectCollection("p2", "other")
	back := step(t, s, key("6"))
	if got := back.State.Model.Attention; got.Selected != "other-bb22" || len(got.Requests) != 3 || got.Skipped != 2 {
		t.Fatalf("state on return %+v, want the requests, the skip count and the selection kept", got)
	}
	wantText(t, "returned Page", wireTextOf(t, back.State), "for dana · 3 requests · 2 skipped", "other#bb22")
	if _, ok := askedAttention(back.Cmds); !ok {
		t.Error("returning to the Page reads no fresh requests")
	}
	// The rows stay on screen while the new read is out, and its answer keeps the selection.
	done := answerAttention(t, back, attentionRequests(t), 0)
	if done.Model.Attention.Selected != "other-bb22" {
		t.Errorf("selected %q after the read, want other-bb22", done.Model.Attention.Selected)
	}
}

func TestClickingARequestSelectsIt(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	out := step(t, s, click(view.AttentionID("work-cc33"), view.AttentionClick("work-cc33")))
	if got := out.State.Model.Attention.Selected; got != "work-cc33" {
		t.Fatalf("selected %q after the click", got)
	}
}

func TestAttentionPageEscOpensTheMenuAndPAndIKeepWorking(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	if got := step(t, s, key("escape")).State.Model; !got.Picker.Open {
		t.Error("esc on the Attention Page left the Menu closed")
	}
	if got := step(t, s, key("shift+p")).State.Model; !got.Picker.Open || !got.Picker.Projects {
		t.Errorf("P gave Menu open %v, Projects level %v", got.Picker.Open, got.Picker.Projects)
	}
	if got := step(t, s, key("shift+i")).State.Model; got.Collection != model.InboxCollection() || got.Page != model.PageIssues {
		t.Errorf("I gave Collection %+v on Page %v", got.Collection, got.Page)
	}
}

func TestAttentionPageHelpSheetListsItsKeys(t *testing.T) {
	var lines []string
	for _, group := range model.HelpSheet() {
		if group.Title == "Attention" {
			for _, line := range group.Lines {
				lines = append(lines, line.Text)
			}
		}
	}
	for _, want := range []string{"next request", "previous request", "open the request's Issue, in any Project", "read the requests again"} {
		if !slices.Contains(lines, want) {
			t.Errorf("the Attention help group lacks %q: %v", want, lines)
		}
	}
}
