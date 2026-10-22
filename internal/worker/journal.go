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
	// PhaseResuming: a resume was accepted and saved; the Run's running observation is not yet confirmed. The attempt
	// counter, session id and fresh expiry are already recorded, so a crash here never repeats the acceptance.
	PhaseResuming Phase = "resuming"
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
	// HoldAgentLogin and HoldAgentQuota: omp ended a turn with a provider failure that a retry cannot fix until the
	// user logs in again (401, 403) or the usage limit resets (429). Resume continues the session afterwards.
	HoldAgentLogin = "agent_login"
	HoldAgentQuota = "agent_quota"
)

// IntentKind names a kata write a run still owes (ADR-0006 "Reporting intents").
type IntentKind string

const (
	IntentComment IntentKind = "comment"
	IntentLabel   IntentKind = "label"
	IntentFinal   IntentKind = "final-observation"
	// IntentNotify is the inbox request a held run owes the Issue's owner or author (ADR-0005). Its Recipient is read
	// from the Issue once and saved before the request is sent, so a retry never recomputes it.
	IntentNotify IntentKind = "notify"
)

// Intent is one owed kata write, saved with its exact payload before it is sent; a retry reuses it.
type Intent struct {
	Kind  IntentKind `json:"kind"`
	Key   string     `json:"key,omitempty"`
	Text  string     `json:"text,omitempty"`
	Done  bool       `json:"done,omitempty"`
	Error string     `json:"error,omitempty"`
	// Recipient is a notify intent's resolved inbox address, empty until it is resolved.
	Recipient string `json:"recipient,omitempty"`
	// Inspect says the write cannot be delivered without a person looking: kata holds something a retry would
	// overwrite. The run is settled with it marked and `worker status` and `worker doctor` name it (ADR-0006).
	Inspect string `json:"inspect,omitempty"`
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
	JobUID             string            `json:"job_uid"`
	JobName            string            `json:"job_name"`
	DefinitionEventUID string            `json:"definition_event_uid"`
	Prompt             string            `json:"prompt"`
	IssuePolicy        IssuePolicy       `json:"issue_policy"`
	SecretRefs         map[string]string `json:"secret_refs,omitempty"`
	ProfileName        string            `json:"profile_name"`
	Profile            Profile           `json:"profile"`
	Checkout           string            `json:"checkout,omitempty"`
	TimeoutSeconds     int64             `json:"timeout_seconds"`
	// Notify is set for a notify job's run instead of a prompt: the inbox request it sends and nothing else.
	Notify *NotifySpec `json:"notify,omitempty"`
	// Workflow is set for a workflow job's run instead of a prompt: the definition as it was when the run was prepared,
	// and Steps the state of each of its steps, in the same order (ADR-0008).
	Workflow *WorkflowPlan `json:"workflow,omitempty"`
	Steps    []StepRecord  `json:"steps,omitempty"`
	// ExpiresAt is a workflow run's expiry, shared by every step and retry: it starts when the run is prepared and again,
	// from the frozen timeout, when a resume is accepted (ADR-0008), never when a step happens to start.
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	OccurrenceKey string    `json:"occurrence_key"`
	Occurrence    time.Time `json:"occurrence"`
	Manual        bool      `json:"manual,omitempty"`

	Phase Phase `json:"phase"`
	// Issue is the run's Issue once resolved; Existing marks a job's own Issue, which the worker never closes or labels.
	Issue *JournalIssue `json:"issue,omitempty"`
	// StartedAt stamps the running observation; Revision is the Run's revision at kata as last seen.
	StartedAt time.Time `json:"started_at,omitempty"`
	Revision  int64     `json:"revision"`

	// Attempt counts launches of the run's prompt step: 0 or 1 is the first, each accepted resume adds one.
	Attempt int `json:"attempt,omitempty"`
	// Resume describes the latest accepted resume; nil for a run never resumed.
	Resume *ResumeRecord `json:"resume,omitempty"`

	Launch  *LaunchRecord `json:"launch,omitempty"`
	Exit    *ExitRecord   `json:"exit,omitempty"`
	Result  *AgentResult  `json:"result,omitempty"`
	Outcome *Outcome      `json:"outcome,omitempty"`
	Intents []Intent      `json:"intents,omitempty"`

	// Cancel is a request to stop the run, saved before the process is signalled (ADR-0008); the run then ends cancelled.
	Cancel *CancelRecord `json:"cancel,omitempty"`
	// CarriedInput and CarriedOutput are the tokens of the earlier attempts of the current prompt step (or prompt job)
	// that a resume continued: each accepted resume adds the held attempt's, so the run's total sums every attempt.
	CarriedInput  *int64 `json:"carried_input_tokens,omitempty"`
	CarriedOutput *int64 `json:"carried_output_tokens,omitempty"`
}

