// Package closetranscript decides whether an agent that tern-kata starts records its session on `kata close`.
//
// kata records the session an agent names (KATA_TRANSCRIPT_AGENT, KATA_TRANSCRIPT_SESSION_ID) only when
// [close.transcript] enabled is true in its config.toml, or KATA_CLOSE_TRANSCRIPT_ENABLED says so for one process.
// tern-kata's omp extension sets that switch with the names when Env is on in its environment.
package closetranscript

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Env is the variable that turns the omp extension's switch on.
const Env = "TERN_KATA_CLOSE_TRANSCRIPT"

// On is the value Env carries when the decision is on.
const On = "on"

const kataSwitch = "KATA_CLOSE_TRANSCRIPT_ENABLED"

// Decide reports whether the agent whose environment is environ gets Env=on: the choice stays with the user when the
// environment sets KATA_CLOSE_TRANSCRIPT_ENABLED to a non-blank value (kata reads a blank one as unset) or the config.toml of
// its kata home (KATA_HOME, else ~/.kata) has an `enabled` key under [close.transcript]. No such file means on; an unreadable
// or invalid file means off, so the file stays in charge and kata warns about it on its own.
func Decide(environ []string, caseInsensitive bool) bool {
	home := ""
	for _, kv := range environ {
		name, value, _ := strings.Cut(kv, "=")
		if caseInsensitive {
			name = strings.ToUpper(name)
		}
		switch name {
		case kataSwitch:
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
