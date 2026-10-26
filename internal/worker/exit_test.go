package worker

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A process that exits while a descendant still holds its output pipes makes Wait return ErrWaitDelay; the exit is the
// process's own, so a clean exit is recorded as code 0 and a failure keeps its code.
func TestAnExitWithADescendantHoldingTheOutputIsRecordedByItsOwnStatus(t *testing.T) {
	for _, tc := range []struct {
		script string
		want   int
	}{
		{"sleep 5 & exit 0", 0},
		{"sleep 5 & exit 3", 3},
	} {
		t.Run(tc.script, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", tc.script)
			cmd.WaitDelay = 100 * time.Millisecond
			var out strings.Builder
			cmd.Stdout = &out // a Go writer: Wait copies through a pipe the descendant keeps open
			err := cmd.Run()
			if code, signal := exitStatus(cmd, err); code != tc.want || signal != "" {
				t.Fatalf("exit = %d %q, want %d (wait error %v)", code, signal, tc.want, err)
			}
		})
	}
}

// The kept stderr tail is plain text: colors and other control bytes would swell the journal when JSON-escaped.
func TestTheStderrTailIsPlainTextSmallInTheJournal(t *testing.T) {
	s := newTailSink(Hooks{}, filepath.Join(t.TempDir(), "stderr.log"))
	defer s.close()
	var in strings.Builder
	for range 30 {
		in.WriteString("\x1b[31merror:\x1b[0m bad\x00\x07\x08\r\x1b]0;title\x07 thing\n")
	}
	_, _ = s.Write([]byte(in.String()))
	_, _ = s.Write([]byte("\xff\x01"))
	got := s.tailText()
	if want := strings.TrimSpace(strings.Repeat("error: bad thing\n", 30)); got != want {
		t.Fatalf("tail = %q, want plain text %q", got, want)
	}
	b, err := json.Marshal(got)
	if err != nil || len(b) > len(got)+len(`""`)+64 {
		t.Fatalf("json size %d for %d bytes (%v), want about the byte length", len(b), len(got), err)
	}
}
