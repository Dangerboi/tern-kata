package worker

import (
	"os"
	"path/filepath"
	"testing"
)

// The Supervision rule has one owner: the host half asks the worker, so these cases are the rule.
func TestSupervisedConfigIsOnlyAConfigWithoutTheMarker(t *testing.T) {
	cases := []struct {
		name         string
		config, mark bool
		wantOn       bool
	}{
		{"no config", false, false, false},
		{"config", true, false, true},
		{"config and marker", true, true, false},
		{"marker only", false, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Dirs{Config: filepath.Join(t.TempDir(), "tern-kata")}
			if err := os.MkdirAll(d.Config, 0o700); err != nil {
				t.Fatal(err)
			}
			if c.config {
				if err := os.WriteFile(d.ConfigPath(), []byte("executor = \"x\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if c.mark {
				if err := d.SetSupervision(false); err != nil {
					t.Fatal(err)
				}
			}
			text, on := d.SupervisedConfig()
			if on != c.wantOn || d.SupervisionOn() != c.wantOn {
				t.Fatalf("on = %v, SupervisionOn = %v, want %v", on, d.SupervisionOn(), c.wantOn)
			}
			if want := map[bool]string{true: "executor = \"x\"\n", false: ""}[c.wantOn && c.config]; text != want {
				t.Fatalf("text %q, want %q", text, want)
			}
		})
	}
}
