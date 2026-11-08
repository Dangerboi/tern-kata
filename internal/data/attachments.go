package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/doordash-oss/oapi-codegen-dd/v3/pkg/runtime"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// Docbank's refusals as the Pane words them; `tern-kata docbank setup` reports the same two.
const (
	DocbankUnauthorizedMessage = "Docbank refused the API key kata sends it. Check the API key in the [docbank] section of kata's config.toml."
	DocbankUnreachableMessage  = "kata could not reach Docbank, or Docbank refused the request. Check that Docbank is running and that url in the [docbank] section of kata's config.toml is right."
)

// attachmentLimit is model.MaxAttachmentBytes as the refusal texts word it.
var attachmentLimit = fmt.Sprintf("%d MiB", model.MaxAttachmentBytes>>20)

// attachmentError is the plain-words text of a failed attachment call: what kata's attachment error codes mean for the person at the
// Pane. Any other failure reads as every daemon failure does.
func attachmentError(err error) error {
	switch daemonCode(err) {
	case "docbank_not_configured":
		return errors.New(model.DocbankOffMessage)
	case "docbank_unauthorized":
		return errors.New(DocbankUnauthorizedMessage)
	case "docbank_unreachable":
		return errors.New(DocbankUnreachableMessage)
	case "attachment_too_large":
		return fmt.Errorf("That file is over the %s limit for an attachment.", attachmentLimit)
	case "attachment_content_missing":
		return errors.New("Docbank no longer has this attachment's content.")
	case "attachment_invalid":
		return errors.New("kata refused this attachment: its name must be 1 to 255 bytes of UTF-8 with no slash, backslash or control character.")
	case "attachment_not_found":
		return errors.New("That attachment is no longer on the Issue.")
	case "federated_read_only":
		return errors.New("This Issue is a read-only copy from another kata instance, and this one cannot push changes to it, so its attachments cannot change here.")
	}
	return daemonError(err)
}

// attachmentsConfigured is whether the daemon has a Docbank to store attachments in, as its UI snapshot says.
func (c *Client) attachmentsConfigured(ctx context.Context) (bool, error) {
	view := model.DetailDaemonView()
	response, err := c.ReadUISnapshotWithResponse(ctx, &g.ReadUISnapshotRequestOptions{Query: &g.ReadUISnapshotQuery{View: &view}}, plainOverSocket)
	if err != nil {
		return false, daemonError(err)
	}
	if response.JSON200 == nil {
		return false, errors.New("kata daemon returned no snapshot")
	}
	return docbankConfigured(response.JSON200.Capabilities), nil
}

// docbankConfigured is whether a daemon's capabilities say attachments work. kata leaves the field out when it has no Docbank,
// so absent reads as not configured.
func docbankConfigured(c g.UICapabilities) bool { return c.Attachments != nil && *c.Attachments }

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
	if err := attachableFile(path); err != nil {
		return "", err
	}
	return path, nil
}

// attachableFile refuses what kata takes no Attachment from, before anything is read: a missing file, a directory or other
// file that is not regular, and one over the size limit. Symlinks are followed.
func attachableFile(path string) error {
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s does not exist.", path)
	case err != nil:
		return err
	case info.IsDir():
		return fmt.Errorf("%s is a directory; attach a file.", path)
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file.", path)
	case info.Size() > model.MaxAttachmentBytes:
		return fmt.Errorf("%s is over the %s limit for an attachment.", path, attachmentLimit)
	}
	return nil
}

// upload sends the file at source to kata as an Attachment called name of the Issue uid of projectID. kata stores the bytes in
// Docbank and writes the reference to the Issue's metadata.
func (c *Client) upload(ctx context.Context, projectID int64, uid, actor, name, source string) (g.AddAttachmentResponseBody, error) {
	content, err := os.ReadFile(source)
	if err != nil {
		return g.AddAttachmentResponseBody{}, err
	}
	var body g.AddAttachmentBody
	body.InitFromBytes(content, name)
	response, err := c.AddAttachmentWithResponse(ctx, &g.AddAttachmentRequestOptions{
		PathParams: &g.AddAttachmentPath{ProjectID: projectID, Ref: uid},
		Query:      &g.AddAttachmentQuery{Name: name, Actor: &actor},
		Body:       &body,
	})
	if err != nil {
		return g.AddAttachmentResponseBody{}, attachmentError(err)
	}
	if response.JSON200 == nil {
		return g.AddAttachmentResponseBody{}, errors.New("kata daemon returned no attachment")
	}
	return *response.JSON200, nil
}

// AttachFile is the Attach write: it uploads the file under its own name.
func (c *Client) AttachFile(ctx context.Context, cmd model.AttachFile) (model.FileAttached, error) {
	source, err := attachSource(cmd.Workspace, cmd.Path)
	if err != nil {
		return model.FileAttached{}, err
	}
	added, err := c.upload(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, filepath.Base(source), source)
	if err != nil {
		return model.FileAttached{}, err
	}
	return model.FileAttached{UID: cmd.UID, ID: added.AttachmentID, Name: added.Attachment.Name}, nil
}

