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
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// maxLine is the longest line the live parser holds; a longer one is skipped, except that an agent_end line still
// marks the turn finished (see lineSink.overlong).
const maxLine = 64 << 20

const agentEndPrefix = `{"type":"agent_end"`

// agentEndMarker stands for an agent_end line too long to keep: terminal, no messages. agentEndRetryMarker is the
// same for one whose isTerminal is false, which is not a finished turn (ADR-0009).
const (
	agentEndMarker      = agentEndPrefix + `}`
	agentEndRetryMarker = agentEndPrefix + `,"isTerminal":false}`
)

// endTailBytes is how much of a skipped agent_end's end is kept to read its isTerminal from; omp writes it last.
const endTailBytes = 256

type lineSink struct {
	mu         sync.Mutex
	parser     *agentParser
	file       *cappedFile
	buf        []byte
	skipping   bool
	skippedEnd bool
	endTail    []byte
	limit      int
}

func (s *lineSink) maxLen() int {
	if s.limit > 0 {
		return s.limit
	}
	return maxLine
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(p)
	_, _ = s.file.Write(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		seg := p
		if i >= 0 {
			seg = p[:i]
		}
		switch {
		case s.skipping:
			s.noteEndTail(seg)
		default:
			s.buf = append(s.buf, seg...)
			if len(s.buf) > s.maxLen() {
				s.overlong()
			}
		}
		if i < 0 {
			break
		}
		if s.skipping {
			s.skippedLine()
		} else {
			s.line(s.buf)
		}
		s.buf, s.skipping, s.skippedEnd = s.buf[:0], false, false
		p = p[i+1:]
	}
	return n, nil
}

func (s *lineSink) overlong() {
	s.skippedEnd = bytes.HasPrefix(bytes.TrimSpace(s.buf), []byte(agentEndPrefix))
	s.endTail = s.endTail[:0]
	if s.skippedEnd {
		s.noteEndTail(s.buf)
	}
	s.buf, s.skipping = nil, true
}

func (s *lineSink) noteEndTail(seg []byte) {
	if !s.skippedEnd {
		return
	}
	seg = seg[max(0, len(seg)-endTailBytes):]
	s.endTail = append(s.endTail, seg...)
	if len(s.endTail) > 2*endTailBytes {
		s.endTail = append(s.endTail[:0], s.endTail[len(s.endTail)-endTailBytes:]...)
	}
}

func (s *lineSink) skippedLine() {
	if !s.skippedEnd {
		return
	}
	if endsRetry(s.endTail) {
		s.line([]byte(agentEndRetryMarker))
		return
	}
	s.line([]byte(agentEndMarker))
}

// endsRetry is whether the last isTerminal in tail is false, the case finishesTurn rejects, read from the raw bytes of
// a line the parser never saw.
func endsRetry(tail []byte) bool {
	i := bytes.LastIndex(tail, []byte(`"isTerminal"`))
	if i < 0 {
		return false
	}
	rest := bytes.TrimLeft(tail[i+len(`"isTerminal"`):], " \t")
	if len(rest) == 0 || rest[0] != ':' {
		return false
	}
	return bytes.HasPrefix(bytes.TrimLeft(rest[1:], " \t"), []byte("false"))
}

// line hands one complete line to the parser. omp's last line is agent_end; the cap must not lose it, or a long run
// reads as unfinished, so it is kept past the cap.
func (s *lineSink) line(l []byte) {
	s.parser.Line(l)
	if t := bytes.TrimSpace(l); bytes.HasPrefix(t, []byte(agentEndPrefix)) {
		s.file.keep(t)
	}
}

func (s *lineSink) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.skipping {
		s.skippedLine()
	} else if len(s.buf) > 0 {
		s.line(s.buf)
	}
	s.buf, s.skipping, s.skippedEnd = nil, false, false
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

func (w *tailWindow) Omitted() int64 {
	kept, _ := w.Bytes()
	return w.total - int64(len(kept))
}

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

const stderrFileCap = 64 << 10

const stderrTailBytes = 2048

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

func outputTail(path string, max int) string {
	tail := scanOutputTail(path, max)
	kept, _ := tail.Bytes()
	if int64(len(kept)) < tail.total {
		return strings.ToValidUTF8(string(kept), "")
	}
	return string(kept)
}

func scanOutputTail(path string, max int) *tailWindow {
	tail := &tailWindow{max: max}
	f, err := os.Open(path)
	if err != nil {
		return tail
	}
	defer f.Close()
	messages := 0
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := readTailLine(r)
		if line = bytes.TrimSpace(line); len(line) > 0 && line[0] == '{' {
			var ev struct {
				Type    string          `json:"type"`
				Message json.RawMessage `json:"message"`
			}
			if json.Unmarshal(line, &ev) == nil && ev.Type == "message_end" {
				if m, ok := assistantMessage(ev.Message); ok {
					text, _ := m.text()
					if t := strings.TrimSpace(text); t != "" {
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
	return tail
}

func fileTail(path string, max int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return tailOf(string(raw), max)
}

// tailOf is the last max bytes of text, as trimmed valid UTF-8.
func tailOf(text string, max int) string {
	raw, _ := lastWindow([]byte(text), max)
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

// readTailLine is the next line of an output for the tail scan. An agent_end line, recognised from its first bytes,
// comes back empty and is discarded as it is read: it repeats every message of the run and holds no message_end, so
// holding it would make the scan's memory follow the size of the file. The returned bytes are valid until the next
// call.
func readTailLine(r *bufio.Reader) ([]byte, error) {
	chunk, err := r.ReadSlice('\n')
	if bytes.HasPrefix(bytes.TrimLeft(chunk, " \t"), []byte(agentEndPrefix)) {
		for errors.Is(err, bufio.ErrBufferFull) {
			_, err = r.ReadSlice('\n')
		}
		return nil, err
	}
	if !errors.Is(err, bufio.ErrBufferFull) {
		return chunk, err
	}
	line := append([]byte(nil), chunk...)
	for errors.Is(err, bufio.ErrBufferFull) {
		chunk, err = r.ReadSlice('\n')
		line = append(line, chunk...)
	}
	return line, err
}
