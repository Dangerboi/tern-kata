package data

import (
	"context"
	"errors"
	"net/http"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// ListRuns reads one page of the Project's Runs, newest first, as kata orders them: by when each was first observed. Without a
// job it is every Run of the Project, those of workflows alone included; with one it is that job's. Kata offers no other filter
// and holds at most 100 Runs a page: a full page comes with the cursor of the next older one. Kata answers no Run of an archived
// Project, so there the answer is the Project's own state.
func (c *Client) ListRuns(ctx context.Context, cmd model.LoadRuns) (model.RunsLoaded, error) {
	limit := int64(runsPageSize)
	query := &g.ListCronRunsQuery{Limit: &limit}
	if cmd.JobUID != "" {
		query.JobUID = &cmd.JobUID
	}
	if cmd.Before != "" {
		query.BeforeUID = &cmd.Before
	}
	response, err := c.ListCronRunsWithResponse(ctx, &g.ListCronRunsRequestOptions{PathParams: &g.ListCronRunsPath{ProjectID: cmd.ProjectID}, Query: query})
	if err != nil {
		if gone, checkErr := c.projectArchived(ctx, cmd.ProjectID, err); checkErr != nil || gone {
			return model.RunsLoaded{Request: cmd.Request, Before: cmd.Before, ProjectArchived: gone}, checkErr
		}
		return model.RunsLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.RunsLoaded{}, errors.New("kata daemon returned no Runs")
	}
	names := runNames{c: c, projectID: cmd.ProjectID, jobs: map[string]string{}, workflows: map[string]string{}, issues: map[string]model.Run{}}
	runs := make([]model.Run, len(response.JSON200.Runs))
	for i, wire := range response.JSON200.Runs {
		if runs[i], err = names.translate(ctx, wire); err != nil {
			return model.RunsLoaded{}, err
		}
	}
	loaded := model.RunsLoaded{Request: cmd.Request, Runs: runs, Before: cmd.Before}
	if response.JSON200.NextBeforeUID != nil {
		loaded.NextBefore = *response.JSON200.NextBeforeUID
	}
	return loaded, nil
}

// ShowRun reads one Run by uid.
func (c *Client) ShowRun(ctx context.Context, cmd model.ShowRun) (model.RunShown, error) {
	response, err := c.ShowCronRunWithResponse(ctx, &g.ShowCronRunRequestOptions{PathParams: &g.ShowCronRunPath{ProjectID: cmd.ProjectID, RunUID: cmd.UID}})
	if err != nil {
		return model.RunShown{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.RunShown{}, errors.New("kata daemon returned no Run")
	}
	names := runNames{c: c, projectID: cmd.ProjectID, jobs: map[string]string{}, workflows: map[string]string{}, issues: map[string]model.Run{}}
	run, err := names.translate(ctx, response.JSON200.Run)
	return model.RunShown{UID: cmd.UID, Run: run}, err
}

// runsPageSize is the most Runs kata answers in one page.
const runsPageSize = 100

// runNames resolves the uids a Run holds to the names a row shows: the job's, the workflow's and the Issue's short id and title.
// It reads each once however many Runs share it. What kata no longer answers for stays unnamed.
type runNames struct {
	c         *Client
	projectID int64
	jobs      map[string]string
	workflows map[string]string
	// issues holds the Issue's short id and title in a Run's fields, keyed by the Issue's uid.
	issues map[string]model.Run
}

func (n *runNames) translate(ctx context.Context, wire g.CronRun) (model.Run, error) {
	run := translateRun(wire)
	var err error
	if run.JobUID != "" {
		if run.JobName, err = n.name(ctx, n.jobs, run.JobUID, n.jobName); err != nil {
			return model.Run{}, err
		}
	}
	if run.WorkflowUID != "" {
		if run.WorkflowName, err = n.name(ctx, n.workflows, run.WorkflowUID, n.workflowName); err != nil {
			return model.Run{}, err
		}
	}
	if run.IssueUID != "" {
		issue, err := n.issue(ctx, run.IssueUID)
		if err != nil {
			return model.Run{}, err
		}
		run.IssueShortID, run.IssueTitle = issue.IssueShortID, issue.IssueTitle
	}
	return run, nil
}

// name reads a name once into cache. A uid kata does not know reads as "".
func (n *runNames) name(ctx context.Context, cache map[string]string, uid string, read func(context.Context, string) (string, error)) (string, error) {
	if name, ok := cache[uid]; ok {
		return name, nil
	}
	name, err := read(ctx, uid)
	if client.StatusCode(err) == http.StatusNotFound {
		name, err = "", nil
	}
	if err != nil {
		return "", daemonError(err)
	}
	cache[uid] = name
	return name, nil
}

func (n *runNames) jobName(ctx context.Context, uid string) (string, error) {
	response, err := n.c.ShowCronJobWithResponse(ctx, &g.ShowCronJobRequestOptions{PathParams: &g.ShowCronJobPath{ProjectID: n.projectID, CronUID: uid}})
	if err != nil {
		return "", err
	}
	if response.JSON200 == nil {
		return "", errors.New("kata daemon returned no Cron job")
	}
	return response.JSON200.Job.Name, nil
}

func (n *runNames) workflowName(ctx context.Context, uid string) (string, error) {
	response, err := n.c.ShowCronWorkflowWithResponse(ctx, &g.ShowCronWorkflowRequestOptions{PathParams: &g.ShowCronWorkflowPath{ProjectID: n.projectID, CronUID: uid}})
	if err != nil {
		return "", err
	}
	if response.JSON200 == nil {
		return "", errors.New("kata daemon returned no workflow")
	}
	return response.JSON200.Workflow.Name, nil
}

func (n *runNames) issue(ctx context.Context, uid string) (model.Run, error) {
	if read, ok := n.issues[uid]; ok {
		return read, nil
	}
	deleted := true
	response, err := n.c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{
		PathParams: &g.ShowIssuePath{ProjectID: n.projectID, Ref: uid},
		Query:      &g.ShowIssueQuery{IncludeDeleted: &deleted},
	})
	var read model.Run
	switch {
	case client.StatusCode(err) == http.StatusNotFound:
	case err != nil:
		return model.Run{}, daemonError(err)
	case response.JSON200 == nil:
		return model.Run{}, errors.New("kata daemon returned no Issue")
	default:
		read = model.Run{IssueShortID: response.JSON200.Issue.ShortID, IssueTitle: response.JSON200.Issue.Title}
	}
	n.issues[uid] = read
	return read, nil
}

// translateRun is kata's Run as the model holds it, without the names its uids resolve to.
func translateRun(w g.CronRun) model.Run {
	return model.Run{
		UID: w.UID, JobUID: orEmpty(w.JobUID), DefinitionEventUID: orEmpty(w.DefinitionEventUID),
		WorkflowUID: orEmpty(w.WorkflowUID), WorkflowDefinitionEventUID: orEmpty(w.WorkflowDefinitionEventUID),
		OccurrenceKey: orEmpty(w.OccurrenceKey), IssueUID: orEmpty(w.IssueUID),
		Actor: w.Actor, Teammate: orEmpty(w.Teammate), ExecutorLabel: orEmpty(w.ExecutorLabel),
		Status: model.RunStatus(w.Status), Message: orEmpty(w.Summary.Message),
		InputTokens: w.Summary.InputTokens, OutputTokens: w.Summary.OutputTokens,
		Revision: w.Revision, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, StartedAt: w.StartedAt, EndedAt: w.EndedAt,
	}
}
