package worker

import "testing"

// The tables below pin the Run lifecycle's answers over Journals (ADR-0005, ADR-0006, ADR-0007); they reach it only
// through its interface.

// journalAt is a Journal in phase with the given hold (empty for none). A hold from before admission has no Outcome
// status; every other hold is `unknown`.
func journalAt(phase Phase, hold string, edit ...func(*Journal)) *Journal {
	j := &Journal{RunUID: "run", Phase: phase}
	if hold != "" {
		j.Outcome = &Outcome{Status: StatusUnknown, Hold: hold, Summary: hold + ": why"}
		if hold == HoldAdmissionRejected || hold == HoldCreateRejected {
			j.Outcome.Status = ""
		}
	}
	for _, e := range edit {
		e(j)
	}
	return j
}

func onCommandStep(j *Journal) {
	j.Workflow = &WorkflowPlan{Steps: []PlanStep{{Kind: "command"}}}
	j.Steps = []StepRecord{{State: StepRunning}}
}

func launched(j *Journal)  { j.Launch = &LaunchRecord{PID: 10, PGID: 10} }
func exited(j *Journal)    { j.Exit = &ExitRecord{} }
func cancelled(j *Journal) { j.Cancel = &CancelRecord{By: "ann"} }
func ended(status string) func(*Journal) {
	return func(j *Journal) { j.Outcome = &Outcome{Status: status, Summary: "done"} }
}

func groupIs(state GroupState, reason string) func(*LaunchRecord) GroupProof {
	return func(*LaunchRecord) GroupProof { return GroupProof{State: state, Reason: reason} }
}

// lifeWith is a lifecycle whose group answer, saved session and finished turn are the given ones.
func lifeWith(state GroupState, session, finishedTurn bool) lifecycle {
	return lifecycle{
		Group:        groupIs(state, "the job could not be opened"),
		SessionKnown: func(*Journal) bool { return session },
		FinishedTurn: func(*Journal) bool { return finishedTurn },
	}
}

var allHolds = []string{
	HoldAdmissionRejected, HoldCreateRejected, HoldSecretUnavailable, HoldInvalidPrompt, HoldExecutionError,
	HoldLaunchUnproven, HoldInterrupted, HoldNoReceipt, HoldGroupUnproven, HoldAgentLogin, HoldAgentQuota,
}

func TestSettledHeldAndHoldClass(t *testing.T) {
	cases := []struct {
		name    string
		j       *Journal
		settled bool
		class   holdClass
	}{
		{"prepared", journalAt(PhasePrepared, ""), false, notHeld},
		{"started", journalAt(PhaseStarted, ""), false, notHeld},
		{"finished", journalAt(PhaseFinished, "", ended(StatusSucceeded)), false, notHeld},
		{"settled", journalAt(PhaseSettled, "", ended(StatusSucceeded)), true, notHeld},
		{"settled without outcome", journalAt(PhaseSettled, ""), true, notHeld},
		{"admission rejected", journalAt(PhaseSettled, HoldAdmissionRejected), true, holdBeforeAdmission},
		{"create rejected", journalAt(PhaseSettled, HoldCreateRejected), true, holdBeforeAdmission},
		{"secret unavailable", journalAt(PhaseSettled, HoldSecretUnavailable), true, holdUncertain},
		{"interrupted, reporting", journalAt(PhaseFinished, HoldInterrupted), false, holdUncertain},
		{"agent login", journalAt(PhaseSettled, HoldAgentLogin), true, holdUncertain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.j.settled(); got != c.settled {
				t.Errorf("settled = %v, want %v", got, c.settled)
			}
			if got := c.j.holdClass(); got != c.class {
				t.Errorf("holdClass = %v, want %v", got, c.class)
			}
			if got := c.j.held(); got != (c.class != notHeld) {
				t.Errorf("held = %v", got)
			}
		})
	}
}

