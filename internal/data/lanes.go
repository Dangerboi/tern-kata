package data

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"
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
	Labels    []string `toml:"labels"`
	DoneDays  int      `toml:"done_days"`
	DoneCards int      `toml:"done_cards"`
}

// configFolder is tern-kata's config folder from the gateway's folder hook, which finds the config folder even when the state
// folder cannot be found.
func (g *Gateway) configFolder() (string, error) {
	dirs, err := g.workerDirs()
	if dirs.Config == "" {
		if err == nil {
			err = errors.New("tern-kata's config folder is not known")
		}
		return "", err
	}
	return dirs.Config, nil
}

// readLanes reads the whole file; a missing file is an empty one.
func readLanes(path string) (lanesFile, error) {
	var file lanesFile
	if _, err := toml.DecodeFile(path, &file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return file, nil
}

// updateTables applies change to the file's untyped tables, so a save can write back keys tern-kata doesn't know, and writes
// the result only while the file is still the text that was read. A file another Pane changed meanwhile is read again and
// change applied on top of it, so no save loses another's; a file that keeps changing is refused, saying so. A missing file
// is an empty one and a file that cannot be read or parsed is never written.
func updateTables(path string, change func(file map[string]any)) error {
	return guardedfile.Update(path, 0o755, func(token guardedfile.Token) ([]byte, error) {
		file := map[string]any{}
		if _, err := toml.Decode(token.Text(), &file); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		change(file)
		var buf bytes.Buffer
		err := toml.NewEncoder(&buf).Encode(file)
		return buf.Bytes(), err
	})
}

// loadLanes is the Project's Lane list in its read order: the Project's shared list (tern.lanes) when it holds a valid one, else
// the file's, else none, the default. File labels are normalised and repeated or empty ones dropped. The file is read whichever
// list is used, so its error refuses a save to it. A daemon that cannot be read leaves the file's list with SharedErr saying why.
func (g *Gateway) loadLanes(ctx context.Context, cmd model.LoadLanes) model.LanesLoaded {
	loaded, err := callAs(g, ctx, cmd.Workspace, func(ctx context.Context, c *Client) (model.LanesLoaded, error) {
		return c.SharedLanes(ctx, cmd.ProjectUID)
	})
	loaded.ProjectUID, loaded.Request = cmd.ProjectUID, cmd.Request
	if err != nil {
		loaded.SharedErr = fmt.Errorf("the Project could not be read: %w", err)
	}
	folder, err := g.configFolder()
	var file lanesFile
	if err == nil {
		file, err = readLanes(filepath.Join(folder, lanesFileName))
	}
	loaded.Err = err
	if p := file[cmd.ProjectUID]; err == nil && !loaded.Shared {
		loaded.Lanes, loaded.DoneDays, loaded.DoneCards = cleanLanes(p.Labels), max(p.DoneDays, 0), max(p.DoneCards, 0)
	}
	return loaded
}

// SharedLanes reads the Project with uid as kata lists it, archived Projects included: its id and revision, and its shared
// Lane list when tern.lanes holds a valid one. A value that is not a Lane list is left unused with SharedErr saying why. A
// Project kata does not list shares nothing.
func (c *Client) SharedLanes(ctx context.Context, uid string) (model.LanesLoaded, error) {
	include := "archived"
	response, err := c.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{Query: &g.ListProjectsQuery{Include: &include}})
	if err != nil {
		return model.LanesLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.LanesLoaded{}, errors.New("kata daemon returned no Projects")
	}
	for _, p := range response.JSON200.Projects {
		if p.UID != uid {
			continue
		}
		loaded := model.LanesLoaded{ProjectUID: uid, ProjectID: p.ID, Revision: p.Revision, Archived: !p.Active}
		value, held := p.Metadata[model.SharedLanesKey]
		if !held || value == nil {
			return loaded, nil
		}
		lanes, days, cards, err := model.SharedLanes(value)
		if err != nil {
			loaded.SharedErr = fmt.Errorf("%s is not a Lane list: %w", model.SharedLanesKey, err)
			return loaded, nil
		}
		loaded.Lanes, loaded.DoneDays, loaded.DoneCards, loaded.Shared = lanes, days, cards, true
		return loaded, nil
	}
	return model.LanesLoaded{ProjectUID: uid}, nil
}

