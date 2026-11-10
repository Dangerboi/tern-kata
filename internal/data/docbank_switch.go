package data

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// defaultDocbankFolder is the folder `kata docbank setup` uses without --folder, and the one tern-kata's old setup wrote.
const defaultDocbankFolder = "/kata"

// switchDocbank moves the old pinned Docbank setup to kata's local one: it removes the [docbank] table from the default kata
// daemon's config.toml (every other byte stays), then has kata set Docbank up again with the old folder, `kata docbank setup
// --yes --folder <folder>` with that daemon's KATA_HOME and the Pane's environment otherwise. When kata fails, the original bytes
// go back unless the file changed after the switch wrote it; the answer says so then, and carries kata's error line.
//
// The switch is not cancelled with ctx and Close waits for it, so a closing Pane still gets kata's answer or the restore. A
// switch that would start after Close changes nothing.
func (g *Gateway) switchDocbank(ctx context.Context, _ model.SwitchDocbank) model.Msg {
	done, ok := g.track()
	if !ok {
		return model.DocbankSwitched{Err: errors.New("the Pane is closing, so nothing was changed")}
	}
	defer done()
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
		return fmt.Errorf("the [docbank] section of %s cannot be removed without changing anything else in the file (edit it by hand), so nothing was changed", path)
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
	setupErr := runKata(ctx, home, os.Environ(), "docbank", "setup", "--yes", "--folder", folder)
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

// lastLine is the last non-empty line of text.
func lastLine(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// withoutDocbankTable removes the [docbank] table from the config.toml text: its header line through the lines before the next
// line that starts a table, less the blank and comment lines just before that one, so a comment above the next table stays with it.
// Every other byte is kept as it is, line endings included. The cut is a plain line cut, so it is checked by decoding: the
// result must decode to what the original does less the plain keys under docbank. A [docbank.x] subtable stays; a file
// whose cut would break that (a value that spans lines and holds a line looking like a header, an inline table, dotted keys) is
// refused. It reports false when there is no [docbank] header or the cut is refused.
func withoutDocbankTable(text string) (string, bool) {
	bom := ""
	if strings.HasPrefix(text, "\uFEFF") {
		bom, text = "\uFEFF", text[len("\uFEFF"):]
	}
	lines := strings.SplitAfter(text, "\n")
	start := slices.IndexFunc(lines, func(line string) bool {
		header, _, _ := strings.Cut(line, "#")
		return strings.TrimSpace(header) == "[docbank]"
	})
	if start < 0 {
		return bom + text, false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), "[") {
			end = i
			break
		}
	}
	for end-1 > start {
		if trimmed := strings.TrimSpace(lines[end-1]); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			break
		}
		end--
	}
	cut := strings.Join(lines[:start], "") + strings.Join(lines[end:], "")
	var before, after map[string]any
	if _, err := toml.Decode(text, &before); err != nil {
		return bom + text, false
	}
	if _, err := toml.Decode(cut, &after); err != nil {
		return bom + text, false
	}
	if table, ok := before["docbank"].(map[string]any); ok {
		for key, value := range table {
			if _, sub := value.(map[string]any); !sub {
				delete(table, key)
			}
		}
		if len(table) == 0 {
			delete(before, "docbank")
		}
	}
	if !reflect.DeepEqual(before, after) {
		return bom + text, false
	}
	return bom + cut, true
}
