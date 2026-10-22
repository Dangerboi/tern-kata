package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, body string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "worker.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(path)
}

func TestWorkerConfigIsStrict(t *testing.T) {
	const base = "executor = \"laptop\"\n[profiles.default]\n"
	cases := []struct {
		name, body, want string
	}{
		{"an unknown key", base + "colour = \"red\"\n", "unknown key"},
		{"no executor", "[profiles.default]\n", "executor is required"},
		{"an executor with whitespace", "executor = \"my laptop\"\n[profiles.default]\n", "no whitespace"},
		{"no default profile", "executor = \"laptop\"\n", "profiles.default is required"},
		{"a relative checkout", base + "[checkouts]\nrepo = \"code/repo\"\n", "not an absolute directory"},
		{"an allowlist naming a missing checkout", base + "[[projects]]\nname = \"demo\"\ncheckouts = [\"nope\"]\n", "not in [checkouts]"},
		{"a literal secret", base + "[secrets]\nkey = { value = \"hunter2\" }\n", "unknown key"},
		{"a secret with two sources", base + "[secrets]\nkey = { env = \"A\", file = \"/b\" }\n", "exactly one"},
		{"max_timeout under default_timeout", base + "[limits]\ndefault_timeout = \"2h\"\nmax_timeout = \"1h\"\n", "max_timeout"},
		{"a Project listed twice", base + "[[projects]]\nname = \"demo\"\n[[projects]]\nname = \"demo\"\n", "listed twice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := load(t, c.body); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want one saying %q", err, c.want)
			}
		})
	}
	cfg, err := load(t, base+"[[projects]]\nname = \"demo\"\n")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := cfg.project("demo")
	if p.Capacity != 1 || cfg.defaultTimeout() != DefaultTimeout || cfg.maxTimeout() != MaxTimeout || cfg.maxRunPrompts() != 12 {
		t.Fatalf("defaults: capacity %d, timeouts %s %s, prompts %d", p.Capacity, cfg.defaultTimeout(), cfg.maxTimeout(), cfg.maxRunPrompts())
	}
}

func TestAMissingWorkerConfigSaysWhereItBelongs(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "worker.toml"))
	if err == nil || !strings.Contains(err.Error(), "no worker config at") {
		t.Fatalf("error %v", err)
	}
}
