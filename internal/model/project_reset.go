package model

// pageProjectUID is the Project the Pane shows, by uid: "" while the Collection is not a Project.
func (m Model) pageProjectUID() string {
	if m.Collection.IsProject() {
		return m.Collection.ProjectUID
	}
	return ""
}

// resetPagesForProject is the one Project-change rule of the Pages that act on a Project: a Page that was filled for another
// Project than the Pane's starts empty again and records the Pane's. What it keeps is the Page's request counters (stepped, so
// a read asked before the reset is ignored when it answers) and its detail column flag. The Project's numeric id is not part of
// it: each Page resolves that itself, and a missing id waits for the read.
func (m Model) resetPagesForProject() Model {
	uid := m.pageProjectUID()
	if m.Jobs.ProjectUID != uid {
		m.Jobs = JobsState{
			ProjectUID:   uid,
			request:      m.Jobs.request + 1,
			stale:        true,
			detailHidden: m.Jobs.detailHidden,
			pick:         m.Jobs.pick,
			rev:          m.Jobs.rev,
			Writing:      m.Jobs.Writing,
		}
	}
	if m.Runs.ProjectUID != uid {
		m.Runs = RunsState{
			ProjectUID:   uid,
			request:      m.Runs.request + 1,
			stale:        true,
			detailHidden: m.Runs.detailHidden,
		}
	}
	if w := m.Workflows; w.ProjectUID != uid {
		m.Workflows = WorkflowsState{
			ProjectUID:   uid,
			jobsRequest:  w.jobsRequest - 1,
			runsRequest:  w.runsRequest - 1,
			detailHidden: w.detailHidden,
		}
	}
	if m.Board.ProjectUID != uid {
		m.Board.ProjectUID = uid
		m.Board.Lanes, m.Board.DoneDays, m.Board.DoneCards = nil, 0, 0
		m.Board.fileErr, m.Board.loaded = nil, false
	}
	return m
}
