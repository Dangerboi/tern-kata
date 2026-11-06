package docbanksetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// supervisionEnv is three empty folders: kata's home, Docbank's home and tern-kata's config folder.
func supervisionEnv(t *testing.T, kataConfig, docbankConfig string) Env {
	t.Helper()
	e := Env{KataHome: t.TempDir(), DocbankHome: t.TempDir(), ConfigDir: filepath.Join(t.TempDir(), "tern-kata")}
	e.DefaultKataHome = e.KataHome
	for dir, text := range map[string]string{e.KataHome: kataConfig, e.DocbankHome: docbankConfig} {
		if text == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func TestSupervisedRule(t *testing.T) {
	const local = "[docbank]\nurl = \"http://127.0.0.1:8080\"\napi_key = \"k\"\n"
	const server = "[server]\nbind_addr = \"127.0.0.1\"\napi_port = 8080\napi_key = \"k\"\n"
	for name, tc := range map[string]struct {
		kata, docbank string
		manual        bool
		want          bool
	}{
		"local url, matching port":    {local, server, false, true},
		"localhost name":              {strings.Replace(local, "127.0.0.1", "localhost", 1), server, false, true},
		"remote url":                  {"[docbank]\nurl = \"https://docbank.example\"\napi_key = \"k\"\n", server, false, false},
		"no [docbank] section":        {"[tui]\nconfirm_quit = false\n", server, false, false},
		"no kata config":              {"", server, false, false},
		"port mismatch":               {local, strings.Replace(server, "8080", "9090", 1), false, false},
		"no docbank [server] section": {local, "", false, false},
		"marker present":              {local, server, true, false},
	} {
		e := supervisionEnv(t, tc.kata, tc.docbank)
		if tc.manual {
			if err := e.SetSupervision(false); err != nil {
				t.Fatal(err)
			}
		}
		if got, why := e.Supervised(); got != tc.want || (!got && why == "") {
			t.Errorf("%s: Supervised() = %v, %q; want %v", name, got, why, tc.want)
		}
	}
}

func TestSetSupervisionCreatesAndRemovesThePrivateMarker(t *testing.T) {
	e := supervisionEnv(t, "", "")
	if err := e.SetSupervision(false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(e.ManualPath())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("marker: %v, %v", info, err)
	}
	if err := e.SetSupervision(false); err != nil {
		t.Errorf("a second off: %v", err)
	}
	if err := e.SetSupervision(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.ManualPath()); !os.IsNotExist(err) {
		t.Errorf("marker still there: %v", err)
	}
	if err := e.SetSupervision(true); err != nil {
		t.Errorf("a second on: %v", err)
	}
}

func TestSupervisionRuleAndReportFollowTheDefaultDaemon(t *testing.T) {
	const local = "[docbank]\nurl = \"http://127.0.0.1:8080\"\napi_key = \"k\"\n"
	const server = "[server]\nbind_addr = \"127.0.0.1\"\napi_port = 8080\napi_key = \"k\"\n"
	e := supervisionEnv(t, "", server) // the daemon being set up has no [docbank] section yet
	e.DefaultKataHome = t.TempDir()
	if err := os.WriteFile(filepath.Join(e.DefaultKataHome, "config.toml"), []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}
	if on, why := e.Supervised(); !on {
		t.Errorf("the rule must read the default daemon's config: %q", why)
	}
	var out strings.Builder
	e.Out = &out
	if err := e.reportSupervision(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "only for the default kata daemon") || strings.Contains(out.String(), "its host half starts it") {
		t.Errorf("a daemon that is not the default one was told the plugin keeps its Docbank running:\n%s", out.String())
	}
	out.Reset()
	e.DefaultKataHome = e.KataHome
	if err := os.WriteFile(filepath.Join(e.KataHome, "config.toml"), []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.reportSupervision(); err != nil || !strings.Contains(out.String(), "its host half starts it") {
		t.Errorf("the default daemon: %q, %v", out.String(), err)
	}
}

func TestSupervisionIsOffWhenTheDefaultDaemonCannotBeResolved(t *testing.T) {
	const local = "[docbank]\nurl = \"http://127.0.0.1:8080\"\napi_key = \"k\"\n"
	const server = "[server]\nbind_addr = \"127.0.0.1\"\napi_port = 8080\napi_key = \"k\"\n"
	e := supervisionEnv(t, local, server)
	e.DefaultKataHome = ""
	if on, why := e.Supervised(); on || why == "" {
		t.Errorf("Supervised() = %v, %q; want off with a reason", on, why)
	}
	var out strings.Builder
	e.Out = &out
	if err := e.reportSupervision(); err != nil || !strings.Contains(out.String(), "cannot be resolved") || strings.Contains(out.String(), "its host half starts it") {
		t.Errorf("report: %q, %v", out.String(), err)
	}
}
