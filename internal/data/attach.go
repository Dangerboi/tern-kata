package data

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// errNoHostName refuses an attach that would write a pointer with an empty host.
var errNoHostName = errors.New("This machine's host name can't be read, so the file can't be attached.")

// HostName is this machine's host name, or empty when the OS can't say; the program reads it once and hands it to the model and the Gateway.
func HostName() string {
	host, _ := os.Hostname()
	return host
}

// attachmentStoreName is the Attachment store's folder in the primary checkout.
const attachmentStoreName = ".kata-attachments"

// ulid is the shape of a kata Issue UID; any other ref is a short id, optionally behind "<project>#".
var ulid = regexp.MustCompile(`(?i)^[0-9A-HJKMNP-TV-Z]{26}$`)

// PrimaryCheckoutRoot is the top level of the main worktree of the repository holding workspace, the parent of git's common
// directory, so every linked worktree shares one answer. Outside git it is the directory holding the workspace's .kata.toml.
func PrimaryCheckoutRoot(ctx context.Context, workspace string) (string, error) {
	workspaceRoot, gitRoot, err := discoverProjectRoots(workspace)
	if err != nil {
		return "", err
	}
	if gitRoot == "" {
		if workspaceRoot == "" {
			return "", fmt.Errorf("%s is neither in a git repository nor under a .kata.toml", workspace)
		}
		return workspaceRoot, nil
	}
	raw, err := projectGit.Output(ctx, gitRoot, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir: %w", err)
	}
	// A submodule's common directory sits inside its parent's .git, so its own top level is the root.
	super, err := projectGit.Output(ctx, gitRoot, "rev-parse", "--show-superproject-working-tree")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-superproject-working-tree: %w", err)
	}
	if strings.TrimSpace(string(super)) != "" {
		return gitRoot, nil
	}
	return filepath.Dir(filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(raw))))), nil
}

// attachTarget is an Issue to attach to or detach from, read once.
type attachTarget struct {
	issue   g.ShowIssueOut
	binding model.Project
}

// resolveAttachTarget reads the Issue ref names, as kata show takes it, and the Project the workspace is bound to.
func (c *Client) resolveAttachTarget(ctx context.Context, workspace, ref string) (attachTarget, error) {
	loaded, err := c.ReadProject(ctx, workspace)
	if err != nil {
		return attachTarget{}, err
	}
	target := attachTarget{binding: loaded.Project}
	ref = strings.TrimSpace(ref)
	if ulid.MatchString(ref) {
		response, err := c.ShowIssueByUIDWithResponse(ctx, &g.ShowIssueByUIDRequestOptions{PathParams: &g.ShowIssueByUIDPath{UID: ref}})
		if err != nil {
			return attachTarget{}, daemonError(err)
		}
		if response.JSON200 == nil {
			return attachTarget{}, errors.New("kata daemon returned no Issue")
		}
		target.issue = response.JSON200.Issue
		return target, nil
	}
	projectName, shortID, qualified := strings.Cut(ref, "#")
	if !qualified {
		projectName, shortID = "", ref
	}
	projectID := target.binding.ID
	if projectName != "" {
		projects, err := c.projectNames(ctx)
		if err != nil {
			return attachTarget{}, err
		}
		projectID = 0
		for id, name := range projects {
			if name == projectName {
				projectID = id
			}
		}
		if projectID == 0 {
			return attachTarget{}, fmt.Errorf("kata Project %q not found", projectName)
		}
	} else if target.binding.UID == "" {
		return attachTarget{}, fmt.Errorf("This workspace is not bound to a kata Project, so %q does not name an Issue; give <project>#<short id> or the Issue's ULID.", ref)
	}
	response, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: shortID}})
	if err != nil {
		return attachTarget{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return attachTarget{}, errors.New("kata daemon returned no Issue")
	}
	target.issue = response.JSON200.Issue
	return target, nil
}

