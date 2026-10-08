package model

// FirstLoad reports that nothing has loaded yet, so there is no list or
// Collections to keep on screen. Later loads leave what is shown in place.
func (m Model) FirstLoad() bool {
	return m.Loading && len(m.Issues) == 0 && len(m.Collections) == 0
}

// DetailIssueUID is the Issue whose detail is on screen: while another
// Issue's detail loads, still the previous one.
func (m Model) DetailIssueUID() string {
	if m.Detail == nil || m.Detail.Issue == nil {
		return ""
	}
	return m.Detail.Issue.UID
}
