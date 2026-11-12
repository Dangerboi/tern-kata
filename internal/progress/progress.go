// Package progress reports a long operation to the terminal with OSC 7501, the Program Status Protocol, which Tern draws on
// the Pane's top edge and in its Last command widget. Only "working" and "clear" are ever written: "blocked" would page the
// user for a program that only waits, and Tern sends a system notification for "done" and "error".
package progress

import (
	"io"
	"os"
	"strconv"

	"golang.org/x/term"
)

// Unknown is the percent of a report that has no percentage: Tern then draws a bar with no fill level.
const Unknown = -1

const (
	prefix = "\x1b]7501;state="
	suffix = "\x1b\\"
	app    = "app=tern-kata"
	clear  = prefix + "clear" + suffix
)

// Reporter writes a report only when it differs from the last one written. The zero value and a nil Reporter write nothing.
// It is not safe for concurrent use.
type Reporter struct {
	out  io.Writer
	last string
}

// New returns a Reporter for out, which writes nothing unless out is a terminal and TERN_TSP is not 0 (the user's switch for
// leaving Tern's protocols out).
func New(out io.Writer) *Reporter {
	if !enabled(out, term.IsTerminal, os.Getenv("TERN_TSP")) {
		return nil
	}
	return &Reporter{out: out}
}

func enabled(out io.Writer, isTerminal func(fd int) bool, tsp string) bool {
	f, ok := out.(*os.File)
	return ok && isTerminal(int(f.Fd())) && tsp != "0"
}

// Working reports work in progress at percent, clamped to 0-100, or with no percentage for Unknown or any negative value.
func (r *Reporter) Working(percent int) {
	report := prefix + "working:" + app
	if percent >= 0 {
		report += ":progress=" + strconv.Itoa(min(percent, 100))
	}
	r.write(report + suffix)
}

// Clear removes the report. It writes nothing when no report is showing.
func (r *Reporter) Clear() {
	if r != nil && r.last == "" {
		return
	}
	r.write(clear)
}

func (r *Reporter) write(report string) {
	if r == nil || r.out == nil || report == r.last {
		return
	}
	// A failed write leaves last as it was, so the next change tries again; the report is decoration and never fails the work.
	if _, err := io.WriteString(r.out, report); err == nil {
		r.last = report
		if report == clear {
			r.last = ""
		}
	}
}