// projectNames is the name of every Project by id.
func (c *Client) projectNames(ctx context.Context) (map[int64]string, error) {
	response, err := c.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{})
	if err != nil {
		return nil, daemonError(err)
	}
	if response.JSON200 == nil {
		return nil, errors.New("kata daemon returned no Projects")
	}
	names := map[int64]string{}
	for _, p := range response.JSON200.Projects {
		names[p.ID] = p.Name
	}
	return names, nil
}

// requireBinding refuses an Issue whose Project is not the workspace's Binding, naming the Project to attach from.
func (c *Client) requireBinding(ctx context.Context, t attachTarget) error {
	if model.CanAttach(t.binding, t.issue.ProjectID) {
		return nil
	}
	names, err := c.projectNames(ctx)
	if err != nil {
		return err
	}
	return model.AttachRefusal(t.binding, t.issue.ProjectID, t.issue.ShortID, names[t.issue.ProjectID])
}

// patchAttachment sets (pointer non-nil) or clears the attachment.<name> key. A set is guarded so it applies only while the key is absent.
func (c *Client) patchAttachment(ctx context.Context, t attachTarget, actor, name string, pointer map[string]any) error {
	key := model.AttachmentKey(name)
	body := g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{key: nil}}
	if pointer != nil {
		body.Patch[key] = pointer
		body.Guard = &g.MetadataPatchGuard{MetadataPatchGuard_OneOf: &g.MetadataPatchGuard_OneOf{
			Either: runtime.NewEitherFromB[g.MetadataPatchGuard_OneOf_0, g.MetadataPatchGuard_OneOf_1](g.MetadataPatchGuard_OneOf_1{Key: key, IfAbsent: g.True}),
		}}
	}
	_, err := c.PatchIssueMetadataWithResponse(ctx, &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: t.issue.ProjectID, Ref: t.issue.UID}, Body: &body,
	})
	return err
}

// AttachFile is the one shared Attach write. The copy is made before the pointer so a pointer never names a missing file, and
// deleted again if the pointer is not written, so the store only holds files an Issue points to.
func (c *Client) AttachFile(ctx context.Context, host string, cmd model.AttachFile) (model.FileAttached, error) {
	if host == "" {
		return model.FileAttached{}, errNoHostName
	}
	target, err := c.resolveAttachTarget(ctx, cmd.Workspace, cmd.Ref)
	if err != nil {
		return model.FileAttached{}, err
	}
	if err := c.requireBinding(ctx, target); err != nil {
		return model.FileAttached{}, err
	}
	source, err := attachSource(cmd.Workspace, cmd.Path)
	if err != nil {
		return model.FileAttached{}, err
	}
	root, err := PrimaryCheckoutRoot(ctx, cmd.Workspace)
	if err != nil {
		return model.FileAttached{}, err
	}
	store := filepath.Join(root, attachmentStoreName)
	dir := filepath.Join(store, target.issue.UID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.FileAttached{}, err
	}
	// The store keeps itself out of git; O_EXCL leaves an existing file, and the repo's own ignore files alone.
	switch f, err := os.OpenFile(filepath.Join(store, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644); {
	case err == nil:
		_, werr := f.WriteString("*\n")
		if err := errors.Join(werr, f.Close()); err != nil {
			return model.FileAttached{}, err
		}
	case !errors.Is(err, fs.ErrExist):
		return model.FileAttached{}, err
	}
	base := filepath.Base(source)
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return model.FileAttached{}, err
		}
		name := numberedName(base, n)
		if _, taken := target.issue.Metadata[model.AttachmentKey(name)]; taken {
			continue
		}
		stored := filepath.Join(dir, name)
		if err := copyNew(source, stored); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return model.FileAttached{}, err
		}
		err := c.patchAttachment(ctx, target, cmd.Actor, name, model.Attachment{Host: host, Path: stored}.Pointer())
		if err == nil {
			return model.FileAttached{Ref: cmd.Ref, Name: name, Path: stored}, nil
		}
		_ = os.Remove(stored)
		// Another attach took the key since the Issue was read: try the next name.
		if daemonCode(err) != "metadata_guard_failed" {
			return model.FileAttached{}, daemonError(err)
		}
	}
}