func TestOwes(t *testing.T) {
	cases := []struct {
		name string
		j    *Journal
		owes bool
	}{
		{"prepared", journalAt(PhasePrepared, ""), true},
		{"admitted", journalAt(PhaseAdmitted, ""), true},
		{"resuming", journalAt(PhaseResuming, ""), true},
		{"launching", journalAt(PhaseLaunching, ""), true},
		{"started", journalAt(PhaseStarted, ""), true},
		{"finished and unreported", journalAt(PhaseFinished, "", ended(StatusFailed)), true},
		{"settled", journalAt(PhaseSettled, "", ended(StatusFailed)), false},
		{"held while its hold is reported", journalAt(PhaseFinished, HoldInterrupted), false},
		{"held and reported", journalAt(PhaseSettled, HoldInterrupted), false},
		{"held before admission", journalAt(PhaseSettled, HoldAdmissionRejected), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.j.owes(); got != c.owes {
				t.Errorf("owes = %v, want %v", got, c.owes)
			}
		})
	}
}

func TestClosesGate(t *testing.T) {
	cases := []struct {
		name string
		j    *Journal
		gate bool
	}{
		{"prepared", journalAt(PhasePrepared, ""), true},
		{"started", journalAt(PhaseStarted, ""), true},
		{"finished", journalAt(PhaseFinished, "", ended(StatusSucceeded)), true},
		{"settled closes none", journalAt(PhaseSettled, "", ended(StatusSucceeded)), false},
		{"settled cancelled closes none", journalAt(PhaseSettled, "", ended(StatusCancelled)), false},
		{"pre-admission hold frees the gate at once", journalAt(PhaseSettled, HoldAdmissionRejected), false},
		{"create rejected frees the gate at once", journalAt(PhaseSettled, HoldCreateRejected), false},
		{"secret hold keeps it", journalAt(PhaseSettled, HoldSecretUnavailable), true},
		{"invalid prompt hold keeps it", journalAt(PhaseSettled, HoldInvalidPrompt), true},
		{"execution error hold keeps it", journalAt(PhaseSettled, HoldExecutionError), true},
		{"launch unproven hold keeps it", journalAt(PhaseSettled, HoldLaunchUnproven), true},
		{"group unproven hold keeps it", journalAt(PhaseSettled, HoldGroupUnproven), true},
		{"quota hold keeps it", journalAt(PhaseSettled, HoldAgentQuota), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.j.closesGate(); got != c.gate {
				t.Errorf("closesGate = %v, want %v", got, c.gate)
			}
		})
	}
}

