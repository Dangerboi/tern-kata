package worker

import "fmt"

// The Run lifecycle (ADR-0005, ADR-0006, ADR-0007): every question the Worker asks about a Run is answered here, from
// its Journal and, where liveness matters, from the group proof of its process. Nothing outside this file (and the
// Journal's own storage) compares a Journal's Phase or its Outcome's hold. The questions that need no outside fact are
// methods of the Journal; the ones that need the group, a saved session or the agent's saved output are methods of
// lifecycle, whose probes the caller supplies, so a test can give any answer.

// holdClass says what kind of hold a Run is in (ADR-0005).
type holdClass int

const (
	// notHeld: the Outcome names no hold reason.
	notHeld holdClass = iota
	// holdBeforeAdmission: the Run ended before kata saw it (no Outcome status). Nothing ran, it never resumes, and
	// it frees its slot and its job's gate at once.
	holdBeforeAdmission
	// holdUncertain: the Outcome is `unknown`. The gate stays closed until the user resumes or abandons the Run, and
	// the slot stays only while a process may be alive.
	holdUncertain
)

// holdClass is the class of the Run's hold, notHeld for a Run without one.
func (j *Journal) holdClass() holdClass {
	switch {
	case j.Outcome == nil || j.Outcome.Hold == "":
		return notHeld
	case j.Outcome.Status == "":
		return holdBeforeAdmission
	}
	return holdUncertain
}

// settled is whether the Run's Phase is final: every Issue write and the final observation have been sent.
func (j *Journal) settled() bool { return j.Phase == PhaseSettled }

// held is whether the Run waits for the user.
func (j *Journal) held() bool { return j.holdClass() != notHeld }

// owes is whether the Run is accepted, not held and not yet settled, so a report or its settlement is still due. An
// idle stop waits for such a Run. A held Run never owes, even while its hold is still being reported: the Project loop
// retries that report but a drain does not wait for it (ADR-0005: held runs are not in flight).
func (j *Journal) owes() bool { return !j.settled() && !j.held() }

// closesGate is whether the Run closes its job's overlap gate (ADR-0007): it is not settled, or it settled as a hold
// that needs the user. A settled Run, and a hold from before admission, close none.
func (j *Journal) closesGate() bool { return !j.settled() || j.holdClass() == holdUncertain }

// reportDue is whether the Run's process is gone and only its reports to kata remain.
func (j *Journal) reportDue() bool { return j.Phase == PhaseFinished }

// driveStep is the work a goroutine driving the Run does next.
type driveStep int

const (
	// driveNone: the Run is settled.
	driveNone driveStep = iota
	driveAdmit
	driveReopen
	driveLaunch
	driveReport
	// driveRecover: a launching or started Run, which only recovery may classify.
	driveRecover
)

// driveStep is the step that carries the Run on from its Phase.
func (j *Journal) driveStep() driveStep {
	switch j.Phase {
	case PhasePrepared:
		return driveAdmit
	case PhaseResuming:
		return driveReopen
	case PhaseAdmitted:
		return driveLaunch
	case PhaseFinished:
		return driveReport
	case PhaseSettled:
		return driveNone
	}
	return driveRecover
}

// retryable is whether the Project loop's pass may hand the Run to a goroutine: a transient kata failure left it
// half-admitted or half-reported. A launching or started Run is left to recovery.
func (j *Journal) retryable() bool {
	s := j.driveStep()
	return s != driveNone && s != driveRecover
}

// stateName is the Run's state as `worker status` and the answers to requests show it: held with its reason, the
// Outcome's status, cancelling, running, or the Phase's name.
func (j *Journal) stateName() string {
	switch {
	case j.held():
		return "held: " + j.Outcome.Hold
	case j.Outcome != nil:
		return j.Outcome.Status
	case j.Cancel != nil:
		return "cancelling"
	case j.Phase == PhaseStarted, j.Phase == PhaseLaunching:
		return "running"
	}
	return string(j.Phase)
}