// numberedName is name for n == 1, else <stem>-n<ext>.
func numberedName(name string, n int) string {
	if n == 1 {
		return name
	}
	ext := filepath.Ext(name)
	if ext == name { // a dotfile has no stem
		ext = ""
	}
	return fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
}

// attachSource is the regular file input names: whitespace and one pair of wrapping quotes stripped, a leading ~ expanded, a
// relative path taken from the workspace, symlinks followed.
func attachSource(workspace, input string) (string, error) {
	path := strings.TrimSpace(input)
	if len(path) >= 2 && (path[0] == '"' || path[0] == '\'') && path[len(path)-1] == path[0] {
		path = path[1 : len(path)-1]
	}
	if path == "" {
		return "", errors.New("No file given.")
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("%s does not exist.", path)
	case err != nil:
		return "", err
	case info.IsDir():
		return "", fmt.Errorf("%s is a directory; attach a file.", path)
	case !info.Mode().IsRegular():
		return "", fmt.Errorf("%s is not a regular file.", path)
	}
	return path, nil
}

// copyNew copies source to a new file at dst, failing with fs.ErrExist if dst exists, and leaves nothing at dst on failure.
func copyNew(source, dst string) (err error) {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(dst)
	}
	return err
}

// DetachFile is the one shared Detach write. It clears the pointer first, so the worst partial failure is a leftover file, then
// deletes the copy if it is on this machine, still there and inside this Issue's folder of the Attachment store. A key that
// holds no readable pointer is cleared too, touching no file.
func (c *Client) DetachFile(ctx context.Context, host string, cmd model.DetachFile) (model.FileDetached, error) {
	target, err := c.resolveAttachTarget(ctx, cmd.Workspace, cmd.Ref)
	if err != nil {
		return model.FileDetached{}, err
	}
	if err := c.requireBinding(ctx, target); err != nil {
		return model.FileDetached{}, err
	}
	if _, exists := target.issue.Metadata[model.AttachmentKey(cmd.Name)]; !exists {
		return model.FileDetached{}, model.NoAttachmentError(target.issue.ShortID, cmd.Name)
	}
	var pointer *model.Attachment
	for _, a := range model.Attachments(target.issue.Metadata) {
		if a.Name == cmd.Name {
			pointer = &a
		}
	}
	if err := c.patchAttachment(ctx, target, cmd.Actor, cmd.Name, nil); err != nil {
		return model.FileDetached{}, daemonError(err)
	}
	result := model.FileDetached{Ref: cmd.Ref, Name: cmd.Name, Fate: model.FileLeft}
	if pointer == nil {
		return result, nil
	}
	result.Host, result.Path = pointer.Host, pointer.Path
	if !pointer.OnHost(host) {
		return result, nil
	}
	if _, err := os.Lstat(pointer.Path); errors.Is(err, fs.ErrNotExist) {
		result.Fate = model.FileGone
		return result, nil
	}
	root, err := PrimaryCheckoutRoot(ctx, cmd.Workspace)
	if err != nil {
		return result, err
	}
	// A pointer is data any writer can set; only delete what this Issue's folder of the store holds.
	rel, err := filepath.Rel(filepath.Join(root, attachmentStoreName, target.issue.UID), pointer.Path)
	if err != nil || !filepath.IsAbs(pointer.Path) || rel == "." || !filepath.IsLocal(rel) {
		return result, nil
	}
	switch err := os.Remove(pointer.Path); {
	case err == nil:
		result.Fate = model.FileDeleted
	case errors.Is(err, fs.ErrNotExist):
		result.Fate = model.FileGone
	default:
		return result, err
	}
	return result, nil
}
