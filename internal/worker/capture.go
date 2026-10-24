package worker

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// capture is a process's output file, recorded best effort (ADR-0008): a file that cannot be opened or written is
// dropped from that point on, the process carries on and its Run settles as it would have, and the reason is kept to
// be saved with the attempt's exit. Write never fails, so a full disk or a directory in the log's place never reaches
// the process or the run.
type capture struct {
	mu   sync.Mutex
	f    *os.File
	lost string
	// room is how many more bytes a test hook lets through before writes fail; negative when no fault is set.
	room int
}

// errCaptureWrite is the write error a Hooks.CaptureWriteLimit fault produces.
var errCaptureWrite = errors.New("injected write failure")

// openCapture creates (or truncates) the file at path; when it cannot, the capture starts out lost.
func openCapture(hooks Hooks, path string) *capture {
	c := &capture{room: -1}
	if hooks.CaptureWriteLimit != nil {
		if n, ok := hooks.CaptureWriteLimit(path); ok {
			c.room = n
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		c.lose(path, err)
		return c
	}
	c.f = f
	return c
}

// lose drops the file and keeps the first reason.
func (c *capture) lose(path string, err error) {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		err = pe.Err
	}
	if c.lost == "" {
		c.lost = filepath.Base(path) + ": " + err.Error()
	}
	if c.f != nil {
		_ = c.f.Close()
		c.f = nil
	}
}

// Write records p unless the capture is lost; it always reports p as written.
func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f != nil {
		err := c.write(p)
		if err != nil {
			c.lose(c.f.Name(), err)
		}
	}
	return len(p), nil
}

func (c *capture) write(p []byte) error {
	if c.room < 0 {
		_, err := c.f.Write(p)
		return err
	}
	keep := min(len(p), c.room)
	c.room -= keep
	if _, err := c.f.Write(p[:keep]); err != nil {
		return err
	}
	if keep < len(p) {
		return errCaptureWrite
	}
	return nil
}

// Loss is why output stopped being recorded, empty while it is recorded in full (apart from a cap's own cut).
func (c *capture) Loss() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lost
}

// Close closes the file; a failing close loses what could not be flushed.
func (c *capture) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f != nil {
		if err := c.f.Close(); err != nil {
			c.lose(c.f.Name(), err)
		}
		c.f = nil
	}
	return nil
}

// lossOf joins the reasons of the captures that were lost, one per distinct reason.
func lossOf(caps ...interface{ Loss() string }) string {
	var reasons []string
	for _, c := range caps {
		if r := c.Loss(); r != "" {
			reasons = append(reasons, r)
		}
	}
	return strings.Join(reasons, "; ")
}

// outputNote is the words that say a run's output was not recorded and why.
func outputNote(reason string) string { return "output not recorded: " + reason }

// outputLoss is why the run's latest attempt's output was not recorded; empty when it was, or no attempt has ended.
func (j *Journal) outputLoss() string {
	if j.Exit != nil {
		return j.Exit.Unrecorded
	}
	if j.activeStep() >= 0 { // an attempt in flight has no exit yet
		return ""
	}
	for i := len(j.Steps) - 1; i >= 0; i-- {
		if e := j.Steps[i].Exit; e != nil {
			return e.Unrecorded
		}
	}
	return ""
}

// logUnrecorded tells the Worker's log once that an attempt's output was not recorded.
func (w *Worker) logUnrecorded(j *Journal, exit *ExitRecord) {
	if exit.Unrecorded != "" {
		w.opt.Log("run %s: %s", j.RunUID, outputNote(exit.Unrecorded))
	}
}
