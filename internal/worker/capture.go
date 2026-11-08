package worker

import (
	"errors"
	"fmt"
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

var errCaptureWrite = errors.New("injected write failure")

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

func lossOf(caps ...interface{ Loss() string }) string {
	var reasons []string
	for _, c := range caps {
		if r := c.Loss(); r != "" {
			reasons = append(reasons, r)
		}
	}
	return strings.Join(reasons, "; ")
}

func outputNote(reason string) string { return "output not recorded: " + reason }

// outputLoss is why attempts' output was not recorded, empty when it all was: a prompt job's attempt, or every ended
// step of a workflow that lost its capture, each named by its key, whatever later steps recorded. An attempt in flight
// has no exit yet, so a resumed attempt's note is gone until it ends.
func (j *Journal) outputLoss() string {
	if j.Workflow == nil {
		if j.Exit != nil {
			return j.Exit.Unrecorded
		}
		return ""
	}
	var notes []string
	active := j.activeStep()
	for i, r := range j.Steps {
		exit := r.Exit
		if i == active {
			exit = j.Exit
		}
		if exit != nil && exit.Unrecorded != "" {
			notes = append(notes, fmt.Sprintf("step %q: %s", r.Key, exit.Unrecorded))
		}
	}
	return strings.Join(notes, "; ")
}

func (w *Worker) logUnrecorded(j *Journal, exit *ExitRecord) {
	if exit.Unrecorded != "" {
		w.opt.Log("run %s: %s", j.RunUID, outputNote(exit.Unrecorded))
	}
}
