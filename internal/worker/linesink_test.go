package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const endLine = `{"type":"agent_end","messages":[],"isTerminal":true}` + "\n"

func TestACappedOutputFileKeepsItsAgentEnd(t *testing.T) {
	const limit = 64
	noise := func(n int) string { return `{"type":"x","p":"` + strings.Repeat("a", n) + `"}` + "\n" }
	cases := []struct {
		name  string
		out   string
		limit int
		whole bool // the file is the stdout byte for byte
	}{
		{"cut mid-line", strings.Repeat(noise(20), 10) + endLine, limit, false},
		{"cut on a line end", strings.Repeat(noise(limit/2-len(`{"type":"x","p":""}`)-1), 6) + endLine, limit, false},
		{"under the cap", noise(20) + endLine, 1 << 10, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.jsonl")
			sink := &lineSink{parser: &agentParser{}, file: openCapped(Hooks{}, path, tc.limit)}
			// Chunks of 7 bytes cut lines in the middle on their way in, as a pipe does.
			for rest := tc.out; rest != ""; {
				n := min(7, len(rest))
				_, _ = sink.Write([]byte(rest[:n]))
				rest = rest[n:]
			}
			sink.flush()
			sink.close()

			if !sink.parser.Result().Terminal {
				t.Fatal("the live parser missed agent_end")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !readOutput(path).Terminal {
				t.Fatalf("saved file lost agent_end: %q", got)
			}
			if tc.whole {
				if string(got) != tc.out {
					t.Fatalf("file %q, want stdout %q", got, tc.out)
				}
				return
			}
			if len(got) > limit+len(endLine)+1 {
				t.Fatalf("file is %d bytes, want at most the cap plus the agent_end line (%d)", len(got), limit+len(endLine)+1)
			}
			if strings.Count(string(got), `"agent_end"`) != 1 {
				t.Fatalf("agent_end not kept exactly once: %q", got)
			}
		})
	}
}
