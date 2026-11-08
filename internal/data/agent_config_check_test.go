package data

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestCheckAgentConfigAppliesEveryRuleAnAgentStartApplies(t *testing.T) {
	for name, tc := range map[string]struct{ text, want string }{
		"unknown key":              {"bogus = 1\n", "unknown key bogus"},
		"unknown key in a Project": {"[projects.a]\nbogus = 1\n", "projects.a.bogus"},
		"host in a Project":        {"[projects.a]\nhost = \"x\"\n", "projects.a.host"},
		"empty host":               {"host = \" \"\n", "host: must not be empty"},
		"bad name":                 {"name = \"a b\"\n", "name:"},
		"bad Project name":         {"[projects.a]\nname = \"a/b\"\n", "projects.a.name:"},
		"empty command":            {"command = []\n", "command: must name a program"},
		"blank program":            {"[projects.a]\ncommand = [\" \"]\n", "projects.a.command:"},
		"relative checkout":        {"[projects.a]\ncheckout = \"rel\"\n", "projects.a.checkout:"},
		"message_from comma":       {"message_from = [\"a,b\"]\n", "message_from:"},
		"Project message_from":     {"[projects.a]\nmessage_from = [\"\"]\n", "projects.a.message_from:"},
		"not toml":                 {"name = \n", "agent.toml"},
		"forward_comments string":  {"forward_comments = \"yes\"\n", "agent.toml"},
	} {
		err := CheckAgentConfig(tc.text)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: check gave %v, want an error with %q", name, err, tc.want)
			continue
		}
		path := filepath.Join(t.TempDir(), AgentConfigFile)
		if werr := os.WriteFile(path, []byte(tc.text), 0o600); werr != nil {
			t.Fatal(werr)
		}
		_, readErr := ReadAgentConfig(path)
		if readErr == nil || strings.ReplaceAll(readErr.Error(), path, AgentConfigFile) != err.Error() {
			t.Errorf("%s: the file read gave %v, the text check %v; they must be one rule", name, readErr, err)
		}
	}
	for name, text := range map[string]string{
		"empty": "", "every key": "actor = \"a\"\nhost = \"h\"\nname = \"n\"\ncommand = [\"omp\"]\nforward_comments = true\nmessage_from = [\"d\"]\n[projects.p]\nactor = \"b\"\nname = \"m\"\ncommand = [\"x\"]\nforward_comments = false\nmessage_from = []\ncheckout = \"" + filepath.ToSlash(t.TempDir()) + "\"\n",
	} {
		if err := CheckAgentConfig(text); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// tomlKeys are the keys a struct decodes from, by its toml tags.
func tomlKeys(v any) map[string]bool {
	keys := map[string]bool{}
	typ := reflect.TypeOf(v)
	for i := range typ.NumField() {
		keys[typ.Field(i).Tag.Get("toml")] = true
	}
	return keys
}

func TestAgentConfigStarterShowsEveryKeyTheParserAcceptsAsComments(t *testing.T) {
	top, projects, found := strings.Cut(AgentConfigStarter, "# [projects.")
	if !found {
		t.Fatal("the starter has no [projects.<name>] table")
	}
	for _, line := range strings.Split(strings.TrimSpace(AgentConfigStarter), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			t.Errorf("starter line %q is not a comment", line)
		}
	}
	topKeys := tomlKeys(AgentConfig{})
	delete(topKeys, "projects")
	for key := range topKeys {
		if !strings.Contains(top, "\n# "+key+" = ") {
			t.Errorf("the starter's top level lacks %q", key)
		}
	}
	for key := range tomlKeys(AgentProject{}) {
		if !strings.Contains(projects, "\n# "+key+" = ") {
			t.Errorf("the starter's [projects.<name>] table lacks %q", key)
		}
	}
	if strings.Contains(projects, "\n# host = ") {
		t.Error("the starter shows host inside a Project, where it is refused")
	}
}

