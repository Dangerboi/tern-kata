package data

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// openableExtensions are the only extensions tern-kata hands to the OS on its own, when an Attachment is opened with enter or a
// click: images, video, audio, PDFs and plain text. Everything else is saved and left for the user, because what the OS does with a
// file it knows (run it, follow a link inside it, launch a handler registered for it) cannot be told from the name alone, and
// anyone who can write the Project can name an Attachment. "Open with the system app" is the user's explicit choice and sends any type.
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

// inTern reports whether this program runs in a Tern Pane or a headless Tern session, the two cases where `tern open` finds the window
// that shows it.
func inTern() bool {
	return os.Getenv("TERN_PANE") != "" || os.Getenv("TERN_WINDOW_SOCKET") != ""
}

// ternOpen runs `tern open` with args with the Pane's environment, which carries the window it answers to. `--preview` shows a file
// in the preview block of the Pane's tab, leaving the focus in the Pane and replacing the preview it showed before; `--split right`
// opens a block of its own beside the Pane.
func (g *Gateway) ternOpen(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, ternOpenTimeout)
	defer cancel()
	out, err := g.runTern(ctx, append([]string{"open"}, args...))
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
		return model.AttachmentOpened{UID: cmd.UID, AttachmentID: cmd.Attachment.ID, Name: cmd.Attachment.Name, Way: cmd.Way, Err: err}
	}
	// Open beside can only end in Tern, so it is refused before the download it would waste.
	if cmd.Way == model.WayBeside {
		if !model.TernDraws(cmd.Attachment.Name) {
			return failed(errors.New(model.AttachmentRefusal(cmd.Attachment, model.OpenAttachmentBesideAction)))
		}
		if !inTern() {
			return failed(errors.New("this Pane does not run in a Tern window"))
		}
	}
	dir, err := g.cacheDir()
	if err != nil {
		return failed(err)
	}
	opened, err := callAsWithin(g, ctx, cmd.Workspace, 0, func(ctx context.Context, c *Client) (model.AttachmentOpened, error) {
		path, err := c.DownloadAttachment(ctx, dir, cmd)
		return model.AttachmentOpened{UID: cmd.UID, AttachmentID: cmd.Attachment.ID, Name: cmd.Attachment.Name, Path: path, Way: cmd.Way}, err
	})
	if err != nil {
		return failed(err)
	}
	switch cmd.Way {
	case model.WayCopy:
		return opened
	case model.WaySave:
		saved, err := g.saveToDownloads(opened.Path)
		if err != nil {
			return failed(err)
		}
		opened.Path = saved
		return opened
	case model.WaySystem:
		if err := g.openFile(opened.Path); err != nil {
			return failed(fmt.Errorf("Saved %s but could not open it: %w", opened.Path, err))
		}
		return opened
	case model.WayBeside:
		if err := g.ternOpen(ctx, "--split", "right", opened.Path); err != nil {
			return failed(err)
		}
		return opened
	}
	if !isOpenable(cmd.Attachment.Name) {
		opened.Err = fmt.Errorf("saved at %s and not opened: tern-kata opens only images, video, audio, PDFs and text", opened.Path)
		return opened
	}
	if inTern() && model.TernDraws(cmd.Attachment.Name) && g.ternOpen(ctx, "--preview", opened.Path) == nil {
		return opened
	}
	opened.OutsideTern = true
	if err := g.openFile(opened.Path); err != nil {
		opened.Err = fmt.Errorf("Saved %s but could not open it: %w", opened.Path, err)
	}
	return opened
}

// saveToDownloads copies the downloaded file at path into the Downloads folder and returns the copy's path.
func (g *Gateway) saveToDownloads(path string) (string, error) {
	dir, err := g.downloadsDir()
	if err != nil {
		return "", err
	}
	return saveUnique(path, dir)
}

// downloadsDir is the Downloads folder of the machine the Pane runs on: $XDG_DOWNLOAD_DIR or what `xdg-user-dir DOWNLOAD` says on
// Linux and the BSDs, else Downloads in the home folder, which is also where macOS and Windows keep it.
func downloadsDir() (string, error) {
	home, err := os.UserHomeDir() // %USERPROFILE% on Windows
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if dir := os.Getenv("XDG_DOWNLOAD_DIR"); filepath.IsAbs(dir) {
			return dir, nil
		}
		// xdg-user-dir answers the home folder itself for a folder nobody configured.
		if out, err := exec.Command("xdg-user-dir", "DOWNLOAD").Output(); err == nil {
			if dir := strings.TrimSpace(string(out)); filepath.IsAbs(dir) && dir != home {
				return dir, nil
			}
		}
	}
	return filepath.Join(home, "Downloads"), nil
}

// saveUnique copies the file at src into dir, creating dir, under its own name or, when a file of that name is there, as
// "name (1).ext", "name (2).ext" and so on: the first name nothing has. It never overwrites, and returns the path it wrote.
func saveUnique(src, dir string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: the user's Downloads folder, made as a file manager would
		return "", err
	}
	name := filepath.Base(src)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" { // a dotfile: its whole name is the stem
		stem, ext = name, ""
	}
	for n := range 10000 {
		candidate := name
		if n > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		path := filepath.Join(dir, candidate)
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // G302: a download the user reads like any other
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = io.Copy(out, in)
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("%s already has 10000 copies of %s", dir, name)
}
