package data

import (
 "context"
 "errors"
 "strconv"
 "github.com/Dangerboi/tern-kata/model"
 g "go.kenn.io/kata/pkg/client/generated"
)

func (c *Client) CreateIssue(ctx context.Context, cmd model.CreateIssueCmd) model.IssueCreated {
 body:=g.CreateIssueBody{Title:cmd.Title,Body:&cmd.Body,Actor:&cmd.Actor,Labels:cmd.Labels,Priority:cmd.Priority}
 if cmd.Owner!="" {body.Owner=&cmd.Owner}
 if cmd.ParentUID!="" {body.Links=[]g.CreateInitialLinkBody{{Type:"parent",ToRef:cmd.ParentUID}}}
 response,err:=c.CreateIssueWithResponse(ctx,&g.CreateIssueRequestOptions{PathParams:&g.CreateIssuePath{ProjectID:strconv.FormatInt(cmd.ProjectID,10)},Body:&body})
 if err!=nil {if response!=nil {err=ReadError(response.Body,err)};return model.IssueCreated{Err:err}}
 if response==nil || response.JSON200==nil {return model.IssueCreated{Err:errors.New("kata returned no created Issue")}}
 return model.IssueCreated{UID:response.JSON200.Issue.UID}
}
