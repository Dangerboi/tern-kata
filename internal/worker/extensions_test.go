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
		{"a missing key disables discovery", "", []string{"--no-extensions"}},
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
			args := agentArgs(cfg.Profiles["default"], t.TempDir(), "/p", "", "")
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

const ompManifest = `{"name":"kata-omp","version":"1","omp":{"extensions":["kata.ts"]}}`

// installKataPackage registers pkg under the plugins root the way npm does: in the root's package.json dependencies, with
// an omp manifest in node_modules.
func installKataPackage(t *testing.T, plugins, pkg string) {
	t.Helper()
	writeTestFile(t, filepath.Join(plugins, "package.json"), `{"dependencies":{"`+pkg+`":"1"}}`)
	writeKataModule(t, plugins, pkg, ompManifest)
}

func writeKataModule(t *testing.T, plugins, pkg, manifest string) {
	t.Helper()
	writeTestFile(t, filepath.Join(plugins, "node_modules", filepath.FromSlash(pkg), "package.json"), manifest)
}

func TestDoctorWarnsOfKataPluginOnlyForAnAmbientProfile(t *testing.T) {
	installs := map[string]func(home string, omp *OmpEnv){
		"the runtime registration": func(home string, _ *OmpEnv) {
			installKataPackage(t, filepath.Join(home, ".omp", "plugins"), "kata-omp")
		},
		"the lock file with the package in node_modules": func(home string, _ *OmpEnv) {
			plugins := filepath.Join(home, ".omp", "plugins")
			writeTestFile(t, filepath.Join(plugins, "omp-plugins.lock.json"), `{"plugins":{"kata-omp":{"version":"1","enabled":true}}}`)
			writeKataModule(t, plugins, "kata-omp", ompManifest)
		},
		// npm and marketplace installs register under the scoped name since kata-plugin published the package.
		"the scoped runtime registration": func(home string, _ *OmpEnv) {
			installKataPackage(t, filepath.Join(home, ".omp", "plugins"), "@dangerboi/kata-omp")
		},
		"the ambient extensions folder": func(home string, _ *OmpEnv) {
			writeTestFile(t, filepath.Join(home, ".omp", "agent", "extensions", "kata.ts"), "export default {}")
		},
		"a moved agent folder": func(home string, omp *OmpEnv) {
			elsewhere := filepath.Join(home, "elsewhere")
			omp.Lookup = envOf(map[string]string{"PI_CODING_AGENT_DIR": elsewhere})
			writeTestFile(t, filepath.Join(elsewhere, "extensions", "kata.ts"), "export default {}")
		},
		"the settings": func(home string, _ *OmpEnv) {
			writeTestFile(t, filepath.Join(home, ".omp", "agent", "config.yml"), "extensions:\n  - /src/kata-plugin/plugins/oh-my-pi/kata.ts\n")
		},
	}
	for name, install := range installs {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			omp := OmpEnv{Home: home}
			install(home, &omp)
			out := doctorFor(t, "", omp)
			if bare := doctorWith(t, "", omp); strings.Contains(bare, "todo takeover") {
				t.Fatalf("a profile with no key was warned:\n%s", bare)
			}
			for _, want := range []string{"warn", "profile default", "todo takeover", "3 extra turns", `extensions = "ambient"`} {
				if !strings.Contains(out, want) {
					t.Fatalf("no %q in:\n%s", want, out)
				}
			}
			if out := doctorWith(t, "extensions = \"none\"\n", omp); strings.Contains(out, "todo takeover") {
				t.Fatalf("a profile with none was warned:\n%s", out)
			}
			if out := doctorWith(t, "extensions = ['"+filepath.Join(home, "x.ts")+"']\n", omp); strings.Contains(out, "todo takeover") {
				t.Fatalf("a profile with a path list was warned:\n%s", out)
			}
		})
	}
	t.Run("nothing installed", func(t *testing.T) {
		if out := doctorFor(t, "", OmpEnv{Home: t.TempDir()}); strings.Contains(out, "todo takeover") {
			t.Fatalf("no plugin installed, yet a warning:\n%s", out)
		}
	})
	t.Run("the zero OmpEnv looks nowhere", func(t *testing.T) {
		if out := doctorFor(t, "", OmpEnv{}); strings.Contains(out, "todo takeover") {
			t.Fatalf("the zero OmpEnv looked somewhere:\n%s", out)
		}
	})
}

