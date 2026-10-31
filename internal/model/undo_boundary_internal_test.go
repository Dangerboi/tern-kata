package model

import "testing"

func TestUndoBoundaryByCmd(t *testing.T) {
	table := []struct {
		name   string
		cmd    Cmd
		clears bool
	}{
		{"share lane list", ShareLanes{}, false},
		{"save lane list", SaveLanes{}, false},
		{"load lanes", LoadLanes{}, false},
		{"close issue", CloseIssueCmd{}, false},
		{"write field", WriteField{}, false},
		{"restore issue", RestoreIssueCmd{}, true},
		{"create issue", CreateIssueCmd{}, true},
		{"attach file", AttachFile{}, true},
		{"save job", SaveJobCmd{}, true},
		{"save workflow", SaveWorkflow{}, true},
	}
	for _, row := range table {
		t.Run(row.name, func(t *testing.T) {
			if _, clears := undoBoundary(row.cmd); clears != row.clears {
				t.Errorf("clears the Undo history = %v, want %v", clears, row.clears)
			}
		})
	}
}

func TestSharingTheLaneListKeepsTheUndoHistory(t *testing.T) {
	m := Model{}
	m.Undo = m.Undo.pushed(UndoEntry{Kind: UndoPriority, UID: "zzz"})
	m = m.noteWrites([]Cmd{ShareLanes{ProjectUID: "p", Lanes: []string{"Todo"}}})
	if len(m.Undo.Entries) != 1 || m.Undo.Boundary != "" {
		t.Fatalf("history = %+v, want the entry kept and no boundary", m.Undo)
	}
}
