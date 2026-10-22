package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Phase is how far a Run has got. The journal is saved before each side effect of the next phase (ADR-0006), so a
// worker restarted on the same state folder continues from the phase it finds and never infers a new attempt.
type Phase string

const (
	// PhasePrepared: the journal exists with its frozen definition; nothing has been sent to kata.
	PhasePrepared Phase = "prepared"
	// PhaseAdmitted: kata accepted the revision-0 running observation (and the per-run Issue exists).
	PhaseAdmitted Phase = "admitted"
	// PhaseLaunching: saved before omp is started. A journal found here has no start record, so the process may exist.
	PhaseLaunching Phase = "launching"
	// PhaseStarted: the start record (pid, process group, kernel start time, session folder) is saved; the worker waits.
	PhaseStarted Phase = "started"
	// PhaseFinished: the process is gone and its Outcome is decided (or the run is held); only reporting remains.
	PhaseFinished Phase = "finished"
	// PhaseSettled: every Issue write and the final observation have been sent.
	PhaseSettled Phase = "settled"
)

// Status values of an Outcome, as kata records them.
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	// StatusUnknown is a held run's status (ADR-0005).
	StatusUnknown = "unknown"
)

// Hold reasons the tracer can reach (ADR-0005). Later tickets add the rest.
const (
	HoldAdmissionRejected = "admission_rejected"
	HoldCreateRejected    = "create_rejected"
	HoldSecretUnavailable = "secret_unavailable"
	HoldLaunchUnproven    = "launch_unproven"
	HoldInterrupted       = "interrupted"
	HoldNoReceipt         = "no_receipt"
	HoldGroupUnproven     = "group_unproven"
	HoldInvalidPrompt     = "invalid_prompt"
	HoldExecutionError    = "execution_error"
)

// IntentKind names a kata write a run still owes (ADR-0006 "Reporting intents").
type IntentKind string

const (
	IntentComment IntentKind = "comment"
	IntentLabel   IntentKind = "label"
	IntentFinal   IntentKind = "final-observation"
)

// Intent is one owed kata write, saved with its exact payload before it is sent; a retry reuses it.
type Intent struct {
	Kind  IntentKind `json:"kind"`
	Key   string     `json:"key,omitempty"`
	Text  string     `json:"text,omitempty"`
	Done  bool       `json:"done,omitempty"`
	Error string     `json:"error,omitempty"`
}

// Journal is run.json: everything the worker needs to continue one Run alone.
type Journal struct {
	Version   int    `json:"version"`
	RunUID    string `json:"run_uid"`
	JournalID string `json:"journal_id"`
	Executor  string `json:"executor"`
	Actor     string `json:"actor"`
	// Generation is the Project lease generation the run was prepared under (ADR-0003).
	Generation int64 `json:"generation"`

	ProjectUID  string `json:"project_uid"`
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name"`

	// The job as it was when the run was prepared; later edits never change a prepared run.
	JobUID             string        `json:"job_uid"`
	JobName            string        `json:"job_name"`
	DefinitionEventUID string        `json:"definition_event_uid"`
	Prompt             string        `json:"prompt"`
	IssuePolicy        IssuePolicy   `json:"issue_policy"`
	SecretRefs         map[string]string `json:"secret_refs,omitempty"`
	ProfileName        string        `json:"profile_name"`
	Profile            Profile       `json:"profile"`
	Checkout           string        `json:"checkout,omitempty"`
	TimeoutSeconds     int64         `json:"timeout_seconds"`

	OccurrenceKey string    `json:"occurrence_key"`
	Occurrence    time.Time `json:"occurrence"`
	Manual        bool      `json:"manual,omitempty"`

	Phase Phase `json:"phase"`
	// Issue is the run's Issue once resolved; Existing marks a job's own Issue, which the worker never closes or labels.
	Issue *JournalIssue `json:"issue,omitempty"`
	// StartedAt stamps the running observation; Revision is the Run's revision at kata as last seen.
	StartedAt time.Time `json:"started_at,omitempty"`
	Revision  int64     `json:"revision"`

	Launch  *LaunchRecord `json:"launch,omitempty"`
	Exit    *ExitRecord   `json:"exit,omitempty"`
	Result  *AgentResult  `json:"result,omitempty"`
	Outcome *Outcome      `json:"outcome,omitempty"`
	Intents []Intent      `json:"intents,omitempty"`
}

// JournalIssue is the run's Issue.
type JournalIssue struct {
	UID      string `json:"uid"`
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Existing bool   `json:"existing,omitempty"`
}

// LaunchRecord is the start record: enough to prove the process gone later (ADR-0006 "Settlement proof").
type LaunchRecord struct {
	PID        int       `json:"pid"`
	PGID       int       `json:"pgid"`
	StartTicks uint64    `json:"start_ticks,omitempty"`
	SessionDir string    `json:"session_dir"`
	SessionID  string    `json:"session_id,omitempty"`
	At         time.Time `json:"at"`
}

// ExitRecord is the process's recorded exit status.
type ExitRecord struct {
	Code     int       `json:"code"`
	Signal   string    `json:"signal,omitempty"`
	TimedOut bool      `json:"timed_out,omitempty"`
	At       time.Time `json:"at"`
}

// Outcome is what the run ended as: Status and Summary go to kata, Hold is the reason of a held run.
type Outcome struct {
	Status  string    `json:"status"`
	Summary string    `json:"summary"`
	Hold    string    `json:"hold,omitempty"`
	At      time.Time `json:"at"`
	// Tokens come from omp's JSON output; nil when it reported none.
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
}

