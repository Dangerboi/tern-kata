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
		"a blank actor":            func(m *model.Model) { m.Actor = "  " },
		"an inbox user with a tab": func(m *model.Model) { m.InboxUser = "da\tna" },
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
	for _, want := range []string{"next request", "previous request", "open the request's Issue, in any Project", "clear the request, if it still reads as shown", "read the requests again"} {
		if !slices.Contains(lines, want) {
			t.Errorf("the Attention help group lacks %q: %v", want, lines)
		}
	}
}

func askedClear(cmds []model.Cmd) []model.ClearAttention {
	var clears []model.ClearAttention
	for _, cmd := range cmds {
		if clear, ok := cmd.(model.ClearAttention); ok {
			clears = append(clears, clear)
		}
	}
	return clears
}

func TestCSendsOneGuardedClearAndKeysOnTheRowWaitUntilItIsAnswered(t *testing.T) {
	requests := attentionRequests(t)
	s := attentionPageWith(t, requests, 0)
	out := step(t, s, key("c"))
	clears := askedClear(out.Cmds)
	want := model.ClearAttention{Workspace: "/repo", ProjectID: 7, UID: "work-aa11", Key: model.NotificationKey("dana"), Seen: requests[0].Slot.Raw, Actor: "dana"}
	if len(clears) != 1 || clears[0] != want {
		t.Fatalf("cmds %#v, want exactly the guarded clear %+v", out.Cmds, want)
	}
	if want.Seen == "" {
		t.Fatal("the guard holds no value")
	}

	// A second c or an Enter on the row waits.
	again := step(t, out.State, key("c"))
	if len(askedClear(again.Cmds)) != 0 {
		t.Errorf("a second c sent another clear: %#v", again.Cmds)
	}
	waiting := step(t, again.State, key("enter"))
	if waiting.State.Model.Page != model.PageAttention {
		t.Errorf("enter on the row being cleared left the Page for %v", waiting.State.Model.Page)
	}

	// Another row is not waiting.
	other := step(t, out.State, key("down"))
	if clears := askedClear(step(t, other.State, key("c")).Cmds); len(clears) != 1 || clears[0].UID != "other-bb22" {
		t.Errorf("c on another row sent %#v", clears)
	}
}

func TestAClearedRequestLeavesTheListAndThePageReadsAgain(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	cleared := step(t, step(t, s, key("c")).State, model.AttentionCleared{UID: "work-aa11"})
	m := cleared.State.Model
	if len(m.Attention.Requests) != 2 || m.Attention.Selected != "other-bb22" {
		t.Fatalf("requests %d selected %q, want the row gone and the next one selected", len(m.Attention.Requests), m.Attention.Selected)
	}
	if strings.Contains(wireTextOf(t, cleared.State), "work#aa11") {
		t.Error("the cleared request is still drawn")
	}
	wantText(t, "header", wireTextOf(t, cleared.State), "for dana · 2 requests")
	if _, ok := askedAttention(cleared.Cmds); !ok {
		t.Error("no read asked for to confirm the clear")
	}
	// The row is free again if the same request comes back.
	back := answerAttention(t, cleared, attentionRequests(t), 0)
	if len(askedClear(step(t, back, key("c")).Cmds)) != 1 {
		t.Error("c waits after the clear was answered")
	}
}

func TestAClearThatFindsTheRequestChangedSaysSoAndReadsAgain(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	out := step(t, step(t, s, key("c")).State, model.AttentionCleared{UID: "work-aa11", Changed: true})
	m := out.State.Model
	if m.Notice != "The request changed meanwhile; nothing was cleared" {
		t.Fatalf("notice %q", m.Notice)
	}
	if len(m.Attention.Requests) != 3 {
		t.Errorf("%d requests, want the row kept until the read shows the new value", len(m.Attention.Requests))
	}
	wantText(t, "page", wireTextOf(t, out.State), "nothing was cleared")
	if _, ok := askedAttention(out.Cmds); !ok {
		t.Error("no reload asked for")
	}
	fresh := attentionRequest(t, 7, "work", "aa11", "Deploy the fix", `{"from":"bob","message":"The new ask"}`, 1)
	done := answerAttention(t, out, append([]model.AttentionRequest{fresh}, attentionRequests(t)[1:]...), 0)
	wantText(t, "reloaded row", attentionCardText(t, done, "work-aa11"), "The new ask")
}