// ShareLanes writes the Project's tern.lanes, or clears it to stop sharing, as one metadata patch of that key alone guarded on
// the revision the Lanes card opened with. A stale revision reads the Project again and answers with its list as it stands;
// any other refusal is kata's reason.
func (c *Client) ShareLanes(ctx context.Context, cmd model.ShareLanes) (model.LanesShared, error) {
	var value any
	if !cmd.Stop {
		value = model.SharedLanesValue(cmd.Lanes, cmd.DoneDays, cmd.DoneCards)
	}
	tag := etag(cmd.Revision)
	response, err := c.PatchProjectMetadataWithResponse(ctx, &g.PatchProjectMetadataRequestOptions{
		PathParams: &g.PatchProjectMetadataPath{ProjectID: cmd.ProjectID},
		Header:     &g.PatchProjectMetadataHeaders{IfMatch: &tag},
		Body:       &g.PatchProjectMetadataBody{Actor: cmd.Actor, Patch: map[string]any{model.SharedLanesKey: value}},
	})
	switch {
	case client.StatusCode(err) == http.StatusPreconditionFailed:
		current, err := c.SharedLanes(ctx, cmd.ProjectUID)
		if err != nil {
			return model.LanesShared{}, fmt.Errorf("the Project changed since this card opened, and reading it again failed: %w", err)
		}
		return model.LanesShared{Share: cmd, Conflict: true, Current: current}, nil
	case err != nil:
		return model.LanesShared{}, daemonError(err)
	case response.JSON200 == nil:
		return model.LanesShared{}, errors.New("kata daemon returned no Project")
	}
	return model.LanesShared{Share: cmd, Revision: response.JSON200.Project.Revision}, nil
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

// saveLanes writes the Project's list into the file, keeping every other table and every key it doesn't know. It refuses a file
// it cannot read, and renames a temporary file into place so a failed write leaves the old file. A save from another Pane in
// between is read again and this one applied on top. shortcut: comments in a hand-edited file are lost on save.
func (g *Gateway) saveLanes(cmd model.SaveLanes) model.LanesSaved {
	return model.LanesSaved{ProjectUID: cmd.ProjectUID, Lanes: cmd.Lanes, DoneDays: cmd.DoneDays, DoneCards: cmd.DoneCards, Err: g.writeLanes(cmd)}
}

func (g *Gateway) writeLanes(cmd model.SaveLanes) error {
	folder, err := g.configFolder()
	if err != nil {
		return err
	}
	return updateTables(filepath.Join(folder, lanesFileName), func(file map[string]any) {
		table, _ := file[cmd.ProjectUID].(map[string]any)
		if table == nil {
			table = map[string]any{}
		}
		for _, key := range []string{"name", "labels", "done_days", "done_cards"} {
			delete(table, key)
		}
		if len(cmd.Lanes) == 0 && cmd.DoneDays == 0 && cmd.DoneCards == 0 && len(table) == 0 {
			// All default and nothing else in it: the Project's table goes, so a later change of the default reaches it.
			delete(file, cmd.ProjectUID)
			return
		}
		if len(cmd.Lanes) > 0 || cmd.DoneDays != 0 || cmd.DoneCards != 0 {
			table["name"] = cmd.ProjectName
		}
		if len(cmd.Lanes) > 0 {
			table["labels"] = cmd.Lanes
		}
		if cmd.DoneDays != 0 {
			table["done_days"] = cmd.DoneDays
		}
		if cmd.DoneCards != 0 {
			table["done_cards"] = cmd.DoneCards
		}
		file[cmd.ProjectUID] = table
	})
}
