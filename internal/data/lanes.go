package data

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// lanesFile is tern-kata's Lane lists: one table per Project UID, named lanes.toml in tern-kata's folder of the user config dir.
//
//	["01JABC..."]
//	name = "work"
//	labels = ["needs-triage", "in-review"]
//	done_days = 14
//	done_cards = 20
type lanesFile map[string]projectLanes

type projectLanes struct {
	Name      string   `toml:"name"`
	Labels    []string `toml:"labels,omitempty"`
	DoneDays  int      `toml:"done_days,omitzero"`
	DoneCards int      `toml:"done_cards,omitzero"`
}

func lanesPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tern-kata", "lanes.toml"), nil
}

// readLanes reads the whole file; a missing file is an empty one.
func readLanes(path string) (lanesFile, error) {
	var file lanesFile
	if _, err := toml.DecodeFile(path, &file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

// loadLanes is the Project's saved Lane list; labels are normalised and repeated or empty ones dropped. No list means none saved.
func loadLanes(cmd model.LoadLanes) model.LanesLoaded {
	path, err := lanesPath()
	if err == nil {
		var file lanesFile
		if file, err = readLanes(path); err == nil {
			p := file[cmd.ProjectUID]
			return model.LanesLoaded{ProjectUID: cmd.ProjectUID, Lanes: cleanLanes(p.Labels), DoneDays: max(p.DoneDays, 0), DoneCards: max(p.DoneCards, 0)}
		}
	}
	return model.LanesLoaded{ProjectUID: cmd.ProjectUID, Err: err}
}

func cleanLanes(labels []string) []string {
	var lanes []string
	for _, text := range labels {
		if label, err := model.CheckLabel(text); err == nil && !slices.Contains(lanes, label) {
			lanes = append(lanes, label)
		}
	}
	return lanes
}

// saveLanes writes the Project's list into the file, keeping every other Project's table. It refuses a file it cannot read, and
// renames a temporary file into place so a failed write leaves the old file. shortcut: comments in a hand-edited file are
// lost on save, and two Panes saving at once race; upgrade if either bites.
func saveLanes(cmd model.SaveLanes) model.LanesSaved {
	return model.LanesSaved{ProjectUID: cmd.ProjectUID, Lanes: cmd.Lanes, DoneDays: cmd.DoneDays, DoneCards: cmd.DoneCards, Err: writeLanes(cmd)}
}

func writeLanes(cmd model.SaveLanes) error {
	path, err := lanesPath()
	if err != nil {
		return err
	}
	file, err := readLanes(path)
	if err != nil {
		return err
	}
	if file == nil {
		file = lanesFile{}
	}
	if len(cmd.Lanes) == 0 && cmd.DoneDays == 0 && cmd.DoneCards == 0 {
		// All default: the Project's table goes, so a later change of the default reaches it.
		delete(file, cmd.ProjectUID)
	} else {
		file[cmd.ProjectUID] = projectLanes{Name: cmd.ProjectName, Labels: cmd.Lanes, DoneDays: cmd.DoneDays, DoneCards: cmd.DoneCards}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "lanes-*.toml")
	if err != nil {
		return err
	}
	err = toml.NewEncoder(tmp).Encode(file)
	if err = errors.Join(err, tmp.Close()); err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
