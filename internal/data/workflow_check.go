package data

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/worker"
)

// The Worker check: the Worker's own refusals of a Workflow definition, asked of the Worker's code in this process, with the
// prompt budget from the Worker's own loading of this machine's worker.toml.

// promptBudget is max_run_prompts as this machine's worker.toml gives it: max is nil, with why, when it could not be read, and
// missing says that is only because the file is not there.
type promptBudget struct {
	max     *int
	why     string
	missing bool
}

// readPromptBudget loads worker.toml the way the Worker does and reads its prompt limit.
func (g *Gateway) readPromptBudget() promptBudget {
	dirs, err := g.workerDirs()
	if err != nil {
		return promptBudget{why: err.Error()}
	}
	path := dirs.ConfigPath()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return promptBudget{why: "there is no worker.toml on this machine", missing: true}
	}
	cfg, err := worker.LoadConfig(path)
	if err != nil {
		return promptBudget{why: "worker.toml could not be read: " + err.Error()}
	}
	limit := cfg.MaxRunPrompts()
	return promptBudget{max: &limit}
}

// reading is the Worker's reading of one definition against the budget.
func (b promptBudget) reading(d cronjob.WorkflowDefinition) model.WorkerReading {
	check := worker.CheckWorkflow(d, b.max)
	out := model.WorkerReading{Refusals: check.Refusals, BudgetUnchecked: check.BudgetUnchecked}
	if check.BudgetUnchecked {
		out.BudgetWhy, out.ConfigMissing = b.why, b.missing
	}
	return out
}

// readWorkflowText answers the card's ask, After its pause, with the request number it came with.
func (g *Gateway) readWorkflowText(ctx context.Context, cmd model.ReadWorkflowText) model.WorkflowTextRead {
	if cmd.After > 0 {
		select {
		case <-time.After(cmd.After):
		case <-ctx.Done():
		}
	}
	return model.WorkflowTextRead{Request: cmd.Request, Reading: g.readPromptBudget().reading(cmd.Definition)}
}

// readWorkflowsAfter puts the Worker's reading of each loaded Workflow into the load.
var readWorkflowsAfter = afterStep{"read the Worker's reading of each Workflow", func(g *Gateway, _ context.Context, _ model.Cmd, result model.Msg) model.Msg {
	loaded, ok := result.(model.WorkflowsLoaded)
	if !ok || loaded.Err != nil {
		return result
	}
	budget := g.readPromptBudget()
	loaded.Readings = make(map[string]model.WorkerReading, len(loaded.Workflows))
	for _, w := range loaded.Workflows {
		loaded.Readings[w.UID] = budget.reading(w.Definition())
	}
	return loaded
}}

// readWrittenWorkflowAfter puts the Worker's reading of the stored Workflow into a write: the version a write stored, or the
// version a refused write found.
var readWrittenWorkflowAfter = afterStep{"read the Worker's reading of the written Workflow", func(g *Gateway, _ context.Context, _ model.Cmd, result model.Msg) model.Msg {
	written, ok := result.(model.WorkflowWritten)
	if !ok || written.Err != nil {
		return result
	}
	stored := written.Workflow
	if stored == nil {
		stored = written.Current
	}
	if stored == nil {
		return result
	}
	reading := g.readPromptBudget().reading(stored.Definition())
	written.Reading = &reading
	return written
}}
