package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// ulid is the shape of a kata Issue UID; any other ref is a short id, optionally behind "<project>#".
var ulid = regexp.MustCompile(`(?i)^[0-9A-HJKMNP-TV-Z]{26}$`)

// attachTarget is an Issue to attach to or detach from, read once.
type attachTarget struct {
	issue   g.ShowIssueOut
	binding model.Project
	// raw is the Issue's metadata as the daemon wrote it: the generated client decodes numbers to float64, which would change a
	// literal such as 1.0 that kata compares byte for byte in a guard.
	raw map[string]json.RawMessage
}

// rawMetadata is the metadata of an Issue response body with every value left as its JSON text.
func rawMetadata(body []byte) (map[string]json.RawMessage, error) {
	var shown struct {
		Issue struct {
			Metadata map[string]json.RawMessage `json:"metadata"`
		} `json:"issue"`
	}
	return shown.Issue.Metadata, json.Unmarshal(body, &shown)
}

// resolveAttachTarget is the step Attach and Detach share: the Issue ref names, refused when it is outside the workspace's Binding.
func (c *Client) resolveAttachTarget(ctx context.Context, workspace, ref string) (attachTarget, error) {
	target, err := c.readAttachTarget(ctx, workspace, ref)
	if err != nil {
		return attachTarget{}, err
	}
	return target, c.requireBinding(ctx, target)
}

// readAttachTarget reads the Issue ref names, as kata show takes it, and the Project the workspace is bound to.
func (c *Client) readAttachTarget(ctx context.Context, workspace, ref string) (attachTarget, error) {
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
		if target.raw, err = rawMetadata(response.Body); err != nil {
			return attachTarget{}, err
		}
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
	if target.raw, err = rawMetadata(response.Body); err != nil {
		return attachTarget{}, err
	}
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

// patchAttachment sets (pointer non-nil) or clears the tern-kata.attachment.<name> key. A set applies only while the key is absent; a
// clear only while the key still holds the JSON text clearIf.
func (c *Client) patchAttachment(ctx context.Context, t attachTarget, actor, name string, pointer map[string]any, clearIf string) error {
	key := model.AttachmentKey(name)
	if pointer != nil {
		_, err := c.patchMetadataKey(ctx, t.issue.ProjectID, t.issue.UID, actor, key, pointer, nil)
		return err
	}
	_, err := c.patchMetadataKey(ctx, t.issue.ProjectID, t.issue.UID, actor, key, nil, &clearIf)
	return err
}

// patchMetadataKey writes one metadata key of an Issue, a null value clearing it, guarded on the key still holding the JSON
// text ifValue, or on its being absent when ifValue is nil.
func (c *Client) patchMetadataKey(ctx context.Context, projectID int64, uid, actor, key string, value any, ifValue *string) (issueWrite, error) {
	body := g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{key: value}}
	either := runtime.NewEitherFromB[g.MetadataPatchGuard_OneOf_0, g.MetadataPatchGuard_OneOf_1](g.MetadataPatchGuard_OneOf_1{Key: key, IfAbsent: g.True})
	if ifValue != nil {
		either = runtime.NewEitherFromA[g.MetadataPatchGuard_OneOf_0, g.MetadataPatchGuard_OneOf_1](g.MetadataPatchGuard_OneOf_0{Key: key, IfValue: *ifValue})
	}
	body.Guard = &g.MetadataPatchGuard{MetadataPatchGuard_OneOf: &g.MetadataPatchGuard_OneOf{Either: either}}
	r, err := c.PatchIssueMetadataWithResponse(ctx, &g.PatchIssueMetadataRequestOptions{
		PathParams: &g.PatchIssueMetadataPath{ProjectID: projectID, Ref: uid}, Body: &body,
	})
	if err != nil {
		return issueWrite{}, err
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
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
	source, err := attachSource(cmd.Workspace, cmd.Path)
	if err != nil {
		return model.FileAttached{}, err
	}
	root, err := PrimaryCheckoutRoot(ctx, cmd.Workspace)
	if err != nil {
		return model.FileAttached{}, err
	}
	store := attachmentStore{root}
	var taken []string
	for key := range target.issue.Metadata {
		if name, ok := strings.CutPrefix(key, model.AttachmentKey("")); ok {
			taken = append(taken, name)
		}
	}
	for {
		name, stored, err := store.put(ctx, target.issue.UID, source, filepath.Base(source), taken)
		if err != nil {
			return model.FileAttached{}, err
		}
		err = c.patchAttachment(ctx, target, cmd.Actor, name, model.Attachment{Host: host, Path: stored}.Pointer(), "")
		if err == nil {
			return model.FileAttached{Ref: cmd.Ref, Name: name, Path: stored}, nil
		}
		_, _ = store.remove(target.issue.UID, stored)
		// Another attach took the key since the Issue was read: ask the store for the next name.
		if daemonCode(err) != "metadata_guard_failed" {
			return model.FileAttached{}, daemonError(err)
		}
		taken = append(taken, name)
	}
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

// DetachFile is the one shared Detach write. It clears the pointer first, so the worst partial failure is a leftover file, then
// deletes the copy if it is on this machine, still there and inside this Issue's folder of the Attachment store. A key that
// holds no readable pointer is cleared too, touching no file. The key is cleared only while it still holds the value read, so a
// pointer written meanwhile survives. Once the key is cleared, a later failure still returns the report's facts with its error.
func (c *Client) DetachFile(ctx context.Context, host string, cmd model.DetachFile) (model.FileDetached, error) {
	target, err := c.resolveAttachTarget(ctx, cmd.Workspace, cmd.Ref)
	if err != nil {
		return model.FileDetached{}, err
	}
	_, exists := target.issue.Metadata[model.AttachmentKey(cmd.Name)]
	if !exists {
		return model.FileDetached{}, model.NoAttachmentError(target.issue.ShortID, cmd.Name)
	}
	var pointer *model.Attachment
	for _, a := range model.Attachments(target.issue.Metadata) {
		if a.Name == cmd.Name {
			pointer = &a
		}
	}
	read := string(target.raw[model.AttachmentKey(cmd.Name)])
	if err := c.patchAttachment(ctx, target, cmd.Actor, cmd.Name, nil, read); err != nil {
		if daemonCode(err) != "metadata_guard_failed" {
			return model.FileDetached{}, daemonError(err)
		}
		// The guard fails for a key that changed and for one that is gone; the Issue says which.
		now, err := c.resolveAttachTarget(ctx, cmd.Workspace, cmd.Ref)
		if err != nil {
			return model.FileDetached{}, err
		}
		if _, still := now.issue.Metadata[model.AttachmentKey(cmd.Name)]; !still {
			return model.FileDetached{}, model.NoAttachmentError(target.issue.ShortID, cmd.Name)
		}
		return model.FileDetached{}, model.AttachmentChangedError(target.issue.ShortID, cmd.Name)
	}
	result := model.FileDetached{Ref: cmd.Ref, Name: cmd.Name, Fate: model.FileLeft, Cleared: true}
	if pointer == nil {
		return result, nil
	}
	result.Host, result.Path = pointer.Host, pointer.Path
	if !pointer.OnHost(host) {
		return result, nil
	}
	root, err := PrimaryCheckoutRoot(ctx, cmd.Workspace)
	if err != nil {
		return result, err
	}
	result.Fate, err = attachmentStore{root}.remove(target.issue.UID, pointer.Path)
	return result, err
}
