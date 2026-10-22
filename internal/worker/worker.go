package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Options is everything a Worker needs from outside, so a test runs the whole worker against kata in process with
// its own folders, clock and agent program.
type Options struct {
	Config Config
	Dirs   Dirs
	Kata   *Kata
	// Clock defaults to SystemClock.
	Clock Clock
	// Actor is the identity of runs when neither the config nor a Project names one.
	Actor string
	// Takeover claims the lease of every served Project even when another executor holds it (ADR-0003).
	Takeover bool
	Hooks    Hooks
	Version  string
	// Log receives one line per notable event; nil discards.
	Log func(format string, args ...any)
}

// Worker is the adapter process. Build it with New and run it with Run.
type Worker struct {
	opt     Options
	store   *Store
	clock   Clock
	started time.Time
	ready   chan struct{}
	pid     int

	ctx    context.Context // set by Run; every run goroutine ends with it
	halted atomic.Bool     // set when a test hook killed the worker: no run work happens after it
	mu     sync.Mutex      // guards every projectState field and pendingTotal
	// pendingTotal counts slots reserved across Projects (worker-wide max_active_runs).
	projects []*projectState
	runs     sync.WaitGroup
}

// projectState is one served Project: its config, what kata resolved it to, its lease, its definitions and its slots.
type projectState struct {
	cfg   ProjectConfig
	actor string

	project  Project
	resolved bool
	// blocked is why the Project admits nothing for a reason other than its lease: not found, route pinned elsewhere,
	// no cron API. Empty when fine.
	blocked string
	lease   LeaseState
	leaseAt time.Time

	jobs       map[string]Job
	jobsErr    string
	jobsAt     time.Time
	streamOpen bool
	cursor     int64

	// pending counts slots reserved before a uid exists; held maps each slot-holding run uid to its job uid.
	pending int
	held    map[string]string
	running map[string]bool // run uids with a goroutine driving them

	wake chan struct{}
	// kick asks the Project loop to look at its queued occurrences again; sched is that loop's own schedule state.
	kick  chan struct{}
	sched map[string]*jobSchedule
	// scheduled, queued and nextRun summarize sched for Status (guarded by Worker.mu).
	scheduled, queued int
	nextRun           time.Time
}

// New checks opts and builds a worker.
func New(opts Options) (*Worker, error) {
	if opts.Kata == nil {
		return nil, errors.New("worker: no kata client")
	}
	if opts.Clock == nil {
		opts.Clock = SystemClock{}
	}
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	if opts.Config.Executor == "" {
		return nil, errors.New("worker: no executor label")
	}
	w := &Worker{opt: opts, clock: opts.Clock, store: NewStore(opts.Dirs, opts.Hooks), ready: make(chan struct{}), pid: os.Getpid()}
	for _, p := range opts.Config.Projects {
		cfg, _ := opts.Config.project(p.Name)
		actor := cmpOr(cfg.Actor, opts.Config.Actor, opts.Actor)
		w.projects = append(w.projects, &projectState{cfg: cfg, actor: actor, held: map[string]string{}, wake: make(chan struct{}, 1), kick: make(chan struct{}, 1)})
	}
	return w, nil
}

func cmpOr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Ready closes once the worker holds its lock and answers on its control socket.
func (w *Worker) Ready() <-chan struct{} { return w.ready }

// Store is the worker's journals, for tests and for tools that read them.
func (w *Worker) Store() *Store { return w.store }

