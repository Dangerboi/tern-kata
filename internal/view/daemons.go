package view

import (
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

// DaemonListID is the Daemons Page's table.
const DaemonListID = "main.columns.list.daemons"

// daemons is the Daemons Page: the catalog's daemons as a table of name, endpoint, authentication and state, with the one under
// the cursor drawn selected. The table keeps its selected row in view itself.
func daemons(m model.Model) tern.Element {
	d := m.Daemons
	notice, noticed := d.Notice()
	count := ""
	if !noticed {
		count = strconv.Itoa(len(d.Entries))
	}
	children := ui.Nodes(pageHead("heading", "Daemons", count))
	if noticed {
		if notice.Empty {
			empty := emptyState(notice.Text, notice.Detail, emptyButton(m, "new", model.NewDaemon, "New daemon", true)...)
			empty.Key = "empty"
			return listPanel(m, 0.4, append(children, empty))
		}
		return listPanel(m, 0.4, append(children, noticeLines(notice.Err, !notice.Pending, notice.Text, notice.Detail)...))
	}
	rows := make([]ui.TableRow, len(d.Entries))
	for i, e := range d.Entries {
		rows[i] = ui.TableRow{ID: e.Name, Cells: map[string]ui.Cell{
			"name":     ui.Spans(ui.Span{T: e.Label()}),
			"endpoint": ui.Spans(ui.Span{T: e.Where()}),
			"auth":     ui.Spans(ui.Span{T: e.Auth(), S: "muted"}),
			"state":    ui.Spans(ui.Span{T: e.State(), S: string(ui.ToneSuccess)}),
		}}
	}
	selected := ""
	if entry := m.SelectedDaemon(); entry != nil {
		selected = entry.Name
	}
	children = append(children, ui.Table{
		Common: ui.Common{Key: "daemons", Grow: 1},
		Cols: []ui.Column{
			{ID: "name", Head: ui.T("Name"), Grow: 1, Priority: 4, Truncate: ui.TruncateEnd},
			{ID: "endpoint", Head: ui.T("Endpoint"), Mono: true, Grow: 1, Priority: 3, Truncate: ui.TruncateEnd},
			{ID: "auth", Head: ui.T("Auth"), Priority: 2},
			{ID: "state", Head: ui.T("State"), Priority: 1},
		},
		Rows:     rows,
		Selected: selected,
	})
	if entry := m.SelectedDaemon(); entry != nil && entry.AllowInsecure {
		children = append(children, plainText("footer", ui.ToneMuted, "Insecure connection allowed."))
	}
	if line := d.SwitchingLine(); line != "" {
		children = append(children, plainText("switching", ui.TonePending, line))
	}
	return listPanel(m, 0.4, children)
}
