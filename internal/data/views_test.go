package data

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func viewsStore(t *testing.T, content string) ViewsStore {
	t.Helper()
	s := NewViewsStore(t.TempDir())
	if content != "" {
		if err := os.WriteFile(s.Path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func mustRead(t *testing.T, s ViewsStore) ViewsRead {
	t.Helper()
	r, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func disk(t *testing.T, s ViewsStore) string {
	t.Helper()
	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const handEdited = `# my views, hand edited
theme = "dark" # an unknown top-level key

[[view]]
# the morning list
name = "Mine"
base = "today"
owner = "@me"
future_key = 42 # unknown key kept

# a table of another kind
[other]
keep = true

[[view]]
name = "Work bugs"
base = "project"
project_uid = "01JABC"
project_name = "Work"
label = "bug"
status = "open"
`

func TestViewsRoundTripKeepsCommentsAndUnknownKeys(t *testing.T) {
	s := viewsStore(t, handEdited)
	r := mustRead(t, s)
	if len(r.Views) != 2 || len(r.Problems) != 0 {
		t.Fatalf("read %+v", r)
	}
	if got := r.Views[1]; got.Base != model.ProjectCollection("01JABC", "Work") || got.Filters.Label != "bug" || got.Filters.Status != "open" {
		t.Fatalf("project view: %+v", got)
	}
	if got := r.Views[0]; got.Base.View != "today" || got.Filters.Owner != "@me" {
		t.Fatalf("fixed view: %+v", got)
	}

	// Rename and edit the first view, add a third, delete the second.
	if err := s.Replace(r.Token, "mine", model.SavedView{Name: "Morning", Base: model.Collection{View: "today"}, Filters: model.Filters{Owner: "@me", Search: `say "hi"`}}); err != nil {
		t.Fatal(err)
	}
	r = mustRead(t, s)
	if err := s.Add(r.Token, model.SavedView{Name: "Later", Base: model.Collection{View: "upcoming"}}); err != nil {
		t.Fatal(err)
	}
	r = mustRead(t, s)
	if err := s.Delete(r.Token, "WORK BUGS"); err != nil {
		t.Fatal(err)
	}
	r = mustRead(t, s)
	if len(r.Views) != 2 || r.Views[0].Name != "Morning" || r.Views[0].Filters.Search != `say "hi"` || r.Views[1].Name != "Later" {
		t.Fatalf("after edits: %+v", r.Views)
	}
	text := disk(t, s)
	for _, want := range []string{"# my views, hand edited", `theme = "dark" # an unknown top-level key`, "# the morning list", "future_key = 42 # unknown key kept", "# a table of another kind", "[other]\nkeep = true"} {
		if !strings.Contains(text, want) {
			t.Errorf("lost %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Work bugs") {
		t.Errorf("deleted view left behind:\n%s", text)
	}
	if strings.Index(text, "Morning") > strings.Index(text, "Later") {
		t.Errorf("rename moved the view:\n%s", text)
	}
}

func TestViewsMissingFileAndFirstAdd(t *testing.T) {
	s := viewsStore(t, "")
	r := mustRead(t, s)
	if len(r.Views) != 0 {
		t.Fatalf("views: %+v", r.Views)
	}
	if err := s.Add(r.Token, model.SavedView{Name: "A", Base: model.Collection{View: "all-open"}, Filters: model.Filters{Status: "open"}}); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, s).Views; len(got) != 1 || got[0].Name != "A" || got[0].Base.Title != "All Open" {
		t.Fatalf("got %+v", got)
	}
	if info, err := os.Stat(s.Path); err != nil || info.Mode().Perm()&0o077 != 0 && filepath.Separator == '/' {
		t.Fatalf("mode: %v %v", info, err)
	}
}

func TestViewsUnreadableFileIsLeftAlone(t *testing.T) {
	const bad = "[[view]\nname = \"x\"\n"
	s := viewsStore(t, bad)
	_, err := s.Read()
	if err == nil || !strings.Contains(err.Error(), s.Path) {
		t.Fatalf("read error %v", err)
	}
	// A token of the bad text, as a raw guardedfile read gives it, still writes nothing.
	raw, err := guardedfile.Read(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	tok := raw
	for name, write := range map[string]error{
		"add":     s.Add(tok, model.SavedView{Name: "A", Base: model.Collection{View: "today"}}),
		"replace": s.Replace(tok, "x", model.SavedView{Name: "A", Base: model.Collection{View: "today"}}),
		"delete":  s.Delete(tok, "x"),
	} {
		if write == nil {
			t.Errorf("%s wrote to an unreadable file", name)
		}
	}
	if got := disk(t, s); got != bad {
		t.Fatalf("file changed: %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(s.Path))
	if len(entries) != 1 {
		t.Fatalf("stray files: %v", entries)
	}
}

func TestViewsConcurrentChangeRefused(t *testing.T) {
	s := viewsStore(t, handEdited)
	r := mustRead(t, s)
	other := "# touched by another pane\n" + handEdited
	if err := os.WriteFile(s.Path, []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"add":     s.Add(r.Token, model.SavedView{Name: "A", Base: model.Collection{View: "today"}}),
		"replace": s.Replace(r.Token, "Mine", model.SavedView{Name: "Mine", Base: model.Collection{View: "today"}}),
		"delete":  s.Delete(r.Token, "Mine"),
	} {
		if !errors.Is(err, ErrViewsChanged) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got := disk(t, s); got != other {
		t.Fatalf("file changed: %q", got)
	}

	// The file created by someone else after a read of a missing file is a change too.
	s2 := viewsStore(t, "")
	r2 := mustRead(t, s2)
	if err := os.WriteFile(s2.Path, []byte("# new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s2.Add(r2.Token, model.SavedView{Name: "A", Base: model.Collection{View: "today"}}); !errors.Is(err, ErrViewsChanged) {
		t.Fatalf("got %v", err)
	}
}

func TestViewsWriteChecks(t *testing.T) {
	s := viewsStore(t, handEdited)
	r := mustRead(t, s)
	today := model.Collection{View: "today"}
	if err := s.Add(r.Token, model.SavedView{Name: "mine", Base: today}); err == nil || !strings.HasPrefix(err.Error(), "Name") {
		t.Errorf("duplicate add: %v", err)
	}
	if err := s.Add(r.Token, model.SavedView{Name: "Today", Base: today}); err == nil || !strings.HasPrefix(err.Error(), "Name") {
		t.Errorf("fixed Collection add: %v", err)
	}
	if err := s.Replace(r.Token, "Mine", model.SavedView{Name: "work BUGS", Base: today}); err == nil || !strings.HasPrefix(err.Error(), "Name") {
		t.Errorf("rename onto another: %v", err)
	}
	if err := s.Replace(r.Token, "Mine", model.SavedView{Name: "MINE", Base: today}); err != nil {
		t.Errorf("recase own name: %v", err)
	}
	r = mustRead(t, s)
	if err := s.Delete(r.Token, "ghost"); !errors.Is(err, ErrViewNotFound) {
		t.Errorf("delete unknown: %v", err)
	}
}

func TestViewsInvalidEntryIsReportedAndKept(t *testing.T) {
	const text = "[[view]]\nname = \"Bad\"\nbase = \"nowhere\"\n\n[[view]]\nname = \"Good\"\nbase = \"today\"\n"
	s := viewsStore(t, text)
	r := mustRead(t, s)
	if len(r.Views) != 1 || r.Views[0].Name != "Good" || len(r.Problems) != 1 || !strings.Contains(r.Problems[0].Error(), `"Bad"`) {
		t.Fatalf("read %+v", r)
	}
	if err := s.Delete(r.Token, "good"); err != nil {
		t.Fatal(err)
	}
	if got := disk(t, s); got != "[[view]]\nname = \"Bad\"\nbase = \"nowhere\"\n" {
		t.Fatalf("got %q", got)
	}
}

func TestViewsNamesAreComparedTrimmedLikeRead(t *testing.T) {
	s := viewsStore(t, "[[view]]\nname = \" Work \"\nbase = \"today\"\n")
	r := mustRead(t, s)
	if len(r.Views) != 1 || r.Views[0].Name != "Work" {
		t.Fatalf("read %+v", r)
	}
	if err := s.Add(r.Token, model.SavedView{Name: "work", Base: model.Collection{View: "today"}}); err == nil || !strings.HasPrefix(err.Error(), "Name") {
		t.Errorf("add of a name the file already has trimmed: %v", err)
	}
	if err := s.Delete(r.Token, "Work"); err != nil {
		t.Errorf("delete by trimmed name: %v", err)
	}
}

func TestViewsWritesKeepCRLFLineEndings(t *testing.T) {
	s := viewsStore(t, "# mine\r\n[[view]]\r\nname = \"A\"\r\nbase = \"today\"\r\n")
	r := mustRead(t, s)
	if err := s.Add(r.Token, model.SavedView{Name: "B", Base: model.Collection{View: "upcoming"}, Filters: model.Filters{Owner: "@me"}}); err != nil {
		t.Fatal(err)
	}
	r = mustRead(t, s)
	if err := s.Replace(r.Token, "A", model.SavedView{Name: "A2", Base: model.Collection{View: "today"}, Filters: model.Filters{Status: "open"}}); err != nil {
		t.Fatal(err)
	}
	text := disk(t, s)
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Errorf("bare LF in a CRLF file: %q", text)
	}
	if got := mustRead(t, s).Views; len(got) != 2 || got[0].Name != "A2" || got[1].Name != "B" {
		t.Errorf("views %+v", got)
	}
}

func TestViewsFileWithABOMReadsAndTakesWrites(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	s := viewsStore(t, bom+"[[view]]\nname = \"A\"\nbase = \"today\"\n")
	r := mustRead(t, s)
	if len(r.Views) != 1 || r.Views[0].Name != "A" || len(r.Problems) != 0 {
		t.Fatalf("read %+v", r)
	}
	if err := s.Add(r.Token, model.SavedView{Name: "B", Base: model.Collection{View: "upcoming"}}); err != nil {
		t.Fatal(err)
	}
	r = mustRead(t, s)
	if err := s.Replace(r.Token, "A", model.SavedView{Name: "A2", Base: model.Collection{View: "today"}}); err != nil {
		t.Fatal(err)
	}
	r = mustRead(t, s)
	if err := s.Delete(r.Token, "B"); err != nil {
		t.Fatal(err)
	}
	if got := disk(t, s); !strings.HasPrefix(got, bom+"[[view]]\nname = \"A2\"") || strings.Count(got, bom) != 1 || strings.Contains(got, `"B"`) {
		t.Errorf("got %q", got)
	}
}

func TestViewsDuplicateNameIsAProblemAndListDeleteEditAgree(t *testing.T) {
	const text = "[[view]]\nname = \"A\"\nbase = \"today\"\nstatus = \"Open\"\n\n[[view]]\nname = \"a\"\nbase = \"today\"\n"
	s := viewsStore(t, text)
	r := mustRead(t, s)
	if len(r.Views) != 0 || len(r.Problems) != 2 || !strings.Contains(r.Problems[1].Error(), "[[view]] 2") {
		t.Fatalf("read %+v", r)
	}
	if err := s.Replace(r.Token, "A", model.SavedView{Name: "A", Base: model.Collection{View: "today"}}); err != nil {
		t.Fatalf("edit of the first entry: %v", err)
	}
	r = mustRead(t, s)
	if len(r.Views) != 1 || r.Views[0].Base.View != "today" || len(r.Problems) != 1 {
		t.Fatalf("read %+v", r)
	}
	if err := s.Delete(r.Token, "A"); err != nil {
		t.Fatal(err)
	}
	r = mustRead(t, s)
	if len(r.Views) != 1 || r.Views[0].Name != "a" || len(r.Problems) != 0 {
		t.Errorf("after delete %+v", r)
	}
}
