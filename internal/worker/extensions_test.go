package worker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
		"the runtime registration": func() {
			writeFile(filepath.Join(home, ".omp", "plugins", "node_modules", "kata-omp", "package.json"), `{"name":"kata-omp"}`)
		},
		"the lock file": func() {
			writeFile(filepath.Join(home, ".omp", "plugins", "omp-plugins.lock.json"), `{"plugins":{"kata-omp":{"version":"1","enabled":true}}}`)
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

func TestDoctorFindsKataPluginInACheckoutsPlugins(t *testing.T) {
	checkout := t.TempDir()
	pkg := filepath.Join(checkout, ".omp", "plugins", "node_modules", "kata-omp", "package.json")
	if err := os.MkdirAll(filepath.Dir(pkg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkg, []byte(`{"name":"kata-omp"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	body := "executor = \"laptop\"\n[checkouts]\nrepo = '" + checkout + "'\n[profiles.default]\ncommand = \"go\"\n"
	if err := os.WriteFile(dirs.ConfigPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: OmpEnv{Home: t.TempDir()}}).String()
	if !strings.Contains(out, "todo takeover") {
		t.Fatalf("no warning for a checkout's plugins:\n%s", out)
	}
}

func TestDoctorIgnoresADisabledKataPlugin(t *testing.T) {
	home := t.TempDir()
	plugins := filepath.Join(home, ".omp", "plugins")
	files := map[string]string{
		filepath.Join(plugins, "omp-plugins.lock.json"):                    `{"plugins":{"kata-omp":{"enabled":false}}}`,
		filepath.Join(plugins, "node_modules", "kata-omp", "package.json"): `{"name":"kata-omp"}`,
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	if err := os.WriteFile(dirs.ConfigPath(), []byte("executor = \"laptop\"\n[profiles.default]\ncommand = \"go\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: OmpEnv{Home: home, AgentDir: filepath.Join(home, ".omp", "agent")}}).String()
	if strings.Contains(out, "todo takeover") {
		t.Fatalf("a disabled plugin was warned of:\n%s", out)
	}
}

// doctorFor runs the doctor on a worker.toml with a default profile plus profiles, in the given omp environment.
func doctorFor(t *testing.T, profiles string, omp OmpEnv) string {
	t.Helper()
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	writeTestFile(t, dirs.ConfigPath(), "executor = \"laptop\"\n[profiles.default]\ncommand = \"go\"\n"+profiles)
	return Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: omp}).String()
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func envOf(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
}

func TestDoctorIgnoresTheMarketplaceRegistryAlone(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".omp", "plugins", "installed_plugins.json"), `{"version":2,"plugins":{"kata@kata-plugin":[{"scope":"user"}]}}`)
	if out := doctorFor(t, "", OmpEnv{Home: home}); strings.Contains(out, "todo takeover") {
		t.Fatalf("a registry entry that loads nothing was warned of:\n%s", out)
	}
}

func TestDoctorFollowsOmpsPluginsFolder(t *testing.T) {
	install := func(root string) {
		writeTestFile(t, filepath.Join(root, "plugins", "node_modules", "kata-omp", "package.json"), `{}`)
	}
	cases := []struct {
		name  string
		vars  map[string]string
		where func(home string) string // the folder holding plugins/
		args  string
		want  bool
	}{
		{"OMP_PROFILE", map[string]string{"OMP_PROFILE": "work"}, func(h string) string { return filepath.Join(h, ".omp", "profiles", "work") }, "", true},
		{"PI_PROFILE", map[string]string{"PI_PROFILE": "work"}, func(h string) string { return filepath.Join(h, ".omp", "profiles", "work") }, "", true},
		{"an empty OMP_PROFILE beats PI_PROFILE", map[string]string{"OMP_PROFILE": "", "PI_PROFILE": "work"}, func(h string) string { return filepath.Join(h, ".omp", "profiles", "work") }, "", false},
		{"the default profile does not see a profile's plugins", nil, func(h string) string { return filepath.Join(h, ".omp", "profiles", "work") }, "", false},
		{"PI_CONFIG_DIR", map[string]string{"PI_CONFIG_DIR": ".myomp"}, func(h string) string { return filepath.Join(h, ".myomp") }, "", true},
		{"--profile in args", nil, func(h string) string { return filepath.Join(h, ".omp", "profiles", "work") }, "args = ['--profile', 'work']\n", true},
		{"--profile= in args beats the environment", map[string]string{"OMP_PROFILE": "other"}, func(h string) string { return filepath.Join(h, ".omp", "profiles", "work") }, "args = ['--profile=work']\n", true},
		{"a refused profile name", map[string]string{"OMP_PROFILE": "Bad Name"}, func(h string) string { return filepath.Join(h, ".omp") }, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			install(c.where(home))
			out := doctorFor(t, "[profiles.work]\ncommand = \"go\"\n"+c.args, OmpEnv{Home: home, Lookup: envOf(c.vars)})
			if got := strings.Contains(out, "profile work: kata-plugin") || strings.Contains(out, "profile default: kata-plugin") && c.args == ""; got != c.want {
				t.Fatalf("warned %v, want %v:\n%s", got, c.want, out)
			}
		})
	}
}

func TestDoctorFollowsXDGDataHomeOnlyOnceOmpsFolderExistsThere(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("omp's XDG redirect is Linux and macOS only")
	}
	home, xdg := t.TempDir(), t.TempDir()
	omp := OmpEnv{Home: home, Lookup: envOf(map[string]string{"XDG_DATA_HOME": xdg})}
	writeTestFile(t, filepath.Join(xdg, "omp", "plugins", "node_modules", "kata-omp", "package.json"), `{}`)
	if out := doctorFor(t, "", omp); !strings.Contains(out, "todo takeover") {
		t.Fatalf("no warning for the XDG plugins folder:\n%s", out)
	}
	omp.AgentDir = filepath.Join(home, "elsewhere")
	if out := doctorFor(t, "", omp); strings.Contains(out, "todo takeover") {
		t.Fatalf("a moved agent folder kept the XDG plugins folder:\n%s", out)
	}
	bare := OmpEnv{Home: home, Lookup: envOf(map[string]string{"XDG_DATA_HOME": t.TempDir()})}
	writeTestFile(t, filepath.Join(home, ".omp", "plugins", "node_modules", "kata-omp", "package.json"), `{}`)
	if out := doctorFor(t, "", bare); !strings.Contains(out, "todo takeover") {
		t.Fatalf("without omp's XDG folder the config root's plugins apply:\n%s", out)
	}
}

func TestDoctorDoesNotWarnForAProfileWhoseArgsDisableDiscovery(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".omp", "plugins", "node_modules", "kata-omp", "package.json"), `{}`)
	for _, args := range []string{"['--no-extensions']", "['--trusted-extension', '/x/k.ts']", "['--trusted-extension=/x/k.ts']"} {
		out := doctorFor(t, "[profiles.quiet]\ncommand = \"go\"\nargs = "+args+"\n", OmpEnv{Home: home})
		if strings.Contains(out, "profile quiet: kata-plugin") {
			t.Fatalf("args %s were warned of:\n%s", args, out)
		}
		if !strings.Contains(out, "profile default: kata-plugin") {
			t.Fatalf("the default profile lost its warning:\n%s", out)
		}
	}
}

func TestAnExtensionsListWithExtensionArgsIsRefusedWithTheProfilesName(t *testing.T) {
	for _, arg := range []string{"'-e', '/x.ts'", "'--extension', '/x.ts'", "'--extension=/x.ts'", "'--hook', '/x.ts'"} {
		_, err := load(t, "executor = \"laptop\"\n[profiles.default]\n[profiles.worker]\nextensions = ['/abs/k.ts']\nargs = ["+arg+"]\n")
		if err == nil || !strings.Contains(err.Error(), "profiles.worker: extensions") || !strings.Contains(err.Error(), "--hook") {
			t.Fatalf("args %s: error %v, want one naming profiles.worker", arg, err)
		}
	}
	if _, err := load(t, "executor = \"laptop\"\n[profiles.default]\nextensions = \"none\"\nargs = ['-e', '/x.ts']\n"); err != nil {
		t.Fatalf("none with -e is omp's own business: %v", err)
	}
}

func TestAReportWithAWarningSaysNothingIsMissingAndCountsIt(t *testing.T) {
	out := Report{Checks: []Check{{"ok", "config", "x", ""}, {"warn", "profile default", "y", ""}}}.String()
	if !strings.Contains(out, "Nothing is missing; 1 warning above") || strings.Contains(out, "Nothing is missing.\n") {
		t.Fatalf("summary:\n%s", out)
	}
	if out := (Report{Checks: []Check{{"ok", "config", "x", ""}}}).String(); !strings.HasSuffix(out, "Nothing is missing.\n") {
		t.Fatalf("a clean report:\n%s", out)
	}
}

func TestDoctorSuggestsCreatingWorkerTomlOnlyWhenItIsMissing(t *testing.T) {
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	if out := Doctor(context.Background(), DoctorOptions{Dirs: dirs}).String(); !strings.Contains(out, "Create "+dirs.ConfigPath()) {
		t.Fatalf("a missing file got no hint:\n%s", out)
	}
	writeTestFile(t, dirs.ConfigPath(), "executor = \"laptop\"\n[profiles.default]\nextensions = \"all\"\n")
	out := Doctor(context.Background(), DoctorOptions{Dirs: dirs}).String()
	if !strings.Contains(out, "profiles.default: extensions") || strings.Contains(out, "Create ") {
		t.Fatalf("a bad key in an existing file:\n%s", out)
	}
}

func TestDoctorLeavesTheCallersCheckoutSliceAlone(t *testing.T) {
	backing := make([]string, 1, 8)
	backing[0] = t.TempDir()
	omp := OmpEnv{Home: t.TempDir(), Checkouts: backing}
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	writeTestFile(t, dirs.ConfigPath(), "executor = \"laptop\"\n[checkouts]\nb = '"+t.TempDir()+"'\na = '"+t.TempDir()+"'\n[profiles.default]\ncommand = \"go\"\n")
	Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: omp})
	if extra := backing[:cap(backing)][1]; extra != "" {
		t.Fatalf("the doctor wrote %q into the caller's slice", extra)
	}
}
