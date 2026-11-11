package data

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

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

// ternShows are the extensions Tern's own file block draws in place beside the Pane: images, plain text, Markdown and JSON. Video, audio
// and PDFs open in a block that only hands them to the OS of the machine showing the window, and a CSV opens a table block that
// takes the focus, so those go to the OS opener of the session host as before.
var ternShows = []string{".png", ".jpg", ".jpeg", ".gif", ".txt", ".md", ".json"}

// inTern reports whether this program runs in a Tern Pane or a headless Tern session, the two cases where `tern open` finds the window
// that shows it.
func inTern() bool {
	return os.Getenv("TERN_PANE") != "" || os.Getenv("TERN_WINDOW_SOCKET") != ""
}

// openInTern shows path in the preview block of the Pane's tab with `tern open --preview`, which leaves the focus in the Pane and
// replaces the preview it showed before. It runs with the Pane's environment, which carries the window `tern open` answers to.
func (g *Gateway) openInTern(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, ternOpenTimeout)
	defer cancel()
	out, err := g.runTern(ctx, []string{"open", "--preview", path})
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("tern open: %s: %w", msg, err)
		}
		return fmt.Errorf("tern open: %w", err)
	}
	return nil
}

const ternOpenTimeout = 15 * time.Second

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
	if inTern() && slices.Contains(ternShows, strings.ToLower(filepath.Ext(cmd.Attachment.Name))) && g.openInTern(ctx, opened.Path) == nil {
		return opened
	}
	opened.OutsideTern = true
	if err := g.openFile(opened.Path); err != nil {
		opened.Err = fmt.Errorf("Saved %s but could not open it: %w", opened.Path, err)
	}
	return opened
}