// mayCancel is why a Run that is not held cannot be asked to stop now, empty when it can: it is admitted or running
// with its process not yet reported over, and nobody asked yet.
func (j *Journal) mayCancel() string {
	switch {
	case j.Cancel != nil:
		return fmt.Sprintf("%s already asked to cancel it", j.Cancel.By)
	case j.Outcome != nil || j.Phase == PhaseFinished || j.Phase == PhaseSettled:
		return "it is " + j.stateName() + ", not held"
	case j.Notify != nil:
		return "its notification is being sent and it ends in a moment"
	case j.Phase == PhasePrepared:
		return "it is still being admitted; try again in a moment"
	case j.Phase == PhaseResuming:
		return "its resume is still being confirmed; try again in a moment"
	case j.Exit != nil:
		return "its process has already ended and its outcome is being decided"
	}
	return ""
}

// lifecycle answers the questions that need an outside fact. Group is the group proof of a launch, SessionKnown whether
// the held attempt left an omp session to continue and FinishedTurn whether its saved agent output shows a finished
// turn; a probe a question never needs may be nil.
type lifecycle struct {
	Group        func(*LaunchRecord) GroupProof
	SessionKnown func(*Journal) bool
	FinishedTurn func(*Journal) bool
}

// life is the lifecycle with the Worker's own probes, for Run j: its group answers are logged once per change of reason.
func (w *Worker) life(j *Journal) lifecycle {
	return lifecycle{
		Group: func(l *LaunchRecord) GroupProof {
			proof := w.proveGroup(l)
			w.noteGroup(j.RunUID, l, proof)
			return proof
		},
		SessionKnown: func(j *Journal) bool { return w.sessionID(j) != "" },
		FinishedTurn: w.finishedItsTurn,
	}
}

// mayBeAlive is the settlement proof for a Run the Worker spawned: its recorded process group is not proven gone
// (ADR-0006). With no start record the process cannot be ruled out when the Run was launching or the hold says the
// launch is unproven.
func (l lifecycle) mayBeAlive(j *Journal) bool {
	alive, _ := l.groupAnswer(j)
	return alive
}

// groupAnswer is mayBeAlive with the group proof it rests on: a Run with no start record has no proof to show (the
// zero GroupProof), a started one has the platform's answer, which may be that the group cannot be checked.
func (l lifecycle) groupAnswer(j *Journal) (alive bool, proof GroupProof) {
	if j.Launch == nil {
		return j.Phase == PhaseLaunching || (j.Outcome != nil && j.Outcome.Hold == HoldLaunchUnproven), GroupProof{}
	}
	proof = l.Group(j.Launch)
	return !proof.Gone(), proof
}

// groupLine says why a Run's group is not proven gone, for the sentence a refusal or hold detail builds around it:
// "process group N is still present" or "process group N cannot be checked: <reason>". A group that cannot be checked
// is never called present: the Worker does not know, and says what it does about not knowing (ADR-0005).
func groupLine(who string, pgid int, proof GroupProof) string {
	if proof.State == GroupUnchecked {
		return fmt.Sprintf("%s's process group %d cannot be checked: %s", who, pgid, proof.Reason)
	}
	return fmt.Sprintf("%s's process group %d is still present", who, pgid)
}

// holdLine is groupLine as a hold detail: for a group that cannot be checked it adds what the Worker keeps.
func holdLine(who string, pgid int, proof GroupProof) string {
	line := groupLine(who, pgid, proof)
	if proof.State == GroupUnchecked {
		line += "; " + uncheckedKeeps
	}
	return line
}

// exitedHold is the hold detail of a group not proven gone after its process ended.
func exitedHold(what string, pgid int, proof GroupProof) string {
	if proof.State == GroupUnchecked {
		return fmt.Sprintf("process group %d cannot be checked after the %s exited: %s; %s", pgid, what, proof.Reason, uncheckedKeeps)
	}
	return fmt.Sprintf("process group %d is still present after the %s exited", pgid, what)
}

// uncheckedKeeps is what the Worker does while a group cannot be checked, appended to every refusal and hold detail
// that names one.
const uncheckedKeeps = "the worker keeps the run's slot until the check succeeds and will not free capacity it cannot prove empty"

// occupiesSlot is whether the Run still takes one of its Project's slots (ADR-0007): it is in flight, or it is a held
// Run of a class whose process may be alive and its group is not proven gone. A hold from before admission and a
// hold of the secret, prompt or command kind free the slot once settled.
func (l lifecycle) occupiesSlot(j *Journal) bool {
	switch j.Phase {
	case PhasePrepared, PhaseAdmitted, PhaseResuming, PhaseLaunching, PhaseStarted:
		return true
	}
	if j.Outcome != nil {
		switch j.Outcome.Hold {
		case HoldLaunchUnproven, HoldInterrupted, HoldNoReceipt, HoldGroupUnproven:
			return l.mayBeAlive(j)
		}
	}
	return !j.settled()
}

