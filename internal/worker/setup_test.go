package worker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddProjectPutsTheCheckoutInsideAQuotedCheckoutsTable(t *testing.T) {
	for _, header := range []string{`["checkouts"]`, `['checkouts']`} {
		t.Run(header, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "worker.toml")
			first, second := t.TempDir(), t.TempDir()
			existing := "executor = \"laptop\"\n\n[profiles.default]\n\n" + header + " # where\nfirst = " + tomlString(first) + "\n"
			if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := AddProject(path, Binding{Project: "demo", Checkout: second}); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(path)
			if strings.Count(string(got), header) != 1 || !strings.Contains(string(got), header+" # where\n") {
				t.Errorf("the header was not kept once:\n%s", got)
			}
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatalf("the result does not load: %v\n%s", err, got)
			}
			if len(cfg.Checkouts) != 2 {
				t.Errorf("checkouts %v, want both", cfg.Checkouts)
			}
		})
	}
}

func TestReplaceConfigRefusesAFileThatChangedSinceTheBase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.toml")
	base := "executor = \"laptop\"\n\n[profiles.default]\n"
	other := "executor = \"other\"\n\n[profiles.default]\n"
	if err := os.WriteFile(path, []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	mine := "executor = \"mine\"\n\n[profiles.default]\n"
	err := ReplaceConfig(path, base, mine)
	if !errors.Is(err, ErrConfigChanged) {
		t.Fatalf("error %v, want ErrConfigChanged", err)
	}
	if got, _ := os.ReadFile(path); string(got) != other {
		t.Errorf("the file lost the other change:\n%s", got)
	}
	if err := ReplaceConfig(path, other, mine); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != mine {
		t.Errorf("file:\n%s", got)
	}
}
