package worker

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// lostStateSummary is the summary kata gets for a run this machine lost the state of (ADR-0002).
const lostStateSummary = "local state lost"

// abandonLost settles a run this worker holds no readable journal of (ADR-0002 "Lost journal", ADR-0006 "Quarantine").
// kata is the only evidence left: a run it records as running for this executor is recorded `unknown` with the
// summary "local state lost" and the run's Issue gets a comment; it is never rerun. A run that was in quarantine is
// acknowledged as well: its folder stays and keeps vetoing the uid, and `worker doctor` stops listing it.
func (w *Worker) abandonLost(ctx context.Context, lost *noRunError, by string) (RunTransition, error) {
	var run cronjob.Run
	var ps *projectState
	var unreadable []string
	for _, cand := range w.projects {
		if why := w.admitsNothing(cand); why != "" {
			unreadable = append(unreadable, fmt.Sprintf("Project %s admits nothing: %s", cand.cfg.Name, why))
			continue
		}
		r, err := w.opt.Kata.ShowRun(ctx, cand.project.ID, lost.uid)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return RunTransition{}, fmt.Errorf("run %s cannot be abandoned: kata could not be asked in Project %s: %w", lost.uid, cand.cfg.Name, err)
		}
		run, ps = r, cand
		break
	}
	if ps == nil {
		if len(unreadable) > 0 {
			return RunTransition{}, fmt.Errorf("run %s cannot be abandoned: it is not known which Project holds it (%s)", lost.uid, joinWords(unreadable))
		}
		if !lost.quarantined {
			return RunTransition{}, lost
		}
		return w.acknowledge(lost.uid, "", cmp.Or(by, w.opt.Actor))
	}
	by = cmp.Or(by, ps.actor)
	if run.ExecutorLabel != w.opt.Config.Executor {
		return RunTransition{}, fmt.Errorf("run %s belongs to worker %q, not %q", lost.uid, run.ExecutorLabel, w.opt.Config.Executor)
	}
	switch {
	case run.Status == cronjob.RunRunning:
		obs := Observation{
			RunUID: run.UID, JobUID: run.JobUID, DefinitionEventUID: run.DefinitionEventUID, WorkflowUID: run.WorkflowUID, WorkflowDefinitionEventUID: run.WorkflowDefinitionEventUID,
			OccurrenceKey: run.OccurrenceKey, IssueUID: run.IssueUID, Executor: run.ExecutorLabel, Actor: ps.actor, Status: cronjob.RunUnknown,
			Message: lostStateSummary, ExpectedRevision: run.Revision,
		}
		if run.StartedAt != nil {
			obs.StartedAt = *run.StartedAt
		}
		end := w.clock.Now().UTC()
		obs.EndedAt = &end
		if _, _, err := w.opt.Kata.Observe(ctx, ps.project.ID, obs); err != nil {
			return RunTransition{}, fmt.Errorf("run %s cannot be abandoned: kata did not accept the record: %w", lost.uid, err)
		}
	case run.Status == cronjob.RunUnknown && run.Summary == lostStateSummary:
		// A previous abandon recorded it and stopped before the comment: finish that, which is safe to repeat.
	case lost.quarantined:
		// kata already holds an outcome for it: nothing to record, the acknowledgement is all that is left.
		return w.acknowledge(lost.uid, ps.cfg.Name, by)
	default:
		return RunTransition{}, fmt.Errorf("run %s cannot be abandoned: kata records it as %s and this worker has no journal to settle", lost.uid, run.Status)
	}
	if run.IssueUID != "" {
		text := fmt.Sprintf("The worker's saved state of run %s was lost, so it was not rerun. kata now records the run as unknown (\"%s\"); abandoned by %s.", run.UID, lostStateSummary, by)
		if err := w.opt.Kata.Comment(ctx, ps.project.ID, ps.actor, run.IssueUID, "tern-kata:"+run.UID+":comment:state-lost", text); err != nil {
			return RunTransition{}, fmt.Errorf("run %s is recorded unknown in kata, but its Issue comment failed (abandon it again to retry): %w", lost.uid, err)
		}
	}
	if lost.quarantined {
		if err := w.store.Acknowledge(lost.uid, by, w.clock.Now()); err != nil {
			return RunTransition{}, err
		}
	}
	return RunTransition{RunUID: run.UID, Project: ps.cfg.Name, State: string(cronjob.RunUnknown)}, nil
}

// acknowledge records that a quarantined run was seen; with no Project it holds nothing in kata.
func (w *Worker) acknowledge(uid, project, by string) (RunTransition, error) {
	if err := w.store.Acknowledge(uid, by, w.clock.Now()); err != nil {
		return RunTransition{}, err
	}
	return RunTransition{RunUID: uid, Project: project, State: "acknowledged"}, nil
}