// metadataPatch is the request that sets one metadata key of an Issue, a nil value clearing it, with no guard.
func metadataPatch(projectID int64, uid, actor, key string, value any) (*g.PatchIssueMetadataRequestOptions, *g.PatchIssueMetadataBody) {
	body := &g.PatchIssueMetadataBody{Actor: &actor, Patch: map[string]any{key: value}}
	return &g.PatchIssueMetadataRequestOptions{PathParams: &g.PatchIssueMetadataPath{ProjectID: projectID, Ref: uid}, Body: body}, body
}

// patchMetadataKey writes one metadata key of an Issue, a null value clearing it, guarded on the key still holding the JSON
// text ifValue, or on its being absent when ifValue is nil.
func (c *Client) patchMetadataKey(ctx context.Context, projectID int64, uid, actor, key string, value any, ifValue *string) (issueWrite, error) {
	options, body := metadataPatch(projectID, uid, actor, key, value)
	either := runtime.NewEitherFromB[g.MetadataPatchGuard_OneOf_0, g.MetadataPatchGuard_OneOf_1](g.MetadataPatchGuard_OneOf_1{Key: key, IfAbsent: g.True})
	if ifValue != nil {
		either = runtime.NewEitherFromA[g.MetadataPatchGuard_OneOf_0, g.MetadataPatchGuard_OneOf_1](g.MetadataPatchGuard_OneOf_0{Key: key, IfValue: *ifValue})
	}
	body.Guard = &g.MetadataPatchGuard{MetadataPatchGuard_OneOf: &g.MetadataPatchGuard_OneOf{Either: either}}
	r, err := c.PatchIssueMetadataWithResponse(ctx, options)
	if err != nil {
		return issueWrite{}, err
	}
	if r.JSON200 == nil {
		return issueWrite{}, errors.New("kata daemon returned no metadata write")
	}
	return written(r.JSON200.Changed, r.JSON200.Issue, nil), nil
}

// writeMetadataKey patches one metadata key of an Issue with no guard, a nil value clearing it, and returns the client's raw error.
func (c *Client) writeMetadataKey(ctx context.Context, projectID int64, uid, actor, key string, value any) error {
	options, _ := metadataPatch(projectID, uid, actor, key, value)
	_, err := c.PatchIssueMetadataWithResponse(ctx, options)
	return err
}

// clearMetadataKey clears one metadata key of an Issue with no guard, as `kata docbank attachments rm` does.
func (c *Client) clearMetadataKey(ctx context.Context, projectID int64, uid, actor, key string) error {
	if err := c.writeMetadataKey(ctx, projectID, uid, actor, key, nil); err != nil {
		return attachmentError(err)
	}
	return nil
}

// UnlinkAttachment unlinks an Attachment: its meta key is cleared, as `kata docbank attachments rm` does. The bytes stay in Docbank.
func (c *Client) UnlinkAttachment(ctx context.Context, cmd model.UnlinkAttachment) error {
	return c.clearMetadataKey(ctx, cmd.ProjectID, cmd.UID, cmd.Actor, model.AttachmentKey(cmd.ID))
}

// attachmentCacheDir is the folder downloaded Attachments are kept in.
func attachmentCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "tern-kata", "attachments"), nil
}

// DownloadAttachment makes sure the Attachment's bytes are at <dir>/<Issue UID>/<id>/<hash of the Docbank version>/<name> and returns that
// path. kata can point an id at another version, so the version is part of the path, and a file of the right size already there
// is used as it is. A download of another size than the reference says is
// refused and leaves nothing.
func (c *Client) DownloadAttachment(ctx context.Context, dir string, cmd model.OpenAttachment) (string, error) {
	a := cmd.Attachment
	if !ulid.MatchString(cmd.UID) {
		return "", fmt.Errorf("%q is not an Issue UID", cmd.UID)
	}
	version := sha256.Sum256([]byte(a.Version))
	path := filepath.Join(dir, cmd.UID, a.ID, hex.EncodeToString(version[:8]), a.Name)
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() == a.Size {
		return path, nil
	}
	response, err := c.AttachmentContentWithResponse(ctx, &g.AttachmentContentRequestOptions{
		PathParams: &g.AttachmentContentPath{ProjectID: cmd.ProjectID, Ref: cmd.UID, AttachmentID: a.ID},
	})
	if err != nil {
		return "", attachmentError(err)
	}
	if int64(len(response.Body)) != a.Size {
		return "", fmt.Errorf("Docbank sent %d bytes for %s, but the attachment is %d bytes long, so nothing was saved.", len(response.Body), a.Name, a.Size)
	}
	return path, atomicfile.Write(path, response.Body, atomicfile.Options{Mode: 0o600, DirMode: 0o700})
}
