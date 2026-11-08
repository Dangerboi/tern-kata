package data

import (
	"path/filepath"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

const seenFileName = "seen.toml"

// seenFile is what this machine has looked at on the Runs Page: one table per Project UID, named seen.toml beside filters.toml.
// It is per machine, as kata has nowhere to keep it.
//
//	["01JABC..."]
//	seen = 2026-11-07T08:00:00Z
//	timeline = true
//
// seen is when the Runs ended since last looked at were marked seen; timeline says the Runs Page is in its "since you last
// looked" mode. Either may be missing.
type seenFile map[string]projectSeen

type projectSeen struct {
	Seen     time.Time `toml:"seen"`
	Timeline bool      `toml:"timeline"`
}

// The keys of a Project's table that a save may set; any other key is kept.
const (
	seenKey     = "seen"
	timelineKey = "timeline"
)

func (g *Gateway) seenPath() (string, error) {
	folder, err := g.configFolder()
	if err != nil {
		return "", err
	}
	return filepath.Join(folder, seenFileName), nil
}

// readSeen is the Project's table in the file at path. No file or no table is the zero value; an unreadable file is an error
// naming it.
func readSeen(path, projectUID string) (model.SeenLoaded, error) {
	file, err := readTables[seenFile](path)
	if err != nil {
		return model.SeenLoaded{}, err
	}
	p := file[projectUID]
	return model.SeenLoaded{ProjectUID: projectUID, Seen: p.Seen, Timeline: p.Timeline}, nil
}

// writeSeen applies cmd to the Project's table, keeping every other table and every key of this one it doesn't set. It refuses
// a file it cannot read.
func writeSeen(path string, cmd model.SaveSeen) error {
	return updateTables(path, func(file map[string]any) {
		table, _ := file[cmd.ProjectUID].(map[string]any)
		if table == nil {
			table = map[string]any{}
		}
		if cmd.SetSeen {
			table[seenKey] = cmd.Seen.UTC()
		}
		if cmd.SetTimeline {
			if cmd.Timeline {
				table[timelineKey] = true
			} else {
				delete(table, timelineKey)
			}
		}
		if len(table) == 0 {
			delete(file, cmd.ProjectUID)
		} else {
			file[cmd.ProjectUID] = table
		}
	})
}

func (g *Gateway) loadSeen(cmd model.LoadSeen) model.SeenLoaded {
	path, err := g.seenPath()
	if err != nil {
		return model.SeenLoaded{ProjectUID: cmd.ProjectUID, Err: err}
	}
	loaded, err := readSeen(path, cmd.ProjectUID)
	loaded.ProjectUID, loaded.Err = cmd.ProjectUID, err
	return loaded
}

func (g *Gateway) saveSeen(cmd model.SaveSeen) model.SeenSaved {
	path, err := g.seenPath()
	if err == nil {
		err = writeSeen(path, cmd)
	}
	return model.SeenSaved{ProjectUID: cmd.ProjectUID, Err: err}
}
