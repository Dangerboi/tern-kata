package data

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kit/consolewindow"
)

// openerWait is how long openFile waits for the OS opener to fail. An opener still running by then has opened the file (some stay up
// as long as the viewer does) and is reaped when it exits.
const openerWait = 2 * time.Second

// openerWaitDelay is how long openFile waits for an exited opener's stderr to close: a viewer it started may keep the pipe.
const openerWaitDelay = 100 * time.Millisecond

// openFile hands path to the OS opener and reports an opener that cannot start or exits non-zero within openerWait.
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = openerWaitDelay
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start %s: %w", cmd.Args[0], err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil || errors.Is(err, exec.ErrWaitDelay) {
			return nil
		}
		if text := strings.TrimSpace(stderr.String()); text != "" {
			return fmt.Errorf("%s: %w: %s", cmd.Args[0], err, text)
		}
		return fmt.Errorf("%s: %w", cmd.Args[0], err)
	case <-time.After(openerWait):
		return nil
	}
}

// inTern reports whether this program runs in a Tern Pane or a headless Tern session, the two cases where `tern open` finds the window
// that shows it.
func inTern() bool {
	return os.Getenv("TERN_PANE") != "" || os.Getenv("TERN_WINDOW_SOCKET") != ""
}

// ternOpen runs `tern open` with args. `--preview` shows a file in the preview block of the Pane's tab, leaving the focus in the
// Pane and replacing the preview it showed before; `--split right` opens a block of its own beside the Pane.
func (g *Gateway) ternOpen(ctx context.Context, args ...string) error {
	if _, err := g.ternOutput(ctx, ternOpenTimeout, append([]string{"open"}, args...)); err != nil {
		return fmt.Errorf("tern open: %w", err)
	}
	return nil
}

const ternOpenTimeout = 15 * time.Second

// ternCannotDraw says why Tern cannot draw name, a kind TernDraws names: "" when it can. A kind that needs a newer Tern
// (model.TernNeeds) is not drawn by an older one, nor by one whose version cannot be read.
func (g *Gateway) ternCannotDraw(ctx context.Context, name string) string {
	kind, since := model.TernNeeds(name)
	if since == "" {
		return ""
	}
	switch ok, err := g.ternAtLeast(ctx, since); {
	case err != nil:
		return "could not tell whether this Tern draws " + kind + ": " + err.Error()
	case !ok:
		return kind + " need Tern " + strings.TrimPrefix(since, "v") + " or newer"
	}
	return ""
}

// noteSuffix is why Tern did not show a file, for the end of a sentence.
func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	return " (Tern: " + note + ")"
}

// systemOpenError says where the file is when the OS opener could not open it.
func systemOpenError(path string, err error) error {
	return fmt.Errorf("the file is at %s but the system app could not open it: %w", path, err)
}

