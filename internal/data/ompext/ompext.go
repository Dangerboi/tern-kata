// Package ompext holds tern-kata's omp extension, tern-kata.ts, and the one rule for loading it: the Pane's agents and the
// worker's runs both ask here whether a program is omp, which flag loads the extension, and where the file goes.
package ompext

import (
	"bytes"
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
)

// Source is tern-kata.ts: in an omp agent it names the session to `kata close` (KATA_TRANSCRIPT_AGENT and
// KATA_TRANSCRIPT_SESSION_ID) and, given an Agent address, delivers the messages written to it.
//
//go:embed tern-kata.ts
var Source []byte

const FileName = "tern-kata.ts"

const Program = "omp"

func IsOmp(program string) bool {
	base := filepath.Base(program)
	return strings.TrimSuffix(base, filepath.Ext(base)) == Program
}

// Argv is the omp flag and path that load the extension at path. omp refuses -e beside --trusted-extension, so an agent that
// trusts paths (args hold --trusted-extension, or the caller's `extensions` list does: trustsPaths) gets one more of those; omp
// keeps either under --no-extensions. omp refuses a relative trusted path, so path must be absolute.
func Argv(path string, args []string, trustsPaths bool) []string {
	if trustsPaths || HasFlag(args, "--trusted-extension") {
		return []string{"--trusted-extension", path}
	}
	return []string{"-e", path}
}

// HasFlag reports whether args hold any of flags, as `--flag` or `--flag=value`.
func HasFlag(args []string, flags ...string) bool {
	return slices.ContainsFunc(args, func(a string) bool {
		flag, _, _ := strings.Cut(a, "=")
		return slices.Contains(flags, flag)
	})
}

// Install puts the extension in folder and returns its absolute path. A file of other content (an older tern-kata's) is
// replaced; one already current is left alone. An agent already running keeps the extension it loaded.
func Install(folder string) (string, error) {
	path, err := filepath.Abs(filepath.Join(folder, FileName))
	if err != nil {
		return "", err
	}
	if held, err := os.ReadFile(path); err == nil && bytes.Equal(held, Source) {
		return path, nil
	}
	if err := atomicfile.Write(path, Source, atomicfile.Options{Mode: 0o600, DirMode: 0o700}); err != nil {
		return "", err
	}
	return path, nil
}

// PromptFileEnv names the agent's prompt file for the extension, which writes the session path beside it, when the agent's
// command line has no `@<prompt file>` word: a chat Tern reopened, whose line the reopen hook gave back without it.
const PromptFileEnv = "TERN_KATA_PROMPT_FILE"

// CloseTranscriptEnv is the variable that turns the extension's close-transcript switch on. kata records the session an
// agent names (KATA_TRANSCRIPT_AGENT, KATA_TRANSCRIPT_SESSION_ID) only when [close.transcript] enabled is true in its
// config.toml, or KATA_CLOSE_TRANSCRIPT_ENABLED says so for one process; the extension sets that switch with the names when
// this variable is CloseTranscriptOnValue in its environment.
const CloseTranscriptEnv = "TERN_KATA_CLOSE_TRANSCRIPT"

// CloseTranscriptOnValue is the value CloseTranscriptEnv carries when the decision is on.
const CloseTranscriptOnValue = "on"

// KataSwitch is kata's own variable for the same decision, which a user may set; CloseTranscriptOn leaves the choice to it.
const KataSwitch = "KATA_CLOSE_TRANSCRIPT_ENABLED"

// CloseTranscriptOn reports whether the agent whose environment is environ gets CloseTranscriptEnv=on: the choice stays with
// the user when the environment sets KATA_CLOSE_TRANSCRIPT_ENABLED to a non-blank value (kata reads a blank one as unset) or
// the config.toml of its kata home (KATA_HOME, else ~/.kata) has an `enabled` key under [close.transcript]. No such file means
// on; an unreadable or invalid file means off, so the file stays in charge and kata warns about it on its own.
func CloseTranscriptOn(environ []string, caseInsensitive bool) bool {
	home := ""
	for _, kv := range environ {
		name, value, _ := strings.Cut(kv, "=")
		if caseInsensitive {
			name = strings.ToUpper(name)
		}
		switch name {
		case KataSwitch:
			if strings.TrimSpace(value) != "" {
				return false
			}
		case "KATA_HOME":
			home = value
		}
	}
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		home = filepath.Join(user, ".kata")
	}
	text, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	var doc struct {
		Close struct {
			Transcript map[string]any `toml:"transcript"`
		} `toml:"close"`
	}
	if err := toml.Unmarshal(text, &doc); err != nil {
		return false
	}
	_, set := doc.Close.Transcript["enabled"]
	return !set
}

// WithCloseTranscript returns env for a child with CloseTranscriptEnv decided afresh: an inherited value is dropped, and
// CloseTranscriptOnValue is added when CloseTranscriptOn says so for decideFrom, the environment the child's kata commands
// will read.
func WithCloseTranscript(env, decideFrom []string, caseInsensitive bool) []string {
	out := slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		if caseInsensitive {
			name = strings.ToUpper(name)
		}
		return name == CloseTranscriptEnv
	})
	if CloseTranscriptOn(decideFrom, caseInsensitive) {
		out = append(out, CloseTranscriptEnv+"="+CloseTranscriptOnValue)
	}
	return out
}
