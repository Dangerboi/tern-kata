package data

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kit/consolewindow"
)

func openFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	consolewindow.Hide(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start %s: %w", cmd.Args[0], err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openableExtensions are the only extensions tern-kata hands to the OS: images, video, audio, PDFs and plain text. Everything else is
// saved and left for the user, because what the OS does with a file it knows (run it, follow a link inside it, launch a handler
// registered for it) cannot be told from the name alone, and anyone who can write the Project can name an Attachment.
var openableExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".heic", ".tif", ".tiff",
	".mp4", ".mov", ".webm", ".mkv", ".avi", ".m4v",
	".mp3", ".wav", ".ogg", ".flac", ".m4a",
	".pdf",
	".txt", ".md", ".log", ".csv", ".json",
}

// isOpenable reports whether name ends in one of openableExtensions, lower-cased and otherwise exactly as stored: no trimming, so
// "x.png." and "x.png " (which Windows would read as other names) are not openable.
func isOpenable(name string) bool {
	return slices.Contains(openableExtensions, strings.ToLower(filepath.Ext(name)))
}

func (g *Gateway) openAttachment(ctx context.Context, cmd model.OpenAttachment) model.Msg {
	failed := func(err error) model.Msg {
		return model.AttachmentOpened{UID: cmd.UID, Name: cmd.Attachment.Name, Err: err}
	}
	dir, err := g.cacheDir()
	if err != nil {
		return failed(err)
	}
	opened, err := callAsWithin(g, ctx, cmd.Workspace, 0, func(ctx context.Context, c *Client) (model.AttachmentOpened, error) {
		path, err := c.DownloadAttachment(ctx, dir, cmd)
		return model.AttachmentOpened{UID: cmd.UID, Name: cmd.Attachment.Name, Path: path}, err
	})
	if err != nil {
		return failed(err)
	}
	if !isOpenable(cmd.Attachment.Name) {
		opened.Err = fmt.Errorf("saved at %s and not opened: tern-kata opens only images, video, audio, PDFs and text", opened.Path)
		return opened
	}
	if err := g.openFile(opened.Path); err != nil {
		opened.Err = fmt.Errorf("Saved %s but could not open it: %w", opened.Path, err)
	}
	return opened
}
