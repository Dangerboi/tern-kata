package data

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// legacyAttachmentPrefix is where tern-kata first kept its Attachment pointers. kata's native attachments claim it now, so
// nothing reads it but the conversion that moves the pointers tern-kata wrote there.
const legacyAttachmentPrefix = "attachment."

// attachmentMarkerDir is the folder of tern-kata's state that holds one empty file per daemon instance and Project whose old
// pointers have all been moved. It is its own folder, so the worker's files never mix with it.
const attachmentMarkerDir = "attachment-pointers"

// legacyPointer is the pointer an old key holds when its value is exactly {"host": string, "path": string}, the shape tern-kata
// wrote. kata's native references and anything else never match.
func legacyPointer(value any) (map[string]any, bool) {
	object, isObject := value.(map[string]any)
	if !isObject || len(object) != 2 {
		return nil, false
	}
	host, hostOK := object["host"].(string)
	path, pathOK := object["path"].(string)
	if !hostOK || !pathOK {
		return nil, false
	}
	return model.Attachment{Host: host, Path: path}.Pointer(), true
}

// conversionPatch is the one metadata patch that moves every old pointer of an Issue, or nil when it holds none. A name taken
// under the new prefix, by a pointer already there or by one moved earlier in this patch, gets the Attachment name rule's
// next free number.
func conversionPatch(metadata map[string]any) map[string]any {
	var names []string
	var taken []string
	for key, value := range metadata {
		if name, ok := strings.CutPrefix(key, model.AttachmentKey("")); ok {
			taken = append(taken, name)
		} else if name, ok := strings.CutPrefix(key, legacyAttachmentPrefix); ok && name != "" {
			if _, shaped := legacyPointer(value); shaped {
				names = append(names, name)
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	slices.Sort(names)
	patch := map[string]any{}
	for _, name := range names {
		pointer, _ := legacyPointer(metadata[legacyAttachmentPrefix+name])
		moved := name
		for n := 2; slices.Contains(taken, moved); n++ {
			moved = numberedName(name, n)
		}
		taken = append(taken, moved)
		patch[model.AttachmentKey(moved)] = pointer
		patch[legacyAttachmentPrefix+name] = nil
	}
	return patch
}

// convertIssue moves the old pointers of the Issue as read (revision and metadata) in one patch guarded on that revision. A
// stale revision reads the Issue again and tries once more; any other failure leaves the Issue for the next pass.
func (c *Client) convertIssue(ctx context.Context, projectID int64, uid, actor string, revision int64, metadata map[string]any) error {
	for attempt := 0; ; attempt++ {
		patch := conversionPatch(metadata)
		if patch == nil {
			return nil
		}
		tag := etag(revision)
		body := g.PatchIssueMetadataBody{Actor: &actor, Patch: patch}
		_, err := c.PatchIssueMetadataWithResponse(ctx, &g.PatchIssueMetadataRequestOptions{
			PathParams: &g.PatchIssueMetadataPath{ProjectID: projectID, Ref: uid},
			Header:     &g.PatchIssueMetadataHeaders{IfMatch: &tag},
			Body:       &body,
		})
		if err == nil {
			return nil
		}
		if attempt > 0 || client.StatusCode(err) != http.StatusPreconditionFailed {
			return daemonError(err)
		}
		shown, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: uid}})
		if err != nil {
			return daemonError(err)
		}
		if shown.JSON200 == nil {
			return errors.New("kata daemon returned no Issue")
		}
		revision, metadata = shown.JSON200.Issue.Revision, shown.JSON200.Issue.Metadata
	}
}

// ConvertAttachmentPointers moves the old Attachment pointers of every Issue of the Project, open and closed, to the
// tern-kata.attachment.* keys. An Issue that fails is left and the pass goes on; the first failure comes back once the
// pass has seen every Issue, nil when every Issue holding old pointers was converted. kata requires the actor.
func (c *Client) ConvertAttachmentPointers(ctx context.Context, projectID int64, actor string) error {
	var first error
	for _, status := range []g.ListIssuesQueryStatus{g.ListIssuesQueryStatusOpen, g.ListIssuesQueryStatusClosed} {
		var cursor *string
		for {
			page, err := c.ListIssuesWithResponse(ctx, &g.ListIssuesRequestOptions{
				PathParams: &g.ListIssuesPath{ProjectID: projectID},
				Query:      &g.ListIssuesQuery{Status: &status, Cursor: cursor},
			})
			if err != nil {
				return errors.Join(first, daemonError(err))
			}
			if page.JSON200 == nil {
				return errors.Join(first, errors.New("kata daemon returned no Issues"))
			}
			for _, issue := range page.JSON200.Issues {
				if err := c.convertIssue(ctx, projectID, issue.UID, actor, issue.Revision, issue.Metadata); err != nil && first == nil {
					first = err
				}
			}
			if cursor = page.JSON200.NextCursor; cursor == nil || *cursor == "" {
				break
			}
		}
	}
	return first
}

// convertPointers is the Gateway's once-per-Project-per-daemon pass over the workspace's Project: it lists nothing when this
// machine has already finished the Project under this daemon instance, and writes the marker only when every Issue holding
// old pointers was converted. It is best effort: a failure leaves the Issues for the next pass and never stops the caller.
func (g *Gateway) convertPointers(ctx context.Context, workspace, actor string) {
	g.convertMu.Lock()
	defer g.convertMu.Unlock()
	_, _ = g.call(ctx, workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		return nil, g.convertWith(ctx, c, workspace, actor)
	})
}

func (g *Gateway) convertWith(ctx context.Context, c *Client, workspace, actor string) error {
	loaded, err := c.ReadProject(ctx, workspace)
	if err != nil || loaded.Project.UID == "" {
		return err
	}
	instance, err := c.InstanceWithResponse(ctx)
	if err != nil {
		return daemonError(err)
	}
	if instance.JSON200 == nil {
		return errors.New("kata daemon returned no instance")
	}
	marker := ""
	if dirs, err := g.workerDirs(); err == nil && safeFileName(instance.JSON200.InstanceUID) && safeFileName(loaded.Project.UID) {
		marker = filepath.Join(dirs.State, attachmentMarkerDir, instance.JSON200.InstanceUID+"."+loaded.Project.UID)
	}
	done := instance.JSON200.InstanceUID + "/" + loaded.Project.UID
	if g.converted[done] {
		return nil
	}
	if marker != "" {
		if _, err := os.Stat(marker); err == nil {
			g.converted[done] = true
			return nil
		}
	}
	if err := c.ConvertAttachmentPointers(ctx, loaded.Project.ID, actor); err != nil {
		return err
	}
	g.converted[done] = true
	if marker == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil { // the worker's state folder, which holds its journals, is private
		return err
	}
	return os.WriteFile(marker, nil, 0o600)
}

// safeFileName is whether name can be one folder entry as it stands.
func safeFileName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\:`)
}

// startPointerMove runs the conversion for a Pane's workspace in the background, once per Gateway and workspace, so the Pane
// draws without waiting for it; its Issues refresh as kata's events arrive.
func (g *Gateway) startPointerMove(workspace, actor string) {
	g.startMu.Lock()
	if g.convertStarted[workspace] {
		g.startMu.Unlock()
		return
	}
	g.convertStarted[workspace] = true
	g.startMu.Unlock()
	g.background.Add(1)
	go func() {
		defer g.background.Done()
		g.convertPointers(g.backgroundCtx, workspace, actor)
	}()
}
