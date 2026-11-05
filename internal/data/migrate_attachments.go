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
// folder, pointed to by a tern-kata.attachment.<name> meta key of {"host", "path"}. Only the migration still reads them.
const (
	oldAttachmentKeyPrefix = "tern-kata.attachment."
	oldStoreName           = ".kata-attachments"
)

// oldPointer is the host and path of an old Attachment key's value, false when the value is not a {"host", "path"} object of strings.
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
// set, so only such a file is ever deleted.
func inOldStore(root, uid, path string) bool {
	if !ulid.MatchString(uid) || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(filepath.Join(root, oldStoreName, uid), path)
	return err == nil && rel != "." && filepath.IsLocal(rel)
}

// tidyOldStore removes the Issue folders of the old store that the migration emptied, and the store itself once only its own
// .gitignore is left.
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
	var out model.AttachmentsMigrated
	var touched []string
	for _, status := range []g.ListIssuesQueryStatus{g.ListIssuesQueryStatusOpen, g.ListIssuesQueryStatusClosed} {
		var cursor *string
		for {
			page, err := c.ListIssuesWithResponse(ctx, &g.ListIssuesRequestOptions{
				PathParams: &g.ListIssuesPath{ProjectID: loaded.Project.ID},
				Query:      &g.ListIssuesQuery{Status: &status, Cursor: cursor},
			})
			if err != nil {
				out.Err = daemonError(err)
				return c.migrated(root, touched, out)
			}
			if page.JSON200 == nil {
				out.Err = errors.New("kata daemon returned no Issues")
				return c.migrated(root, touched, out)
			}
			for _, issue := range page.JSON200.Issues {
				if c.migrateIssue(ctx, host, root, cmd.Actor, issue, &out) {
					touched = append(touched, issue.UID)
				}
			}
			if cursor = page.JSON200.NextCursor; cursor == nil || *cursor == "" {
				break
			}
		}
	}
	return c.migrated(root, touched, out)
}

// migrated finishes a migration: it tidies the old store and sets Err when a key failed to move.
func (c *Client) migrated(root string, touched []string, out model.AttachmentsMigrated) model.AttachmentsMigrated {
	tidyOldStore(root, touched)
	if failed := countFailed(out.Left); failed > 0 && out.Err == nil {
		out.Err = fmt.Errorf("%d attachment(s) could not be moved; run the command again once the cause is fixed.", failed)
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

// migrateIssue moves the old keys of one Issue into out, reporting whether it deleted a copy under the Issue's folder.
func (c *Client) migrateIssue(ctx context.Context, host, root, actor string, issue g.IssueOut, out *model.AttachmentsMigrated) bool {
	var names []string
	for key := range issue.Metadata {
		if name, ok := strings.CutPrefix(key, oldAttachmentKeyPrefix); ok && name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	deleted := false
	for _, name := range names {
		item := model.MigratedAttachment{ShortID: issue.ShortID, Name: name}
		left := func(reason string, failed bool) {
			item.Reason, item.Failed = reason, failed
			out.Left = append(out.Left, item)
		}
		pointerHost, path, shaped := oldPointer(issue.Metadata[oldAttachmentKeyPrefix+name])
		item.Host, item.Path = pointerHost, path
		switch {
		case !shaped:
			left("the key does not hold a {host, path} pointer, so it is not an old attachment", false)
			continue
		case host == "" || pointerHost == "" || !strings.EqualFold(pointerHost, host):
			left(fmt.Sprintf("the file is on %s at %s; run this command there", pointerHost, path), false)
			continue
		case !inOldStore(root, issue.UID, path):
			left(fmt.Sprintf("the file %s is outside this Project's old attachment folder; add it with `kata docbank attachments add` and unset the key by hand", path), false)
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
		if err := c.clearMetadataKey(ctx, issue.ProjectID, issue.UID, actor, oldAttachmentKeyPrefix+name); err != nil {
			reason := fmt.Sprintf("the old key could not be cleared, so the file was kept: %v", err)
			if unlinkErr := c.UnlinkAttachment(ctx, model.UnlinkAttachment{ProjectID: issue.ProjectID, UID: issue.UID, Actor: actor, ID: added.AttachmentID}); unlinkErr != nil {
				reason = fmt.Sprintf("uploaded as %s, but the old key could not be cleared (%v) and the new attachment could not be unlinked (%v), so a duplicate may exist; the file was kept", added.AttachmentID, err, unlinkErr)
			} else {
				item.ID = ""
			}
			left(reason, true)
			continue
		}
		out.Moved = append(out.Moved, item)
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			out.Left = append(out.Left, model.MigratedAttachment{ShortID: issue.ShortID, Name: name, Path: path, Reason: fmt.Sprintf("moved, but the local copy could not be deleted: %v", err), Failed: true})
			continue
		}
		deleted = true
	}
	return deleted
}