func TestDoctorFindsKataPluginInACheckoutsPlugins(t *testing.T) {
	checkout := t.TempDir()
	installKataPackage(t, filepath.Join(checkout, ".omp", "plugins"), "kata-omp")
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	writeTestFile(t, dirs.ConfigPath(), "executor = \"laptop\"\n[checkouts]\nrepo = '"+checkout+"'\n[profiles.default]\ncommand = \"go\"\nextensions = \"ambient\"\n")
	out := Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: OmpEnv{Home: t.TempDir()}}).String()
	if !strings.Contains(out, "todo takeover") {
		t.Fatalf("no warning for a checkout's plugins:\n%s", out)
	}
}

func TestDoctorFollowsOmpsRuleForWhichKataPluginsLoad(t *testing.T) {
	const scoped, legacy = "@dangerboi/kata-omp", "kata-omp"
	cases := []struct {
		name  string
		setup func(home, checkout string)
		want  bool
	}{
		{"a lock entry whose package is gone", func(home, _ string) {
			writeTestFile(t, filepath.Join(home, ".omp", "plugins", "omp-plugins.lock.json"), `{"plugins":{"kata-omp":{"enabled":true}}}`)
		}, false},
		{"a lock entry with no enabled value", func(home, _ string) {
			plugins := filepath.Join(home, ".omp", "plugins")
			writeTestFile(t, filepath.Join(plugins, "omp-plugins.lock.json"), `{"plugins":{"kata-omp":{"version":"1"}}}`)
			writeKataModule(t, plugins, legacy, ompManifest)
		}, false},
		{"a lock entry turned off", func(home, _ string) {
			plugins := filepath.Join(home, ".omp", "plugins")
			writeTestFile(t, filepath.Join(plugins, "omp-plugins.lock.json"), `{"plugins":{"kata-omp":{"enabled":false}}}`)
			writeKataModule(t, plugins, legacy, ompManifest)
		}, false},
		{"the scoped name turned off alone", func(home, _ string) {
			plugins := filepath.Join(home, ".omp", "plugins")
			writeTestFile(t, filepath.Join(plugins, "omp-plugins.lock.json"), `{"plugins":{"@dangerboi/kata-omp":{"enabled":false}}}`)
			writeTestFile(t, filepath.Join(plugins, "package.json"), `{"dependencies":{"@dangerboi/kata-omp":"1"}}`)
			writeKataModule(t, plugins, scoped, ompManifest)
		}, false},
		{"the scoped name off beside the legacy name on", func(home, _ string) {
			plugins := filepath.Join(home, ".omp", "plugins")
			writeTestFile(t, filepath.Join(plugins, "omp-plugins.lock.json"), `{"plugins":{"@dangerboi/kata-omp":{"enabled":false},"kata-omp":{"enabled":true}}}`)
			writeKataModule(t, plugins, scoped, ompManifest)
			writeKataModule(t, plugins, legacy, ompManifest)
		}, true},
		{"a package folder nobody registered", func(home, _ string) {
			plugins := filepath.Join(home, ".omp", "plugins")
			writeTestFile(t, filepath.Join(plugins, "package.json"), `{}`)
			writeKataModule(t, plugins, legacy, ompManifest)
		}, false},
		{"a package with no omp manifest", func(home, _ string) {
			plugins := filepath.Join(home, ".omp", "plugins")
			writeTestFile(t, filepath.Join(plugins, "package.json"), `{"dependencies":{"kata-omp":"1"}}`)
			writeKataModule(t, plugins, legacy, `{"name":"kata-omp"}`)
		}, false},
		{"a malformed lock file", func(home, _ string) {
			plugins := filepath.Join(home, ".omp", "plugins")
			installKataPackage(t, plugins, legacy)
			writeTestFile(t, filepath.Join(plugins, "omp-plugins.lock.json"), `{`)
		}, false},
		{"a project override that disables it", func(home, checkout string) {
			installKataPackage(t, filepath.Join(home, ".omp", "plugins"), scoped)
			writeTestFile(t, filepath.Join(checkout, ".omp", "plugin-overrides.json"), `{"disabled":["@dangerboi/kata-omp"]}`)
		}, false},
		{"the settings listing it as disabled", func(home, _ string) {
			writeTestFile(t, filepath.Join(home, ".omp", "agent", "config.yml"), "extensions:\n  - /src/kata-plugin/plugins/oh-my-pi/kata.ts\ndisabledExtensions:\n  - extension-module:kata\n")
		}, false},
		{"the extensions folder with the settings disabling it", func(home, _ string) {
			writeTestFile(t, filepath.Join(home, ".omp", "agent", "extensions", "kata.ts"), "export default {}")
			writeTestFile(t, filepath.Join(home, ".omp", "agent", "config.yml"), "disabledExtensions: ['extension-module:kata']\n")
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, checkout := t.TempDir(), t.TempDir()
			c.setup(home, checkout)
			dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
			writeTestFile(t, dirs.ConfigPath(), "executor = \"laptop\"\n[checkouts]\nrepo = '"+checkout+"'\n[profiles.default]\ncommand = \"go\"\nextensions = \"ambient\"\n")
			out := Doctor(context.Background(), DoctorOptions{Dirs: dirs, Omp: OmpEnv{Home: home}}).String()
			if got := strings.Contains(out, "todo takeover"); got != c.want {
				t.Fatalf("warned %v, want %v:\n%s", got, c.want, out)
			}
		})
	}
}

