package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// numbered is n lines of 10 bytes each ("000000001\n" ...).
func numbered(from, n int) string {
	var b strings.Builder
	for i := from; i < from+n; i++ {
		fmt.Fprintf(&b, "%09d\n", i)
	}
	return b.String()
}

// The tail keeps the last max bytes however the stream is chunked, including a stream that ends exactly at the point
// where the window trims itself (twice max) and one byte past it.
func TestTheTailWindowKeepsTheLastBytesAcrossItsTrimThreshold(t *testing.T) {
	const keep = 20
	for _, total := range []int{0, keep - 1, keep, keep + 1, 2*keep - 1, 2 * keep, 2*keep + 1, 3 * keep, 10*keep + 3} {
		for _, chunk := range []int{1, 7, total + 1} {
			t.Run(fmt.Sprintf("total %d chunk %d", total, chunk), func(t *testing.T) {
				in := strings.Repeat("abcdefghi\n", total/10+1)[:total]
				w := tailWindow{max: keep}
				for rest := in; rest != ""; {
					n := min(chunk, len(rest))
					w.Write([]byte(rest[:n]))
					rest = rest[n:]
				}
				kept, atLine := w.Bytes()
				want := in[max(0, len(in)-keep):]
				if string(kept) != want {
					t.Fatalf("kept %q, want %q", kept, want)
				}
				wantLine := len(in) <= keep || in[len(in)-keep-1] == '\n'
				if atLine != wantLine {
					t.Fatalf("atLineStart = %v, want %v", atLine, wantLine)
				}
				if got, want := w.Omitted(), int64(len(in)-len(want)); got != want {
					t.Fatalf("omitted %d, want %d", got, want)
				}
				if len(w.buf) > 2*keep {
					t.Fatalf("holds %d bytes, want at most %d", len(w.buf), 2*keep)
				}
			})
		}
	}
}

// A partial first line is dropped and a line that begins exactly at the window is kept, through the sink and through
// the file readers alike.
func TestAWindowThatBeginsMidLineLosesThatLineOnly(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"mid-line", "xx" + numbered(0, 3), numbered(1, 2)},
		{"on a line start", numbered(0, 3), numbered(0, 3)},
		{"no complete line", strings.Repeat("z", 40), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, at := lastWindow([]byte(tc.in), 30)
			if got := string(dropPartialLine(raw, at)); got != tc.want {
				t.Fatalf("window %q, want %q", got, tc.want)
			}
			w := tailWindow{max: 30}
			w.Write([]byte(tc.in))
			raw, at = w.Bytes()
			if got := string(dropPartialLine(raw, at)); got != tc.want {
				t.Fatalf("tail window %q, want %q", got, tc.want)
			}
		})
	}
}

// The cap keeps the first bytes, writes nothing past it and still keeps a line it is told to.
func TestACappedFileKeepsItsStartAndAnExplicitLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"exactly the cap", "aaaa\nbbbb\n", 10, "aaaa\nbbbb\n"},
		{"one byte past the cap", "aaaa\nbbbb\nc", 10, "aaaa\nbbbb\nKEEP\n"},
		{"cut mid-line", "aaaa\nbbbb\ncccc\n", 7, "aaaa\nbb\nKEEP\n"},
		{"under the cap", "aaaa\n", 10, "aaaa\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "out")
			f := openCapped(Hooks{}, path, tc.max)
			_, _ = f.Write([]byte(tc.in))
			f.keep([]byte("KEEP"))
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != tc.want {
				t.Fatalf("file %q (%v), want %q", got, err, tc.want)
			}
		})
	}
}

