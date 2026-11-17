package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/filelock"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/whatsnew"
)

// whatsNewFile keeps, in the state folder, the last version whose notes a Pane on this machine showed (or would have, outside
// Tern), so each version's notes open once per machine.
const whatsNewFile = "whats-new.json"

type whatsNewState struct {
	Shown string `json:"shown"`
}

// showWhatsNew opens the notes of cmd.Version beside the Pane when they are due: the first Pane of a new version on this machine
// writes them to a Markdown file in the cache folder and opens it with `tern open --split right`, then gives the focus back to the
// Pane's block (cmd.PaneBlock), as the split takes it and keys typed into a Pane just opened belong to the Pane. It works under a
// lock, and a Pane that finds the lock taken leaves the notes to the Pane holding it. The version counts as shown once the notes
// are written, whether or not they open, so a Tern that cannot open them is not asked again by every Pane; outside Tern nothing
// opens and the version counts as shown. A dev build, which has no section, records nothing.
func (g *Gateway) showWhatsNew(ctx context.Context, cmd model.ShowWhatsNew) model.WhatsNewShown {
	dirs, _ := g.workerDirs()
	if dirs.State == "" || os.MkdirAll(dirs.State, 0o700) != nil {
		return model.WhatsNewShown{}
	}
	path := filepath.Join(dirs.State, whatsNewFile)
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return model.WhatsNewShown{}
	}
	defer lock.Close()
	if filelock.TryLock(lock) != nil {
		return model.WhatsNewShown{}
	}
	defer filelock.Unlock(lock)
	var state whatsNewState
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &state)
	}
	notes, due := whatsnew.Notes(g.changelog, state.Shown, cmd.Version)
	if !due {
		return model.WhatsNewShown{}
	}
	file := ""
	if inTern() {
		if file, err = writeNotes(cmd.Version, notes); err != nil {
			return model.WhatsNewShown{Version: cmd.Version, Err: err}
		}
	}
	if raw, err := json.Marshal(whatsNewState{Shown: cmd.Version}); err == nil {
		_ = atomicfile.Write(path, raw, atomicfile.Options{Mode: 0o600})
	}
	if file == "" {
		return model.WhatsNewShown{}
	}
	if err := g.ternOpen(ctx, "--split", "right", file); err != nil {
		return model.WhatsNewShown{Version: cmd.Version, Err: err}
	}
	if cmd.PaneBlock != "" {
		_, _ = g.ternOutput(ctx, ternTimeout, []string{"focus", cmd.PaneBlock}) // the notes are open; a focus left on them is no failure
	}
	return model.WhatsNewShown{}
}

// writeNotes puts notes at <cache>/tern-kata/whats-new/tern-kata-<version>.md, where Tern draws them as Markdown.
func writeNotes(version, notes string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	file := filepath.Join(base, "tern-kata", "whats-new", "tern-kata-"+version+".md")
	return file, atomicfile.Write(file, []byte(notes), atomicfile.Options{Mode: 0o600, DirMode: 0o700})
}
