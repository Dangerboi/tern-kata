package data

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

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
	if err := g.openFile(opened.Path); err != nil {
		opened.Err = fmt.Errorf("Saved %s but could not open it: %w", opened.Path, err)
	}
	return opened
}
