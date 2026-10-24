package data

import (
	"context"
	"errors"
	"net/http"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/cronjobclient"
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
	names, err := c.readRunNames(ctx, cmd.ProjectID)
	if err != nil {
		return model.RunsLoaded{}, err
	}
	runs := make([]model.Run, len(response.JSON200.Runs))
	for i, wire := range response.JSON200.Runs {
		runs[i] = names.translate(wire)
	}
	loaded := model.RunsLoaded{Request: cmd.Request, Runs: runs, Before: cmd.Before, Refresh: cmd.Refresh}
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
	names, err := c.readOneRunNames(ctx, cmd.ProjectID, response.JSON200.Run)
	if err != nil {
		return model.RunShown{}, err
	}
	return model.RunShown{UID: cmd.UID, Run: names.translate(response.JSON200.Run)}, nil
}

// readOneRunNames reads only the names one Run holds: its Issue (deleted or not), its job and its workflow, one request each.
// What kata no longer answers for stays unnamed.
func (c *Client) readOneRunNames(ctx context.Context, projectID int64, wire g.CronRun) (runNames, error) {
	names := runNames{jobs: map[string]string{}, workflows: map[string]string{}, issues: map[string]model.Run{}}
	run := cronjobclient.Run(wire)
	if run.JobUID != "" {
		response, err := c.ShowCronJobWithResponse(ctx, &g.ShowCronJobRequestOptions{PathParams: &g.ShowCronJobPath{ProjectID: projectID, CronUID: run.JobUID}})
		switch {
		case client.StatusCode(err) == http.StatusNotFound:
		case err != nil:
			return names, daemonError(err)
		case response.JSON200 == nil:
			return names, errors.New("kata daemon returned no Cron job")
		default:
			names.jobs[run.JobUID] = response.JSON200.Job.Name
		}
	}
	if run.WorkflowUID != "" {
		response, err := c.ShowCronWorkflowWithResponse(ctx, &g.ShowCronWorkflowRequestOptions{PathParams: &g.ShowCronWorkflowPath{ProjectID: projectID, CronUID: run.WorkflowUID}})
		switch {
		case client.StatusCode(err) == http.StatusNotFound:
		case err != nil:
			return names, daemonError(err)
		case response.JSON200 == nil:
			return names, errors.New("kata daemon returned no workflow")
		default:
			names.workflows[run.WorkflowUID] = response.JSON200.Workflow.Name
		}
	}
	if run.IssueUID != "" {
		deleted := true
		response, err := c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{
			PathParams: &g.ShowIssuePath{ProjectID: projectID, Ref: run.IssueUID},
			Query:      &g.ShowIssueQuery{IncludeDeleted: &deleted},
		})
		switch {
		case client.StatusCode(err) == http.StatusNotFound:
		case err != nil:
			return names, daemonError(err)
		case response.JSON200 == nil:
			return names, errors.New("kata daemon returned no Issue")
		default:
			names.issues[run.IssueUID] = model.Run{IssueShortID: response.JSON200.Issue.ShortID, IssueTitle: response.JSON200.Issue.Title}
		}
	}
	return names, nil
}

// runsPageSize is the most Runs kata answers in one page.
const runsPageSize = 100

// runNames resolves the uids a Run holds to the names a row shows: the job's, the workflow's and the Issue's short id and title.
// ListRuns reads each kind once for the Project, however many Runs there are. What kata does not list stays unnamed: a deleted Issue then
// shows by its uid.
type runNames struct {
	jobs      map[string]string
	workflows map[string]string
	// issues holds the Issue's short id and title in a Run's fields, keyed by the Issue's uid.
	issues map[string]model.Run
}

// readRunNames reads the Project's Issues of every status, jobs and workflows (the archived ones too), one request of each kind.
func (c *Client) readRunNames(ctx context.Context, projectID int64) (runNames, error) {
	names := runNames{jobs: map[string]string{}, workflows: map[string]string{}, issues: map[string]model.Run{}}
	archived := true
	jobs, err := c.ListCronJobsWithResponse(ctx, &g.ListCronJobsRequestOptions{PathParams: &g.ListCronJobsPath{ProjectID: projectID}, Query: &g.ListCronJobsQuery{IncludeDeleted: &archived}})
	if err != nil {
		return names, daemonError(err)
	}
	if jobs.JSON200 == nil {
		return names, errors.New("kata daemon returned no Cron jobs")
	}
	for _, job := range jobs.JSON200.Jobs {
		names.jobs[job.UID] = job.Name
	}
	workflows, err := c.ListCronWorkflowsWithResponse(ctx, &g.ListCronWorkflowsRequestOptions{PathParams: &g.ListCronWorkflowsPath{ProjectID: projectID}, Query: &g.ListCronWorkflowsQuery{IncludeDeleted: &archived}})
	if err != nil {
		return names, daemonError(err)
	}
	if workflows.JSON200 == nil {
		return names, errors.New("kata daemon returned no workflows")
	}
	for _, workflow := range workflows.JSON200.Workflows {
		names.workflows[workflow.UID] = workflow.Name
	}
	issues, err := c.ListIssuesWithResponse(ctx, &g.ListIssuesRequestOptions{PathParams: &g.ListIssuesPath{ProjectID: projectID}})
	if err != nil {
		return names, daemonError(err)
	}
	if issues.JSON200 == nil {
		return names, errors.New("kata daemon returned no Issues")
	}
	for _, issue := range issues.JSON200.Issues {
		names.issues[issue.UID] = model.Run{IssueShortID: issue.ShortID, IssueTitle: issue.Title}
	}
	return names, nil
}

func (n runNames) translate(wire g.CronRun) model.Run {
	run := model.Run{Run: cronjobclient.Run(wire)}
	run.JobName, run.WorkflowName = n.jobs[run.JobUID], n.workflows[run.WorkflowUID]
	issue := n.issues[run.IssueUID]
	run.IssueShortID, run.IssueTitle = issue.IssueShortID, issue.IssueTitle
	return run
}
