package data

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"go.kenn.io/kata/pkg/client"
	g "go.kenn.io/kata/pkg/client/generated"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/cronjobclient"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// ListJobs reads the Cron jobs of the Project, the archived ones too when asked, each native one with its parent. Kata answers no
// job of an archived Project, so there the answer is the Project's own state: ProjectArchived and no jobs.
func (c *Client) ListJobs(ctx context.Context, cmd model.LoadJobs) (model.JobsLoaded, error) {
	archived := cmd.Archived
	response, err := c.ListCronJobsWithResponse(ctx, &g.ListCronJobsRequestOptions{
		PathParams: &g.ListCronJobsPath{ProjectID: cmd.ProjectID},
		Query:      &g.ListCronJobsQuery{IncludeDeleted: &archived},
	})
	if err != nil {
		if gone, checkErr := c.projectArchived(ctx, cmd.ProjectID, err); checkErr != nil || gone {
			return model.JobsLoaded{Request: cmd.Request, ProjectArchived: gone}, checkErr
		}
		return model.JobsLoaded{}, daemonError(err)
	}
	if response.JSON200 == nil {
		return model.JobsLoaded{}, errors.New("kata daemon returned no Cron jobs")
	}
	parents := parentReader{c: c, projectID: cmd.ProjectID, read: map[string]readParent{}}
	jobs := make([]model.CronJob, len(response.JSON200.Jobs))
	for i, wire := range response.JSON200.Jobs {
		if jobs[i], err = parents.translate(ctx, wire); err != nil {
			return model.JobsLoaded{}, err
		}
	}
	loaded := model.JobsLoaded{Request: cmd.Request, Jobs: jobs}
	if cmd.Activity {
		if loaded.Activity, err = c.jobActivity(ctx, cmd.ProjectID, jobs, &parents); err != nil {
			return model.JobsLoaded{}, err
		}
	}
	return loaded, nil
}

// jobActivityPages bounds how many pages of the Project's Runs are read to find each adapter job's recent runs.
const jobActivityPages = 5

// jobActivity is what each job has done lately: for a native job the children its parent holds that carry its uid in their
// cron.occurrence metadata, newest first; for the others the newest Runs of the Project, read page by page until every live
// adapter job has its share or kata has no more.
func (c *Client) jobActivity(ctx context.Context, projectID int64, jobs []model.CronJob, parents *parentReader) (map[string]model.JobActivity, error) {
	activity := map[string]model.JobActivity{}
	waiting := 0
	for _, job := range jobs {
		switch {
		case job.Native():
			if children := parents.read[job.Definition.Issue.ParentUID].children[job.UID]; len(children) > 0 {
				activity[job.UID] = model.JobActivity{Children: children[:min(len(children), model.JobActivityShown)]}
			}
		case !job.Archived():
			waiting++
		}
	}
	if waiting == 0 {
		return activity, nil
	}
	before := ""
	for page := 0; page < jobActivityPages; page++ {
		limit := int64(runsPageSize)
		query := &g.ListCronRunsQuery{Limit: &limit}
		if before != "" {
			query.BeforeUID = &before
		}
		response, err := c.ListCronRunsWithResponse(ctx, &g.ListCronRunsRequestOptions{PathParams: &g.ListCronRunsPath{ProjectID: projectID}, Query: query})
		if err != nil {
			return nil, daemonError(err)
		}
		if response.JSON200 == nil {
			return nil, errors.New("kata daemon returned no Runs")
		}
		for _, wire := range response.JSON200.Runs {
			if run := (model.Run{Run: cronjobclient.Run(wire)}); run.JobUID != "" && !isNativeJob(jobs, run.JobUID) {
				if held := activity[run.JobUID]; len(held.Runs) < model.JobActivityShown {
					held.Runs = append(held.Runs, run)
					activity[run.JobUID] = held
				}
			}
		}
		if response.JSON200.NextBeforeUID == nil {
			break
		}
		before = *response.JSON200.NextBeforeUID
		if satisfied(jobs, activity) {
			break
		}
	}
	return activity, nil
}

func isNativeJob(jobs []model.CronJob, uid string) bool {
	return slices.ContainsFunc(jobs, func(j model.CronJob) bool { return j.UID == uid && j.Native() })
}

