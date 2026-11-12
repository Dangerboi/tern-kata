package progress

import (
	"bytes"
	"os"
	"testing"
)

func TestReportBytes(t *testing.T) {
	var out bytes.Buffer
	r := &Reporter{out: &out}
	for _, step := range []struct {
		do   func()
		want string
	}{
		{func() { r.Working(Unknown) }, "\x1b]7501;state=working:app=tern-kata\x1b\\"},
		{func() { r.Working(0) }, "\x1b]7501;state=working:app=tern-kata:progress=0\x1b\\"},
		{func() { r.Working(40) }, "\x1b]7501;state=working:app=tern-kata:progress=40\x1b\\"},
		{func() { r.Working(250) }, "\x1b]7501;state=working:app=tern-kata:progress=100\x1b\\"},
		{func() { r.Working(-7) }, "\x1b]7501;state=working:app=tern-kata\x1b\\"},
		{func() { r.Clear() }, "\x1b]7501;state=clear\x1b\\"},
	} {
		out.Reset()
		step.do()
		if out.String() != step.want {
			t.Errorf("wrote %q, want %q", out.String(), step.want)
		}
	}
}

func TestWritesOnlyChanges(t *testing.T) {
	var out bytes.Buffer
	r := &Reporter{out: &out}
	r.Clear() // nothing is showing
	r.Working(10)
	r.Working(10)
	r.Working(11)
	r.Clear()
	r.Clear()
	r.Working(11) // showing again after a clear
	want := "\x1b]7501;state=working:app=tern-kata:progress=10\x1b\\" +
		"\x1b]7501;state=working:app=tern-kata:progress=11\x1b\\" +
		"\x1b]7501;state=clear\x1b\\" +
		"\x1b]7501;state=working:app=tern-kata:progress=11\x1b\\"
	if out.String() != want {
		t.Errorf("wrote %q, want %q", out.String(), want)
	}
}

func TestNoTerminalNoReports(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if r := New(&bytes.Buffer{}); r != nil {
		t.Error("a buffer is no terminal")
	}
	if r := New(file); r != nil {
		t.Error("a file is no terminal")
	}
	var nilReporter *Reporter
	nilReporter.Working(5)
	nilReporter.Clear()
}

func TestEnabled(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	yes := func(int) bool { return true }
	no := func(int) bool { return false }
	for _, c := range []struct {
		name string
		out  *os.File
		term func(int) bool
		tsp  string
		want bool
	}{
		{"terminal", file, yes, "", true},
		{"TERN_TSP=1", file, yes, "1", true},
		{"TERN_TSP=0", file, yes, "0", false},
		{"not a terminal", file, no, "", false},
	} {
		if got := enabled(c.out, c.term, c.tsp); got != c.want {
			t.Errorf("%s: enabled = %v, want %v", c.name, got, c.want)
		}
	}
	if enabled(&bytes.Buffer{}, yes, "") {
		t.Error("a buffer is enabled")
	}
}
