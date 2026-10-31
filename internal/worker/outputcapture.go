package worker

// This file is where a run's output is capped, cut and tailed. Every consumer takes its rules from here: the live
// line sink and the capped file (what an agent's stdout and stderr leave on disk), the head/tail of a command step,
// the agent output parser's feed and the run detail's tail. The rules are three:
//
//   - a cap keeps the first bytes of a stream and drops the rest (cappedFile), except a line the reader needs;
//   - a tail keeps the last bytes of a stream in bounded memory (tailWindow);
//   - a window that begins inside a line has that partial first line dropped (dropPartialLine), because it may start
//     inside an escape sequence or a JSON value.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// maxLine is the longest line the live parser holds; a longer one is skipped.
//
// shortcut: a line past this is skipped by the live parser, so an agent_end longer than 64 MiB leaves a finished run
// recorded failed ("no agent_end").
const maxLine = 64 << 20

// lineSink feeds omp's stdout, line by line, to the parser and keeps the first bytes in a capped file.
type lineSink struct {
	mu       sync.Mutex
	parser   *agentParser
	file     *cappedFile
	buf      []byte
	skipping bool
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(p)
	_, _ = s.file.Write(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if !s.skipping {
				s.buf = append(s.buf, p...)
				if len(s.buf) > maxLine {
					s.buf, s.skipping = nil, true
				}
			}
			break
		}
		if !s.skipping {
			s.buf = append(s.buf, p[:i]...)
			s.line(s.buf)
		}
		s.buf, s.skipping = s.buf[:0], false
		p = p[i+1:]
	}
	return n, nil
}

// line hands one complete line to the parser. omp's last line is agent_end; the cap must not lose it, or a long run
// reads as unfinished, so it is kept past the cap.
func (s *lineSink) line(l []byte) {
	s.parser.Line(l)
	if t := bytes.TrimSpace(l); bytes.HasPrefix(t, []byte(`{"type":"agent_end"`)) {
		s.file.keep(t)
	}
}

// flush parses a last line that had no newline.
func (s *lineSink) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buf) > 0 && !s.skipping {
		s.line(s.buf)
	}
	s.buf = nil
}

func (s *lineSink) close() { _ = s.file.Close() }

// cappedFile records the first max bytes of a capture and drops the rest, so a chatty agent cannot fill the disk.
type cappedFile struct {
	*capture
	left int
	cut  bool // bytes were dropped
	last byte // the last byte written; a newline while the file is empty
}

func openCapped(hooks Hooks, path string, max int) *cappedFile {
	return &cappedFile{capture: openCapture(hooks, path), left: max, last: '\n'}
}

func (c *cappedFile) Write(p []byte) (int, error) {
	n := min(len(p), c.left)
	if n > 0 {
		_, _ = c.capture.Write(p[:n])
		c.left -= n
		c.last = p[n-1]
	}
	c.cut = c.cut || n < len(p)
	return len(p), nil
}

// keep writes line past the cap when the cap has dropped bytes; a cut that fell mid-line leaves that partial line
// to stand alone, which readers skip as garbage. It is the only write allowed past the cap.
func (c *cappedFile) keep(line []byte) {
	if !c.cut {
		return
	}
	if c.last != '\n' {
		_, _ = c.capture.Write([]byte{'\n'})
	}
	_, _ = c.capture.Write(line)
	_, _ = c.capture.Write([]byte{'\n'})
	c.last = '\n'
}

// tailWindow keeps the last max bytes of a stream and remembers whether they begin at a line start. It holds at most
// twice max bytes, trimming back to max when it passes that, so a write does not move the kept bytes every time.
type tailWindow struct {
	max   int
	buf   []byte
	total int64
	cut   bool // bytes were trimmed away
	// startsLine is whether the byte before buf is a newline; meaningful only once cut.
	startsLine bool
}

func (w *tailWindow) Write(p []byte) {
	w.total += int64(len(p))
	w.buf = append(w.buf, p...)
	w.trim()
}

// WriteString is Write for a string, without a byte copy of it.
func (w *tailWindow) WriteString(s string) {
	w.total += int64(len(s))
	w.buf = append(w.buf, s...)
	w.trim()
}

func (w *tailWindow) trim() {
	if len(w.buf) > 2*w.max {
		at := len(w.buf) - w.max
		w.startsLine = w.buf[at-1] == '\n'
		w.cut = true
		w.buf = append(w.buf[:0], w.buf[at:]...)
	}
}

// Bytes is the last max bytes written and whether they begin at a line start (a stream that never passed max
// always does).
func (w *tailWindow) Bytes() (kept []byte, atLineStart bool) {
	if len(w.buf) > w.max {
		return lastWindow(w.buf, w.max)
	}
	return w.buf, !w.cut || w.startsLine
}

// Omitted is how many bytes written are not in Bytes.
func (w *tailWindow) Omitted() int64 {
	kept, _ := w.Bytes()
	return w.total - int64(len(kept))
}

