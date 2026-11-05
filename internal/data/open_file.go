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

	"github.com/Dangerboi/tern-kata/internal/model"
)

// openFile opens path with the program the OS has for its type, without waiting for that program to end.
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
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start %s: %w", cmd.Args[0], err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// programExtensions are the file extensions the OS runs rather than shows, per GOOS; anything else is opened. Linux's .desktop is
// here too, since xdg-open launches it; our downloads never carry an exec bit.
var programExtensions = map[string][]string{
	"windows": {".exe", ".bat", ".cmd", ".com", ".hta", ".js", ".jse", ".vbs", ".vbe", ".wsf", ".wsh", ".ps1", ".msi", ".msp", ".scr", ".lnk", ".cpl", ".reg", ".pif", ".msc", ".vbscript"},
	"darwin":  {".app", ".command", ".tool", ".terminal", ".pkg", ".workflow"},
	"linux":   {".desktop"},
}

// isProgram reports whether path names a program or script that opening would run, judged by its extension on goos (case-insensitive).
// On Windows the PATHEXT list counts too.
func isProgram(goos, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	list := programExtensions[goos]
	if goos == "windows" {
		list = append(append([]string(nil), list...), strings.Split(strings.ToLower(os.Getenv("PATHEXT")), ";")...)
	}
	if goos != "windows" && goos != "darwin" {
		list = programExtensions["linux"]
	}
	return slices.Contains(list, ext)
}

// openAttachment downloads the Attachment into the cache folder and opens it.
func (g *Gateway) openAttachment(ctx context.Context, cmd model.OpenAttachment) model.Msg {
	failed := func(err error) model.Msg {
		return model.AttachmentOpened{UID: cmd.UID, Name: cmd.Attachment.Name, Err: err}
	}
	dir, err := g.cacheDir()
	if err != nil {
		return failed(err)
	}
	msg, err := g.call(ctx, cmd.Workspace, func(ctx context.Context, c *Client) (model.Msg, error) {
		path, err := c.DownloadAttachment(ctx, dir, cmd)
		return ok(model.AttachmentOpened{UID: cmd.UID, Name: cmd.Attachment.Name, Path: path}, err)
	})
	if err != nil {
		return failed(err)
	}
	opened := msg.(model.AttachmentOpened)
	if isProgram(runtime.GOOS, opened.Path) {
		opened.Err = fmt.Errorf("saved at %s and not opened because it is a program", opened.Path)
		return opened
	}
	if err := g.openFile(opened.Path); err != nil {
		opened.Err = fmt.Errorf("Saved %s but could not open it: %w", opened.Path, err)
	}
	return opened
}
