package data

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.kenn.io/kit/consolewindow"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// defaultDocbankFolder is the folder `kata docbank setup` uses without --folder, and the one tern-kata's old setup wrote.
const defaultDocbankFolder = "/kata"

// switchDocbank moves the old pinned Docbank setup to kata's local one: it removes the [docbank] table from the default kata
// daemon's config.toml (every other byte stays), then has kata set Docbank up again with the old folder, `kata docbank setup
// --yes --folder <folder>` with that daemon's KATA_HOME and the Pane's environment otherwise. When kata fails, the original bytes
// go back unless the file changed after the switch wrote it; the answer says so then, and carries kata's error line.
func (g *Gateway) switchDocbank(ctx context.Context, _ model.SwitchDocbank) model.Msg {
	return model.DocbankSwitched{Err: switchPinnedDocbank(ctx)}
}

func switchPinnedDocbank(ctx context.Context) error {
	home, err := kataHome()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "config.toml")
	token, err := guardedfile.Read(path)
	if err != nil {
		return err
	}
	original := token.Text()
	section, err := parseDocbankSection(path, original)
	if err != nil {
		return err
	}
	docbankHome, _ := DocbankHome()
	if ClassifyDocbank(section, docbankHome) != DocbankPinned {
		return fmt.Errorf("%s no longer has tern-kata's old Docbank setup, so nothing was changed", path)
	}
	switched, ok := withoutDocbankTable(original)
	if !ok {
		return fmt.Errorf("%s has no [docbank] table to remove (edit it by hand), so nothing was changed", path)
	}
	if err := guardedfile.Write(path, 0o700, token, []byte(switched)); err != nil {
		if errors.Is(err, guardedfile.ErrChanged) {
			return fmt.Errorf("%s changed while the switch read it, so nothing was changed", path)
		}
		return err
	}
	folder := strings.TrimSpace(section.Folder)
	if folder == "" {
		folder = defaultDocbankFolder
	}
	setupErr := runDocbankSetup(ctx, home, folder)
	if setupErr == nil {
		return nil
	}
	return fmt.Errorf("%w%s", setupErr, restoreConfig(path, original, switched))
}

// restoreConfig puts original back over the text the switch wrote. It returns what to add to kata's error line: nothing when the
// file is as it was before, else why it was not restored. A file that differs from what the switch wrote was changed by someone
// else and is left alone.
func restoreConfig(path, original, switched string) string {
	token, err := guardedfile.Read(path)
	if err != nil {
		return "; " + path + " was not restored: " + err.Error()
	}
	if token.Text() != switched {
		return changedAfterSwitch(path)
	}
	if err := guardedfile.Write(path, 0o700, token, []byte(original)); err != nil {
		if errors.Is(err, guardedfile.ErrChanged) {
			return changedAfterSwitch(path)
		}
		return "; " + path + " was not restored: " + err.Error()
	}
	return ""
}

// changedAfterSwitch is what is added to kata's error line when the file differs from what the switch wrote: it is left as it is,
// and nothing is claimed about its content.
func changedAfterSwitch(path string) string {
	return "; " + path + " was not restored because it changed after the switch wrote it, so it was left as it is"
}

// runDocbankSetup runs `kata docbank setup --yes --folder <folder>` for the daemon whose home is kataHome. Its error is kata's last
// non-empty stderr line, or the run's own error when kata printed none.
func runDocbankSetup(ctx context.Context, kataHome, folder string) error {
	exe := kataExecutable()
	if exe == "" {
		return errors.New("kata is not on PATH")
	}
	cmd := exec.CommandContext(ctx, exe, "docbank", "setup", "--yes", "--folder", folder)
	cmd.Env = append(os.Environ(), "KATA_HOME="+kataHome)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	consolewindow.Hide(cmd)
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if line := lastLine(stderr.String()); line != "" {
		return errors.New(line)
	}
	return err
}

