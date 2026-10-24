package model

import "testing"

// fakeWrite stands for a write added after this was written, in neither list.
type fakeWrite struct{}

func (fakeWrite) cmd() {}

func TestAnUnlistedCmdClearsTheHistoryAndReadsDoNot(t *testing.T) {
	for _, cmd := range []Cmd{fakeWrite{}, RemoveLinkCmd{}} {
		if reason, clears := undoBoundary(cmd); !clears || reason != undoUnknownBoundary {
			t.Errorf("%T: clears %v with %q", cmd, clears, reason)
		}
	}
	for _, cmd := range []Cmd{ReadSnapshot{}, LoadDetail{}, LoadAttention{}, LoadAuthCapabilities{}, LoadCredentials{}, LoadFederation{}, LoadFederationHubs{}, LoadFederationHubProjects{}, BrowseFederationHubs{}, WriteField{Action: AddLabelAction}, CloseIssueCmd{}, ReopenIssueCmd{}, MoveIssueCmd{}, SetDatesCmd{}, SaveDraft{}, UndoChange{}} {
		if reason, clears := undoBoundary(cmd); clears {
			t.Errorf("%T clears the history with %q", cmd, reason)
		}
	}
}
