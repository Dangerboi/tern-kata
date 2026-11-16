package ompext_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/data/ompext"
)

// tern-kata.ts spells the names it shares with the Pane; this reads the embedded source so a rename on either side fails here.
func TestTheExtensionSourceSpellsTheNamesTheGoSideSharesWithIt(t *testing.T) {
	src := string(ompext.Source)
	for _, c := range []struct{ constant, want string }{
		{"SESSION_PATH_SUFFIX", data.AgentSessionSuffix},
		{"PROMPT_FILE_ENV", ompext.PromptFileEnv},
	} {
		m := regexp.MustCompile(`export const ` + c.constant + ` = "([^"]*)";`).FindStringSubmatch(src)
		if m == nil {
			t.Fatalf("tern-kata.ts has no `export const %s = \"...\"`", c.constant)
		}
		if m[1] != c.want {
			t.Errorf("%s is %q in tern-kata.ts, %q in Go", c.constant, m[1], c.want)
		}
	}
}

// The prompt file rule: the folder data.AgentPromptsFolder and the names data.AgentPromptName makes, whole path.
func TestTheExtensionRecognisesThePromptFileNamesThePaneMakes(t *testing.T) {
	m := regexp.MustCompile(`const arg = argv\[i\];\n\s*if \(arg\.startsWith\("@"\) && /(.+)/\.test\(arg\)\)`).FindStringSubmatch(string(ompext.Source))
	if m == nil {
		t.Fatal("promptFileFromArgv in tern-kata.ts no longer tests an @word against one regular expression")
	}
	pattern := m[1]
	if !strings.Contains(pattern, data.AgentPromptsFolder+`[\\/]prompt-`) {
		t.Errorf("the extension's prompt file pattern %q does not name the %s folder and prompt- prefix", m[1], data.AgentPromptsFolder)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("pattern %q: %v", pattern, err)
	}
	for _, starter := range []string{"", "24734716657667"} {
		name := strings.Replace(data.AgentPromptName(starter), "*", "3836283849", 1)
		for _, sep := range []string{"/", `\`} {
			path := "@/s" + sep + data.AgentPromptsFolder + sep + name
			if !re.MatchString(path) {
				t.Errorf("%q is a prompt file the Pane makes, the extension's pattern %q does not match it", path, m[1])
			}
			if re.MatchString(path + ".token") {
				t.Errorf("the extension's pattern matches a secret file beside %q", path)
			}
		}
	}
}