// satisfied says every live adapter job has its share of runs.
func satisfied(jobs []model.CronJob, activity map[string]model.JobActivity) bool {
	return !slices.ContainsFunc(jobs, func(j model.CronJob) bool {
		return !j.Native() && !j.Archived() && len(activity[j.UID].Runs) < model.JobActivityShown
	})
}

// projectArchived tells apart the two reasons a Project's cron routes answer "project not found": the Project is archived, or
// it is gone. It reports true only when the Project's own state says archived; any other failure is err as it came.
func (c *Client) projectArchived(ctx context.Context, projectID int64, err error) (bool, error) {
	if envelope, ok := errors.AsType[g.ErrorEnvelope](err); !ok || envelope.ErrorData.Code != "project_not_found" {
		return false, nil
	}
	include := "archived"
	response, listErr := c.ListProjectsWithResponse(ctx, &g.ListProjectsRequestOptions{Query: &g.ListProjectsQuery{Include: &include}})
	if listErr != nil {
		return false, daemonError(listErr)
	}
	if response.JSON200 == nil {
		return false, errors.New("kata daemon returned no Projects")
	}
	for _, p := range response.JSON200.Projects {
		if p.ID == projectID && p.DeletedAt != nil {
			return true, nil
		}
	}
	return false, nil
}

// parentReader reads a native job's parent Issue once however many jobs hang from it.
type parentReader struct {
	c         *Client
	projectID int64
	read      map[string]readParent
}

// translate is kata's Cron job as the model holds it. A native job comes with its parent: its status and the job's own
// progress key.
func (p *parentReader) translate(ctx context.Context, wire g.CronJob) (model.CronJob, error) {
	job := model.CronJob{Job: cronjobclient.Job(wire)}
	if !job.Native() {
		return job, nil
	}
	issue, err := p.parent(ctx, job.Definition.Issue.ParentUID)
	if err != nil {
		return model.CronJob{}, err
	}
	job.Parent = &issue.parent
	if progress, ok := cronjob.ReadProgress(issue.metadata[cronjob.ProgressKeyPrefix+job.UID]); ok {
		job.Parent.Progress = &progress
	}
	return job, nil
}

type readParent struct {
	parent   model.CronJobParent
	metadata map[string]any
	// children are the parent's children that a Cron job made, newest first, by the job's uid.
	children map[string][]model.CronChild
}

func (p *parentReader) parent(ctx context.Context, uid string) (readParent, error) {
	if read, ok := p.read[uid]; ok {
		return read, nil
	}
	deleted := true
	response, err := p.c.ShowIssueWithResponse(ctx, &g.ShowIssueRequestOptions{
		PathParams: &g.ShowIssuePath{ProjectID: p.projectID, Ref: uid},
		Query:      &g.ShowIssueQuery{IncludeDeleted: &deleted},
	})
	var read readParent
	switch {
	case client.StatusCode(err) == http.StatusNotFound:
		// Kata has no such Issue: it was purged, which reads as deleted.
		read = readParent{parent: model.CronJobParent{UID: uid, Deleted: true}}
	case err != nil:
		return readParent{}, daemonError(err)
	case response.JSON200 == nil:
		return readParent{}, errors.New("kata daemon returned no parent Issue")
	default:
		issue := response.JSON200.Issue
		read = readParent{
			parent:   model.CronJobParent{UID: issue.UID, ShortID: issue.ShortID, Title: issue.Title, Status: issue.Status, Deleted: issue.DeletedAt != nil},
			metadata: issue.Metadata,
			children: cronChildren(response.JSON200.Children),
		}
	}
	p.read[uid] = read
	return read, nil
}

// cronChildren groups the children a Cron job made by that job, newest first by the run each was made for.
func cronChildren(wire []g.IssueOut) map[string][]model.CronChild {
	byJob := map[string][]model.CronChild{}
	for _, child := range wire {
		occurrence := translateOccurrence(child.Metadata)
		if occurrence == nil || child.DeletedAt != nil {
			continue
		}
		byJob[occurrence.JobUID] = append(byJob[occurrence.JobUID], model.CronChild{
			UID: child.UID, ShortID: child.ShortID, Title: child.Title, Status: child.Status, ScheduledAt: occurrence.ScheduledAt, CreatedAt: child.CreatedAt,
		})
	}
	for _, children := range byJob {
		slices.SortStableFunc(children, func(a, b model.CronChild) int { return b.When().Compare(a.When()) })
	}
	return byJob
}