func TestOccupiesSlot(t *testing.T) {
	const (
		gone    = GroupGone
		present = GroupPresent
		unknown = GroupUnchecked
	)
	cases := []struct {
		name  string
		j     *Journal
		group GroupState
		slot  bool
	}{
		{"prepared", journalAt(PhasePrepared, ""), gone, true},
		{"admitted", journalAt(PhaseAdmitted, ""), gone, true},
		{"resuming", journalAt(PhaseResuming, "", launched), gone, true},
		{"launching with no start record", journalAt(PhaseLaunching, ""), gone, true},
		{"started, group gone", journalAt(PhaseStarted, "", launched), gone, true},
		{"finished and unreported", journalAt(PhaseFinished, "", ended(StatusSucceeded)), gone, true},
		{"settled", journalAt(PhaseSettled, "", ended(StatusSucceeded)), present, false},

		{"pre-admission hold frees the slot", journalAt(PhaseSettled, HoldAdmissionRejected), present, false},
		{"create rejected frees the slot", journalAt(PhaseSettled, HoldCreateRejected), present, false},
		{"secret hold frees the slot", journalAt(PhaseSettled, HoldSecretUnavailable), present, false},
		{"invalid prompt hold frees the slot", journalAt(PhaseSettled, HoldInvalidPrompt), present, false},
		{"execution error hold frees the slot", journalAt(PhaseSettled, HoldExecutionError, launched), present, false},
		{"agent login hold frees the slot", journalAt(PhaseSettled, HoldAgentLogin, launched), present, false},
		{"agent quota hold frees the slot", journalAt(PhaseSettled, HoldAgentQuota, launched), unknown, false},
		{"secret hold still being reported keeps the slot", journalAt(PhaseFinished, HoldSecretUnavailable), gone, true},

		{"launch unproven with no start record keeps the slot", journalAt(PhaseSettled, HoldLaunchUnproven), gone, true},
		{"launch unproven with a start record, group gone", journalAt(PhaseSettled, HoldLaunchUnproven, launched), gone, false},
		{"launch unproven, group present", journalAt(PhaseSettled, HoldLaunchUnproven, launched), present, true},
		{"interrupted, group gone", journalAt(PhaseSettled, HoldInterrupted, launched), gone, false},
		{"interrupted, group present", journalAt(PhaseSettled, HoldInterrupted, launched), present, true},
		{"interrupted, group cannot be checked", journalAt(PhaseSettled, HoldInterrupted, launched), unknown, true},
		{"no receipt, group gone", journalAt(PhaseSettled, HoldNoReceipt, launched), gone, false},
		{"no receipt, group present", journalAt(PhaseSettled, HoldNoReceipt, launched), present, true},
		{"no receipt, group cannot be checked", journalAt(PhaseSettled, HoldNoReceipt, launched), unknown, true},
		{"group unproven, group gone", journalAt(PhaseSettled, HoldGroupUnproven, launched), gone, false},
		{"group unproven, group present", journalAt(PhaseSettled, HoldGroupUnproven, launched), present, true},
		{"group unproven, group cannot be checked", journalAt(PhaseSettled, HoldGroupUnproven, launched), unknown, true},
		{"interrupted, reporting, group gone", journalAt(PhaseFinished, HoldInterrupted, launched), gone, false},
		{"interrupted, reporting, group present", journalAt(PhaseFinished, HoldInterrupted, launched), present, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lifeWith(c.group, true, false).occupiesSlot(c.j); got != c.slot {
				t.Errorf("occupiesSlot = %v, want %v", got, c.slot)
			}
		})
	}
}

func TestMayResume(t *testing.T) {
	const (
		still  = "the agent's process group 10 is still present; the worker never stops a process to make a resume possible"
		unchecked = "the agent's process group 10 cannot be checked: the job could not be opened; the worker keeps the run's slot until the check succeeds and will not free capacity it cannot prove empty, so it cannot be resumed until then"
		noProc = "the worker cannot prove the agent was never started, so it will not start another on this run; abandon it"
	)
	cases := []struct {
		name    string
		j       *Journal
		group   GroupState
		session bool
		refusal string
	}{
		{"running", journalAt(PhaseStarted, ""), GroupGone, true, "it is running, not held"},
		{"cancelling", journalAt(PhaseStarted, "", cancelled), GroupGone, true, "it is cancelling, not held"},
		{"settled succeeded", journalAt(PhaseSettled, "", ended(StatusSucceeded)), GroupGone, true, "it is succeeded, not held"},
		{"prepared", journalAt(PhasePrepared, ""), GroupGone, true, "it is prepared, not held"},
		{"held before admission", journalAt(PhaseSettled, HoldAdmissionRejected), GroupGone, true,
			"it was held (admission_rejected) before kata saw it and nothing ran, so there is nothing to continue; abandon it"},
		{"create rejected", journalAt(PhaseSettled, HoldCreateRejected), GroupGone, true,
			"it was held (create_rejected) before kata saw it and nothing ran, so there is nothing to continue; abandon it"},
		{"hold still being reported", journalAt(PhaseFinished, HoldInterrupted, launched), GroupGone, true,
			"its hold is still being reported to kata; try again in a moment"},
		{"launch unproven with no start record", journalAt(PhaseSettled, HoldLaunchUnproven), GroupGone, true, noProc},
		{"interrupted, group present", journalAt(PhaseSettled, HoldInterrupted, launched), GroupPresent, true, still},
		{"no receipt, group cannot be checked", journalAt(PhaseSettled, HoldNoReceipt, launched), GroupUnchecked, true, unchecked},
		{"group unproven, group present", journalAt(PhaseSettled, HoldGroupUnproven, launched), GroupPresent, true, still},
		{"interrupted, group gone, session kept", journalAt(PhaseSettled, HoldInterrupted, launched), GroupGone, true, ""},
		{"group unproven, group gone, session kept", journalAt(PhaseSettled, HoldGroupUnproven, launched), GroupGone, true, ""},
		{"interrupted, group gone, no session", journalAt(PhaseSettled, HoldInterrupted, launched), GroupGone, false,
			"there is no omp session to continue (interrupted); only abandon remains"},
		{"secret hold needs no group answer", journalAt(PhaseSettled, HoldSecretUnavailable), GroupPresent, true, ""},
		{"quota hold, session kept", journalAt(PhaseSettled, HoldAgentQuota, launched), GroupGone, true, ""},
		{"login hold, no session", journalAt(PhaseSettled, HoldAgentLogin, launched), GroupGone, false,
			"there is no omp session to continue (agent_login); only abandon remains"},
		{"execution error of a command step has no session to continue", journalAt(PhaseSettled, HoldExecutionError, onCommandStep), GroupGone, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lifeWith(c.group, c.session, false).mayResume(c.j); got != c.refusal {
				t.Errorf("mayResume = %q, want %q", got, c.refusal)
			}
		})
	}
}

