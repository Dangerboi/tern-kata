package data

import (
	"context"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// SearchIssues is kata's ranked search of one Project (mode auto: words and meaning together when kata's embeddings work, words
// alone otherwise). The answer lists the hits' Issue UIDs in kata's order.
func (c *Client) SearchIssues(ctx context.Context, cmd model.SearchByMeaning) (model.SearchByMeaningDone, error) {
	mode, limit := g.Auto, int64(model.SearchLimit)
	query := &g.SearchIssuesQuery{Q: cmd.Query, Mode: &mode, Limit: &limit, Label: cmd.Labels}
	if cmd.Status != "" {
		status := g.SearchIssuesQueryStatus(cmd.Status)
		query.Status = &status
	}
	response, err := c.SearchIssuesWithResponse(ctx, &g.SearchIssuesRequestOptions{PathParams: &g.SearchIssuesPath{ProjectID: cmd.ProjectID}, Query: query})
	if err != nil {
		return model.SearchByMeaningDone{}, daemonError(err)
	}
	body := response.JSON200
	done := model.SearchByMeaningDone{Request: cmd.Request, Query: cmd.Query, Lexical: body.Mode == string(g.Lexical), Truncated: body.Truncated}
	if body.DegradedReason != nil {
		done.Reason = *body.DegradedReason
	}
	done.UIDs = make([]string, len(body.Results))
	for i, hit := range body.Results {
		done.UIDs[i] = hit.Issue.UID
	}
	return done, nil
}
