package data

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/BurntSushi/toml"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// filtersFile is tern-kata's per-Project default filters: one table per Project UID, named filters.toml beside lanes.toml. An
// empty field is not written; Search text is never saved.
//
//	["01JABC..."]
//	name = "work"
//	status = "open"
//	owner = "me"
type filtersFile map[string]projectFilters

type projectFilters struct {
	Project      string `toml:"project"`
	Status       string `toml:"status"`
	Owner        string `toml:"owner"`
	Label        string `toml:"label"`
	Relationship string `toml:"relationship"`
}

// filterKeys are the keys of a Project's table that a save owns; any other key is kept.
var filterKeys = []string{"name", "project", "status", "owner", "label", "relationship"}

func filtersPath() (string, error) { return configPath("filters.toml") }

// loadFilters is the Project's saved default filter. No file or no table means none; an unreadable file is an error naming it.
func loadFilters(projectUID string) (model.Filters, error) {
	path, err := filtersPath()
	if err != nil {
		return model.Filters{}, err
	}
	var file filtersFile
	if _, err := toml.DecodeFile(path, &file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return model.Filters{}, fmt.Errorf("%s: %w", path, err)
	}
	p := file[projectUID]
	return model.Filters{ProjectUID: p.Project, Status: p.Status, Owner: p.Owner, Label: p.Label, Relationship: p.Relationship}, nil
}

// saveFilters writes the Project's default filter, keeping every other table and every key of this one it doesn't know. It
// refuses a file it cannot read. shortcut: comments in a hand-edited file are lost on save, and two Panes saving at once race.
func saveFilters(cmd model.SaveFilters) model.FiltersSaved {
	return model.FiltersSaved{ProjectUID: cmd.ProjectUID, Default: cmd.Default, Err: writeFilters(cmd)}
}

func writeFilters(cmd model.SaveFilters) error {
	path, err := filtersPath()
	if err != nil {
		return err
	}
	file, err := readRawTables(path)
	if err != nil {
		return err
	}
	table, _ := file[cmd.ProjectUID].(map[string]any)
	if table == nil {
		table = map[string]any{}
	}
	for _, key := range filterKeys {
		delete(table, key)
	}
	fields := map[string]string{"project": cmd.Default.ProjectUID, "status": cmd.Default.Status, "owner": cmd.Default.Owner, "label": cmd.Default.Label, "relationship": cmd.Default.Relationship}
	set := false
	for key, value := range fields {
		if value != "" {
			table[key], set = value, true
		}
	}
	if set {
		table["name"] = cmd.ProjectName
	}
	if !set && len(table) == 0 {
		delete(file, cmd.ProjectUID)
	} else {
		file[cmd.ProjectUID] = table
	}
	return writeTables(path, "filters-*.toml", file)
}
