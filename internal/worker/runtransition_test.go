package worker

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

// The table below pins every transition of the Run lifecycle from every Phase: a legal one produces the expected
// Journal, any other returns an error and changes nothing. It calls the Journal's unexported transitions directly.

var allPhases = []Phase{PhasePrepared, PhaseAdmitted, PhaseLaunching, PhaseStarted, PhaseResuming, PhaseFinished, PhaseSettled}

func TestTransitionsFromEveryPhase(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	launch := &LaunchRecord{PID: 7, PGID: 7}
	exit := &ExitRecord{}
	result := &AgentResult{}
	step := func(j *Journal) {
		j.Workflow = &WorkflowPlan{Steps: []PlanStep{{Kind: "command"}}}
		j.Steps = []StepRecord{{State: StepRunning, Tries: 1}}
	}

	tests := []struct {
		name  string
		legal []Phase
		edit  []func(*Journal)
		do    func(*Journal) error
		want  Phase
		check func(*testing.T, *Journal)
	}{
		{name: "hold", legal: inFlight, do: func(j *Journal) error { return j.hold(HoldInterrupted, "why", at) }, want: PhaseFinished,
			check: func(t *testing.T, j *Journal) {
				if j.Outcome.Status != cronjob.RunUnknown || j.Outcome.Hold != HoldInterrupted || j.Outcome.Summary != "reason: interrupted\nwhy" || !j.Outcome.At.Equal(at) {
					t.Errorf("outcome = %+v", j.Outcome)
				}
			}},
		{name: "holdBeforeAdmission", legal: []Phase{PhasePrepared}, do: func(j *Journal) error { return j.holdBeforeAdmission(HoldCreateRejected, "why", at) }, want: PhaseSettled,
			check: func(t *testing.T, j *Journal) {
				if j.holdClass() != holdBeforeAdmission || j.Outcome.Summary != "reason: create_rejected\nwhy" {
					t.Errorf("outcome = %+v", j.Outcome)
				}
			}},
		{name: "admit", legal: []Phase{PhasePrepared}, do: func(j *Journal) error { return j.admit(4) }, want: PhaseAdmitted,
			check: func(t *testing.T, j *Journal) {
				if j.Revision != 4 {
					t.Errorf("revision = %d", j.Revision)
				}
			}},
		{name: "reopen", legal: []Phase{PhaseResuming}, do: func(j *Journal) error { return j.reopen(9) }, want: PhaseAdmitted,
			check: func(t *testing.T, j *Journal) {
				if j.Revision != 9 {
					t.Errorf("revision = %d", j.Revision)
				}
			}},
		{name: "markLaunching", legal: []Phase{PhaseAdmitted}, edit: []func(*Journal){launched}, do: func(j *Journal) error { return j.markLaunching() }, want: PhaseLaunching,
			check: func(t *testing.T, j *Journal) {
				if j.Launch != nil {
					t.Errorf("launch = %+v, want none", j.Launch)
				}
			}},
		{name: "markStarted", legal: []Phase{PhaseLaunching}, do: func(j *Journal) error { return j.markStarted(launch) }, want: PhaseStarted,
			check: func(t *testing.T, j *Journal) {
				if j.Launch != launch {
					t.Errorf("launch = %+v", j.Launch)
				}
			}},
		{name: "markExited", legal: []Phase{PhaseStarted}, do: func(j *Journal) error { return j.markExited(exit, result) }, want: PhaseStarted,
			check: func(t *testing.T, j *Journal) {
				if j.Exit != exit || j.Result != result {
					t.Errorf("exit = %+v result = %+v", j.Exit, j.Result)
				}
			}},
		{name: "finish", legal: inFlight, do: func(j *Journal) error { return j.finish(&Outcome{Status: cronjob.RunSucceeded, Summary: "done"}, at) }, want: PhaseFinished,
			check: func(t *testing.T, j *Journal) {
				if j.Outcome.Status != cronjob.RunSucceeded || !j.Outcome.At.Equal(at) {
					t.Errorf("outcome = %+v", j.Outcome)
				}
			}},
		{name: "settle", legal: []Phase{PhaseFinished}, edit: []func(*Journal){ended(cronjob.RunSucceeded)}, do: func(j *Journal) error { return j.settle() }, want: PhaseSettled},
		{name: "concludeCancel", legal: []Phase{PhaseAdmitted, PhaseStarted}, edit: []func(*Journal){cancelled}, do: func(j *Journal) error { return j.concludeCancel(at) }, want: PhaseFinished,
			check: func(t *testing.T, j *Journal) {
				if j.Outcome.Status != cronjob.RunCancelled || j.Outcome.Summary != "abandoned by ann (was running)" {
					t.Errorf("outcome = %+v", j.Outcome)
				}
			}},
		{name: "concludeStep", legal: inFlight, edit: []func(*Journal){step}, do: func(j *Journal) error {
			return j.concludeStep(&Outcome{Status: cronjob.RunSucceeded, Summary: "ok"}, at)
		}, want: PhaseAdmitted,
			check: func(t *testing.T, j *Journal) {
				if j.Steps[0].State != StepSucceeded || j.Steps[0].Summary != "ok" {
					t.Errorf("step = %+v", j.Steps[0])
				}
			}},
		{name: "finishWorkflow", legal: []Phase{PhaseAdmitted}, edit: []func(*Journal){func(j *Journal) {
			step(j)
			j.Steps[0].State = StepSucceeded
			j.Workflow.Name = "w"
		}}, do: func(j *Journal) error { return j.finishWorkflow(at) }, want: PhaseFinished,
			check: func(t *testing.T, j *Journal) {
				if j.Outcome.Status != cronjob.RunSucceeded {
					t.Errorf("outcome = %+v", j.Outcome)
				}
			}},
		{name: "timeOutWorkflow", legal: []Phase{PhaseAdmitted}, edit: []func(*Journal){func(j *Journal) {
			step(j)
			j.Steps[0].State = StepPending
			j.Workflow.Name = "w"
			j.TimeoutSeconds = 60
		}}, do: func(j *Journal) error { return j.timeOutWorkflow(at) }, want: PhaseFinished,
			check: func(t *testing.T, j *Journal) {
				if j.Outcome.Status != cronjob.RunFailed || j.Steps[0].State != StepSkipped {
					t.Errorf("outcome = %+v steps = %+v", j.Outcome, j.Steps)
				}
			}},
	}
	for _, tt := range tests {
		for _, p := range allPhases {
			t.Run(tt.name+"/"+string(p), func(t *testing.T) {
				j := journalAt(p, "", tt.edit...)
				before := *j
				err := tt.do(j)
				legal := false
				for _, l := range tt.legal {
					legal = legal || l == p
				}
				if !legal {
					if err == nil {
						t.Fatalf("from %s: no error", p)
					}
					if !reflect.DeepEqual(*j, before) {
						t.Errorf("a refused transition changed the journal: %+v, was %+v", *j, before)
					}
					return
				}
				if err != nil {
					t.Fatalf("from %s: %v", p, err)
				}
				if j.Phase != tt.want {
					t.Errorf("phase = %s, want %s", j.Phase, tt.want)
				}
				if tt.check != nil {
					tt.check(t, j)
				}
			})
		}
	}
}