func TestAClearThatFailsShowsTheDaemonsErrorAndKeepsTheRow(t *testing.T) {
	s := attentionPageWith(t, attentionRequests(t), 0)
	out := step(t, step(t, s, key("c")).State, model.AttentionCleared{UID: "work-aa11", Err: errors.New("the daemon refused")})
	if out.State.Model.Notice != "the daemon refused" || len(out.State.Model.Attention.Requests) != 3 {
		t.Fatalf("notice %q, %d requests", out.State.Model.Notice, len(out.State.Model.Attention.Requests))
	}
	if len(askedClear(step(t, out.State, key("c")).Cmds)) != 1 {
		t.Error("c still waits after the failure was answered")
	}
}

func TestCOnAnEmptyAttentionPageSendsNothing(t *testing.T) {
	if len(askedClear(step(t, attentionPageWith(t, nil, 0), key("c")).Cmds)) != 0 {
		t.Error("c sent a clear with no request")
	}
}

// attentionEvent is a stream event of kind on the Pane's current stream.
func attentionEvent(s driver.State, kind string) model.StreamEvent {
	return model.StreamEvent{Generation: s.Model.StreamGeneration, ID: nextEvent(), Type: kind}
}

// attentionEntryText is what the Page bar draws for the Attention entry.
func attentionEntryText(t *testing.T, s driver.State) string {
	t.Helper()
	return pageBarTexts(t, s)["attention"]
}

func countedReads(cmds []model.Cmd) int {
	n := 0
	for _, cmd := range cmds {
		if _, ok := cmd.(model.LoadAttention); ok {
			n++
		}
	}
	return n
}

func TestTheAttentionReadIsAskedWhenTheStreamOpensAndTheCountShowsOnAnyPage(t *testing.T) {
	s := attentionPane(t)
	if text := attentionEntryText(t, s); !strings.Contains(text, `"t":"Attention"`) || strings.Contains(text, "Attention 0") {
		t.Fatalf("Page bar entry %s before any read, want plain Attention", text)
	}
	out := step(t, s, model.StreamOpened{Generation: s.Model.StreamGeneration})
	if countedReads(out.Cmds) != 1 {
		t.Fatalf("cmds %#v, want one Attention read when the stream opens", out.Cmds)
	}
	if out.State.Model.Page != model.PageIssues {
		t.Fatalf("page %v, want the Issues Page", out.State.Model.Page)
	}
	done := answerAttention(t, out, attentionRequests(t), 0)
	if text := attentionEntryText(t, done); !strings.Contains(text, `"t":"Attention 3"`) {
		t.Errorf("Page bar entry %s, want Attention 3 from the Issues Page", text)
	}
	if text := attentionEntryText(t, done); strings.Contains(text, "request") {
		t.Errorf("Page bar entry %s", text)
	}

	// Another stream's opening is not this Pane's.
	if _, asked := askedAttention(step(t, s, model.StreamOpened{Generation: s.Model.StreamGeneration + 5}).Cmds); asked {
		t.Error("an old stream's opening asked for a read")
	}
}

func TestThePageBarEntryIsPlainAtZeroAndFollowsEachGoodRead(t *testing.T) {
	s := answerAttention(t, step(t, attentionPane(t), model.StreamOpened{Generation: attentionPane(t).Model.StreamGeneration}), attentionRequests(t), 0)
	out := step(t, s, attentionEvent(s, "issue.metadata_updated"))
	s = answerAttention(t, out, attentionRequests(t)[:1], 0)
	if text := attentionEntryText(t, s); !strings.Contains(text, `"t":"Attention 1"`) {
		t.Fatalf("Page bar entry %s, want Attention 1", text)
	}
	s = answerAttention(t, step(t, s, attentionEvent(s, "issue.updated")), nil, 0)
	if text := attentionEntryText(t, s); !strings.Contains(text, `"t":"Attention"`) || strings.Contains(text, "Attention 0") {
		t.Fatalf("Page bar entry %s, want plain Attention at zero", text)
	}
}