func TestMayAbandon(t *testing.T) {
	const stillPresent = "the agent's process group 10 is still present; stop it first"
	const unchecked = "the agent's process group 10 cannot be checked: the job could not be opened; the worker keeps the run's slot until the check succeeds and will not free capacity it cannot prove empty, so it cannot be abandoned until then"
	const reporting = "its hold is still being reported to kata; try again in a moment"
	cases := []struct {
		name    string
		j       *Journal
		group   GroupState
		refusal string
	}{
		{"held before admission", journalAt(PhaseSettled, HoldAdmissionRejected), GroupPresent, ""},
		{"secret hold", journalAt(PhaseSettled, HoldSecretUnavailable), GroupPresent, ""},
		{"hold still being reported", journalAt(PhaseFinished, HoldInterrupted, launched), GroupGone, reporting},
		{"interrupted, group gone", journalAt(PhaseSettled, HoldInterrupted, launched), GroupGone, ""},
		{"interrupted, group present", journalAt(PhaseSettled, HoldInterrupted, launched), GroupPresent, stillPresent},
		{"no receipt, group cannot be checked", journalAt(PhaseSettled, HoldNoReceipt, launched), GroupUnchecked, unchecked},
		{"group unproven, group cannot be checked", journalAt(PhaseSettled, HoldGroupUnproven, launched), GroupUnchecked, unchecked},
		{"group unproven, group present", journalAt(PhaseSettled, HoldGroupUnproven, launched), GroupPresent, stillPresent},
		{"launch unproven with no start record", journalAt(PhaseSettled, HoldLaunchUnproven), GroupGone, ""},
		{"running is asked to stop", journalAt(PhaseStarted, "", launched), GroupPresent, ""},
		{"settled", journalAt(PhaseSettled, "", ended(StatusFailed)), GroupGone, "it is failed, not held"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lifeWith(c.group, true, false).mayAbandon(c.j); got != c.refusal {
				t.Errorf("mayAbandon = %q, want %q", got, c.refusal)
			}
		})
	}
}

func TestMayCancel(t *testing.T) {
	cases := []struct {
		name    string
		j       *Journal
		refusal string
	}{
		{"started", journalAt(PhaseStarted, "", launched), ""},
		{"admitted", journalAt(PhaseAdmitted, ""), ""},
		{"launching", journalAt(PhaseLaunching, ""), ""},
		{"already asked", journalAt(PhaseStarted, "", launched, cancelled), "ann already asked to cancel it"},
		{"settled", journalAt(PhaseSettled, "", ended(StatusSucceeded)), "it is succeeded, not held"},
		{"finished", journalAt(PhaseFinished, "", ended(StatusFailed)), "it is failed, not held"},
		{"notify job", journalAt(PhaseAdmitted, "", func(j *Journal) { j.Notify = &NotifySpec{} }), "its notification is being sent and it ends in a moment"},
		{"prepared", journalAt(PhasePrepared, ""), "it is still being admitted; try again in a moment"},
		{"resuming", journalAt(PhaseResuming, ""), "its resume is still being confirmed; try again in a moment"},
		{"process ended", journalAt(PhaseStarted, "", launched, exited), "its process has already ended and its outcome is being decided"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.j.mayCancel(); got != c.refusal {
				t.Errorf("mayCancel = %q, want %q", got, c.refusal)
			}
		})
	}
}