// Every hold reason reaches kata as `reason: <code>` with the detail on the next line, and the worker's own words for
// people (status detail, Issue comment, abandon summary) read the same from that message and from a journal saved in the
// "<code>: <detail>" shape written before it.
func TestEveryHoldReasonIsWrittenAsAReasonLine(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, reason := range []string{HoldAdmissionRejected, HoldCreateRejected, HoldSecretUnavailable, HoldLaunchUnproven, HoldInterrupted,
		HoldNoReceipt, HoldGroupUnproven, HoldInvalidPrompt, HoldExecutionError, HoldAgentLogin, HoldAgentQuota} {
		j := &Journal{RunUID: "run", Phase: PhaseStarted}
		hold := j.hold
		if reason == HoldAdmissionRejected || reason == HoldCreateRejected {
			j.Phase, hold = PhasePrepared, j.holdBeforeAdmission
		}
		if err := hold(reason, "it said: no\nsecond line", at); err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		if want := "reason: " + reason + "\nit said: no\nsecond line"; j.Outcome.Summary != want || j.Outcome.Hold != reason {
			t.Errorf("%s: outcome %+v, want summary %q", reason, j.Outcome, want)
		}
		earlier := &Outcome{Hold: reason, Summary: reason + ": it said: no\nsecond line"}
		for _, o := range []*Outcome{j.Outcome, earlier} {
			if o.holdDetail() != "it said: no\nsecond line" || o.holdLine() != reason+": it said: no\nsecond line" {
				t.Errorf("%s: %q reads as detail %q, line %q", reason, o.Summary, o.holdDetail(), o.holdLine())
			}
		}
	}
}

func TestMarkLaunchingRefusesACancelledRun(t *testing.T) {
	j := journalAt(PhaseAdmitted, "", cancelled)
	if err := j.markLaunching(); !errors.Is(err, errRunCancelled) || j.Phase != PhaseAdmitted {
		t.Fatalf("err = %v, phase = %s", err, j.Phase)
	}
}