// doctorFor runs the doctor on a worker.toml with a default profile plus profiles, in the given omp environment.
func doctorFor(t *testing.T, profiles string, omp OmpEnv) string {
	t.Helper()
	return doctorWith(t, "extensions = \"ambient\"\n"+profiles, omp)
}

// doctorWith is doctorFor with the default profile's own lines given, so the test can leave out or change `extensions`.
func doctorWith(t *testing.T, defaultProfile string, omp OmpEnv) string {
	t.Helper()
	dirs := Dirs{Config: t.TempDir(), State: t.TempDir()}
	writeTestFile(t, dirs.ConfigPath(), "executor = \"laptop\"\n[profiles.default]\ncommand = \"go\"\n"+defaultProfile)
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
		installKataPackage(t, filepath.Join(root, "plugins"), "kata-omp")
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
			out := doctorFor(t, "[profiles.work]\ncommand = \"go\"\nextensions = \"ambient\"\n"+c.args, OmpEnv{Home: home, Lookup: envOf(c.vars)})
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
	installKataPackage(t, filepath.Join(xdg, "omp", "plugins"), "kata-omp")
	if out := doctorFor(t, "", omp); !strings.Contains(out, "todo takeover") {
		t.Fatalf("no warning for the XDG plugins folder:\n%s", out)
	}
	omp.Lookup = envOf(map[string]string{"XDG_DATA_HOME": xdg, "PI_CODING_AGENT_DIR": filepath.Join(home, "elsewhere")})
	if out := doctorFor(t, "", omp); strings.Contains(out, "todo takeover") {
		t.Fatalf("a moved agent folder kept the XDG plugins folder:\n%s", out)
	}
	bare := OmpEnv{Home: home, Lookup: envOf(map[string]string{"XDG_DATA_HOME": t.TempDir()})}
	installKataPackage(t, filepath.Join(home, ".omp", "plugins"), "kata-omp")
	if out := doctorFor(t, "", bare); !strings.Contains(out, "todo takeover") {
		t.Fatalf("without omp's XDG folder the config root's plugins apply:\n%s", out)
	}
}

func TestDoctorDoesNotWarnForAProfileWhoseArgsDisableDiscovery(t *testing.T) {
	home := t.TempDir()
	installKataPackage(t, filepath.Join(home, ".omp", "plugins"), "kata-omp")
	for _, args := range []string{"['--no-extensions']", "['--trusted-extension', '/x/k.ts']", "['--trusted-extension=/x/k.ts']"} {
		out := doctorFor(t, "[profiles.quiet]\ncommand = \"go\"\nextensions = \"ambient\"\nargs = "+args+"\n", OmpEnv{Home: home})
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
		_, err := load(t, "executor = \"laptop\"\n[profiles.default]\n[profiles.worker]\nextensions = ['"+filepath.Join(t.TempDir(), "k.ts")+"']\nargs = ["+arg+"]\n")
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

func TestARunLoadsTernKatasExtensionWithTheFlagItsProfileAllows(t *testing.T) {
	trusted := filepath.Join(t.TempDir(), "a.ts")
	const ext = "/runs/r/tern-kata.ts"
	cases := []struct{ name, profile, flag string }{
		{"a missing key loads it under --no-extensions", "", "-e"},
		{"ambient loads it beside what omp finds", "extensions = \"ambient\"\n", "-e"},
		{"a list trusts it too, as omp refuses -e beside --trusted-extension", "extensions = ['" + trusted + "']\n", "--trusted-extension"},
		{"a --trusted-extension in args does the same", "args = ['--trusted-extension', '" + trusted + "']\n", "--trusted-extension"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := load(t, "executor = \"laptop\"\n[profiles.default]\n"+c.profile)
			if err != nil {
				t.Fatal(err)
			}
			args := agentArgs(cfg.Profiles["default"], t.TempDir(), "/p", "", ext)
			if n := len(args); n < 3 || args[n-3] != c.flag || args[n-2] != ext || args[n-1] != "@/p" {
				t.Fatalf("args %v: want %s %s just before the prompt", args, c.flag, ext)
			}
		})
	}
}