// CancelRecord is who asked a running run to stop.
type CancelRecord struct {
	By string    `json:"by"`
	At time.Time `json:"at"`
}

// addTokens sums two optional token counts; nil only when neither is known.
func addTokens(a, b *int64) *int64 {
	if a == nil && b == nil {
		return nil
	}
	var sum int64
	if a != nil {
		sum += *a
	}
	if b != nil {
		sum += *b
	}
	return &sum
}

// withCarried adds the carried tokens of earlier resumed attempts to an outcome's own.
func (j *Journal) withCarried(o *Outcome) *Outcome {
	o.InputTokens, o.OutputTokens = addTokens(j.CarriedInput, o.InputTokens), addTokens(j.CarriedOutput, o.OutputTokens)
	return o
}

// ResumeRecord is a resume the worker accepted: the session to continue and why the run was held.
type ResumeRecord struct {
	Attempt   int       `json:"attempt"`
	SessionID string    `json:"session_id"`
	By        string    `json:"by"`
	At        time.Time `json:"at"`
	// Hold and Detail are the reason and summary of the hold the resume lifted.
	Hold   string `json:"hold"`
	Detail string `json:"detail"`
}

// attempt is the number of the current attempt of the prompt step, starting at 1.
func (j *Journal) attempt() int { return max(j.Attempt, 1) }

// NotifySpec is a notify job's request as frozen when the run was prepared. Recipient may still be the alias
// `current-owner-or-author`, resolved from a fresh Issue read when the request is sent.
type NotifySpec struct {
	Recipient string `json:"recipient"`
	Message   string `json:"message,omitempty"`
	// Resolved is the request as the worker first worked it out from the Issue; a retry or a restart sends exactly it.
	Resolved *ResolvedNotice `json:"resolved,omitempty"`
}

// ResolvedNotice is an inbox request with its recipient and text settled.
type ResolvedNotice struct {
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
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

// Inspections are the reasons the run's reports could not be delivered without a person looking.
func (j *Journal) Inspections() []string {
	var out []string
	for _, it := range j.Intents {
		if it.Inspect != "" {
			out = append(out, string(it.Kind)+": "+it.Inspect)
		}
	}
	return out
}

// Active reports whether the run still holds a slot: it is not settled and not provably over without a process.
func (j *Journal) Active() bool { return j.Phase != PhaseSettled }

const maxJournalBytes = 1 << 20

// Hooks lets a test observe or kill the worker at named journal points (ADR-0006 "Test seam"). AfterSave is called
// after each save with its point name; an error aborts the run exactly there, as a crash would, and nothing after it
// runs. No build tag: production leaves both nil.
type Hooks struct {
	AfterSave func(point string, j *Journal) error
	// FailDirSync, when it returns an error for a save point, makes that save's folder sync fail with it.
	FailDirSync func(point string) error
}

// ErrKilled wraps what a Hooks.AfterSave returned: the worker stops all run work at that point, as a crash would.
var ErrKilled = errors.New("worker killed by a test hook")

// Store is the journal folder. One run has one writer at a time: Update locks the uid, reads the file, applies the
// change and replaces the file durably.
type Store struct {
	dirs  Dirs
	hooks Hooks
	locks keyedMutex
}

// NewStore opens the journals under dirs.
func NewStore(dirs Dirs, hooks Hooks) *Store {
	return &Store{dirs: dirs, hooks: hooks}
}

// RunDir is the run's folder: journal, prompt, session folder and output.
func (s *Store) RunDir(uid string) string { return filepath.Join(s.dirs.Runs(), uid) }

func (s *Store) path(uid string) string { return filepath.Join(s.RunDir(uid), "run.json") }

func (s *Store) lock(uid string) *sync.Mutex { return s.locks.get(uid) }

// ErrNoJournal means the run has no journal here.
var ErrNoJournal = errors.New("no journal")

// Load reads a run's journal.
func (s *Store) Load(uid string) (*Journal, error) {
	raw, err := readFile(s.path(uid))
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
	sync := syncDir
	if s.hooks.FailDirSync != nil {
		sync = func(dir string) error {
			if err := s.hooks.FailDirSync(point); err != nil {
				return err
			}
			return syncDir(dir)
		}
	}
	if err := replaceFileSyncing(s.path(j.RunUID), raw, sync); err != nil {
		return err
	}
	if s.hooks.AfterSave != nil {
		if err := s.hooks.AfterSave(point, j); err != nil {
			return fmt.Errorf("%w at %s: %w", ErrKilled, point, err)
		}
	}
	return nil
}

// replaceFile writes path durably: temp file, fsync, rename, folder sync (not on Windows). A failed folder sync fails
// the save though the file may already have been replaced; recovery handles either version (ADR-0006).
func replaceFile(path string, raw []byte) error { return replaceFileSyncing(path, raw, syncDir) }

func replaceFileSyncing(path string, raw []byte, sync func(dir string) error) error {
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
		err = renameInDir(dir, filepath.Base(name), filepath.Base(path))
	}
	if err != nil {
		_ = os.Remove(name)
		return err
	}
	return sync(dir)
}

