package worker

import (
	"context"
	"path/filepath"
	"slices"
	"time"
)

// retireUnserved is ADR-0003's retire rule: a Project this worker served and the config no longer lists gets its saved
// reports delivered and, last, the owner of its lease cleared (only this executor's, keeping the generation), so
// another worker need not wait out the lease. Projects are known from their pins and their journals. Nothing here
// admits, starts or resumes anything. It repeats until kata answers.
func (w *Worker) retireUnserved(ctx context.Context) {
	served := map[string]bool{}
	for _, p := range w.opt.Config.Projects {
		served[p.Name] = true
	}
	for wait := 10 * time.Second; ctx.Err() == nil; wait = min(wait*2, 5*time.Minute) {
		if w.retireOnce(ctx, served) {
			return
		}
		if !w.sleep(ctx, jitter(wait/2, wait)) {
			return
		}
	}
}

// retireOnce retires every unserved Project it knows and reports whether none is left to retry.
func (w *Worker) retireOnce(ctx context.Context, served map[string]bool) (done bool) {
	journals, _, _ := w.store.Scan()
	known := map[string]string{} // Project uid -> the name it was served under
	pins, _ := filepath.Glob(filepath.Join(w.opt.Dirs.State, "projects", "*", "pin.json"))
	for _, path := range pins {
		if p, ok := readPin(path); ok && p.ProjectUID != "" {
			known[p.ProjectUID] = p.Name
		}
	}
	for _, j := range journals {
		if j.Executor == w.opt.Config.Executor {
			known[j.ProjectUID] = j.ProjectName
		}
	}
	if len(known) == 0 {
		return true
	}
	projects, err := w.opt.Kata.Projects(ctx)
	if err != nil {
		return false
	}
	done = true
	for uid := range known {
		i := slices.IndexFunc(projects, func(p Project) bool { return p.UID == uid })
		if i < 0 || served[projects[i].Name] {
			continue // kata no longer has it, or it is served under a new name
		}
		if !w.retire(ctx, projects[i], journals) {
			done = false
		}
	}
	return done
}

// retire delivers the reports the Project's finished runs still owe, then releases its lease. False means kata could
// not be reached for one of them and the whole is tried again.
func (w *Worker) retire(ctx context.Context, p Project, journals []*Journal) bool {
	for _, j := range journals {
		if j.ProjectUID != p.UID || j.Executor != w.opt.Config.Executor || j.Phase != PhaseFinished {
			continue
		}
		ps := &projectState{cfg: ProjectConfig{Name: p.Name, Capacity: 1}, actor: j.Actor, project: p, resolved: true, held: map[string]string{}, kick: make(chan struct{}, 1)}
		if err := w.advance(ctx, ps, j.RunUID); err != nil {
			if ctx.Err() == nil {
				w.opt.Log("retired Project %s: run %s: %v", p.Name, j.RunUID, err)
			}
			return false
		}
	}
	if err := releaseLease(ctx, w.opt.Kata, w.clock, w.opt.Config.Executor, releaseActor(w.opt, journals, p.UID), p.ID); err != nil {
		if ctx.Err() == nil {
			w.opt.Log("retired Project %s: its lease could not be released: %v", p.Name, err)
		}
		return false
	}
	return true
}

// releaseActor is the actor that signs the release of a retired Project's lease: the one its last run used, else the
// worker's.
func releaseActor(o Options, journals []*Journal, projectUID string) string {
	for _, j := range journals {
		if j.ProjectUID == projectUID && j.Executor == o.Config.Executor && j.Actor != "" {
			return j.Actor
		}
	}
	if o.Config.Actor != "" {
		return o.Config.Actor
	}
	return o.Actor
}
