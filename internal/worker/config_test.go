package worker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
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
		{"a Project listing an undefined secret", base + "[[projects]]\nname = \"demo\"\nsecrets = [\"nope\"]\n", "not in [secrets]"},
		{"a literal secret", base + "[secrets]\nkey = { value = \"hunter2\" }\n", "unknown key"},
		{"a secret with two sources", base + "[secrets]\nkey = { env = \"A\", file = \"/b\" }\n", "exactly one"},
		{"max_timeout under default_timeout", base + "[limits]\ndefault_timeout = \"2h\"\nmax_timeout = \"1h\"\n", "max_timeout"},
		{"a Project listed twice", base + "[[projects]]\nname = \"demo\"\n[[projects]]\nname = \"demo\"\n", "listed twice"},
		{"a secret aliasing kata's token", base + "[secrets]\nkey = { env = \"KATA_AUTH_TOKEN\" }\n", "kata's credential"},
		{"a secret aliasing kata's token file variable", base + "[secrets]\nkey = { env = \"KATA_AUTH_TOKEN_FILE\" }\n", "kata's credential"},
		{"a secret file inside kata's home", base + "[secrets]\nkey = { file = '" + filepath.Join(kataHome, "config.toml") + "' }\n", "kata's home"},
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

// A secret file under the home folder is written with ~ and the platform's separator: ~\secrets on Windows, ~/secrets elsewhere.
func TestASecretFileUnderHomeTakesThePlatformsSeparator(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got, err := expandHome("~" + string(filepath.Separator) + filepath.Join("secrets", "gh"))
	if want := filepath.Join(home, "secrets", "gh"); err != nil || got != want {
		t.Fatalf("expanded to %q, %v; want %q", got, err, want)
	}
}

func TestAMissingWorkerConfigSaysWhereItBelongs(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "worker.toml"))
	if err == nil || !strings.Contains(err.Error(), "no worker config at") {
		t.Fatalf("error %v", err)
	}
}

func TestAProjectWithoutCapacityGetsThreeSlotsAndAnExplicitOneIsKept(t *testing.T) {
	const base = "executor = \"laptop\"\n[profiles.default]\n"
	for _, c := range []struct {
		name, project string
		want          int
	}{
		{"no capacity line", "[[projects]]\nname = \"demo\"\n", 3},
		{"capacity one", "[[projects]]\nname = \"demo\"\ncapacity = 1\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := load(t, base+c.project)
			if err != nil {
				t.Fatal(err)
			}
			p, ok := cfg.project("demo")
			if !ok || p.Capacity != c.want {
				t.Fatalf("project %+v, ok %v; want capacity %d", p, ok, c.want)
			}
		})
	}
}

func TestPauseAfterFailuresIsThreeUnlessSetAndZeroTurnsItOff(t *testing.T) {
	const base = "executor = \"laptop\"\n[profiles.default]\n"
	for _, tt := range []struct {
		name, body string
		want       int
	}{
		{"unset", base, DefaultPauseAfterFailures},
		{"five", "executor = \"laptop\"\n[limits]\npause_after_failures = 5\n[profiles.default]\n", 5},
		{"zero", "executor = \"laptop\"\n[limits]\npause_after_failures = 0\n[profiles.default]\n", 0},
	} {
		cfg, err := load(t, tt.body)
		if err != nil || cfg.PauseAfterFailures() != tt.want {
			t.Errorf("%s: pause_after_failures is %d, %v; want %d", tt.name, cfg.PauseAfterFailures(), err, tt.want)
		}
	}
	if _, err := load(t, "executor = \"laptop\"\n[limits]\npause_after_failures = -1\n[profiles.default]\n"); err == nil || !strings.Contains(err.Error(), "pause_after_failures") {
		t.Errorf("a negative pause_after_failures loaded: %v", err)
	}
}

