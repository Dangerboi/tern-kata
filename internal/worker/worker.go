package worker

import (
	"cmp"
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
	// Supervisor names what keeps this worker running ("tern" for the plugin host half in Tern's session daemon);
	// empty when it was started by hand. `worker status` says it.
	Supervisor string
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
	// starting limits how many Projects resolve and recover at the same time (ADR-0004).
	starting chan struct{}
	// gates serializes the overlap check with the journal that closes it, per Project and job; runOps gives resume and
	// abandon one owner per run at a time.
	gates, runOps keyedMutex
	cancels       map[string]*cancelSignal // guarded by mu: wakes the goroutine supervising a run that was asked to stop
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
	// route is the last route check; instanceUID is the daemon instance the Project is pinned to.
	route       routeState
	instanceUID string

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
	// dates caches the planning dates of the Issues Issue-date jobs watch (the Project loop's own); datesDirty is set
	// by the event stream when an Issue changed.
	dates      map[string]issueDates
	datesDirty atomic.Bool
	// scheduled, queued and nextRun summarize sched for Status (guarded by Worker.mu).
	scheduled, queued int
	nextRun           time.Time
	// aside and warnings are the jobs the worker sets aside with their reasons, and the per-run Issue jobs that create
	// many Issues a day, as of the last definition refresh (guarded by Worker.mu).
	aside, warnings []JobNote
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
	w := &Worker{opt: opts, clock: opts.Clock, store: NewStore(opts.Dirs, opts.Hooks), ready: make(chan struct{}), pid: os.Getpid(), starting: make(chan struct{}, startingProjects)}
	for _, p := range opts.Config.Projects {
		cfg, _ := opts.Config.project(p.Name)
		actor := cmp.Or(cfg.Actor, opts.Config.Actor, opts.Actor)
		w.projects = append(w.projects, &projectState{cfg: cfg, actor: actor, held: map[string]string{}, wake: make(chan struct{}, 1), kick: make(chan struct{}, 1)})
	}
	return w, nil
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
	w.runs.Add(1)
	go func() { defer w.runs.Done(); w.retireUnserved(ctx) }()
	w.runs.Add(1)
	go func() { defer w.runs.Done(); w.retain(ctx) }()
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
// else admits nothing there (ADR-0010): the Project uid and name, the daemon's instance uid and the actor its
// credential authenticates as, and the configured actor and executor.
type pin struct {
	ProjectUID  string `json:"project_uid"`
	InstanceUID string `json:"instance_uid,omitempty"`
	Name        string `json:"name"`
	Actor       string `json:"actor"`
	AuthActor   string `json:"auth_actor,omitempty"`
	Executor    string `json:"executor"`
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
	caps, err := w.opt.Kata.Capabilities(ctx, p.ID)
	if err != nil {
		return "", err
	}
	if !caps.Has("cron_v1") {
		return "the kata daemon does not advertise cron_v1; upgrade kata to v0.19.0 or newer", nil
	}
	if caps.ProjectUID != p.UID {
		return fmt.Sprintf("the kata daemon answers for Project %s where %q resolves to %s", caps.ProjectUID, p.Name, p.UID), nil
	}
	instance, err := w.opt.Kata.Instance(ctx)
	if err != nil {
		return "", err
	}
	if reason := w.checkPin(p, ps.actor, instance); reason != "" {
		return reason, nil
	}
	w.mu.Lock()
	ps.project, ps.instanceUID, ps.route = p, instance.UID, routeState{passedAt: w.clock.Now()}
	w.mu.Unlock()
	return "", nil
}

// checkPin compares the Project with its pin and returns why it must admit nothing, empty when it may. A first start
// saves the pin; a Project that cannot be pinned is not served, since nothing would then notice a changed route.
func (w *Worker) checkPin(p Project, actor string, instance Instance) string {
	dir := w.opt.Dirs.ProjectDir(p.UID)
	want := pin{ProjectUID: p.UID, InstanceUID: instance.UID, Name: p.Name, Actor: actor, AuthActor: instance.Actor, Executor: w.opt.Config.Executor}
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
	prev, ok := readPin(path)
	if !ok {
		if _, err := os.Stat(path); err == nil {
			return fmt.Sprintf("the pin of Project %q (%s) cannot be read; an intentional move needs a new state folder and executor", p.Name, path)
		}
		return w.savePin(path, want)
	}
	if diff := pinDifferences(prev, want); len(diff) > 0 {
		return fmt.Sprintf("the pinned route of Project %q differs: %s; an intentional move needs a new state folder and executor", p.Name, joinWords(diff))
	}
	if prev.InstanceUID == "" && want.InstanceUID != "" { // a pin saved before instances were pinned learns its daemon now
		return w.savePin(path, want)
	}
	return ""
}

// pinDifferences names each pinned field whose value now differs; a field the pin never recorded is no difference.
func pinDifferences(prev, now pin) []string {
	var diff []string
	if prev.Actor != now.Actor {
		diff = append(diff, fmt.Sprintf("actor %q is now %q", prev.Actor, now.Actor))
	}
	if prev.Executor != now.Executor {
		diff = append(diff, fmt.Sprintf("executor %q is now %q", prev.Executor, now.Executor))
	}
	if prev.InstanceUID != "" && prev.InstanceUID != now.InstanceUID {
		diff = append(diff, fmt.Sprintf("daemon instance %s is now %s", prev.InstanceUID, now.InstanceUID))
	}
	if prev.AuthActor != "" && prev.AuthActor != now.AuthActor {
		diff = append(diff, fmt.Sprintf("authenticated actor %q is now %q", prev.AuthActor, now.AuthActor))
	}
	return diff
}

func (w *Worker) savePin(path string, p pin) string {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Sprintf("its pin cannot be saved, so the worker would not notice a changed route: %v", err)
	}
	raw, _ := json.Marshal(p)
	if err := replaceFile(path, raw); err != nil {
		return fmt.Sprintf("its pin cannot be saved, so the worker would not notice a changed route: %v", err)
	}
	return ""
}

func readPin(path string) (pin, bool) {
	raw, err := readFile(path)
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
	raw, err := readFile(w.cursorPath(uid))
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