func TestStateName(t *testing.T) {
	cases := []struct {
		name  string
		j     *Journal
		state string
	}{
		{"prepared", journalAt(PhasePrepared, ""), "prepared"},
		{"admitted", journalAt(PhaseAdmitted, ""), "admitted"},
		{"resuming", journalAt(PhaseResuming, ""), "resuming"},
		{"launching", journalAt(PhaseLaunching, ""), "running"},
		{"started", journalAt(PhaseStarted, ""), "running"},
		{"cancelling", journalAt(PhaseStarted, "", cancelled), "cancelling"},
		{"finished with an outcome", journalAt(PhaseFinished, "", ended(StatusFailed)), "failed"},
		{"settled", journalAt(PhaseSettled, "", ended(StatusSucceeded)), "succeeded"},
		{"cancelled", journalAt(PhaseSettled, "", ended(StatusCancelled)), "cancelled"},
		{"held", journalAt(PhaseSettled, HoldNoReceipt), "held: no_receipt"},
		{"held before admission", journalAt(PhaseSettled, HoldAdmissionRejected), "held: admission_rejected"},
		{"held, cancel recorded", journalAt(PhaseFinished, HoldInterrupted, cancelled), "held: interrupted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.j.stateName(); got != c.state {
				t.Errorf("stateName = %q, want %q", got, c.state)
			}
		})
	}
}

func TestDriveStep(t *testing.T) {
	cases := []struct {
		phase     Phase
		step      driveStep
		retryable bool
		report    bool
	}{
		{PhasePrepared, driveAdmit, true, false},
		{PhaseAdmitted, driveLaunch, true, false},
		{PhaseResuming, driveReopen, true, false},
		{PhaseFinished, driveReport, true, true},
		{PhaseLaunching, driveRecover, false, false},
		{PhaseStarted, driveRecover, false, false},
		{PhaseSettled, driveNone, false, false},
	}
	for _, c := range cases {
		t.Run(string(c.phase), func(t *testing.T) {
			j := journalAt(c.phase, "")
			if j.driveStep() != c.step || j.retryable() != c.retryable || j.reportDue() != c.report {
				t.Errorf("driveStep %v retryable %v reportDue %v", j.driveStep(), j.retryable(), j.reportDue())
			}
		})
	}
}

