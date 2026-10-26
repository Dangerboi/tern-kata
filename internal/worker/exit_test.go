package worker

import (
	"encoding/json"
	"fmt"
	"os"
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

// The window is cut at a line start or mid-line; only a mid-line start loses its first line, and a window with no
// complete line left is empty rather than a fragment.
func TestDropPartialLineKeepsAWindowThatStartsAtALineStart(t *testing.T) {
	for _, tc := range []struct {
		name        string
		in          string
		atLineStart bool
		want        string
	}{
		{"at a line start", "one\ntwo\n", true, "one\ntwo\n"},
		{"mid-line", "rt\ntwo\n", false, "two\n"},
		{"newline is the last byte", "fragment\n", false, ""},
		{"no newline", "fragment", false, ""},
		{"at a line start without newline", "one", true, "one"},
	} {
		if got := string(dropPartialLine([]byte(tc.in), tc.atLineStart)); got != tc.want {
			t.Errorf("%s: dropPartialLine(%q, %v) = %q, want %q", tc.name, tc.in, tc.atLineStart, got, tc.want)
		}
	}
}

func TestTheStderrTailKeepsAFirstLineThatStartsExactlyAtTheWindow(t *testing.T) {
	s := newTailSink(Hooks{}, filepath.Join(t.TempDir(), "stderr.log"))
	defer s.close()
	first := "FIRST" + strings.Repeat("x", 2042) + "\n" // 2048 bytes with the newline
	_, _ = s.Write([]byte("older line\n" + first))
	if got := s.tailText(); !strings.HasPrefix(got, "FIRST") {
		t.Fatalf("tail starts %q, want the line that begins at the window kept", got[:min(len(got), 10)])
	}
}

// A retried step's stored tail belongs to the earlier attempt: while the step runs it must not stand in for the
// new attempt's stderr.
func TestARunningStepDoesNotShowTheEarlierAttemptsStderrTail(t *testing.T) {
	j := &Journal{
		Workflow: &WorkflowPlan{Steps: []PlanStep{{Kind: "command"}}},
		Steps:    []StepRecord{{State: StepRunning, Tries: 2, Dir: "steps/1-2", StderrTail: "attempt one failed"}},
	}
	if got := shownStderrTail(j); got != "" {
		t.Fatalf("running step shows %q, want nothing from the earlier attempt", got)
	}
	j.Steps[0].State = StepFailed
	if got := shownStderrTail(j); got != "attempt one failed" {
		t.Fatalf("concluded step shows %q, want its stored tail", got)
	}
}

// stderr.log keeps only the first bytes, so once it is full it holds no end: the fallback shows nothing, not the middle.
func TestTheStderrFallbackIsEmptyOnceTheFileReachedItsCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("middle of a long stream\n", stderrFileCap/24+1)[:stderrFileCap]), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := stderrFallback(path); got != "" {
		t.Fatalf("fallback = %q, want empty", got)
	}
}

// A window that starts inside a line leaves no fragment of it.
func TestTheStderrFallbackDropsThePartialFirstLine(t *testing.T) {
	var in strings.Builder
	for i := range 300 {
		fmt.Fprintf(&in, "%09d\n", i)
	}
	path := filepath.Join(t.TempDir(), "stderr.log")
	if err := os.WriteFile(path, []byte(in.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got := stderrFallback(path)
	for _, l := range strings.Split(got, "\n") {
		if len(l) != 9 {
			t.Fatalf("line %q is a fragment in %q", l, got)
		}
	}
	if !strings.HasSuffix(got, "000000299") {
		t.Fatalf("fallback %q does not end at the file's end", got)
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

func TestTheStderrTailKeepsTextAroundBrokenEscapes(t *testing.T) {
	cut := "\x1b" + "[31m" + strings.Repeat("x", 43) + "\n" + strings.Repeat("ok line\n", 250)
	for _, tc := range []struct{ name, in, want string }{
		{"unterminated OSC", "\x1b]0;title no end\nerror: disk full\n", "error: disk full"},
		{"charset escape", "\x1b(Bok", "ok"},
		{"save cursor", "\x1b7a\x1b8b\x1bc", "ab"},
		{"window starts inside CSI", cut, strings.TrimSpace(strings.Repeat("ok line\n", 250))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTailSink(Hooks{}, filepath.Join(t.TempDir(), "stderr.log"))
			defer s.close()
			_, _ = s.Write([]byte(tc.in))
			if got := s.tailText(); got != tc.want {
				t.Fatalf("tail = %q, want %q", got, tc.want)
			}
		})
	}
}