func lastLine(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

type tomlLineKind uint8

const (
	tomlContent tomlLineKind = iota
	tomlBlank                // empty, or a comment only
	tomlHeader
)

// withoutDocbankTable removes the [docbank] table from the config.toml text: its header line through its last line that holds
// a key or the rest of a value. The blank and comment lines after that stay, so a comment above the next table stays with it, and
// every other byte is kept as it is, line endings included. Only the table with exactly that name goes: subtables such as
// [docbank.x] and a [docbank] given as dotted keys or an inline table are not touched, which tern-kata's old setup never wrote.
// It reports false when there is no [docbank] header.
func withoutDocbankTable(text string) (string, bool) {
	bom := ""
	if strings.HasPrefix(text, "\uFEFF") {
		bom, text = "\uFEFF", text[len("\uFEFF"):]
	}
	lines := strings.SplitAfter(text, "\n")
	kinds := make([]tomlLineKind, len(lines))
	names := make([][]string, len(lines))
	var st tomlScan
	for i, line := range lines {
		if st.clean() {
			trimmed := strings.TrimLeft(line, " \t")
			switch {
			case strings.HasPrefix(trimmed, "["):
				kinds[i], names[i] = tomlHeader, tomlHeaderName(trimmed)
				continue
			case strings.TrimSpace(trimmed) == "" || strings.HasPrefix(trimmed, "#"):
				kinds[i] = tomlBlank
				continue
			}
		}
		st.scan(line)
	}
	start := -1
	for i, kind := range kinds {
		if kind == tomlHeader && len(names[i]) == 1 && names[i][0] == "docbank" {
			start = i
			break
		}
	}
	if start < 0 {
		return bom + text, false
	}
	end := start + 1
	for i := start + 1; i < len(lines) && kinds[i] != tomlHeader; i++ {
		if kinds[i] == tomlContent {
			end = i + 1
		}
	}
	return bom + strings.Join(lines[:start], "") + strings.Join(lines[end:], ""), true
}

// tomlHeaderName is the dotted key of a [table] header line, nil for an [[array of tables]] header or one it cannot read.
func tomlHeaderName(line string) []string {
	if strings.HasPrefix(line, "[[") {
		return nil
	}
	rest := line[1:]
	var parts []string
	for {
		rest = strings.TrimLeft(rest, " \t")
		var part string
		switch {
		case rest == "":
			return nil
		case rest[0] == '"' || rest[0] == '\'':
			quote := rest[0]
			i := 1
			var b strings.Builder
			for ; i < len(rest) && rest[i] != quote; i++ {
				if quote == '"' && rest[i] == '\\' && i+1 < len(rest) {
					i++
				}
				b.WriteByte(rest[i])
			}
			if i >= len(rest) {
				return nil
			}
			part, rest = b.String(), rest[i+1:]
		default:
			i := strings.IndexAny(rest, " \t.]")
			if i <= 0 {
				return nil
			}
			part, rest = rest[:i], rest[i:]
		}
		parts = append(parts, part)
		rest = strings.TrimLeft(rest, " \t")
		switch {
		case strings.HasPrefix(rest, "."):
			rest = rest[1:]
		case strings.HasPrefix(rest, "]"):
			return parts
		default:
			return nil
		}
	}
}

// tomlScan follows what a line leaves open, so a line that looks like a header inside a multi-line string or array is not one.
type tomlScan struct {
	basic, literal bool // inside a """ or ''' string
	depth          int  // open [ and { of a value
}

func (s *tomlScan) clean() bool { return !s.basic && !s.literal && s.depth == 0 }

func (s *tomlScan) scan(line string) {
	for i := 0; i < len(line); {
		switch {
		case s.basic:
			switch {
			case line[i] == '\\':
				i += 2
			case strings.HasPrefix(line[i:], `"""`):
				s.basic, i = false, i+3
			default:
				i++
			}
		case s.literal:
			if strings.HasPrefix(line[i:], "'''") {
				s.literal, i = false, i+3
			} else {
				i++
			}
		default:
			switch c := line[i]; c {
			case '#':
				return
			case '"', '\'':
				triple := strings.Repeat(string(c), 3)
				if strings.HasPrefix(line[i:], triple) {
					s.basic, s.literal, i = c == '"', c == '\'', i+3
					continue
				}
				i++
				for i < len(line) && line[i] != c {
					if c == '"' && line[i] == '\\' {
						i++
					}
					i++
				}
				i++
			case '[', '{':
				s.depth++
				i++
			case ']', '}':
				s.depth = max(0, s.depth-1)
				i++
			default:
				i++
			}
		}
	}
}
