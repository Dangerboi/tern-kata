package model

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// The Attention Page lists the Attention requests kata holds for the Pane's Inbox address, across every Project on the daemon,
// newest first. It acts on no Project: its state survives Project and Collection changes, and it is one full-width list.

// AttentionRequest is one Attention request: the open Issue it sits on and the slot kata holds for the Inbox address.
type AttentionRequest struct {
	IssueUID    string
	ProjectID   int64
	ProjectName string
	ShortID     string
	Title       string
	Updated     time.Time
	// Slot is the request as the shared slot reader decodes it; its Raw is the stored value as read.
	Slot NotificationSlot
}

// QualifiedID is the Issue as kata names it across Projects: project#short.
func (r AttentionRequest) QualifiedID() string { return r.ProjectName + "#" + r.ShortID }

// Sender is who asked: the actor, or actor/teammate.
func (r AttentionRequest) Sender() string {
	if r.Slot.Teammate == "" {
		return r.Slot.From
	}
	return r.Slot.From + "/" + r.Slot.Teammate
}

// Tag tells what the slot is: "request" for a plain request, the reply kind for a reply notice, and "broadcast" for one sent to
// everyone on an Issue tree.
func (r AttentionRequest) Tag() string {
	var tags []string
	switch {
	case r.Slot.Kind != "":
		tags = append(tags, r.Slot.Kind)
	case !r.Slot.Broadcast:
		tags = append(tags, "request")
	}
	if r.Slot.Broadcast {
		tags = append(tags, "broadcast")
	}
	return strings.Join(tags, " ")
}

// commentHandleLen is how many characters of a comment's ULID its handle keeps.
const commentHandleLen = 6

// CommentHandle is the handle of the comment the request names, as kata prints it ("c:" and the last six characters of the ULID in
// lower case); empty when it names none.
func (r AttentionRequest) CommentHandle() string {
	handle := strings.ToLower(strings.TrimSpace(r.Slot.Re))
	if handle == "" {
		return ""
	}
	if len(handle) > commentHandleLen {
		handle = handle[len(handle)-commentHandleLen:]
	}
	return "c:" + handle
}

// FirstLine is the first line of the message.
func (r AttentionRequest) FirstLine() string {
	line, _, _ := strings.Cut(strings.TrimSpace(r.Slot.Message), "\n")
	return strings.TrimSpace(line)
}

// OrderAttention puts the requests newest Issue update first, ties by qualified id.
func OrderAttention(requests []AttentionRequest) {
	slices.SortFunc(requests, func(a, b AttentionRequest) int {
		if c := b.Updated.Compare(a.Updated); c != 0 {
			return c
		}
		return strings.Compare(a.QualifiedID(), b.QualifiedID())
	})
}

// LoadAttention asks for every open Issue of the daemon that holds a request for Address. Request numbers the asks: an answer to
// an older one is dropped.
type LoadAttention struct {
	Workspace string
	Address   string
	Request   int64
}

func (LoadAttention) cmd() {}

// AttentionLoaded answers LoadAttention: the requests newest first, and how many slots did not read.
type AttentionLoaded struct {
	Request  int64
	Requests []AttentionRequest
	Skipped  int
	Err      error
}

func (AttentionLoaded) msg() {}

// ClearAttention clears the request held at Key on an Issue, guarded on the slot still holding Seen, the value the Page showed.
// The write is attributed to Actor.
type ClearAttention struct {
	Workspace string
	ProjectID int64
	UID       string
	Key       string
	Seen      string
	Actor     string
}

func (ClearAttention) cmd() {}

// AttentionCleared answers ClearAttention for the Issue with UID. Changed says the slot no longer held what the Page showed, so
// nothing was cleared.
type AttentionCleared struct {
	UID     string
	Changed bool
	Err     error
}

func (AttentionCleared) msg() {}

// AttentionChangedNotice is the Notice for a request rewritten between the Page's read and the clear.
const AttentionChangedNotice = "The request changed meanwhile; nothing was cleared"

// AttentionSelectMsg is a click on a request's row; UID is its Issue's.
type AttentionSelectMsg struct{ UID string }

func (AttentionSelectMsg) msg() {}