func TestALoneMaxTimeoutUnderTheDefaultLowersTheDefaultToIt(t *testing.T) {
	cfg, err := load(t, "executor = \"laptop\"\n[profiles.default]\n[limits]\nmax_timeout = \"1h\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.defaultTimeout() != time.Hour || cfg.maxTimeout() != time.Hour {
		t.Fatalf("default %s, max %s; want both 1h", cfg.defaultTimeout(), cfg.maxTimeout())
	}
	_, err = load(t, "executor = \"laptop\"\n[profiles.default]\n[limits]\nmax_timeout = \"1h\"\ndefault_timeout = \"3h\"\n")
	if err == nil || !strings.Contains(err.Error(), "max_timeout must be at least default_timeout") {
		t.Fatalf("an explicit default over the max: %v", err)
	}
}

func TestAnExplicitZeroLimitOrCapacityIsRefused(t *testing.T) {
	const base = "executor = \"laptop\"\n[profiles.default]\n"
	for name, c := range map[string]struct{ body, want string }{
		"max_active_runs": {base + "[limits]\nmax_active_runs = 0\n", "max_active_runs must be positive"},
		"max_run_prompts": {base + "[limits]\nmax_run_prompts = 0\n", "max_run_prompts must be 1 to 100"},
		"capacity":        {base + "[[projects]]\nname = \"a\"\ncapacity = 2\n[[projects]]\nname = \"b\"\ncapacity = 0\n", "capacity must be positive"},
	} {
		if _, err := load(t, c.body); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one saying %q", name, err, c.want)
		}
	}
	if _, err := load(t, base+"[limits]\nmax_active_runs = 2\nmax_run_prompts = 5\n[[projects]]\nname = \"a\"\n[[projects]]\nname = \"b\"\ncapacity = 1\n"); err != nil {
		t.Fatalf("unset and positive values: %v", err)
	}
}

func TestASecretFileInHomeIsAcceptedWithThePlatformsSeparator(t *testing.T) {
	file := "~" + string(filepath.Separator) + filepath.Join("secrets", "gh")
	if _, err := load(t, "executor = \"laptop\"\n[profiles.default]\n[secrets]\ntoken = { file = '"+file+"' }\n"); err != nil {
		t.Fatalf("file %q: %v", file, err)
	}
}

func TestPickerChoicesFollowTheProjectsAllowlistsAndSkipKeysTheCardCannotSave(t *testing.T) {
	cfg, err := load(t, `executor = "laptop"
[profiles.default]
[profiles.fast]
[checkouts]
repo = '`+filepath.Join(t.TempDir(), "r")+`'
site = '`+filepath.Join(t.TempDir(), "s")+`'
"my app" = '`+filepath.Join(t.TempDir(), "m")+`'
[[projects]]
name = "narrow"
profiles = ["fast"]
checkouts = ["site"]
[[projects]]
name = "open"
`)
	if err != nil {
		t.Fatal(err)
	}
	for project, want := range map[string][2][]string{
		"narrow": {{"fast"}, {"site"}},
		"open":   {{"default", "fast"}, {"repo", "site"}},
		"":       {{"default", "fast"}, {"repo", "site"}},
	} {
		profiles, checkouts := cfg.PickerChoices(project)
		if !slices.Equal(profiles, want[0]) || !slices.Equal(checkouts, want[1]) {
			t.Errorf("Project %q: %v and %v, want %v and %v", project, profiles, checkouts, want[0], want[1])
		}
	}
}

func TestDoctorReportsACheckoutKeyTheCardCannotSave(t *testing.T) {
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	dir := t.TempDir()
	body := "executor = \"laptop\"\n[profiles.default]\ncommand = \"go\"\n[checkouts]\n\"my app\" = '" + dir + "'\n"
	if err := os.WriteFile(dirs.ConfigPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Doctor(context.Background(), DoctorOptions{Dirs: dirs}).String()
	if !strings.Contains(out, "checkout my app") || !strings.Contains(out, "not a portable name") {
		t.Fatalf("doctor said:\n%s", out)
	}
}
