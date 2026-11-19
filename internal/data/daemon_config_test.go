package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = `# kata client config
active_daemon = "lab"

[tui]
confirm_quit = false # ask first

# the shared lab box
[[daemon]]
name = "lab"
url = "https://lab.example:7474"
token_env = "LAB_TOKEN"
# kept: a note on the entry
allow_insecure = true

[[daemon]]
name = "home"
local = true
`

func configHome(t *testing.T, text string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	if text != "" {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(home, "config.toml")
}

func configText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAddingAnEntryAppendsATableAndKeepsEveryOtherLine(t *testing.T) {
	path := configHome(t, sampleConfig)
	if err := AddDaemon(DaemonSpec{Name: "work", URL: "https://work.example", Token: "s3cret"}); err != nil {
		t.Fatal(err)
	}
	got := configText(t, path)
	if !strings.HasPrefix(got, sampleConfig) {
		t.Errorf("an add changed what was there:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n[[daemon]]\nname = \"work\"\nurl = \"https://work.example\"\ntoken = \"s3cret\"\n") {
		t.Errorf("the new entry is not appended as a table:\n%s", got)
	}
	cat, err := ReadCatalogAt(filepath.Dir(path))
	if err != nil || len(cat.Daemons) != 3 || cat.Daemons[2].InlineToken != "s3cret" {
		t.Errorf("catalog %+v, err %v", cat.Daemons, err)
	}
}

func TestAddingToAMissingFileCreatesItPrivate(t *testing.T) {
	path := configHome(t, "")
	if err := AddDaemon(DaemonSpec{Name: "home", Local: true}); err != nil {
		t.Fatal(err)
	}
	if got := configText(t, path); got != "[[daemon]]\nname = \"home\"\nlocal = true\n" {
		t.Errorf("file %q", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, err %v", info.Mode(), err)
	}
}

func TestEditingChangesOnlyTheEntrysKeysAndKeepsItsComments(t *testing.T) {
	path := configHome(t, sampleConfig)
	if err := UpdateDaemon("lab", DaemonSpec{Name: "lab", URL: "https://lab2.example", TokenEnv: "OTHER"}); err != nil {
		t.Fatal(err)
	}
	got := configText(t, path)
	for _, want := range []string{"# kata client config", "confirm_quit = false # ask first", "# the shared lab box", "# kept: a note on the entry",
		`url = "https://lab2.example"`, `token_env = "OTHER"`, "[[daemon]]\nname = \"home\"\nlocal = true\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("the edit lost %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"lab.example", "LAB_TOKEN", "allow_insecure"} {
		if strings.Contains(got, gone) {
			t.Errorf("the edit kept %q:\n%s", gone, got)
		}
	}
}

func TestEditingWithKeepTokenLeavesTheTokenLines(t *testing.T) {
	path := configHome(t, sampleConfig)
	if err := UpdateDaemon("lab", DaemonSpec{Name: "lab", URL: "https://lab.example:7474", KeepToken: true, AllowInsecure: true}); err != nil {
		t.Fatal(err)
	}
	if got := configText(t, path); !strings.Contains(got, `token_env = "LAB_TOKEN"`) || strings.Count(got, "token_env") != 1 {
		t.Errorf("the token was not kept:\n%s", got)
	}
}

func TestRenamingTheActiveEntryIsRefusedAndWritesNothing(t *testing.T) {
	path := configHome(t, sampleConfig)
	err := UpdateDaemon("lab", DaemonSpec{Name: "lab2", URL: "https://lab.example:7474"})
	if err == nil || !strings.Contains(err.Error(), `active_daemon "lab" is not in daemon catalog`) {
		t.Fatalf("err %v", err)
	}
	if configText(t, path) != sampleConfig {
		t.Error("a refused edit changed the file")
	}
}

func TestRemovingDropsTheEntryItsCommentsAndNothingElse(t *testing.T) {
	path := configHome(t, strings.Replace(sampleConfig, `active_daemon = "lab"`, `active_daemon = "home"`, 1))
	if err := RemoveDaemon("lab"); err != nil {
		t.Fatal(err)
	}
	got := configText(t, path)
	want := "# kata client config\nactive_daemon = \"home\"\n\n[tui]\nconfirm_quit = false # ask first\n\n[[daemon]]\nname = \"home\"\nlocal = true\n"
	if got != want {
		t.Errorf("file:\n%s\nwant:\n%s", got, want)
	}
}

func TestRemovingTheActiveDaemonIsRefused(t *testing.T) {
	path := configHome(t, sampleConfig)
	err := RemoveDaemon("lab")
	if err == nil || !strings.Contains(err.Error(), "active_daemon") {
		t.Fatalf("err %v", err)
	}
	if configText(t, path) != sampleConfig {
		t.Error("a refused remove changed the file")
	}
}

func TestADuplicateNameOrAnInvalidEntryIsRefused(t *testing.T) {
	path := configHome(t, sampleConfig)
	for name, spec := range map[string]DaemonSpec{
		"duplicate":        {Name: "home", Local: true},
		"both":             {Name: "x", Local: true, URL: "https://x"},
		"neither":          {Name: "x"},
		"no name":          {Local: true},
		"two tokens":       {Name: "x", URL: "https://x", TokenEnv: "A", Token: "b"},
		"control in name":  {Name: "a\nb", Local: true},
		"control in token": {Name: "x", URL: "https://x", Token: "a\nb"},
	} {
		if err := AddDaemon(spec); err == nil {
			t.Errorf("%s: no refusal", name)
		}
	}
	if configText(t, path) != sampleConfig {
		t.Error("a refused add changed the file")
	}
}

func TestAnUnparsableFileIsNeverOverwritten(t *testing.T) {
	path := configHome(t, "[[daemon\nname = ")
	if err := RemoveDaemon("x"); err == nil {
		t.Error("no refusal")
	}
	if configText(t, path) != "[[daemon\nname = " {
		t.Error("the broken file was changed")
	}
}

func TestQuotesAndBackslashesInValuesRoundTrip(t *testing.T) {
	path := configHome(t, "")
	if err := AddDaemon(DaemonSpec{Name: `my "box"`, URL: `https://x/a\b`}); err != nil {
		t.Fatal(err)
	}
	cat, err := ReadCatalogAt(filepath.Dir(path))
	if err != nil || cat.Daemons[0].Name != `my "box"` || cat.Daemons[0].URL != `https://x/a\b` {
		t.Errorf("catalog %+v, err %v", cat.Daemons, err)
	}
}