func TestRecover(t *testing.T) {
	const agentRan = "the worker stopped while the agent ran"
	cases := []struct {
		name         string
		j            *Journal
		group        GroupState
		finishedTurn bool
		want         recovery
	}{
		{"prepared continues", journalAt(PhasePrepared, ""), GroupGone, false, recovery{Step: recoverContinue}},
		{"admitted continues", journalAt(PhaseAdmitted, ""), GroupGone, false, recovery{Step: recoverContinue}},
		{"resuming continues", journalAt(PhaseResuming, ""), GroupGone, false, recovery{Step: recoverContinue}},
		{"finished continues", journalAt(PhaseFinished, "", ended(StatusSucceeded)), GroupGone, false, recovery{Step: recoverContinue}},
		{"launching is held launch unproven", journalAt(PhaseLaunching, ""), GroupGone, false,
			recovery{recoverHold, HoldLaunchUnproven, "the worker stopped while starting the agent, so it may have started"}},
		{"launching a command", journalAt(PhaseLaunching, "", onCommandStep), GroupGone, false, recovery{recoverHold, HoldLaunchUnproven, "the worker stopped while starting the command, so it may have started"}},
		{"started with an exit status is concluded", journalAt(PhaseStarted, "", launched, exited), GroupPresent, false, recovery{Step: recoverConclude}},
		{"cancel in flight, group gone", journalAt(PhaseStarted, "", launched, cancelled), GroupGone, false, recovery{Step: recoverConcludeCancel}},
		{"cancel in flight, group present", journalAt(PhaseStarted, "", launched, cancelled), GroupPresent, false,
			recovery{recoverHold, HoldGroupUnproven, "the agent's process group 10 is still present"}},
		{"cancel in flight, group cannot be checked", journalAt(PhaseStarted, "", launched, cancelled), GroupUnchecked, false,
			recovery{recoverHold, HoldGroupUnproven, "the agent's process group 10 cannot be checked: the job could not be opened; the worker keeps the run's slot until the check succeeds and will not free capacity it cannot prove empty"}},
		{"started, group present", journalAt(PhaseStarted, "", launched), GroupPresent, true,
			recovery{recoverHold, HoldGroupUnproven, "the agent's process group 10 is still present"}},
		{"started, group cannot be checked", journalAt(PhaseStarted, "", launched), GroupUnchecked, false,
			recovery{recoverHold, HoldGroupUnproven, "the agent's process group 10 cannot be checked: the job could not be opened; the worker keeps the run's slot until the check succeeds and will not free capacity it cannot prove empty"}},
		{"started, gone, turn finished", journalAt(PhaseStarted, "", launched), GroupGone, true,
			recovery{recoverHold, HoldNoReceipt, "the agent finished its turn and its process is gone, but the worker saved no exit status"}},
		{"started, gone, turn unfinished", journalAt(PhaseStarted, "", launched), GroupGone, false, recovery{recoverHold, HoldInterrupted, agentRan}},
		{"command step, gone, never a finished turn", journalAt(PhaseStarted, "", launched, onCommandStep), GroupGone, true, recovery{recoverHold, HoldInterrupted, "the worker stopped while the command ran"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lifeWith(c.group, true, c.finishedTurn).recover(c.j); got != c.want {
				t.Errorf("recover = %+v, want %+v", got, c.want)
			}
		})
	}
}

// Every hold class has an answer to every question for every group answer: none panics and none is inconsistent.
func TestEveryHoldHasConsistentAnswers(t *testing.T) {
	for _, hold := range allHolds {
		for _, state := range []GroupState{GroupGone, GroupPresent, GroupUnchecked} {
			for _, phase := range []Phase{PhaseFinished, PhaseSettled} {
				j := journalAt(phase, hold, launched)
				l := lifeWith(state, true, false)
				if !j.held() || j.owes() {
					t.Errorf("%s: a hold is held and owes nothing", hold)
				}
				if l.occupiesSlot(j) && j.settled() && j.holdClass() == holdBeforeAdmission {
					t.Errorf("%s: a hold from before admission keeps its slot", hold)
				}
				if j.settled() && j.holdClass() == holdBeforeAdmission && j.closesGate() {
					t.Errorf("%s: a hold from before admission keeps its gate", hold)
				}
				if j.holdClass() == holdUncertain && !j.closesGate() {
					t.Errorf("%s: an uncertainty hold frees its gate", hold)
				}
			}
		}
	}
}

// A group that cannot be checked is logged once per run per change of reason; any other answer forgets the reason.
func TestUncheckedLogSaysEachReasonOncePerRun(t *testing.T) {
	var u uncheckedLog
	cannot := func(reason string) GroupProof { return GroupProof{State: GroupUnchecked, Reason: reason} }
	steps := []struct {
		run   string
		proof GroupProof
		said  bool
	}{
		{"a", cannot("open failed"), true},
		{"a", cannot("open failed"), false},
		{"b", cannot("open failed"), true},
		{"a", cannot("query failed"), true},
		{"a", cannot("query failed"), false},
		{"a", GroupProof{State: GroupPresent}, false},
		{"a", cannot("query failed"), true},
		{"a", GroupProof{State: GroupGone}, false},
	}
	for i, s := range steps {
		if got := u.changed(s.run, s.proof); got != s.said {
			t.Errorf("step %d (%s, %+v): said = %v, want %v", i, s.run, s.proof, got, s.said)
		}
	}
}