// AttentionState is the Attention Page's state. It is not saved: a Pane starts on the Issues Page.
type AttentionState struct {
	// Requests are the requests of the last good read, newest first.
	Requests []AttentionRequest
	Skipped  int
	// Loaded says a read has answered well; Err is the last read's failure, with Requests kept.
	Loaded bool
	Err    error
	// Selected is the Issue uid of the selected request.
	Selected string
	Loading  bool
	// Clearing holds the Issue uids of the requests whose clear is out; keys on those rows wait.
	Clearing []string
	// request counts the reads asked for.
	request int64
}

// OnAttention says the Attention Page is current.
func (m Model) OnAttention() bool { return m.Page == PageAttention }

// InboxAddress is the address whose requests the Page reads: KATA_INBOX_USER when it is not blank, else the Pane's actor, checked
// by kata's recipient rules. The error names why it is refused.
func (m Model) InboxAddress() (string, error) {
	raw := strings.TrimSpace(m.InboxUser)
	if raw == "" {
		raw = m.Actor
	}
	return NormalizeRecipient(raw)
}

// AttentionHeader is the Page's heading: whose requests these are, how many, and how many slots did not read. It is empty when
// the address is refused.
func (m Model) AttentionHeader() string {
	address, err := m.InboxAddress()
	if err != nil {
		return ""
	}
	noun := "requests"
	if len(m.Attention.Requests) == 1 {
		noun = "request"
	}
	header := fmt.Sprintf("for %s · %d %s", address, len(m.Attention.Requests), noun)
	if m.Attention.Skipped > 0 {
		header += fmt.Sprintf(" · %d skipped", m.Attention.Skipped)
	}
	return header
}

// SelectedAttention is the selected request, false when the list holds none.
func (m Model) SelectedAttention() (AttentionRequest, bool) {
	i := m.attentionIndex()
	if i < 0 {
		return AttentionRequest{}, false
	}
	return m.Attention.Requests[i], true
}

func (m Model) attentionIndex() int {
	return slices.IndexFunc(m.Attention.Requests, func(r AttentionRequest) bool { return r.IssueUID == m.Attention.Selected })
}

// askAttention reads the requests again. A refused address reads nothing.
func askAttention(m Model) (Model, []Cmd) {
	address, err := m.InboxAddress()
	if err != nil {
		return m, nil
	}
	m.Attention.request++
	m.Attention.Loading = true
	return m, []Cmd{LoadAttention{Workspace: m.Workspace, Address: address, Request: m.Attention.request}}
}

// enterAttention is the Attention Page's own work on entry (see switchPage): the state as the Page left it, read again.
func enterAttention(m Model) (Model, []Cmd) { return askAttention(m) }

// attentionLoaded applies the answer to the latest read. A failed read keeps the requests held; a good one replaces them and
// keeps the selection on the same Issue, else on the row now at its place.
func attentionLoaded(m Model, msg AttentionLoaded) Model {
	if msg.Request != m.Attention.request {
		return m
	}
	m.Attention.Loading = false
	m.Attention.Err = msg.Err
	if msg.Err != nil {
		return m
	}
	row := max(m.attentionIndex(), 0)
	m.Attention.Requests, m.Attention.Skipped, m.Attention.Loaded = msg.Requests, msg.Skipped, true
	return m.settleAttentionSelection(row)
}

// settleAttentionSelection keeps the selection on a listed request: the one selected, else the one now at row, else the last.
func (m Model) settleAttentionSelection(row int) Model {
	switch {
	case len(m.Attention.Requests) == 0:
		m.Attention.Selected = ""
	case m.attentionIndex() < 0:
		m.Attention.Selected = m.Attention.Requests[min(row, len(m.Attention.Requests)-1)].IssueUID
	}
	return m
}

// moveAttention moves the selection by action: one row, to either end, or a page.
func moveAttention(m Model, action Action) Model {
	count := len(m.Attention.Requests)
	if count == 0 {
		return m
	}
	index := max(m.attentionIndex(), 0)
	switch action {
	case MoveDown:
		index = min(index+1, count-1)
	case MoveUp:
		index = max(index-1, 0)
	case MoveFirst:
		index = 0
	case MoveLast:
		index = count - 1
	case PageDown:
		index = min(index+pageRows, count-1)
	case PageUp:
		index = max(index-pageRows, 0)
	}
	m.Attention.Selected = m.Attention.Requests[index].IssueUID
	return m
}

