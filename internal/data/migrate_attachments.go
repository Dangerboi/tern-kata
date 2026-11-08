package data

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// The attachments tern-kata kept itself before kata stored them in Docbank: a file under the primary checkout's .kata-attachments
// folder, pointed to by a tern-kata.attachment.<name> meta key (or, from 0.3, an attachment.<name> key) of {"host", "path"}. Only
// the migration still reads them.
const (
	oldAttachmentKeyPrefix  = "tern-kata.attachment."
	bareAttachmentKeyPrefix = "attachment." // the 0.3 key; kata's native references use it too, so only a {host, path} value counts
	oldStoreName            = ".kata-attachments"
)

func oldPointer(value any) (host, path string, ok bool) {
	object, isObject := value.(map[string]any)
	if !isObject {
		return "", "", false
	}
	host, hostOK := object["host"].(string)
	path, pathOK := object["path"].(string)
	return host, path, hostOK && pathOK
}

// inOldStore is whether path is a file inside the Issue's own folder of the old store at root. A pointer is data any writer can
// set, so only such a file is ever uploaded or deleted: a symlink may not stand in for the file, the Issue's folder or the store,
// or the link's target would be read instead. A file that is gone passes when its path is inside the folder, so the caller can say it is gone.
func inOldStore(root, uid, path string) bool {
	if !ulid.MatchString(uid) || !filepath.IsAbs(path) {
		return false
	}
	folder := filepath.Join(root, oldStoreName, uid)
	rel, err := filepath.Rel(folder, path)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return false
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil || info.Mode()&fs.ModeSymlink != 0 {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realFolder, err := filepath.EvalSymlinks(folder)
	if err != nil || realFolder != filepath.Join(realRoot, oldStoreName, uid) {
		return false
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err = filepath.Rel(realFolder, realPath)
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

func tidyOldStore(root string, uids []string) {
	store := filepath.Join(root, oldStoreName)
	for _, uid := range uids {
		_ = os.Remove(filepath.Join(store, uid)) // fails while the folder holds a file, which stays
	}
	entries, err := os.ReadDir(store)
	if err != nil {
		return
	}
	switch len(entries) {
	case 0:
		_ = os.Remove(store)
	case 1:
		if ignore := filepath.Join(store, ".gitignore"); entries[0].Name() == ".gitignore" {
			if text, err := os.ReadFile(ignore); err == nil && string(text) == "*\n" {
				_ = os.Remove(ignore)
				_ = os.Remove(store)
			}
		}
	}
}

// MigrateAttachments moves the old Attachments of the workspace's Project, open and closed Issues alike, to Docbank. For each
// tern-kata.attachment.<name> key whose file is on this machine in the old store it uploads the file, clears the key and
// deletes the copy, in that order, so a failure never loses the file. Everything else is left and listed with the reason.
func (c *Client) MigrateAttachments(ctx context.Context, host string, cmd model.MigrateAttachments) model.Msg {
	loaded, err := c.ReadProject(ctx, cmd.Workspace)
	if err != nil {
		return model.AttachmentsMigrated{Err: err}
	}
	if loaded.Project.UID == "" {
		return model.AttachmentsMigrated{Err: errors.New("This workspace is not bound to a kata Project, so there is nothing to move.")}
	}
	configured, err := c.attachmentsConfigured(ctx)
	if err != nil {
		return model.AttachmentsMigrated{Err: err}
	}
	if !configured {
		return model.AttachmentsMigrated{Err: errors.New(model.DocbankOffMessage)}
	}
	root, err := PrimaryCheckoutRoot(ctx, cmd.Workspace)
	if err != nil {
		return model.AttachmentsMigrated{Err: err}
	}
	page, err := c.ListIssuesWithResponse(ctx, &g.ListIssuesRequestOptions{
		PathParams: &g.ListIssuesPath{ProjectID: loaded.Project.ID},
	})
	var out model.AttachmentsMigrated
	if err != nil {
		out.Err = daemonError(err)
		return c.migrated(root, nil, out)
	}
	var touched []string
	for _, issue := range page.JSON200.Issues {
		if c.migrateIssue(ctx, host, root, cmd.Actor, issue, &out) {
			touched = append(touched, issue.UID)
		}
	}
	return c.migrated(root, touched, out)
}

func (c *Client) migrated(root string, touched []string, out model.AttachmentsMigrated) model.AttachmentsMigrated {
	tidyOldStore(root, touched)
	if failed := countFailed(out.Left); failed > 0 && out.Err == nil {
		out.Err = fmt.Errorf("%d attachment(s) need attention; see the list.", failed)
	}
	return out
}

func countFailed(left []model.MigratedAttachment) int {
	n := 0
	for _, l := range left {
		if l.Failed {
			n++
		}
	}
	return n
}

type oldKey struct {
	name, key  string
	host, path string
	shaped     bool
}

// oldKeys lists the keys of metadata that hold old attachments, sorted by key: every tern-kata.attachment.<name> key, and every
// attachment.<name> key (the 0.3 name) whose value is exactly a {"host", "path"} object, which kata's native references never are.
func oldKeys(metadata map[string]any) []oldKey {
	var keys []oldKey
	for key, value := range metadata {
		if name, ok := strings.CutPrefix(key, oldAttachmentKeyPrefix); ok && name != "" {
			host, path, shaped := oldPointer(value)
			keys = append(keys, oldKey{name, key, host, path, shaped})
		} else if name, ok := strings.CutPrefix(key, bareAttachmentKeyPrefix); ok && name != "" {
			if object, isObject := value.(map[string]any); isObject && len(object) == 2 {
				if host, path, shaped := oldPointer(value); shaped {
					keys = append(keys, oldKey{name, key, host, path, true})
				}
			}
		}
	}
	slices.SortFunc(keys, func(a, b oldKey) int { return strings.Compare(a.key, b.key) })
	return keys
}

func (c *Client) migrateIssue(ctx context.Context, host, root, actor string, issue g.IssueOut, out *model.AttachmentsMigrated) bool {
	deleted := false
	for _, old := range oldKeys(issue.Metadata) {
		name, key, path := old.name, old.key, old.path
		item := model.MigratedAttachment{ShortID: issue.ShortID, Name: name, Host: old.host, Path: path}
		left := func(reason string, failed bool) {
			item.Reason, item.Failed = reason, failed
			out.Left = append(out.Left, item)
		}
		switch {
		case !old.shaped:
			left("the key does not hold a {host, path} pointer, so it is not an old attachment", false)
			continue
		case host == "" || old.host == "" || !strings.EqualFold(old.host, host):
			left(fmt.Sprintf("the file is on %s at %s; run this command there", old.host, path), false)
			continue
		case !inOldStore(root, issue.UID, path):
			left(fmt.Sprintf("the file %s is outside this Project's old attachment folder; add it with `kata docbank attachments add` and unset the key `%s` by hand", path, key), false)
			continue
		}
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			left(fmt.Sprintf("the file %s is gone from this machine", path), false)
			continue
		}
		if err := attachableFile(path); err != nil {
			left(err.Error(), true)
			continue
		}
		added, err := c.upload(ctx, issue.ProjectID, issue.UID, actor, name, path)
		if err != nil {
			left(err.Error(), true)
			continue
		}
		item.ID = added.AttachmentID
		if err := c.clearMetadataKey(ctx, issue.ProjectID, issue.UID, actor, key); err != nil {
			// The clear may have committed though the client saw an error; unlinking then would drop the only reference left.
			shown, readErr := c.showIssue(ctx, issue.ProjectID, issue.UID)
			if readErr == nil {
				if _, held := shown.Issue.Metadata[key]; !held {
					err = nil
				}
			}
			if err != nil {
				reason := fmt.Sprintf("the old key could not be cleared, so the file was kept: %v", err)
				if readErr != nil {
					reason = fmt.Sprintf("uploaded as %s, but the old key could not be cleared (%v) and the Issue could not be read to check it (%v), so the new attachment was kept and a duplicate may exist; the file was kept", added.AttachmentID, err, readErr)
				} else if unlinkErr := c.UnlinkAttachment(ctx, model.UnlinkAttachment{ProjectID: issue.ProjectID, UID: issue.UID, Actor: actor, ID: added.AttachmentID}); unlinkErr != nil {
					reason = fmt.Sprintf("uploaded as %s, but the old key could not be cleared (%v) and the new attachment could not be unlinked (%v), so a duplicate may exist; the file was kept", added.AttachmentID, err, unlinkErr)
				} else {
					item.ID = ""
				}
				left(reason, true)
				continue
			}
		}
		out.Moved = append(out.Moved, item)
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			out.Left = append(out.Left, model.MigratedAttachment{ShortID: issue.ShortID, Name: name, Path: path, Reason: fmt.Sprintf("moved, but the local copy %s could not be deleted (%v); delete the file by hand", path, err), Failed: true})
			continue
		}
		deleted = true
	}
	return deleted
}
