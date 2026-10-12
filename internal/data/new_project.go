package data

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// CreateProject is `kata projects create`: the daemon creates the Project and
// no workspace file changes. A name already taken is an error, so the form can
// keep the input, where BindProject welcomes the existing Project.
func (c *Client) CreateProject(ctx context.Context, cmd model.CreateProject) (model.Project, error) {
	project, created, err := c.initProject(ctx, g.InitProjectBody{Name: &cmd.Name, Actor: &cmd.Actor})
	if err == nil && !created {
		return model.Project{}, fmt.Errorf("project %q already exists", cmd.Name)
	}
	return project, err
}

// BindProject is `kata init --project`: the daemon creates the Project, or
// returns the active one of that name, and attaches this workspace's alias; then
// .kata.toml binds the directory, written here because a remote daemon cannot
// reach it. Like kata, it refuses a .kata.toml that names another Project, and
// asks the daemon only after that check so a refusal leaves nothing behind. A
// failed write can be retried: the daemon's answer is the same the second time.
func (c *Client) BindProject(ctx context.Context, cmd model.BindProject) (model.Project, error) {
	start, err := filepath.Abs(cmd.Workspace)
	if err != nil {
		return model.Project{}, fmt.Errorf("abs workspace %s: %w", cmd.Workspace, err)
	}
	workspaceRoot, gitRoot, err := discoverProjectRoots(start)
	if err != nil {
		return model.Project{}, err
	}
	bound := ""
	if workspaceRoot != "" {
		if bound, err = readBinding(workspaceRoot); err != nil {
			return model.Project{}, err
		}
		if bound != cmd.Name {
			return model.Project{}, fmt.Errorf(".kata.toml in %s binds this directory to Project %q; use that name, or edit the file", workspaceRoot, bound)
		}
	}
	anchor := workspaceRoot
	if workspaceRoot == "" && gitRoot == "" {
		anchor = start // kata anchors the local alias at the directory when nothing else does
	}
	alias, err := projectAlias(ctx, anchor, gitRoot)
	if err != nil {
		return model.Project{}, err
	}
	project, _, err := c.initProject(ctx, g.InitProjectBody{Name: &cmd.Name, Actor: &cmd.Actor, Alias: alias})
	if err != nil {
		return model.Project{}, err
	}
	dest := cmp.Or(workspaceRoot, gitRoot, start)
	if bound != project.Name {
		body := fmt.Sprintf("version = 1\n\n[project]\nname = %q\n", project.Name)
		if err := os.WriteFile(filepath.Join(dest, ".kata.toml"), []byte(body), 0o644); err != nil {
			return model.Project{}, fmt.Errorf("write .kata.toml: %w", err)
		}
	}
	// kata init only warns when this fails, and the binding is already in place.
	_ = ignoreLocalConfig(dest)
	return project, nil
}

func (c *Client) initProject(ctx context.Context, body g.InitProjectBody) (model.Project, bool, error) {
	response, err := c.InitProjectWithResponse(ctx, &g.InitProjectRequestOptions{Body: &body})
	if err != nil {
		if response != nil {
			err = ReadError(response.Body, err)
		}
		return model.Project{}, false, err
	}
	if response.JSON200 == nil {
		return model.Project{}, false, errors.New("kata daemon returned no Project")
	}
	return response.JSON200.Project, response.JSON200.Created, nil
}

// ignoreLocalConfig lists .kata.local.toml, a developer's per-machine overrides,
// in dir/.gitignore as kata init does, unless a line already names it.
func ignoreLocalConfig(dir string) error {
	const entry = ".kata.local.toml"
	path := filepath.Join(dir, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if slices.ContainsFunc(strings.Split(string(existing), "\n"), func(line string) bool { return strings.TrimSpace(line) == entry }) {
		return nil
	}
	line := entry + "\n"
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		line = "\n" + line
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = file.WriteString(line)
	return errors.Join(err, file.Close())
}