// clearAttention is c: the selected request is cleared, guarded on the value the Page shows, unless its clear is already out.
func clearAttention(m Model) (Model, []Cmd) {
	request, ok := m.SelectedAttention()
	address, err := m.InboxAddress()
	if !ok || err != nil || m.attentionClearing(request.IssueUID) {
		return m, nil
	}
	m.Attention.Clearing = slices.Concat(m.Attention.Clearing, []string{request.IssueUID})
	return m, []Cmd{ClearAttention{Workspace: m.Workspace, ProjectID: request.ProjectID, UID: request.IssueUID, Key: NotificationKey(address), Seen: request.Slot.Raw, Actor: m.Actor}}
}

func (m Model) attentionClearing(uid string) bool { return slices.Contains(m.Attention.Clearing, uid) }

// attentionCleared applies the answer to a clear. Success takes the row off the list; a request changed meanwhile, or a failure,
// says so in a Notice. All but success read again, which shows what is held now, and so does success, to confirm.
func attentionCleared(m Model, msg AttentionCleared) (Model, []Cmd) {
	m.Attention.Clearing = slices.DeleteFunc(slices.Clone(m.Attention.Clearing), func(uid string) bool { return uid == msg.UID })
	switch {
	case msg.Err != nil:
		m.Notice = msg.Err.Error()
	case msg.Changed:
		m.Notice = AttentionChangedNotice
	default:
		row := max(m.attentionIndex(), 0)
		m.Attention.Requests = slices.DeleteFunc(slices.Clone(m.Attention.Requests), func(r AttentionRequest) bool { return r.IssueUID == msg.UID })
		if m.Attention.Selected == msg.UID {
			m.Attention.Selected = ""
		}
		m = m.settleAttentionSelection(row)
	}
	return askAttention(m)
}

// reloadAttention is ctrl+r.
func reloadAttention(m Model) (Model, []Cmd) { return askAttention(m) }

// openAttention is enter: the request's Issue opens on the Issues Page with its detail focused, in whichever Project it is.
func openAttention(m Model) (Model, []Cmd) {
	request, ok := m.SelectedAttention()
	if !ok || m.attentionClearing(request.IssueUID) {
		return m, nil
	}
	m = m.leavePage()
	if m.SelectedUID == request.IssueUID {
		return m.focusDetail(), nil
	}
	if target, listed := m.catalogProject(request.ProjectID); listed && !(m.Collection.IsProject() && m.Collection.ProjectUID == target.Project.UID) {
		return m.crossJump(request.IssueUID, target, nil)
	}
	return m.jumpWithin(request.IssueUID, nil), nil
}

// catalogProject is the catalog's entry for the Project with id.
func (m Model) catalogProject(id int64) (CatalogProject, bool) {
	for _, entry := range m.Snapshot.Catalog {
		if entry.Project.ID == id {
			return entry, true
		}
	}
	return CatalogProject{}, false
}

// updateAttention applies the Attention Page's results and selections, whichever Page is current.
func updateAttention(m Model, msg Msg) (Model, []Cmd, bool) {
	switch msg := msg.(type) {
	case AttentionLoaded:
		return attentionLoaded(m, msg), nil, true
	case AttentionCleared:
		next, cmds := attentionCleared(m, msg)
		return next, cmds, true
	case AttentionSelectMsg:
		m.Attention.Selected, m.Focus = msg.UID, ColumnList
		return m, nil, true
	}
	return m, nil, false
}

// attentionRows are the Attention Page's rows outside its generated movement rows.
var attentionRows = []actionEntry{
	{action: OpenAttentionRequest, modes: []Mode{ModeAttention}, keys: keys("enter", "↵"), help: "open", footer: []Mode{ModeAttention}, footerKey: "↵", sheet: []sheetLine{{sheetAttention, 3, "open the request's Issue, in any Project"}}, handle: openAttention},
	{action: ClearAttentionRequest, modes: []Mode{ModeAttention}, keys: keys("c", "c"), help: "clear", footer: []Mode{ModeAttention}, sheet: []sheetLine{{sheetAttention, 4, "clear the request, if it still reads as shown"}}, handle: clearAttention},
	{action: ReloadAttention, modes: []Mode{ModeAttention}, keys: keys("ctrl+r", "ctrl+r", "meta+r", ""), help: "reload", sheet: []sheetLine{{sheetAttention, 5, "read the requests again"}}, handle: reloadAttention},
}
