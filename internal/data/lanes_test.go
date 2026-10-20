package data

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func lanesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
	return filepath.Join(dir, "tern-kata")
}

func writeLanesFile(t *testing.T, dir, text string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "lanes.toml")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadLanesReadsAHandEditedFile(t *testing.T) {
	dir := lanesDir(t)
	writeLanesFile(t, dir, "[p]\nname = \"work\"\nlabels = [\"In Review\", \"shipping\", \"in-review\", \"\"]\n")
	got := loadLanes(model.LoadLanes{ProjectUID: "p"})
	if got.Err != nil || !slices.Equal(got.Lanes, []string{"in-review", "shipping"}) {
		t.Errorf("loaded %v, %v", got.Lanes, got.Err)
	}
	if got := loadLanes(model.LoadLanes{ProjectUID: "other"}); got.Err != nil || got.Lanes != nil {
		t.Errorf("a missing Project loaded %v, %v", got.Lanes, got.Err)
	}
}

func TestLoadLanesOfAMissingFileIsEmpty(t *testing.T) {
	lanesDir(t)
	if got := loadLanes(model.LoadLanes{ProjectUID: "p"}); got.Err != nil || got.Lanes != nil {
		t.Errorf("loaded %v, %v", got.Lanes, got.Err)
	}
}

func TestSaveLanesWritesAtomicallyAndKeepsOtherProjects(t *testing.T) {
	dir := lanesDir(t)
	path := writeLanesFile(t, dir, "[q]\nname = \"home\"\nlabels = [\"a\"]\n")
	saved := saveLanes(model.SaveLanes{ProjectUID: "p", ProjectName: "work", Lanes: []string{"x", "y"}})
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	if got := loadLanes(model.LoadLanes{ProjectUID: "p"}); !slices.Equal(got.Lanes, []string{"x", "y"}) {
		t.Errorf("read back %v", got.Lanes)
	}
	if got := loadLanes(model.LoadLanes{ProjectUID: "q"}); !slices.Equal(got.Lanes, []string{"a"}) {
		t.Errorf("another Project's list became %v", got.Lanes)
	}
	text, _ := os.ReadFile(path)
	if !strings.Contains(string(text), `name = "work"`) {
		t.Errorf("the Project's name is not in the file:\n%s", text)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the folder holds %d entries, want only lanes.toml", len(entries))
	}
}

func TestSaveLanesCreatesTheFolder(t *testing.T) {
	lanesDir(t)
	if err := saveLanes(model.SaveLanes{ProjectUID: "p", Lanes: []string{"x"}}).Err; err != nil {
		t.Fatal(err)
	}
}

func TestAnUnparsableFileIsReportedAndNeverOverwritten(t *testing.T) {
	path := writeLanesFile(t, lanesDir(t), "[p\nlabels = nonsense")
	if got := loadLanes(model.LoadLanes{ProjectUID: "p"}); got.Err == nil {
		t.Error("an unparsable file loaded without an error")
	}
	if err := saveLanes(model.SaveLanes{ProjectUID: "p", Lanes: []string{"x"}}).Err; err == nil {
		t.Error("a save over an unparsable file was not refused")
	}
	if text, _ := os.ReadFile(path); string(text) != "[p\nlabels = nonsense" {
		t.Errorf("the file was changed to %q", text)
	}
}
