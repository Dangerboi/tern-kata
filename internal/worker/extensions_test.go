package worker

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAProfilesExtensionsBecomeOmpArguments(t *testing.T) {
	a, b := filepath.Join(t.TempDir(), "a.ts"), filepath.Join(t.TempDir(), "b.ts")
	cases := []struct {
		name, key string
		want      []string
	}{
		{"a missing key is ambient", "", nil},
		{"ambient passes no flag", "extensions = \"ambient\"\n", nil},
		{"none disables discovery", "extensions = \"none\"\n", []string{"--no-extensions"}},
		{"a list trusts each path", "extensions = ['" + a + "', '" + b + "']\n", []string{"--trusted-extension", a, "--trusted-extension", b}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := load(t, "executor = \"laptop\"\n[profiles.default]\n"+c.key)
			if err != nil {
				t.Fatal(err)
			}
			args := agentArgs(cfg.Profiles["default"], t.TempDir(), "/p", "")
			for _, flag := range []string{"--no-extensions", "--trusted-extension"} {
				if got, want := slices.Contains(args, flag), slices.Contains(c.want, flag); got != want {
					t.Fatalf("args %v: %s present = %v, want %v", args, flag, got, want)
				}
			}
			if c.want != nil {
				i := slices.Index(args, c.want[0])
				if end := i + len(c.want); i < 0 || end > len(args) || !slices.Equal(args[i:end], c.want) {
					t.Fatalf("args %v lack %v in a row", args, c.want)
				}
			}
		})
	}
}

func TestABadExtensionsKeyIsRefusedWithTheProfilesName(t *testing.T) {
	cases := map[string]string{
		"a relative path":       "extensions = [\"rel/kata.ts\"]\n",
		"an empty list":         "extensions = []\n",
		"an unknown word":       "extensions = \"all\"\n",
		"a number":              "extensions = 3\n",
		"a list of non-strings": "extensions = [1]\n",
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(t, "executor = \"laptop\"\n[profiles.default]\n[profiles.worker]\n"+key)
			if err == nil || !strings.Contains(err.Error(), "profiles.worker: extensions") {
				t.Fatalf("error %v, want one naming profiles.worker", err)
			}
		})
	}
}

func TestDoctorWarnsOfKataPluginOnlyForAnAmbientProfile(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(home, ".omp", "agent")
	writeFile := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	doctor := func(profiles string, omp OmpEnv) string {
		t.Helper()
		dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
		writeFile(dirs.ConfigPath(), "executor = \"laptop\"\n[profiles.default]\ncommand = \"go\"\n"+profiles)
		return Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: omp}).String()
	}
	omp := OmpEnv{Home: home, AgentDir: agent}

	if out := doctor("", omp); strings.Contains(out, "todo takeover") {
		t.Fatalf("no plugin installed, yet a warning:\n%s", out)
	}
	if out := doctor("", OmpEnv{}); strings.Contains(out, "todo takeover") {
		t.Fatalf("the zero OmpEnv looked somewhere:\n%s", out)
	}

	installs := map[string]func(){
		"the user's registry": func() {
			writeFile(filepath.Join(home, ".omp", "plugins", "installed_plugins.json"),
				`{"version":2,"plugins":{"other@x":[{}],"kata@kata-plugin":[{"scope":"user"}]}}`)
		},
		"the plugins' package.json": func() {
			writeFile(filepath.Join(home, ".omp", "plugins", "package.json"), `{"dependencies":{"kata-omp":"^1.0.0"}}`)
		},
		"the ambient extensions folder": func() { writeFile(filepath.Join(agent, "extensions", "kata.ts"), "export default {}") },
		"a moved agent folder": func() {
			omp.AgentDir = filepath.Join(home, "elsewhere")
			writeFile(filepath.Join(omp.AgentDir, "extensions", "kata.ts"), "export default {}")
		},
		"the settings": func() {
			writeFile(filepath.Join(agent, "config.yml"), "extensions:\n  - /src/kata-plugin/plugins/oh-my-pi/kata.ts\n")
		},
	}
	for name, install := range installs {
		t.Run(name, func(t *testing.T) {
			home = t.TempDir()
			agent = filepath.Join(home, ".omp", "agent")
			omp = OmpEnv{Home: home, AgentDir: agent}
			install()
			out := doctor("", omp)
			for _, want := range []string{"warn", "profile default", "todo takeover", "3 extra turns", `extensions = "none"`} {
				if !strings.Contains(out, want) {
					t.Fatalf("no %q in:\n%s", want, out)
				}
			}
			if out := doctor("extensions = \"none\"\n", omp); strings.Contains(out, "todo takeover") {
				t.Fatalf("a profile with none was warned:\n%s", out)
			}
			if out := doctor("extensions = ['"+filepath.Join(home, "x.ts")+"']\n", omp); strings.Contains(out, "todo takeover") {
				t.Fatalf("a profile with a path list was warned:\n%s", out)
			}
		})
	}
}

func TestDoctorFindsKataPluginInACheckoutsRegistry(t *testing.T) {
	checkout := t.TempDir()
	reg := filepath.Join(checkout, ".omp", "plugins", "installed_plugins.json")
	if err := os.MkdirAll(filepath.Dir(reg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reg, []byte(`{"version":2,"plugins":{"kata@kata-plugin":[{}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	body := "executor = \"laptop\"\n[checkouts]\nrepo = '" + checkout + "'\n[profiles.default]\ncommand = \"go\"\n"
	if err := os.WriteFile(dirs.ConfigPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: OmpEnv{Home: t.TempDir()}}).String()
	if !strings.Contains(out, "todo takeover") {
		t.Fatalf("no warning for a checkout's registry:\n%s", out)
	}
}