func (g *Gateway) openAttachment(ctx context.Context, cmd model.OpenAttachment) model.Msg {
	base := model.AttachmentOpened{UID: cmd.UID, AttachmentID: cmd.Attachment.ID, Name: cmd.Attachment.Name, Way: cmd.Way}
	failed := func(err error) model.Msg {
		base.Err = err
		return base
	}
	// A refused way is refused before the download it would waste.
	if reason := model.AttachmentRefusal(cmd.Attachment, cmd.Way); reason != "" {
		return failed(errors.New(reason))
	}
	if cmd.Way == model.WayBeside && !inTern() {
		return failed(errors.New("this Pane does not run in a Tern window"))
	}
	if cmd.Way == model.WayBeside {
		if why := g.ternCannotDraw(ctx, cmd.Attachment.Name); why != "" {
			return failed(errors.New(why))
		}
	}
	dir, err := g.cacheDir()
	if err != nil {
		return failed(err)
	}
	opened, err := callAsWithin(g, ctx, cmd.Workspace, 0, func(ctx context.Context, c *Client) (model.AttachmentOpened, error) {
		path, err := c.DownloadAttachment(ctx, dir, cmd)
		base.Path = path
		return base, err
	})
	if err != nil {
		return failed(err)
	}
	switch cmd.Way {
	case model.WayCopy:
		return opened
	case model.WaySave:
		saved, remote, note, err := g.saveToDownloads(ctx, opened.Path)
		if err != nil {
			return failed(err)
		}
		opened.Path, opened.Remote, opened.Note = saved, remote, note
		return opened
	case model.WaySystem:
		if err := g.openFile(opened.Path); err != nil {
			return failed(systemOpenError(opened.Path, err))
		}
		return opened
	case model.WayBeside:
		if err := g.ternOpen(ctx, "--split", "right", opened.Path); err != nil {
			return failed(err)
		}
		return opened
	}
	if inTern() && model.TernDraws(cmd.Attachment.Name) {
		if opened.Note = g.ternCannotDraw(ctx, cmd.Attachment.Name); opened.Note == "" {
			err := g.ternOpen(ctx, "--preview", opened.Path)
			if err == nil {
				return opened
			}
			opened.Note = err.Error()
		}
	}
	// Only Tern shows a kind the OS must not open, so when Tern cannot, the file is just saved.
	if !model.OSOpens(cmd.Attachment.Name) {
		opened.Err = fmt.Errorf("saved at %s and not opened: tern-kata opens only images, video, audio, PDFs and text%s", opened.Path, noteSuffix(opened.Note))
		return opened
	}
	opened.OutsideTern = true
	if err := g.openFile(opened.Path); err != nil {
		opened.Err = systemOpenError(opened.Path, err)
	}
	return opened
}

// saveToDownloads puts a copy of the downloaded file at path into Downloads and returns where it went. In a Tern Pane that is the
// Downloads folder of the machine whose window shows the Pane, which `tern download` knows and this process does not: remote is set
// when Tern took the file there. A Tern not known to have `download` is never asked (it would open the file in a window), and
// whenever `tern download` does not deliver, for whatever reason, the copy goes into this machine's Downloads folder instead, so a
// change in Tern's wording cannot break Save. A window on this very machine is the plain local save; any other failure comes back
// as note, which the toast shows beside the path. Only a failure of the local copy is an error.
func (g *Gateway) saveToDownloads(ctx context.Context, path string) (saved string, remote bool, note string, err error) {
	if inTern() {
		has, verr := g.ternAtLeast(ctx, ternSubcommandsFloor)
		switch {
		case verr != nil:
			note = verr.Error()
		case has:
			var there string
			there, err = g.ternDownload(ctx, path)
			if err == nil {
				return there, true, "", nil
			}
			if !errors.Is(err, errTernDownloadLocal) {
				note = err.Error()
			}
		}
	}
	dir, err := g.downloadsDir()
	if err != nil {
		return "", false, "", err
	}
	saved, err = saveUnique(path, dir)
	if err != nil {
		return "", false, "", err
	}
	return saved, false, note, nil
}

// errTernDownloadLocal marks the one expected refusal of `tern download` that needs no mention: the window shows a Pane of the
// machine the files are on.
var errTernDownloadLocal = errors.New("tern download copies nothing here")

// ternDownload runs `tern download path` and returns the path it printed on stdout, its last line. A Tern whose window is on
// this machine exits 1 saying "the pane's files are on this computer already"; that comes back as errTernDownloadLocal, found by
// its wording only to keep the toast quiet. Every other failure, and an answer of nothing, is an error carrying Tern's message.
func (g *Gateway) ternDownload(ctx context.Context, path string) (string, error) {
	out, err := g.ternOutput(ctx, ternDownloadTimeout, []string{"download", path})
	if err != nil {
		if strings.Contains(err.Error(), "files are on this computer already") {
			return "", errTernDownloadLocal
		}
		return "", fmt.Errorf("tern download failed: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last, nil
	}
	return "", errors.New("tern download failed: it printed no path")
}

const ternDownloadTimeout = 60 * time.Second

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