// Held reports whether the run waits for the user.
func (j *Journal) Held() bool { return j.Outcome != nil && j.Outcome.Hold != "" }

// Active reports whether the run still holds a slot: it is not settled and not provably over without a process.
func (j *Journal) Active() bool { return j.Phase != PhaseSettled }

const maxJournalBytes = 1 << 20

// Hooks lets a test observe or kill the worker at named journal points (ADR-0006 "Test seam"). AfterSave is called
// after each save with its point name; an error aborts the run exactly there, as a crash would, and nothing after it
// runs. No build tag: production leaves it nil.
type Hooks struct {
	AfterSave func(point string, j *Journal) error
}

// ErrKilled wraps what a Hooks.AfterSave returned: the worker stops all run work at that point, as a crash would.
var ErrKilled = errors.New("worker killed by a test hook")

// Store is the journal folder. One run has one writer at a time: Update locks the uid, reads the file, applies the
// change and replaces the file durably.
type Store struct {
	dirs  Dirs
	hooks Hooks
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewStore opens the journals under dirs.
func NewStore(dirs Dirs, hooks Hooks) *Store {
	return &Store{dirs: dirs, hooks: hooks, locks: map[string]*sync.Mutex{}}
}

// RunDir is the run's folder: journal, prompt, session folder and output.
func (s *Store) RunDir(uid string) string { return filepath.Join(s.dirs.Runs(), uid) }

func (s *Store) path(uid string) string { return filepath.Join(s.RunDir(uid), "run.json") }

func (s *Store) lock(uid string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[uid] == nil {
		s.locks[uid] = new(sync.Mutex)
	}
	return s.locks[uid]
}

// ErrNoJournal means the run has no journal here.
var ErrNoJournal = errors.New("no journal")

// Load reads a run's journal.
func (s *Store) Load(uid string) (*Journal, error) {
	raw, err := os.ReadFile(s.path(uid))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoJournal
	}
	if err != nil {
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, fmt.Errorf("journal of run %s: %w", uid, err)
	}
	if j.Version != 1 || j.RunUID != uid {
		return nil, fmt.Errorf("journal of run %s: unsupported version or uid", uid)
	}
	return &j, nil
}

// Create saves a new journal, failing when one exists, then calls the hook at point.
func (s *Store) Create(j *Journal, point string) error {
	l := s.lock(j.RunUID)
	l.Lock()
	defer l.Unlock()
	if _, err := os.Stat(s.path(j.RunUID)); err == nil {
		return fmt.Errorf("run %s already has a journal", j.RunUID)
	}
	return s.write(j, point)
}

// Update applies change to the saved journal and saves the result, then calls the hook at point. It returns the new journal.
func (s *Store) Update(uid, point string, change func(*Journal) error) (*Journal, error) {
	l := s.lock(uid)
	l.Lock()
	defer l.Unlock()
	j, err := s.Load(uid)
	if err != nil {
		return nil, err
	}
	if err := change(j); err != nil {
		return nil, err
	}
	return j, s.write(j, point)
}

func (s *Store) write(j *Journal, point string) error {
	raw, err := json.MarshalIndent(j, "", " ")
	if err != nil {
		return err
	}
	if len(raw) > maxJournalBytes {
		return fmt.Errorf("journal of run %s is %d bytes, over the %d limit", j.RunUID, len(raw), maxJournalBytes)
	}
	dir := s.RunDir(j.RunUID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := replaceFile(s.path(j.RunUID), raw); err != nil {
		return err
	}
	if s.hooks.AfterSave != nil {
		if err := s.hooks.AfterSave(point, j); err != nil {
			return fmt.Errorf("%w at %s: %w", ErrKilled, point, err)
		}
	}
	return nil
}

// replaceFile writes path durably: temp file, fsync, rename, directory fsync.
func replaceFile(path string, raw []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(raw)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		_ = os.Remove(name)
		return err
	}
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Scan reads every journal. A journal that cannot be read moves whole to quarantine/runs/<uid>, which vetoes that
// uid from then on (ADR-0006); the uids moved are returned.
func (s *Store) Scan() (journals []*Journal, quarantined []string, err error) {
	entries, err := os.ReadDir(s.dirs.Runs())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		j, lerr := s.Load(e.Name())
		if lerr == nil {
			journals = append(journals, j)
			continue
		}
		if errors.Is(lerr, ErrNoJournal) {
			continue
		}
		if mkerr := os.MkdirAll(filepath.Dir(s.Quarantined(e.Name())), 0o700); mkerr != nil {
			return nil, nil, mkerr
		}
		if rerr := os.Rename(s.RunDir(e.Name()), s.Quarantined(e.Name())); rerr != nil {
			return nil, nil, rerr
		}
		quarantined = append(quarantined, e.Name())
	}
	return journals, quarantined, nil
}

// Quarantined is where a damaged journal of uid lives.
func (s *Store) Quarantined(uid string) string { return filepath.Join(s.dirs.Quarantine(), uid) }

// Quarantine lists the quarantined uids.
func (s *Store) Quarantine() []string {
	entries, _ := os.ReadDir(s.dirs.Quarantine())
	var uids []string
	for _, e := range entries {
		uids = append(uids, e.Name())
	}
	return uids
}

// IsQuarantined reports whether uid's journal was set aside.
func (s *Store) IsQuarantined(uid string) bool {
	_, err := os.Stat(s.Quarantined(uid))
	return err == nil
}