// Run serves until ctx ends. It refuses to start while another worker holds the machine's lock. A stop leaves the
// lease and any started run's journal as they are: the next start continues them (ADR-0003, ADR-0006).
func (w *Worker) Run(ctx context.Context) error {
	if err := os.MkdirAll(w.opt.Dirs.State, 0o700); err != nil {
		return err
	}
	lock, err := AcquireLock(w.opt.Dirs.Lock())
	if err != nil {
		return err
	}
	defer lock.Release()
	if _, err := w.opt.Kata.Ping(ctx); err != nil {
		return err
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	w.ctx, w.started = ctx, w.clock.Now()
	_ = os.Remove(w.opt.Dirs.Socket()) // the lock is ours, so a socket file is stale
	ln, err := net.Listen("unix", w.opt.Dirs.Socket())
	if err != nil {
		return fmt.Errorf("control socket: %w", err)
	}
	defer os.Remove(w.opt.Dirs.Socket())
	w.runs.Add(1)
	go func() {
		defer w.runs.Done()
		<-ctx.Done()
		_ = ln.Close()
	}()
	w.runs.Add(1)
	go func() { defer w.runs.Done(); w.serveControl(ctx, ln) }()
	for _, ps := range w.projects {
		w.runs.Add(1)
		go func() { defer w.runs.Done(); w.serveProject(ctx, ps) }()
	}
	close(w.ready)
	<-ctx.Done()
	w.runs.Wait()
	return nil
}

// projectNamed is the served Project called name.
func (w *Worker) projectNamed(name string) *projectState {
	for _, ps := range w.projects {
		if ps.cfg.Name == name {
			return ps
		}
	}
	return nil
}

// pin is projects/<uid>/pin.json: what the worker first saw of a Project, so a later start that sees something
// else admits nothing there (ADR-0010). kata's client API does not expose the daemon's instance uid, so the pin holds
// the Project uid, its name, the actor and the executor.
type pin struct {
	ProjectUID string `json:"project_uid"`
	Name       string `json:"name"`
	Actor      string `json:"actor"`
	Executor   string `json:"executor"`
}

// resolve finds the Project, checks the cron API and the pin, and returns a reason when the Project must admit
// nothing. An error means kata could not be asked; the caller retries.
func (w *Worker) resolve(ctx context.Context, ps *projectState) (blocked string, err error) {
	projects, err := w.opt.Kata.Projects(ctx)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(projects, func(p Project) bool { return p.Name == ps.cfg.Name })
	if i < 0 {
		return fmt.Sprintf("Project %q does not exist in kata", ps.cfg.Name), nil
	}
	p := projects[i]
	features, err := w.opt.Kata.Capabilities(ctx, p.ID)
	if err != nil {
		return "", err
	}
	if !slices.Contains(features, "cron_v1") {
		return "the kata daemon does not advertise cron_v1; upgrade kata to v0.19.0 or newer", nil
	}
	if reason := w.checkPin(p, ps.actor); reason != "" {
		return reason, nil
	}
	w.mu.Lock()
	ps.project = p
	w.mu.Unlock()
	return "", nil
}

func (w *Worker) checkPin(p Project, actor string) string {
	dir := w.opt.Dirs.ProjectDir(p.UID)
	want := pin{ProjectUID: p.UID, Name: p.Name, Actor: actor, Executor: w.opt.Config.Executor}
	// A name that earlier resolved to another uid is a different Project.
	others, _ := filepath.Glob(filepath.Join(w.opt.Dirs.State, "projects", "*", "pin.json"))
	for _, path := range others {
		if filepath.Dir(path) == dir {
			continue
		}
		if prev, ok := readPin(path); ok && prev.Name == p.Name && prev.ProjectUID != p.UID {
			return fmt.Sprintf("Project name %q was pinned to uid %s and now resolves to %s; use a new state folder and executor to serve it", p.Name, prev.ProjectUID, p.UID)
		}
	}
	path := filepath.Join(dir, "pin.json")
	if prev, ok := readPin(path); ok {
		var diff []string
		if prev.Actor != want.Actor {
			diff = append(diff, fmt.Sprintf("actor %q is now %q", prev.Actor, want.Actor))
		}
		if prev.Executor != want.Executor {
			diff = append(diff, fmt.Sprintf("executor %q is now %q", prev.Executor, want.Executor))
		}
		if len(diff) > 0 {
			return fmt.Sprintf("the pinned route of Project %q differs: %s; an intentional move needs a new state folder and executor", p.Name, joinWords(diff))
		}
		return ""
	}
	if err := os.MkdirAll(dir, 0o700); err == nil {
		raw, _ := json.Marshal(want)
		_ = replaceFile(path, raw)
	}
	return ""
}

func readPin(path string) (pin, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return pin{}, false
	}
	var p pin
	return p, json.Unmarshal(raw, &p) == nil
}

func joinWords(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

// readCursor and writeCursor keep the event stream's position as a hint, never as evidence (ADR-0004).
func (w *Worker) cursorPath(uid string) string {
	return filepath.Join(w.opt.Dirs.ProjectDir(uid), "events.cursor")
}

func (w *Worker) readCursor(uid string) int64 {
	raw, err := os.ReadFile(w.cursorPath(uid))
	if err != nil {
		return 0
	}
	var n int64
	_, _ = fmt.Sscan(string(raw), &n)
	return n
}

func (w *Worker) writeCursor(uid string, n int64) {
	if err := os.MkdirAll(filepath.Dir(w.cursorPath(uid)), 0o700); err == nil {
		_ = replaceFile(w.cursorPath(uid), []byte(fmt.Sprint(n)))
	}
}