func TestConcludeCancelNeedsTheRequest(t *testing.T) {
	j := journalAt(PhaseStarted, "")
	if err := j.concludeCancel(time.Now()); err == nil || j.Outcome != nil {
		t.Fatalf("err = %v, outcome = %+v", err, j.Outcome)
	}
}

func TestRequestCancelFromEveryPhase(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	legal := map[Phase]bool{PhaseAdmitted: true, PhaseLaunching: true, PhaseStarted: true}
	for _, p := range allPhases {
		t.Run(string(p), func(t *testing.T) {
			j := journalAt(p, "")
			before := *j
			err := j.requestCancel("ann", at)
			if !legal[p] {
				if err == nil || !reflect.DeepEqual(*j, before) {
					t.Fatalf("err = %v, journal = %+v", err, *j)
				}
				return
			}
			if err != nil || j.Cancel == nil || j.Cancel.By != "ann" || !j.Cancel.At.Equal(at) || j.Phase != p {
				t.Fatalf("err = %v, cancel = %+v", err, j.Cancel)
			}
		})
	}
}

func TestAcceptResumeFromEveryPhase(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	in, cost := int64(5), 0.25
	terms := resumeTerms{
		By: "ann", Generation: 3, At: at,
		Tokens:    func(*Journal) (*int64, *int64) { return &in, nil },
		Cost:      func(*Journal) *float64 { return &cost },
		SessionID: func(*Journal) string { return "sess" },
	}
	life := lifeWith(GroupGone, true, false)
	for _, p := range allPhases {
		t.Run(string(p), func(t *testing.T) {
			j := journalAt(p, HoldInterrupted)
			before := *j
			err := life.acceptResume(j, terms)
			if p != PhaseSettled {
				if err == nil || !reflect.DeepEqual(*j, before) {
					t.Fatalf("err = %v, journal = %+v", err, *j)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if j.Phase != PhaseResuming || j.Outcome != nil || j.Attempt != 2 || j.Generation != 3 || *j.CarriedInput != 5 || *j.CarriedCost != 0.25 ||
				j.Resume == nil || j.Resume.SessionID != "sess" || j.Resume.Hold != HoldInterrupted || j.Resume.By != "ann" {
				t.Errorf("journal = %+v resume = %+v", *j, j.Resume)
			}
		})
	}
}

func TestAcceptResumeClearsTheCancelRequestedBeforeTheHold(t *testing.T) {
	terms := resumeTerms{
		By: "ann", Generation: 3, At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Tokens:    func(*Journal) (*int64, *int64) { return nil, nil },
		Cost:      func(*Journal) *float64 { return nil },
		SessionID: func(*Journal) string { return "sess" },
	}
	j := journalAt(PhaseSettled, HoldInterrupted)
	j.Cancel = &CancelRecord{By: "ann"}
	if err := lifeWith(GroupGone, true, false).acceptResume(j, terms); err != nil {
		t.Fatal(err)
	}
	if j.Cancel != nil {
		t.Fatalf("a resumed run keeps the cancel set before its hold: %+v", j.Cancel)
	}
}

func TestAbandonFromEveryPhase(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	life := lifeWith(GroupGone, true, false)
	for _, p := range allPhases {
		t.Run("held/"+string(p), func(t *testing.T) {
			j := journalAt(p, HoldInterrupted)
			before := *j
			err := life.abandon(j, "ann", at)
			if p != PhaseSettled {
				if err == nil || !reflect.DeepEqual(*j, before) {
					t.Fatalf("err = %v, journal = %+v", err, *j)
				}
				return
			}
			if err != nil || j.Phase != PhaseFinished || j.Outcome.Status != cronjob.RunCancelled || j.Outcome.Summary != "abandoned by ann (was held: interrupted: why)" {
				t.Fatalf("err = %v, journal = %+v outcome = %+v", err, *j, j.Outcome)
			}
		})
	}
	t.Run("held before admission settles at once", func(t *testing.T) {
		j := journalAt(PhaseSettled, HoldCreateRejected)
		if err := life.abandon(j, "ann", at); err != nil || j.Phase != PhaseSettled || j.Outcome.Status != cronjob.RunCancelled {
			t.Fatalf("err = %v, journal = %+v", err, *j)
		}
	})
	t.Run("a Run that is not held", func(t *testing.T) {
		j := journalAt(PhaseAdmitted, "")
		if err := life.abandon(j, "ann", at); err == nil || j.Outcome != nil {
			t.Fatalf("err = %v", err)
		}
	})
}
