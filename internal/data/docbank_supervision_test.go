package data_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/docbanksetup"
	"github.com/Dangerboi/tern-kata/internal/model"
)

const (
	supervisedKata   = "[docbank]\nurl = \"http://127.0.0.1:8080\"\napi_key = \"k\"\n"
	supervisedServer = "[server]\nbind_addr = \"127.0.0.1\"\napi_port = 8080\napi_key = \"k\"\n"
)

// supervisionGateway is a Gateway whose config folder, kata home and Docbank home are temporary folders holding the given
// config.toml texts; "" leaves that file out. It returns the marker's path.
func supervisionGateway(t *testing.T, kata, docbank string) (*data.Gateway, string) {
	t.Helper()
	config, state := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("XDG_STATE_HOME", state)
	kataHome, docbankHome := t.TempDir(), t.TempDir()
	for dir, text := range map[string]string{kataHome: kata, docbankHome: docbank} {
		if text != "" {
			if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	folder := filepath.Join(config, "tern-kata")
	g := data.NewGateway(nil).WithDocbankSupervision(func() data.DocbankSupervision {
		return docbanksetup.Env{ConfigDir: folder, DefaultKataHome: kataHome, DocbankHome: docbankHome}
	})
	return g, filepath.Join(folder, docbanksetup.ManualFileName)
}

func runOnce(t *testing.T, g *data.Gateway, cmd model.Cmd) model.Msg {
	t.Helper()
	messages := make(chan model.Msg, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	g.Run(ctx, cmd, messages)
	select {
	case msg := <-messages:
		return msg
	default:
		t.Fatalf("%T produced no message", cmd)
		return nil
	}
}

func docbankRow(t *testing.T, g *data.Gateway) model.ConfigFile {
	t.Helper()
	loaded, ok := runOnce(t, g, model.LoadConfigFiles{Request: 1}).(model.ConfigFilesLoaded)
	if !ok || loaded.Err != nil {
		t.Fatalf("read of the files: %+v", loaded)
	}
	keys := make([]model.ConfigFileKey, len(loaded.Files))
	var row model.ConfigFile
	for i, f := range loaded.Files {
		keys[i] = f.Key
		if f.Key == model.ConfigDocbank {
			row = f
		}
	}
	agent, docbank := -1, -1
	for i, k := range keys {
		switch k {
		case model.ConfigAgent:
			agent = i
		case model.ConfigDocbank:
			docbank = i
		}
	}
	if docbank != agent+1 {
		t.Fatalf("rows %v: the Docbank row is not right after agent.toml", keys)
	}
	return row
}

func TestDocbankSupervisionRowIsOnWhenTheRuleHoldsAndTheSwitchCreatesAndRemovesTheMarker(t *testing.T) {
	g, marker := supervisionGateway(t, supervisedKata, supervisedServer)
	row := docbankRow(t, g)
	if row.Path != marker || row.Status != model.ConfigMissing || !row.Supervised || row.Summary != "on: the plugin keeps Docbank running" || !row.Has(model.ConfigSwitch) {
		t.Fatalf("row %+v", row)
	}

	off := runOnce(t, g, model.SetDocbankSupervision{On: false}).(model.DocbankSupervisionSet)
	if _, err := os.Stat(marker); off.Err != nil || err != nil {
		t.Fatalf("turning supervision off: %v, marker %v", off.Err, err)
	}
	if again := runOnce(t, g, model.SetDocbankSupervision{On: false}).(model.DocbankSupervisionSet); again.Err != nil {
		t.Errorf("a repeat of off is an error: %v", again.Err)
	}
	row = docbankRow(t, g)
	if row.Status != model.ConfigLoads || !strings.HasPrefix(row.Summary, "off: docbank.manual exists in ") {
		t.Errorf("row after off: %+v", row)
	}

	on := runOnce(t, g, model.SetDocbankSupervision{On: true}).(model.DocbankSupervisionSet)
	if _, err := os.Stat(marker); on.Err != nil || !os.IsNotExist(err) {
		t.Fatalf("turning supervision on: %v, marker %v", on.Err, err)
	}
	if again := runOnce(t, g, model.SetDocbankSupervision{On: true}).(model.DocbankSupervisionSet); again.Err != nil {
		t.Errorf("a repeat of on is an error: %v", again.Err)
	}
}

func TestDocbankSupervisionRowGivesTheRulesReasonWhenItFails(t *testing.T) {
	for name, tc := range map[string]struct {
		kata, docbank, reason string
	}{
		"no [docbank] in kata's config": {"[tui]\nconfirm_quit = false\n", supervisedServer, "has no [docbank] url"},
		"a url on another machine":      {"[docbank]\nurl = \"https://docbank.example\"\napi_key = \"k\"\n", supervisedServer, "is not on this machine"},
		"ports that do not match":       {supervisedKata, strings.Replace(supervisedServer, "8080", "9090", 1), "is not 8080"},
	} {
		g, _ := supervisionGateway(t, tc.kata, tc.docbank)
		row := docbankRow(t, g)
		if !strings.HasPrefix(row.Summary, "off: ") || !strings.Contains(row.Summary, tc.reason) || row.Supervised {
			t.Errorf("%s: summary %q supervised %v, want off with %q", name, row.Summary, row.Supervised, tc.reason)
		}
		if done := runOnce(t, g, model.SetDocbankSupervision{On: true}).(model.DocbankSupervisionSet); done.Err != nil {
			t.Fatalf("%s: %v", name, done.Err)
		}
		if again := docbankRow(t, g); again.Summary != row.Summary {
			t.Errorf("%s: summary after on %q, want the same reason %q", name, again.Summary, row.Summary)
		}
	}
}

func TestDocbankSupervisionSwitchThatCannotWriteReturnsTheError(t *testing.T) {
	g, marker := supervisionGateway(t, supervisedKata, supervisedServer)
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(marker)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(marker), []byte("a file where the config folder should be"), 0o600); err != nil {
		t.Fatal(err)
	}
	if done := runOnce(t, g, model.SetDocbankSupervision{On: false}).(model.DocbankSupervisionSet); done.Err == nil {
		t.Error("creating the marker under a file did not fail")
	}
}