func TestAFailedReadKeepsThePageBarCount(t *testing.T) {
	s := answerAttention(t, step(t, attentionPane(t), model.StreamOpened{Generation: attentionPane(t).Model.StreamGeneration}), attentionRequests(t), 0)
	out := step(t, s, attentionEvent(s, "project.updated"))
	load, ok := askedAttention(out.Cmds)
	if !ok {
		t.Fatal("a project event asked for no read")
	}
	failed := step(t, out.State, model.AttentionLoaded{Request: load.Request, Err: errors.New("the daemon hiccuped")}).State
	if text := attentionEntryText(t, failed); !strings.Contains(text, `"t":"Attention 3"`) {
		t.Errorf("Page bar entry %s after a failed read, want Attention 3", text)
	}
}

func TestABurstOfEventsCostsOneReadInFlightAndOneAfterIt(t *testing.T) {
	s := attentionPane(t)
	first := step(t, s, attentionEvent(s, "issue.created"))
	if countedReads(first.Cmds) != 1 {
		t.Fatalf("the first event asked for %d reads, want 1", countedReads(first.Cmds))
	}
	load, _ := askedAttention(first.Cmds)

	state := first.State
	for _, kind := range []string{"issue.updated", "issue.commented", "project.updated", "sync.reset_required"} {
		out := step(t, state, attentionEvent(state, kind))
		if countedReads(out.Cmds) != 0 {
			t.Fatalf("%s asked for another read while one is out: %#v", kind, out.Cmds)
		}
		state = out.State
		if kind == "sync.reset_required" {
			state.Model.StreamGeneration = s.Model.StreamGeneration // the reset ends the stream; the test goes on with the same one
		}
	}

	answered := step(t, state, model.AttentionLoaded{Request: load.Request, Requests: attentionRequests(t)})
	if countedReads(answered.Cmds) != 1 {
		t.Fatalf("answer asked for %d reads, want the one follow-up", countedReads(answered.Cmds))
	}
	followUp, _ := askedAttention(answered.Cmds)
	if followUp.Request == load.Request {
		t.Fatal("the follow-up repeats the first request number")
	}
	last := step(t, answered.State, model.AttentionLoaded{Request: followUp.Request, Requests: attentionRequests(t)[:2]})
	if countedReads(last.Cmds) != 0 {
		t.Fatalf("the follow-up's answer asked for more: %#v", last.Cmds)
	}
	if len(last.State.Model.Attention.Requests) != 2 {
		t.Errorf("requests %d, want the follow-up's two", len(last.State.Model.Attention.Requests))
	}
}

func TestEventsThatCannotTouchARequestAskForNoRead(t *testing.T) {
	s := attentionPane(t)
	if out := step(t, s, attentionEvent(s, "cron.run.observed")); countedReads(out.Cmds) != 0 {
		t.Errorf("a run event asked for a read: %#v", out.Cmds)
	}
}

func TestAnIssueEventAsksForAReadWhileAnotherPageIsShown(t *testing.T) {
	s := step(t, attentionPane(t), key("4")).State // the Runs Page
	if out := step(t, s, attentionEvent(s, "issue.closed")); countedReads(out.Cmds) != 1 {
		t.Errorf("an event on the Runs Page asked for %d Attention reads, want 1", countedReads(out.Cmds))
	}
}

func TestThePageBarWithACountStillFitsTheWidthRules(t *testing.T) {
	long := model.Project{ID: 7, UID: "p", Name: "a-rather-long-project-name-for-the-top-bar"}
	for _, tt := range []struct {
		cols    int
		compact bool
		ownLine bool
	}{
		{200, false, false},
		{100, true, false},
		{80, true, true},
	} {
		s := loadedWith(t, tt.cols, long, model.Snapshot{Issues: []model.Issue{alpha}})
		s.Model.Attention.Requests = attentionRequests(t)
		texts := pageBarTexts(t, s)
		if got := strings.Contains(texts["attention"], `"t":"Attention 3"`); got == tt.compact {
			t.Errorf("%d columns: Attention entry %s, compact %v", tt.cols, texts["attention"], tt.compact)
		}
		_, own := drawnIDs(t, view.Render(s.Model, now))["main.pagebar"]
		if own != tt.ownLine {
			t.Errorf("%d columns: Page bar on its own line = %v, want %v", tt.cols, own, tt.ownLine)
		}
	}
}
