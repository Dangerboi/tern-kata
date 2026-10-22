package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// The route check (ADR-0004): before the worker trusts a Project's daemon it asks for the Project's cron capabilities and
// the daemon's instance. A daemon that no longer advertises cron_v1, answers for another Project or is another instance
// than the one pinned (ADR-0010) stops admission and report delivery for that Project until a check passes.
const (
	// routeReuse is how long a passing check stands for the next admission or report.
	routeReuse = 5 * time.Second
	// routeIdle is how often an otherwise idle Project is checked.
	routeIdle = 5 * time.Minute
	// routeBackoffStart and routeBackoffMax bound the wait between checks while one is failing; each wait is random
	// between half and all of its limit.
	routeBackoffStart = 30 * time.Second
	routeBackoffMax   = 5 * time.Minute
	// startingProjects is how many Projects resolve and recover at once when the worker starts.
	startingProjects = 2
)

// routeState is one Project's last route check, guarded by Worker.mu.
type routeState struct {
	passedAt time.Time
	// err is why the last check failed, empty while it passes; retryAt is when a failing check may be tried again.
	err     string
	retryAt time.Time
	backoff time.Duration
}

// checkRoute confirms the Project's route. force asks for a fresh probe even when a passing one is recent (a stream
// that just reconnected); a failing check is never repeated before its backoff has passed.
func (w *Worker) checkRoute(ctx context.Context, ps *projectState, force bool) error {
	now := w.clock.Now()
	w.mu.Lock()
	r := ps.route
	project, instance := ps.project, ps.instanceUID
	switch {
	case r.err != "" && now.Before(r.retryAt):
		w.mu.Unlock()
		return errors.New(r.err)
	case r.err == "" && !force && !r.passedAt.IsZero() && now.Sub(r.passedAt) < routeReuse:
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()
	err := w.probeRoute(ctx, project, instance)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err == nil {
		ps.route = routeState{passedAt: now}
		return nil
	}
	backoff := routeBackoffStart
	if ps.route.backoff > 0 {
		backoff = min(ps.route.backoff*2, routeBackoffMax)
	}
	ps.route = routeState{passedAt: ps.route.passedAt, err: err.Error(), backoff: backoff, retryAt: now.Add(jitter(backoff/2, backoff))}
	return err
}

// probeRoute asks the daemon about project and, when the Project was pinned to an instance, about the instance.
func (w *Worker) probeRoute(ctx context.Context, project Project, instance string) error {
	caps, err := w.opt.Kata.Capabilities(ctx, project.ID)
	switch {
	case err != nil:
		return fmt.Errorf("the kata daemon could not be asked for its cron capabilities: %w", err)
	case !caps.Has("cron_v1"):
		return errors.New("the kata daemon no longer advertises cron_v1")
	case caps.ProjectUID != project.UID:
		return fmt.Errorf("the kata daemon answers for Project %s where %s was pinned", caps.ProjectUID, project.UID)
	}
	if instance == "" {
		return nil
	}
	got, err := w.opt.Kata.Instance(ctx)
	switch {
	case err != nil:
		return fmt.Errorf("the kata daemon could not be asked for its instance: %w", err)
	case got.UID != instance:
		return fmt.Errorf("the kata daemon is instance %s where %s was pinned", got.UID, instance)
	}
	return nil
}

// routeFailing reports whether the last route check failed and when another may be tried.
func (w *Worker) routeFailing(ps *projectState) (retryAt time.Time, failing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return ps.route.retryAt, ps.route.err != ""
}

// beginStarting takes one of the startingProjects places a Project needs while it resolves and recovers, so a worker
// serving many Projects does not ask kata about all of them at once. The returned function gives the place back; it
// may be called again. ok is false when ctx ended first.
func (w *Worker) beginStarting(ctx context.Context) (done func(), ok bool) {
	select {
	case w.starting <- struct{}{}:
	case <-ctx.Done():
		return func() {}, false
	}
	var once sync.Once
	return func() { once.Do(func() { <-w.starting }) }, true
}
