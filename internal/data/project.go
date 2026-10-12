package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
	gitcmd "go.kenn.io/kit/git/cmd"
	gitremote "go.kenn.io/kit/git/remote"
	gitrepo "go.kenn.io/kit/git/repo"
)

func (c *Client) ReadProject(ctx context.Context, workspace string) (model.ProjectLoaded, error) {
	request, err := c.projectRequest(ctx, workspace)
	if err != nil {
		return model.ProjectLoaded{}, err
	}
	var project model.Project
	if request != nil {
		response, err := c.ResolveProjectWithResponse(ctx, &g.ResolveProjectRequestOptions{Body: request})
		if err != nil {
			if response == nil {
				return model.ProjectLoaded{}, err
			}
			var envelope g.ErrorEnvelope
			if json.Unmarshal(response.Body, &envelope) != nil || envelope.ErrorData.Code != "project_not_initialized" {
				return model.ProjectLoaded{}, ReadError(response.Body, err)
			}
			if c.remote && request.Name != nil {
				return model.ProjectLoaded{}, ReadError(response.Body, err)
			}
		} else {
			if response.JSON200 == nil {
				return model.ProjectLoaded{}, fmt.Errorf("kata daemon returned no Project")
			}
			project = response.JSON200.Project
		}
	}
	return model.ProjectLoaded{Project: project, Daemon: model.Daemon{Remote: c.remote, Version: c.version}}, nil
}

func (c *Client) projectRequest(ctx context.Context, workspace string) (*g.ResolveProjectBody, error) {
	if !c.remote {
		return &g.ResolveProjectBody{StartPath: &workspace}, nil
	}
	workspaceRoot, gitRoot, err := discoverProjectRoots(workspace)
	if err != nil {
		return nil, err
	}
	if workspaceRoot == "" && gitRoot == "" {
		return nil, nil
	}
	request := &g.ResolveProjectBody{}
	if workspaceRoot != "" {
		name, err := readBinding(workspaceRoot)
		if err != nil {
			return nil, err
		}
		request.Name = &name
	}
	alias, err := projectAlias(ctx, workspaceRoot, gitRoot)
	if err != nil {
		if request.Name == nil {
			return nil, err
		}
	} else {
		request.Alias = alias
	}
	return request, nil
}

// readBinding is the Project name the .kata.toml at workspaceRoot binds to.
func readBinding(workspaceRoot string) (string, error) {
	path := filepath.Join(workspaceRoot, ".kata.toml")
	var cfg struct {
		Version int `toml:"version"`
		Project struct {
			Name     string `toml:"name"`
			Identity string `toml:"identity"`
		} `toml:"project"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Version != 1 {
		return "", fmt.Errorf("unsupported .kata.toml version %d (expected 1)", cfg.Version)
	}
	name := strings.TrimSpace(cfg.Project.Name)
	if identity := strings.TrimSpace(cfg.Project.Identity); name == "" && identity != "" {
		name = identity[strings.LastIndexAny(identity, "/:")+1:]
	}
	if name == "" {
		return "", fmt.Errorf("project.name in %s must be non-empty", path)
	}
	for _, r := range name {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			return "", fmt.Errorf("project.name in %s contains non-printable character %U", path, r)
		}
	}
	return name, nil
}

// Kata keeps config discovery internal; match its Git boundary and symlink rules on the client.
func discoverProjectRoots(start string) (string, string, error) {
	lexical, err := filepath.Abs(start)
	if err != nil {
		return "", "", fmt.Errorf("abs workspace %s: %w", start, err)
	}
	info, err := os.Stat(lexical)
	if err != nil {
		return "", "", fmt.Errorf("stat workspace %s: %w", lexical, err)
	}
	physical, err := filepath.EvalSymlinks(lexical)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace %s: %w", lexical, err)
	}
	if !info.IsDir() {
		lexical, physical = filepath.Dir(lexical), filepath.Dir(physical)
	}
	gitRoot, err := projectAncestor(lexical, ".git", "", true)
	if err != nil {
		return "", "", err
	}
	walkRoot := lexical
	if physical != lexical {
		physicalGitRoot, err := projectAncestor(physical, ".git", "", true)
		if err != nil {
			return "", "", err
		}
		if physicalGitRoot != "" {
			resolvedGitRoot, resolveErr := filepath.EvalSymlinks(gitRoot)
			if resolveErr != nil || resolvedGitRoot != physicalGitRoot {
				walkRoot, gitRoot = physical, physicalGitRoot
			}
		}
	}
	workspaceRoot, err := projectAncestor(walkRoot, ".kata.toml", gitRoot, false)
	return workspaceRoot, gitRoot, err
}

func projectAncestor(start, entry, stop string, allowDir bool) (string, error) {
	for dir := start; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, entry)
		info, err := os.Stat(path)
		if err == nil {
			if allowDir || !info.IsDir() {
				return dir, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		if dir == stop || filepath.Dir(dir) == dir {
			return "", nil
		}
	}
}

var projectGit = gitcmd.New()

func projectAlias(ctx context.Context, workspaceRoot, gitRoot string) (*g.AliasInput, error) {
	if gitRoot == "" {
		return &g.AliasInput{Identity: "local://" + workspaceRoot, Kind: "local"}, nil
	}
	remotes, err := gitrepo.ListRemotes(ctx, gitRoot)
	if err != nil {
		return nil, err
	}
	if len(remotes) == 0 {
		return &g.AliasInput{Identity: "local://" + gitRoot, Kind: "local"}, nil
	}
	target := remotes[0]
	if slices.Contains(remotes, "origin") {
		target = "origin"
	}
	raw, err := projectGit.Output(
		ctx,
		gitRoot,
		"remote",
		"get-url",
		target,
	)
	if err != nil {
		return nil, fmt.Errorf("git remote get-url %s: %w", target, err)
	}
	remote := strings.TrimSpace(string(raw))
	identity := gitremote.RemoteHost(remote)
	if identity == "" {
		return nil, fmt.Errorf("git remote %s has no recognized host", target)
	}
	if path := gitremote.RemoteRepoPath(remote); path != "" {
		identity += "/" + path
	}
	if decoded, err := url.PathUnescape(identity); err == nil {
		identity = decoded
	}
	identity = strings.ReplaceAll(identity, " ", "-")
	return &g.AliasInput{Identity: identity, Kind: "git"}, nil
}
