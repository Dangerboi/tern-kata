package model

// resetPagesForProject is the one Project-change rule of the Pages that act on a Project: a Page that was filled for another
// Project than the Pane's starts empty again (its declaration's reset) and records the Pane's. While the Pane browses a fixed
// Collection, such as the Inbox, no Project is selected and every Page keeps what it holds, so a round trip through the Inbox back
// to the same Project loses nothing. What each reset keeps is the Page's request counters (stepped, so a read asked before the
// reset is ignored when it answers). The Project's numeric id is not part of it: each Page resolves that itself, and a missing id
// waits for the read.
func (m Model) resetPagesForProject() Model {
	if !m.Collection.IsProject() {
		return m
	}
	uid := m.Collection.ProjectUID
	for _, def := range pageDefs {
		if def.reset != nil && def.filledFor(m) != uid {
			m = def.reset(m, uid)
		}
	}
	return m
}

func jobsFilledFor(m Model) string { return m.Jobs.ProjectUID }

func resetJobs(m Model, uid string) Model {
	m.Jobs = JobsState{
		ProjectUID: uid,
		request:    m.Jobs.request + 1,
		stale:      true,
		pick:       m.Jobs.pick,
		rev:        m.Jobs.rev,
		Writing:    m.Jobs.Writing,
	}
	return m
}

func runsFilledFor(m Model) string { return m.Runs.ProjectUID }

func resetRuns(m Model, uid string) Model {
	m.Runs = RunsState{
		ProjectUID: uid,
		request:    m.Runs.request + 1,
		stale:      true,
	}
	return m
}

func workflowsFilledFor(m Model) string { return m.Workflows.ProjectUID }

func resetWorkflows(m Model, uid string) Model {
	w := m.Workflows
	m.Workflows = WorkflowsState{
		ProjectUID:   uid,
		jobsRequest:  w.jobsRequest - 1,
		runsRequest:  w.runsRequest - 1,
		checkRequest: w.checkRequest + 1,
	}
	return m
}