// A command's output keeps its head, then a marker with the omitted count, then its tail; the cut at the trim
// threshold (head + 2*tail) and one byte past it lose nothing but the middle.
func TestACommandsOutputKeepsItsHeadAndItsTail(t *testing.T) {
	const head, tail = 20, 10
	for _, size := range []int{5, head, head + 1, head + tail, head + tail + 1, head + 2*tail, head + 2*tail + 1, head + 5*tail} {
		t.Run(fmt.Sprintf("%d bytes", size), func(t *testing.T) {
			in := strings.Repeat("0123456789", size/10+1)[:size]
			path := filepath.Join(t.TempDir(), "output.log")
			h := openHeadTail(Hooks{}, path, head, tail)
			for rest := in; rest != ""; { // pipe-sized chunks that cut across the head and the threshold
				n := min(3, len(rest))
				_, _ = h.Write([]byte(rest[:n]))
				rest = rest[n:]
			}
			if err := h.Close(); err != nil {
				t.Fatal(err)
			}
			if err := h.Close(); err != nil {
				t.Fatalf("second close: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := in
			if size > head+tail {
				omitted := size - head - tail
				want = in[:head] + fmt.Sprintf("\n[... %d bytes omitted ...]\n", omitted) + in[size-tail:]
			}
			if string(got) != want {
				t.Fatalf("file %q, want %q", got, want)
			}
		})
	}
}

// The stderr sink keeps the first bytes in a file and the last bytes as plain text from a whole line.
func TestTheStderrSinkKeepsAStartInTheFileAndAWholeLineTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.log")
	s := newTailSink(Hooks{}, path)
	in := numbered(0, stderrFileCap/10+300)
	_, _ = s.Write([]byte(in))
	s.close()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != in[:stderrFileCap] {
		t.Fatalf("file is %d bytes (%v), want the first %d", len(got), err, stderrFileCap)
	}
	text := s.tailText()
	for _, l := range strings.Split(text, "\n") {
		if len(l) != 9 {
			t.Fatalf("tail has fragment %q", l)
		}
	}
	if !strings.HasSuffix(text, strings.TrimSpace(in[len(in)-10:])) {
		t.Fatalf("tail %q does not end at the stream's end", text[max(0, len(text)-20):])
	}
}

// The saved file's tail helpers read the end of a file; a missing file is empty.
func TestTheFileTailIsTheLastBytesOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.log")
	if err := os.WriteFile(path, []byte(numbered(0, 5)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := fileTail(path, 20), "000000003\n000000004"; got != want {
		t.Fatalf("fileTail = %q, want %q", got, want)
	}
	if got := fileTail(path+".missing", 20); got != "" {
		t.Fatalf("missing file tail = %q, want empty", got)
	}
}

// writeOutputJournal writes an output.jsonl of n assistant messages of size bytes each, numbered so the tail is
// recognisable, and returns the messages' text joined as the run detail shows it.
func writeOutputJournal(t testing.TB, path string, n, size int) string {
	t.Helper()
	var file, joined strings.Builder
	for i := range n {
		text := fmt.Sprintf("m%07d ", i) + strings.Repeat("x", size-9)
		fmt.Fprintf(&file, `{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":%q}],"stopReason":"stop"}}`+"\n", text)
		if i > 0 {
			joined.WriteString("\n\n")
		}
		joined.WriteString(text)
	}
	if err := os.WriteFile(path, []byte(file.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return joined.String()
}

// The output tail is the last bytes of the joined assistant messages, for a short output and for one far past the tail.
func TestOutputTailIsTheEndOfTheJoinedMessages(t *testing.T) {
	for _, c := range []struct {
		name    string
		n, size int
	}{
		{"shorter than the tail", 3, 100},
		{"a little past the tail", 6, 1000},
		{"far past the tail", 500, 1000},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.jsonl")
			joined := writeOutputJournal(t, path, c.n, c.size)
			want := joined
			if len(want) > OutputTailBytes {
				want = want[len(want)-OutputTailBytes:]
			}
			if got := outputTail(path, OutputTailBytes); got != want {
				t.Fatalf("tail differs: got %d bytes, want %d", len(got), len(want))
			}
		})
	}
}

// Reading the tail of an 8 MiB output allocates for the tail and one line at a time, not for the whole output.
func BenchmarkOutputTail8MiB(b *testing.B) {
	path := filepath.Join(b.TempDir(), "output.jsonl")
	writeOutputJournal(b, path, 8192, 1024)
	b.ReportAllocs()
	for b.Loop() {
		outputTail(path, OutputTailBytes)
	}
}

// Scanning an 8 MiB output keeps no more than the tail window: the bytes held never pass twice the tail size however
// much goes through.
func TestOutputTailMemoryIsBoundedByTheTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.jsonl")
	writeOutputJournal(t, path, 8192, 1024)
	tail := scanOutputTail(path, OutputTailBytes)
	if tail.total < 8<<20 {
		t.Fatalf("only %d bytes went through the window, want the whole 8 MiB output", tail.total)
	}
	if len(tail.buf) > 2*OutputTailBytes || cap(tail.buf) > 4*OutputTailBytes {
		t.Fatalf("the scan holds %d bytes (capacity %d) of an 8 MiB output, want at most twice the %d byte tail", len(tail.buf), cap(tail.buf), OutputTailBytes)
	}
}
