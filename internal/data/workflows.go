package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/model"
	g "go.kenn.io/kata/pkg/client/generated"
)

// ListWorkflows reads every workflow of the Project, archived ones included, in kata's order.
func (c *Client) ListWorkflows(ctx context.Context, projectID int64) ([]model.Workflow, error) {
	all := true
	listed, err := c.ListCronWorkflowsWithResponse(ctx, &g.ListCronWorkflowsRequestOptions{PathParams: &g.ListCronWorkflowsPath{ProjectID: projectID}, Query: &g.ListCronWorkflowsQuery{IncludeDeleted: &all}})
	if err != nil {
		return nil, daemonError(err)
	}
	if listed.JSON200 == nil {
		return nil, errors.New("kata returned no workflows")
	}
	workflows := make([]model.Workflow, len(listed.JSON200.Workflows))
	for i, w := range listed.JSON200.Workflows {
		workflows[i] = translateWorkflow(w)
	}
	return workflows, nil
}

// ShowWorkflow reads one workflow, archived or not.
func (c *Client) ShowWorkflow(ctx context.Context, projectID int64, uid string) (model.Workflow, error) {
	shown, err := c.ShowCronWorkflowWithResponse(ctx, &g.ShowCronWorkflowRequestOptions{PathParams: &g.ShowCronWorkflowPath{ProjectID: projectID, CronUID: uid}})
	if err != nil {
		return model.Workflow{}, daemonError(err)
	}
	if shown.JSON200 == nil {
		return model.Workflow{}, errors.New("kata returned no workflow")
	}
	return translateWorkflow(shown.JSON200.Workflow), nil
}

// SaveWorkflow creates a workflow or replaces one. A replace is guarded on the definition_event_uid the text started against;
// when kata finds the workflow changed, nothing is written and the answer carries the workflow as it stands now.
func (c *Client) SaveWorkflow(ctx context.Context, cmd model.SaveWorkflow) (model.WorkflowWritten, error) {
	var definition g.CronWorkflowDefinition
	if err := json.Unmarshal([]byte(cmd.Definition), &definition); err != nil {
		return model.WorkflowWritten{}, fmt.Errorf("the workflow definition is not JSON kata takes: %w", err)
	}
	body := &g.PutCronWorkflowBody{Name: cmd.Name, Definition: definition}
	if cmd.Actor != "" {
		body.Actor = &cmd.Actor
	}
	var written *g.CronWorkflow
	var err error
	if cmd.UID == "" {
		var created *g.CreateCronWorkflowResp
		created, err = c.CreateCronWorkflowWithResponse(ctx, &g.CreateCronWorkflowRequestOptions{PathParams: &g.CreateCronWorkflowPath{ProjectID: cmd.ProjectID}, Body: body})
		if err == nil && created.JSON201 != nil {
			written = &created.JSON201.Workflow
		}
	} else {
		body.ExpectedEventUID = &cmd.Expected
		var replaced *g.ReplaceCronWorkflowResp
		replaced, err = c.ReplaceCronWorkflowWithResponse(ctx, &g.ReplaceCronWorkflowRequestOptions{PathParams: &g.ReplaceCronWorkflowPath{ProjectID: cmd.ProjectID, CronUID: cmd.UID}, Body: body})
		if err == nil && replaced.JSON200 != nil {
			written = &replaced.JSON200.Workflow
		}
	}
	return c.workflowWritten(ctx, cmd.ProjectID, cmd.ProjectUID, model.WorkflowOpSave, cmd.UID, written, err)
}

// ArchiveWorkflow archives the workflow, guarded on its definition_event_uid. Kata refuses while a live job uses it, and the answer
// carries kata's message.
func (c *Client) ArchiveWorkflow(ctx context.Context, cmd model.ArchiveWorkflow) (model.WorkflowWritten, error) {
	body := &g.CronDefinitionActionBody{ExpectedEventUID: cmd.Expected}
	if cmd.Actor != "" {
		body.Actor = &cmd.Actor
	}
	var written *g.CronWorkflow
	archived, err := c.ArchiveCronWorkflowWithResponse(ctx, &g.ArchiveCronWorkflowRequestOptions{PathParams: &g.ArchiveCronWorkflowPath{ProjectID: cmd.ProjectID, CronUID: cmd.UID}, Body: body})
	if err == nil && archived.JSON200 != nil {
		written = &archived.JSON200.Workflow
	}
	return c.workflowWritten(ctx, cmd.ProjectID, cmd.ProjectUID, model.WorkflowOpArchive, cmd.UID, written, err)
}