// readFile and renameInDir go through os.Root, which on Windows opens files shareable for deletion and renames with
// POSIX semantics: a journal replaced while another goroutine or process reads it neither fails the write nor the read,
// as on Linux. os.ReadFile and os.Rename there make each refuse the other, and a refused read would quarantine a run.
func readFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadFile(filepath.Base(path))
}

func renameInDir(dir, from, to string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Rename(from, to)
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

// acknowledgement is the marker saved inside a quarantined run's folder when someone has seen it (ADR-0006): the
// folder keeps vetoing the run, so nothing is deleted to unblock a job.
type acknowledgement struct {
	By string    `json:"by"`
	At time.Time `json:"at"`
}

func (s *Store) acknowledgementPath(uid string) string {
	return filepath.Join(s.Quarantined(uid), "acknowledged.json")
}

// Acknowledge records that by has seen the quarantined run's damaged journal. The folder stays and keeps its veto.
func (s *Store) Acknowledge(uid, by string, at time.Time) error {
	if !s.IsQuarantined(uid) {
		return fmt.Errorf("run %s is not in quarantine", uid)
	}
	raw, err := json.Marshal(acknowledgement{By: by, At: at.UTC()})
	if err != nil {
		return err
	}
	return replaceFile(s.acknowledgementPath(uid), raw)
}

// Acknowledged says who acknowledged the quarantined run, if anyone did.
func (s *Store) Acknowledged(uid string) (by string, ok bool) {
	raw, err := readFile(s.acknowledgementPath(uid))
	if err != nil {
		return "", false
	}
	var a acknowledgement
	if json.Unmarshal(raw, &a) != nil || a.By == "" {
		return "", false
	}
	return a.By, true
}

// Retain removes the folders of runs that are settled, fully reported and ended before cutoff (ADR-0006 "Retention").
// A held run, one with an unsent write and one still in flight stay, and so does a journal it cannot read (startup
// recovery owns quarantine). The removed runs' mutexes are retired with them.
func (s *Store) Retain(cutoff time.Time) (removed []string, err error) {
	entries, err := os.ReadDir(s.dirs.Runs())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if j, lerr := s.Load(e.Name()); lerr != nil || !expired(j, cutoff) {
			continue
		}
		l := s.lock(e.Name())
		l.Lock()
		// Read again under the run's lock: what was settled a moment ago is what is removed.
		if current, lerr := s.Load(e.Name()); lerr == nil && expired(current, cutoff) {
			if rerr := os.RemoveAll(s.RunDir(e.Name())); rerr != nil {
				err = errors.Join(err, rerr)
			} else {
				s.locks.retire(e.Name())
				removed = append(removed, e.Name())
			}
		}
		l.Unlock()
	}
	return removed, err
}

// expired reports whether a journal may be removed: settled, nothing unsent, not held, ended before cutoff.
func expired(j *Journal, cutoff time.Time) bool {
	if j.Phase != PhaseSettled || j.Held() || j.Outcome == nil || !j.Outcome.At.Before(cutoff) {
		return false
	}
	for _, it := range j.Intents {
		if !it.Done {
			return false
		}
	}
	return true
}