// lastWindow is the last max bytes of raw and whether they begin at a line start.
func lastWindow(raw []byte, max int) ([]byte, bool) {
	if len(raw) <= max {
		return raw, true
	}
	return raw[len(raw)-max:], raw[len(raw)-max-1] == '\n'
}

// dropPartialLine drops the first line of a window that cut the stream mid-line, as it is partial and may start
// inside an escape sequence. A window with no complete line left is empty. atLineStart means the window began at a
// line start, so nothing is dropped.
func dropPartialLine(raw []byte, atLineStart bool) []byte {
	if atLineStart {
		return raw
	}
	if i := bytes.IndexByte(raw, '\n'); i >= 0 {
		return raw[i+1:]
	}
	return nil
}

// stderrFileCap is how much of stderr's start stderr.log keeps.
const stderrFileCap = 64 << 10

// stderrTailBytes is how much of stderr's end is kept for failure reasons.
const stderrTailBytes = 2048

// tailSink keeps the first 64 KiB of stderr in a file and its last 2 KiB for failure reasons.
type tailSink struct {
	file *cappedFile
	mu   sync.Mutex
	tail tailWindow
}

func newTailSink(hooks Hooks, path string) *tailSink {
	return &tailSink{file: openCapped(hooks, path, stderrFileCap), tail: tailWindow{max: stderrTailBytes}}
}

func (t *tailSink) Write(p []byte) (int, error) {
	_, _ = t.file.Write(p)
	t.mu.Lock()
	t.tail.Write(p)
	t.mu.Unlock()
	return len(p), nil
}

// tailText is the kept end of stderr as plain text.
func (t *tailSink) tailText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	raw, atLineStart := t.tail.Bytes()
	return plainText(string(dropPartialLine(raw, atLineStart)))
}

func (t *tailSink) close() { _ = t.file.Close() }

// Output limits of a command step: the first 1 MiB and the last 256 KiB are kept, with a marker between (ADR-0008).
const (
	commandHead = 1 << 20
	commandTail = 256 << 10
)

// headTail drains a command's output to a capture: the first head bytes as they come, then only the last tail bytes,
// written with a marker when it is closed.
type headTail struct {
	*capture
	head    int
	written int
	tail    tailWindow
	closed  bool
}

func openHeadTail(hooks Hooks, path string, head, tail int) *headTail {
	return &headTail{capture: openCapture(hooks, path), head: head, tail: tailWindow{max: tail}}
}

func (h *headTail) Write(p []byte) (int, error) {
	n := len(p)
	if room := h.head - h.written; room > 0 {
		k := min(room, len(p))
		_, _ = h.capture.Write(p[:k])
		h.written += k
		p = p[k:]
	}
	if len(p) > 0 {
		h.tail.Write(p)
	}
	return n, nil
}

// Close writes the marker and the kept tail when output went past the head, then closes the file.
func (h *headTail) Close() error {
	if h.closed {
		return nil
	}
	h.closed = true
	if h.tail.total > 0 {
		tail, _ := h.tail.Bytes()
		if omitted := h.tail.Omitted(); omitted > 0 {
			_, _ = fmt.Fprintf(h.capture, "\n[... %d bytes omitted ...]\n", omitted)
		}
		_, _ = h.capture.Write(tail)
	}
	return h.capture.Close()
}

// outputTail joins the assistant messages of an output.jsonl and keeps the last max bytes, holding no more than a
// tail window of them while it scans.
func outputTail(path string, max int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	tail := tailWindow{max: max}
	messages := 0
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 && line[0] == '{' {
			var ev struct {
				Type    string          `json:"type"`
				Message json.RawMessage `json:"message"`
			}
			if json.Unmarshal(line, &ev) == nil && ev.Type == "message_end" {
				if m, ok := assistantMessage(ev.Message); ok {
					if t := strings.TrimSpace(m.text()); t != "" {
						if messages > 0 {
							tail.WriteString("\n\n")
						}
						tail.WriteString(t)
						messages++
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	kept, _ := tail.Bytes()
	if int64(len(kept)) < tail.total {
		return strings.ToValidUTF8(string(kept), "")
	}
	return string(kept)
}

// fileTail is the last max bytes of a text file, empty when it cannot be read.
func fileTail(path string, max int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	raw, _ = lastWindow(raw, max)
	return strings.TrimSpace(strings.ToValidUTF8(string(raw), ""))
}

// stderrFallback is the end of stderr.log as plain text for a run with no journal tail yet. The file keeps only the
// first stderrFileCap bytes, so a file at the cap holds no end and shows nothing rather than text from the middle.
func stderrFallback(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) >= stderrFileCap {
		return ""
	}
	raw, atLineStart := lastWindow(raw, stderrTailBytes)
	return plainText(string(dropPartialLine(raw, atLineStart)))
}

// commandTail1K is the end of a command step's output, kept in the step's result.
func commandTail1K(dir string) string {
	return fileTail(filepath.Join(dir, "output.log"), 1024)
}
