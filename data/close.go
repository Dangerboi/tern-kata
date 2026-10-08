package data

import (
 "context"
 "fmt"
 "os"
 "os/exec"
 "strings"
 "github.com/Dangerboi/tern-kata/model"
 g "go.kenn.io/kata/pkg/client/generated"
)

func closeActor(ctx context.Context, workspace string) string {
 if actor := os.Getenv("KATA_AUTHOR"); actor != "" { return actor }
 if actor := os.Getenv("USER"); actor != "" { return actor }
 raw, err := exec.CommandContext(ctx, "git", "-C", workspace, "config", "user.name").Output()
 if err == nil && strings.TrimSpace(string(raw)) != "" { return strings.TrimSpace(string(raw)) }
 return "anonymous"
}

func (c *Client) Close(ctx context.Context, cmd model.CloseIssueCmd) error {
 actor := closeActor(ctx, cmd.Workspace)
 body := cmd.Body; body.Actor = &actor
 etag := fmt.Sprintf("\"rev-%d\"", cmd.Revision)
 response, err := c.CloseIssueWithResponse(ctx, &g.CloseIssueRequestOptions{PathParams:&g.CloseIssuePath{ProjectID:cmd.ProjectID, Ref:cmd.UID}, Header:&g.CloseIssueHeaders{IfMatch:&etag}, Body:&body})
 if response != nil && response.StatusCode >= 400 { return ReadError(response.Body, err) }
 return err
}

func (c *Client) Reopen(ctx context.Context, cmd model.ReopenIssueCmd) error {
 actor := closeActor(ctx, cmd.Workspace)
 response, err := c.ReopenIssueWithResponse(ctx, &g.ReopenIssueRequestOptions{PathParams:&g.ReopenIssuePath{ProjectID:cmd.ProjectID, Ref:cmd.UID}, Body:&g.ReopenIssueBody{Actor:&actor}})
 if response != nil && response.StatusCode >= 400 { return ReadError(response.Body, err) }
 return err
}