func TestAgentConfigStarterLoadsWithEveryKeySetAndWhenUnchanged(t *testing.T) {
	if err := CheckAgentConfig(AgentConfigStarter); err != nil {
		t.Fatalf("the starter as it is: %v", err)
	}
	assignment := regexp.MustCompile(`^# (\[|[a-z_]+ = )`)
	var lines []string
	for _, line := range strings.Split(AgentConfigStarter, "\n") {
		if assignment.MatchString(line) {
			line = strings.TrimPrefix(line, "# ")
		}
		lines = append(lines, line)
	}
	text := strings.Join(lines, "\n")
	if err := CheckAgentConfig(text); err != nil {
		t.Fatalf("the starter with every comment taken off: %v\n%s", err, text)
	}
	cfg, err := parseAgentConfig(AgentConfigFile, text)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host == "" || cfg.ForwardComments == nil || len(cfg.MessageFrom) == 0 || len(cfg.Projects) != 1 {
		t.Errorf("uncommenting the starter left keys unset: %+v", cfg)
	}
}

func TestSaveAgentConfigWritesCheckedTextOverTheFileItWasEditedFrom(t *testing.T) {
	g, dirs := setupGateway(t)
	path := filepath.Join(dirs.Config, AgentConfigFile)

	created := as[model.AgentConfigSaved](t, run(t, g, model.SaveAgentConfig{Text: "name = \"one\"\n"}))
	if created.Err != nil || created.Text != "name = \"one\"\n" || readFile(t, path) != "name = \"one\"\n" {
		t.Fatalf("a checked write over a missing file: %+v, file %q", created, readFile(t, path))
	}
	replaced := as[model.AgentConfigSaved](t, run(t, g, model.SaveAgentConfig{Text: "name = \"two\"\n# kept\n", Base: "name = \"one\"\n"}))
	if replaced.Err != nil || readFile(t, path) != "name = \"two\"\n# kept\n" {
		t.Fatalf("a checked write over its base: %+v, file %q", replaced, readFile(t, path))
	}
}

func TestSaveAgentConfigRefusedTextLeavesTheFileByteForByte(t *testing.T) {
	g, dirs := setupGateway(t)
	path := filepath.Join(dirs.Config, AgentConfigFile)
	const before = "name = \"one\"  # mine\r\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	got := as[model.AgentConfigSaved](t, run(t, g, model.SaveAgentConfig{Text: "name = \"one\"\nbogus = 1\n", Base: before}))
	if got.Err == nil || !strings.Contains(got.Err.Error(), "unknown key bogus") || got.Text != "name = \"one\"\nbogus = 1\n" {
		t.Errorf("answer %+v, want the refusal naming the key with the text sent", got)
	}
	if after := readFile(t, path); after != before {
		t.Errorf("a refused text changed the file: %q", after)
	}
	if entries, _ := filepath.Glob(path + "*"); len(entries) > 2 {
		t.Errorf("a refused save left files beside agent.toml: %v", entries)
	}
}

func TestSaveAgentConfigChangedFileIsRefusedWithoutWriting(t *testing.T) {
	g, dirs := setupGateway(t)
	path := filepath.Join(dirs.Config, AgentConfigFile)
	const theirs = "name = \"theirs\"\n"
	if err := os.WriteFile(path, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}
	got := as[model.AgentConfigSaved](t, run(t, g, model.SaveAgentConfig{Text: "name = \"mine\"\n", Base: "name = \"old\"\n"}))
	if !errors.Is(got.Err, model.ErrAgentConfigChanged) {
		t.Errorf("error %v, want the changed-file error", got.Err)
	}
	if readFile(t, path) != theirs {
		t.Errorf("a changed file was overwritten: %q", readFile(t, path))
	}
	missing := as[model.AgentConfigSaved](t, run(t, g, model.SaveAgentConfig{Text: "name = \"mine\"\n", Base: ""}))
	if !errors.Is(missing.Err, model.ErrAgentConfigChanged) || readFile(t, path) != theirs {
		t.Errorf("a file created meanwhile: error %v, file %q", missing.Err, readFile(t, path))
	}
}

func TestConfigFilesAgentRowCarriesTheTextAndTheStarterAnEditNeeds(t *testing.T) {
	rows, _, _ := configFiles(t, nil, "")
	if row := rows[model.ConfigAgent]; row.Text != "" || row.Starter != AgentConfigStarter || !row.Has(model.ConfigEdit) {
		t.Errorf("a missing agent.toml row: %+v", row)
	}
	rows, _, _ = configFiles(t, map[string]string{"agent.toml": "name = \"builder\"\r\n"}, "")
	if row := rows[model.ConfigAgent]; row.Text != "name = \"builder\"\r\n" {
		t.Errorf("the row's text %q, want the file's bytes as the base", row.Text)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