// mayResume is why a Run cannot be resumed, empty when it can (the lease, Issue and capacity are the caller's to check).
func (l lifecycle) mayResume(j *Journal) string {
	switch {
	case !j.held():
		return "it is " + j.stateName() + ", not held"
	case j.holdClass() == holdBeforeAdmission:
		return fmt.Sprintf("it was held (%s) before kata saw it and nothing ran, so there is nothing to continue; abandon it", j.Outcome.Hold)
	case !j.settled():
		return "its hold is still being reported to kata; try again in a moment"
	}
	if alive, proof := l.groupAnswer(j); alive {
		switch {
		case j.Launch == nil:
			return "the worker cannot prove the agent was never started, so it will not start another on this run; abandon it"
		case proof.State == GroupUnchecked:
			return groupLine("the agent", j.Launch.PGID, proof) + "; " + uncheckedKeeps + ", so it cannot be resumed until then"
		}
		return fmt.Sprintf("the agent's process group %d is still present; the worker never stops a process to make a resume possible", j.Launch.PGID)
	}
	if !j.activeIsCommand() && !l.SessionKnown(j) {
		return fmt.Sprintf("there is no omp session to continue (%s); only abandon remains", j.Outcome.Hold)
	}
	return ""
}

// mayAbandon is why a Run cannot be abandoned, empty when it can. A held Run is abandoned once its hold is reported
// and its recorded group is not present; a Run that is not held is asked to stop, which mayCancel decides.
func (l lifecycle) mayAbandon(j *Journal) string {
	if !j.held() {
		return j.mayCancel()
	}
	if !j.settled() {
		return "its hold is still being reported to kata; try again in a moment"
	}
	if j.Launch == nil {
		return ""
	}
	if alive, proof := l.groupAnswer(j); alive {
		if proof.State == GroupUnchecked {
			return groupLine("the agent", j.Launch.PGID, proof) + "; " + uncheckedKeeps + ", so it cannot be abandoned until then"
		}
		return fmt.Sprintf("the agent's process group %d is still present; stop it first", j.Launch.PGID)
	}
	return ""
}

// recoveryStep is what recovery does with a Journal found at start.
type recoveryStep int

const (
	// recoverContinue: nothing to classify; hand the Run to a goroutine.
	recoverContinue recoveryStep = iota
	// recoverHold: hold the Run with the recovery's Hold and Detail.
	recoverHold
	// recoverConcludeCancel: a cancel was in flight and the process is provably gone; end the Run cancelled.
	recoverConcludeCancel
	// recoverConclude: the process ended and its exit status is saved; decide the outcome.
	recoverConclude
)

// recovery is what recovery makes of a Journal: the step and, for recoverHold, the hold reason and its detail.
type recovery struct {
	Step   recoveryStep
	Hold   string
	Detail string
}

// recover classifies a Journal found at start (ADR-0005, ADR-0006). A launch with no start record may have delivered
// the prompt, and a start record with no exit status means the Worker stopped while the process ran; neither is ever
// repeated. Everything else just continues.
func (l lifecycle) recover(j *Journal) recovery {
	what := "agent"
	if j.activeIsCommand() {
		what = "command"
	}
	switch {
	case j.Phase == PhaseLaunching:
		return recovery{recoverHold, HoldLaunchUnproven, "the worker stopped while starting the " + what + ", so it may have started"}
	case j.Phase != PhaseStarted:
		return recovery{Step: recoverContinue}
	case j.Exit != nil:
		return recovery{Step: recoverConclude}
	}
	alive, proof := l.groupAnswer(j)
	switch {
	case j.Cancel != nil && !alive:
		return recovery{Step: recoverConcludeCancel}
	case alive:
		return recovery{recoverHold, HoldGroupUnproven, holdLine("the "+what, j.Launch.PGID, proof)}
	case what == "agent" && l.FinishedTurn(j):
		return recovery{recoverHold, HoldNoReceipt, "the agent finished its turn and its process is gone, but the worker saved no exit status"}
	}
	return recovery{recoverHold, HoldInterrupted, "the worker stopped while the " + what + " ran"}
}
