package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func writeFiltersFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "tern-kata", "filters.toml")
	if content != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestSavedFiltersRoundTripAndKeepOtherTablesAndUnknownKeys(t *testing.T) {
	path := writeFiltersFile(t, "[\"other\"]\nname = \"elsewhere\"\nstatus = \"closed\"\n\n[\"p\"]\nname = \"old\"\nowner = \"me\"\nnote = \"mine\"\n")
	saved := saveFilters(model.SaveFilters{ProjectUID: "p", ProjectName: "work", Default: model.Filters{Status: "open", Label: "bug", Relationship: "child"}})
	if saved.Err != nil {
		t.Fatal(saved.Err)
	}
	raw, _ := os.ReadFile(path)
	for _, want := range []string{"[other]", `status = "closed"`, `note = "mine"`, `name = "work"`, `status = "open"`, `label = "bug"`, `relationship = "child"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("file lacks %q:\n%s", want, raw)
		}
	}
	if strings.Contains(string(raw), "owner") {
		t.Errorf("the emptied owner stayed:\n%s", raw)
	}
	got, err := loadFilters("p")
	if err != nil || got != (model.Filters{Status: "open", Label: "bug", Relationship: "child"}) {
		t.Errorf("loaded %#v, %v", got, err)
	}
	if other, _ := loadFilters("other"); other.Status != "closed" {
		t.Errorf("the other Project's default changed: %#v", other)
	}
}

func TestAnEmptyDefaultRemovesTheTableUnlessItHoldsUnknownKeys(t *testing.T) {
	path := writeFiltersFile(t, "[\"p\"]\nname = \"work\"\nstatus = \"open\"\n\n[\"q\"]\nname = \"w\"\nstatus = \"open\"\nnote = \"keep\"\n")
	for _, uid := range []string{"p", "q"} {
		if saved := saveFilters(model.SaveFilters{ProjectUID: uid, ProjectName: "w"}); saved.Err != nil {
			t.Fatal(saved.Err)
		}
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "[p]") || strings.Contains(string(raw), "status") || !strings.Contains(string(raw), "[q]") || !strings.Contains(string(raw), `note = "keep"`) {
		t.Errorf("file:\n%s", raw)
	}
}

func TestAnUnreadableFiltersFileIsNotOverwrittenAndItsErrorComesBack(t *testing.T) {
	path := writeFiltersFile(t, "this is [not toml")
	saved := saveFilters(model.SaveFilters{ProjectUID: "p", ProjectName: "work", Default: model.Filters{Status: "open"}})
	if saved.Err == nil || !strings.Contains(saved.Err.Error(), "filters.toml") {
		t.Fatalf("error %v, want one naming the file", saved.Err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "this is [not toml" {
		t.Errorf("file overwritten: %q", raw)
	}
	if _, err := loadFilters("p"); err == nil {
		t.Error("loading an unreadable file gave no error")
	}
}

func TestNoFiltersFileMeansNoDefault(t *testing.T) {
	writeFiltersFile(t, "")
	if got, err := loadFilters("p"); err != nil || got != (model.Filters{}) {
		t.Errorf("loaded %#v, %v", got, err)
	}
}
