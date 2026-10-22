package worker

import (
	"os"
	"path/filepath"
	"runtime"
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
	kataHome := t.TempDir()
	t.Setenv("KATA_HOME", kataHome)
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
		{"a secret aliasing kata's token", base + "[secrets]\nkey = { env = \"KATA_AUTH_TOKEN\" }\n", "kata's credential"},
		{"a secret aliasing kata's token file variable", base + "[secrets]\nkey = { env = \"KATA_AUTH_TOKEN_FILE\" }\n", "kata's credential"},
		{"a secret file inside kata's home", base + "[secrets]\nkey = { file = \"" + filepath.Join(kataHome, "config.toml") + "\" }\n", "kata's home"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := load(t, c.body); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want one saying %q", err, c.want)
			}
		})
	}
}

func TestASecretFileOfAnotherUserIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("files carry no owner id here")
	}
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Secrets: map[string]SecretSource{"token": {File: path}}}
	if got, err := cfg.readSecret("token"); err != nil || got != "value" {
		t.Fatalf("own file: %q, %v", got, err)
	}
	own := effectiveUID
	t.Cleanup(func() { effectiveUID = own })
	effectiveUID = func() int { return own() + 1 }
	if _, err := cfg.readSecret("token"); err == nil || !strings.Contains(err.Error(), "owned by") {
		t.Fatalf("a file owned by someone else: %v", err)
	}
}

func TestAMissingWorkerConfigSaysWhereItBelongs(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "worker.toml"))
	if err == nil || !strings.Contains(err.Error(), "no worker config at") {
		t.Fatalf("error %v", err)
	}
}