// RestoreWorkflow brings an archived workflow back, guarded on its definition_event_uid.
func (c *Client) RestoreWorkflow(ctx context.Context, cmd model.RestoreWorkflow) (model.WorkflowWritten, error) {
	body := &g.CronDefinitionActionBody{ExpectedEventUID: cmd.Expected}
	if cmd.Actor != "" {
		body.Actor = &cmd.Actor
	}
	var written *g.CronWorkflow
	restored, err := c.RestoreCronWorkflowWithResponse(ctx, &g.RestoreCronWorkflowRequestOptions{PathParams: &g.RestoreCronWorkflowPath{ProjectID: cmd.ProjectID, CronUID: cmd.UID}, Body: body})
	if err == nil && restored.JSON200 != nil {
		written = &restored.JSON200.Workflow
	}
	return c.workflowWritten(ctx, cmd.ProjectID, cmd.ProjectUID, model.WorkflowOpRestore, cmd.UID, written, err)
}

// workflowWritten turns the outcome of a write into its answer. A refused guard is not a failure: kata's 409 carries the current
// workflow, or when it does not, the workflow is read again.
func (c *Client) workflowWritten(ctx context.Context, projectID int64, projectUID string, op model.WorkflowOp, uid string, written *g.CronWorkflow, err error) (model.WorkflowWritten, error) {
	out := model.WorkflowWritten{ProjectUID: projectUID, Op: op, UID: uid}
	switch {
	case err == nil && written != nil:
		w := translateWorkflow(*written)
		out.Workflow, out.UID = &w, w.UID
		return out, nil
	case err == nil:
		return out, errors.New("kata returned no workflow")
	case daemonCode(err) != "cron_conflict":
		return out, daemonError(err)
	}
	if current, ok := conflictingWorkflow(err); ok {
		out.Current = &current
		return out, nil
	}
	current, readErr := c.ShowWorkflow(ctx, projectID, uid)
	if readErr != nil {
		return out, readErr
	}
	out.Current = &current
	return out, nil
}

// conflictingWorkflow is the current workflow kata attaches to a refused guard, in error.data.current.
func conflictingWorkflow(err error) (model.Workflow, bool) {
	envelope, ok := errors.AsType[g.ErrorEnvelope](err)
	if !ok {
		return model.Workflow{}, false
	}
	raw, ok := envelope.ErrorData.Data["current"]
	if !ok {
		return model.Workflow{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return model.Workflow{}, false
	}
	var current g.CronWorkflow
	if json.Unmarshal(encoded, &current) != nil || current.UID == "" {
		return model.Workflow{}, false
	}
	return translateWorkflow(current), true
}

func translateWorkflow(w g.CronWorkflow) model.Workflow {
	out := model.Workflow{UID: w.UID, Name: w.Name, Author: w.Author, EventUID: w.DefinitionEventUID, Revision: w.Revision, Archived: w.DeletedAt != nil,
		Options: rawOptions(w.Definition.Options), Steps: make([]model.WorkflowStep, len(w.Definition.Steps))}
	if w.Definition.About != nil {
		out.About = *w.Definition.About
	}
	if w.Definition.Input != nil {
		out.Input = *w.Definition.Input
	}
	for i, step := range w.Definition.Steps {
		out.Steps[i] = model.WorkflowStep{Key: step.Key, Kind: step.Kind, After: step.After, Options: rawOptions(step.Options)}
		if step.Command != nil {
			out.Steps[i].Command = *step.Command
		}
		if step.Prompt != nil {
			out.Steps[i].Prompt = *step.Prompt
		}
		if step.Retries != nil {
			out.Steps[i].Retries = int(*step.Retries)
		}
	}
	return out
}

// rawOptions is kata's adapter options as JSON, nil when there are none. Numbers come back as kata wrote them.
func rawOptions(options map[string]any) json.RawMessage {
	if len(options) == 0 {
		return nil
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return nil
	}
	return raw
}
