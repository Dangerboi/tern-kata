package driver_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// The Worker card: W on the Jobs Page opens it; it tells what this machine's worker is and, where something is missing, sets
// the worker up or serves the Project.

var (
	noWorkerFile   = model.WorkerSetup{Project: "work", Checkout: "/repo", Path: "/cfg/tern-kata/worker.toml"}
	brokenFile     = model.WorkerSetup{Project: "work", Checkout: "/repo", Path: "/cfg/tern-kata/worker.toml", Exists: true, LoadErr: `worker.toml: unknown key bogus`}
	otherProject   = model.WorkerSetup{Project: "work", Checkout: "/repo", Path: "/cfg/tern-kata/worker.toml", Exists: true, Executor: "laptop"}
	listedProject  = model.WorkerSetup{Project: "work", Checkout: "/repo", Path: "/cfg/tern-kata/worker.toml", Exists: true, Executor: "laptop", Lists: true, Checkouts: []string{"/repo"}}
	unboundFolder  = model.WorkerSetup{Path: "/cfg/tern-kata/worker.toml"}
	askedSetup     = model.LoadWorkerSetup{Workspace: "/repo"}
	supervisedHere = &model.WorkerStatus{Executor: "laptop", PID: 4242, Supervised: true, Projects: []model.WorkerProject{{Name: "work", Admitting: true, Capacity: 1}}}
)

func manual(s model.WorkerSetup) model.WorkerSetup { s.Manual = true; return s }

// workerCardOpen is the Jobs Page with the Worker card opened by W and its two reads answered.
func workerCardOpen(t *testing.T, status *model.WorkerStatus, setup model.WorkerSetup) driver.State {
	t.Helper()
	s := jobsPaneWithWorker(t, status, promptJob("A1", "Triage"))
	s.Model.Host, s.Model.Actor = "Laptop.local", "dana"
	out := step(t, s, key("shift+w"))
	if !slices.Contains(out.Cmds, model.Cmd(askedSetup)) {
		t.Fatalf("W asked %#v, want a read of the worker setup", out.Cmds)
	}
	out.State = step(t, out.State, model.WorkerSetupLoaded{Setup: setup}).State
	return answerWorker(t, out, status).State
}

func workerField(t *testing.T, s driver.State, name model.Field) string {
	t.Helper()
	text, ok := nodeText(t, s, view.WorkerFieldID(name))
	if !ok {
		t.Fatalf("the Worker card draws no %s field", name)
	}
	return text
}

func TestWOpensTheWorkerCardOnTheJobsPageAndItsInspector(t *testing.T) {
	s := jobsPaneWithWorker(t, nil, promptJob("A1", "Triage"))
	for _, name := range []string{"list", "inspector"} {
		from := s
		if name == "inspector" {
			from = step(t, s, key("enter")).State
			if from.Model.CurrentMode() != model.ModeJobInspector {
				t.Fatalf("mode %s, want the inspector", from.Model.CurrentMode())
			}
		}
		out := step(t, from, key("shift+w"))
		if out.State.Model.TopCard() != model.CardWorker || out.State.Model.CurrentMode() != model.ModeWorkerCard {
			t.Errorf("%s: card %v mode %s", name, out.State.Model.TopCard(), out.State.Model.CurrentMode())
		}
	}
	if off := step(t, projectPane(t, 160, boardCard("a")), key("shift+w")); off.State.Model.TopCard() != model.NoCard {
		t.Errorf("W opened %v off the Jobs Page", off.State.Model.TopCard())
	}
}

func TestWorkerCardStatesAndWhatEachOffers(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status *model.WorkerStatus
		setup  model.WorkerSetup
		want   []string
		field  bool // the setup fields are drawn
		trust  bool // the trust choice is drawn
	}{
		{"no file", nil, noWorkerFile, []string{"none yet", "Project: work", "checkout /repo", "not running", "the plugin starts a worker once a worker.toml loads"}, true, true},
		{"file that does not load", nil, brokenFile, []string{"does not load: worker.toml: unknown key bogus"}, false, false},
		{"file without the Project", nil, otherProject, []string{"loads, and does not list work"}, false, true},
		{"file with the Project, worker supervised", supervisedHere, listedProject, []string{"loads, lists work with checkout /repo", "process 4242", "the plugin supervises it"}, false, false},
		{"worker.manual", nil, manual(listedProject), []string{"worker.manual is present: the plugin neither starts nor restarts"}, false, false},
		{"worker started by hand", &model.WorkerStatus{Executor: "laptop", PID: 7}, listedProject, []string{"started by hand"}, false, false},
		{"folder bound to no Project", nil, unboundFolder, []string{"not bound to a kata Project", "kata init"}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := workerCardOpen(t, tt.status, tt.setup)
			wantText(t, "card", wireTextOf(t, s), tt.want...)
			_, executor := nodeText(t, s, view.WorkerFieldID(model.WorkerFieldExecutor))
			if executor != tt.field {
				t.Errorf("setup fields drawn = %v, want %v", executor, tt.field)
			}
			trust := slices.ContainsFunc(wireNodes(t, view.Render(s.Model, now)), func(n tern.WireNode) bool { return n.ID == view.WorkerFieldID(model.WorkerFieldTrust) })
			if trust != tt.trust {
				t.Errorf("trust choice drawn = %v, want %v", trust, tt.trust)
			}
		})
	}
}

func TestWorkerCardSetUpFieldsArePrefilledAsWorkerInitWould(t *testing.T) {
	s := workerCardOpen(t, nil, noWorkerFile)
	for field, want := range map[model.Field]string{
		model.WorkerFieldExecutor: "laptop-local", model.WorkerFieldActor: "dana", model.WorkerFieldCommand: "omp",
		model.WorkerFieldModel: "", model.WorkerFieldThinking: "",
	} {
		if got := workerField(t, s, field); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if !strings.HasPrefix(s.Focus, view.WorkerID) {
		t.Errorf("text focus %q is not on the Worker card", s.Focus)
	}
	wantText(t, "card", wireTextOf(t, s), "checkout /repo", "Anyone who can write jobs")
}

func TestWorkerCardSaveIsRefusedUntilTheTrustChoiceIsYesAndKeepsTheInput(t *testing.T) {
	s := workerCardOpen(t, nil, noWorkerFile)
	s = typed(t, s, "x")
	refused := step(t, s, key("ctrl+o"))
	if len(refused.Cmds) != 0 || refused.State.Model.TopCard() != model.CardWorker {
		t.Fatalf("cmds %v card %v, want a refusal with the card open", refused.Cmds, refused.State.Model.TopCard())
	}
	msg, ok := nodeText(t, refused.State, view.WorkerID+".error")
	if !ok || !strings.Contains(msg, "Trust") {
		t.Errorf("error = %q (drawn %v), want it to name Trust", msg, ok)
	}
	if got := workerField(t, refused.State, model.WorkerFieldExecutor); got != "laptop-localx" {
		t.Errorf("a refusal changed the executor to %q", got)
	}
	if refused.State.Model.WorkerCard.Field != model.WorkerFieldTrust {
		t.Errorf("focus %q, want it on the trust choice", refused.State.Model.WorkerCard.Field)
	}

	trusted := step(t, refused.State, model.ChooseMsg{Field: model.WorkerFieldTrust, Index: 1}).State
	out := step(t, trusted, key("ctrl+o"))
	want := model.SaveWorkerSetup{Workspace: "/repo", Trusted: true, Executor: "laptop-localx", DefaultActor: "dana", Command: ""}
	if !reflect.DeepEqual(out.Cmds, []model.Cmd{want}) {
		t.Fatalf("cmds %#v, want exactly %#v", out.Cmds, want)
	}
}

func TestWorkerCardSendsWhatWasTypedAndOnlyWhatDiffersFromTheDefaults(t *testing.T) {
	s := workerCardOpen(t, nil, noWorkerFile)
	s = step(t, s, key("ctrl+u")).State
	s = typed(t, s, "desk")
	s = step(t, s, key("tab")).State
	s = step(t, s, key("ctrl+u")).State
	s = typed(t, s, "bot")
	s = step(t, s, model.ChooseMsg{Field: model.WorkerFieldTrust, Index: 1}).State
	for _, f := range []model.Field{model.WorkerFieldModel, model.WorkerFieldThinking} {
		s = step(t, s, model.FocusFieldMsg{Field: f}).State
		s = typed(t, s, map[model.Field]string{model.WorkerFieldModel: "haiku", model.WorkerFieldThinking: "low"}[f])
	}
	out := step(t, s, key("ctrl+o"))
	want := model.SaveWorkerSetup{Workspace: "/repo", Trusted: true, Executor: "desk", Actor: "bot", DefaultActor: "dana", Model: "haiku", Thinking: "low"}
	if !reflect.DeepEqual(out.Cmds, []model.Cmd{want}) {
		t.Fatalf("cmds %#v, want exactly %#v", out.Cmds, want)
	}
}

func TestWorkerCardRefusesABlankExecutorNamingTheField(t *testing.T) {
	s := workerCardOpen(t, nil, noWorkerFile)
	s = step(t, s, model.ChooseMsg{Field: model.WorkerFieldTrust, Index: 1}).State
	s = step(t, s, model.FocusFieldMsg{Field: model.WorkerFieldExecutor}).State
	s = step(t, s, key("ctrl+u")).State
	out := step(t, s, key("ctrl+o"))
	msg, _ := nodeText(t, out.State, view.WorkerID+".error")
	if len(out.Cmds) != 0 || !strings.Contains(msg, "Executor") || out.State.Model.WorkerCard.Field != model.WorkerFieldExecutor {
		t.Fatalf("cmds %v error %q field %q, want a refusal naming Executor with focus there", out.Cmds, msg, out.State.Model.WorkerCard.Field)
	}
}

func TestWorkerCardServeNeedsOnlyTheTrustChoiceAndAddsTheProject(t *testing.T) {
	s := workerCardOpen(t, nil, otherProject)
	wantText(t, "card", wireTextOf(t, s), "Serving lists work with checkout /repo")
	if s.Model.WorkerCard.Field != model.WorkerFieldTrust {
		t.Fatalf("focus %q, want the trust choice, the only field", s.Model.WorkerCard.Field)
	}
	if out := step(t, s, key("ctrl+o")); len(out.Cmds) != 0 {
		t.Fatalf("saved without trust: %#v", out.Cmds)
	}
	s = step(t, s, key("space")).State
	out := step(t, s, key("ctrl+o"))
	if want := (model.SaveWorkerSetup{Workspace: "/repo", Serve: true, Trusted: true}); !reflect.DeepEqual(out.Cmds, []model.Cmd{want}) {
		t.Fatalf("cmds %#v, want exactly %#v", out.Cmds, want)
	}
}

func TestWorkerCardRefusedWriteKeepsTheInputAndFocusesTheNamedField(t *testing.T) {
	s := workerCardOpen(t, nil, noWorkerFile)
	s = step(t, s, key("ctrl+u")).State
	s = typed(t, s, "bad-name")
	s = step(t, s, model.ChooseMsg{Field: model.WorkerFieldTrust, Index: 1}).State
	sent := step(t, s, key("ctrl+o"))
	refused := step(t, sent.State, model.WorkerSetupSaved{Err: errors.New("worker.toml: executor must be at most 256 bytes with no whitespace")})
	msg, _ := nodeText(t, refused.State, view.WorkerID+".error")
	card := refused.State.Model.WorkerCard
	if card.Saving || !strings.Contains(msg, "executor must be") || card.Field != model.WorkerFieldExecutor || card.Executor != "bad-name" || !card.Trusted {
		t.Fatalf("saving %v error %q field %q executor %q trusted %v, want the input kept and the executor focused", card.Saving, msg, card.Field, card.Executor, card.Trusted)
	}
}

func TestWorkerCardAfterASaveWithNoWorkerSaysThePluginStartsOne(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup model.WorkerSetup
		want  string
	}{
		{"supervised", noWorkerFile, "starts the worker within 10 seconds"},
		{"worker.manual", manual(noWorkerFile), "worker.manual is present, so the plugin does not start the worker"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := workerCardOpen(t, nil, tt.setup)
			s = step(t, s, model.ChooseMsg{Field: model.WorkerFieldTrust, Index: 1}).State
			sent := step(t, s, key("ctrl+o"))
			done := step(t, sent.State, model.WorkerSetupSaved{Project: "work", Key: "work"})
			wantText(t, "card", wireTextOf(t, done.State), "worker.toml written (checkout work)", tt.want)
			if done.State.Model.CurrentMode() != model.ModeWorkerCard || done.State.Model.WorkerCard.Restart {
				t.Errorf("mode %s restart %v, want the plain card", done.State.Model.CurrentMode(), done.State.Model.WorkerCard.Restart)
			}
			if !slices.Contains(done.Cmds, model.Cmd(askedSetup)) {
				t.Errorf("cmds %#v, want the setup read again", done.Cmds)
			}
			if closed := step(t, done.State, key("escape")); closed.State.Model.TopCard() != model.NoCard {
				t.Errorf("esc left the card open")
			}
		})
	}
}

func TestWorkerCardAfterASaveWithAWorkerRunningOffersARestartThatSendsTheStopRequest(t *testing.T) {
	running := &model.WorkerStatus{Executor: "laptop", PID: 9, Supervised: true, InFlight: 1,
		Runs: []model.LocalRun{{UID: "R1", Project: "work", Job: "Triage", State: "running"}}}
	saved := func(t *testing.T) driver.State {
		s := workerCardOpen(t, running, otherProject)
		s = step(t, s, key("space")).State
		sent := step(t, s, key("ctrl+o"))
		return step(t, sent.State, model.WorkerSetupSaved{Serve: true, Project: "work", Key: "work"}).State
	}
	s := saved(t)
	if s.Model.CurrentMode() != model.ModeWorkerRestart {
		t.Fatalf("mode %s, want the restart question", s.Model.CurrentMode())
	}
	wantText(t, "card", wireTextOf(t, s), "Restart when idle", "Restart now", "1 run in flight (Triage)", "held `interrupted`", "reads worker.toml only at start")

	idle := step(t, s, key("i"))
	if !reflect.DeepEqual(idle.Cmds, []model.Cmd{model.StopWorker{Mode: "idle"}}) {
		t.Fatalf("i sent %#v, want a stop request when idle", idle.Cmds)
	}
	if enter := step(t, s, key("enter")); !reflect.DeepEqual(enter.Cmds, idle.Cmds) {
		t.Errorf("enter sent %#v, want the default, restart when idle", enter.Cmds)
	}
	now := step(t, s, key("n"))
	if !reflect.DeepEqual(now.Cmds, []model.Cmd{model.StopWorker{Mode: "now"}}) {
		t.Fatalf("n sent %#v, want a stop request now", now.Cmds)
	}

	done := step(t, idle.State, model.WorkerStopped{Mode: "idle", InFlight: 1, Restarts: true})
	wantText(t, "notice", done.State.Model.Notice, "finishes its 1 run in flight", "plugin then starts a fresh one")
	if done.State.Model.TopCard() != model.NoCard {
		t.Errorf("the card stayed open after the stop")
	}
	unsupervised := step(t, now.State, model.WorkerStopped{Mode: "now"})
	wantText(t, "notice", unsupervised.State.Model.Notice, "ending now", "held `interrupted`", "nothing starts it again")

	refused := step(t, idle.State, model.WorkerStopped{Mode: "idle", Err: errors.New("the worker is stopping")})
	if refused.State.Model.TopCard() != model.CardWorker || refused.State.Model.WorkerCard.Err == nil {
		t.Errorf("a refused stop closed the card or hid the reason")
	}
	left := step(t, s, key("escape"))
	if left.State.Model.TopCard() != model.NoCard || len(left.Cmds) != 0 {
		t.Errorf("esc: card %v cmds %v, want it closed with no stop", left.State.Model.TopCard(), left.Cmds)
	}
}

func TestJobsPageBannerPerWorkerState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status *model.WorkerStatus
		setup  model.WorkerSetup
		want   string
	}{
		{"no file", nil, noWorkerFile, "No worker is set up on this machine: press W to set one up for work"},
		{"file does not load", nil, brokenFile, "worker.toml does not load, so no worker can start: press W"},
		{"file without the Project", nil, otherProject, "worker.toml does not list work, so no worker serves it: press W to serve it"},
		{"manual", nil, manual(listedProject), "worker.manual keeps the plugin from starting one"},
		{"plugin starts it", nil, listedProject, "the plugin starts one within 10 seconds (W shows the setup)"},
		{"running and serving", servingWorker(), listedProject, "running and serving work: 1 of 2 slots in use (W: worker)"},
		{"running without the Project", &model.WorkerStatus{Executor: "laptop"}, otherProject, "does not serve work; press W to serve it, then restart the worker"},
		{"running, file lists it", &model.WorkerStatus{Executor: "laptop"}, listedProject, "though worker.toml lists it; restart the worker"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := jobsPaneWithWorker(t, tt.status, promptJob("A1", "Triage"))
			s = step(t, s, model.WorkerSetupLoaded{Setup: tt.setup}).State
			wantText(t, "Jobs Page", wireTextOf(t, s), tt.want)
		})
	}
}

func TestJobsPageReadsTheWorkerSetupAlongsideTheWorkerStatus(t *testing.T) {
	out := step(t, projectPane(t, 160, boardCard("parent-a")), key("3"))
	if !slices.Contains(out.Cmds, model.Cmd(askedSetup)) {
		t.Fatalf("cmds %#v, want the setup read with the Jobs Page", out.Cmds)
	}
}

func TestWorkerKeysFootersAndHelpAgree(t *testing.T) {
	s := workerCardOpen(t, supervisedHere, otherProject)
	card := s
	restart := step(t, step(t, card, key("space")).State, key("ctrl+o"))
	restart = step(t, restart.State, model.WorkerSetupSaved{Serve: true, Project: "work", Key: "work"})
	for name, from := range map[string]driver.State{
		"jobs":    jobsPaneWithWorker(t, nil, promptJob("A1", "Triage")),
		"card":    card,
		"restart": restart.State,
		"off":     step(t, workerCardOpen(t, supervisedHere, listedProject), key("s")).State,
	} {
		mode := from.Model.CurrentMode()
		for binding := range model.BindingsForMode(mode, true) {
			t.Run(name+"/"+binding.Chord, func(t *testing.T) {
				keyboard, keyCmds := keys(binding.Chord)(from)
				pointer, clickCmds := clicked(from, click("", view.KeyClick(binding.Chord)))
				sameAs(t, "hint differs from key", pointer, clickCmds, keyboard, keyCmds)
			})
		}
	}
	var line *model.SheetLine
	for _, group := range model.HelpSheet() {
		for i, l := range group.Lines {
			if group.Title == "Jobs" && strings.Contains(l.Text, "Worker card") {
				line = &group.Lines[i]
			}
		}
	}
	if line == nil || !slices.Contains(line.Keys, "W") {
		t.Errorf("the help sheet's Jobs group lists no W line for the Worker card: %+v", line)
	}
	var footer []string
	for binding := range model.BindingsForMode(model.ModeJobs, true) {
		footer = append(footer, binding.Key)
	}
	if !slices.Contains(footer, "W") {
		t.Errorf("the Jobs footer %v lacks W", footer)
	}
}

func TestWorkerCardSupervisionOffWithNoWorkerWritesTheMarkerAtOnce(t *testing.T) {
	s := workerCardOpen(t, nil, listedProject)
	wantText(t, "card", wireTextOf(t, s), "Turn supervision off")
	off := step(t, s, key("s"))
	if !reflect.DeepEqual(off.Cmds, []model.Cmd{model.SetWorkerSupervision{Manual: true}}) {
		t.Fatalf("s sent %#v, want the marker written with no stop", off.Cmds)
	}
	done := step(t, off.State, model.WorkerSupervisionSet{Manual: true})
	wantText(t, "card", wireTextOf(t, done.State), "Supervision is off: worker.manual is created", "the plugin starts no worker", "Turn supervision on")
	if !slices.Contains(done.Cmds, model.Cmd(askedSetup)) || done.State.Model.TopCard() != model.CardWorker {
		t.Errorf("cmds %#v card %v, want the setup read again and the card open", done.Cmds, done.State.Model.TopCard())
	}
}

func TestWorkerCardSupervisionOffWithAWorkerRunningAsksHowToStopIt(t *testing.T) {
	running := &model.WorkerStatus{Executor: "laptop", PID: 9, Supervised: true, InFlight: 1,
		Runs: []model.LocalRun{{UID: "R1", Project: "work", Job: "Triage", State: "running"}}}
	s := workerCardOpen(t, running, listedProject)
	asked := step(t, s, key("s"))
	if len(asked.Cmds) != 0 || asked.State.Model.CurrentMode() != model.ModeWorkerOff {
		t.Fatalf("s: cmds %#v mode %s, want the question and no write yet", asked.Cmds, asked.State.Model.CurrentMode())
	}
	wantText(t, "card", wireTextOf(t, asked.State), "Stop when idle", "Stop now", "1 run in flight (Triage)", "held `interrupted`")

	idle := step(t, asked.State, key("i"))
	if !reflect.DeepEqual(idle.Cmds, []model.Cmd{model.SetWorkerSupervision{Manual: true, Stop: "idle"}}) {
		t.Fatalf("i sent %#v", idle.Cmds)
	}
	if enter := step(t, asked.State, key("enter")); !reflect.DeepEqual(enter.Cmds, idle.Cmds) {
		t.Errorf("enter sent %#v, want the default, when idle", enter.Cmds)
	}
	now := step(t, asked.State, key("n"))
	if !reflect.DeepEqual(now.Cmds, []model.Cmd{model.SetWorkerSupervision{Manual: true, Stop: "now"}}) {
		t.Fatalf("n sent %#v", now.Cmds)
	}

	done := step(t, idle.State, model.WorkerSupervisionSet{Manual: true, Stop: &model.WorkerStopped{Mode: "idle", InFlight: 1}})
	wantText(t, "card", wireTextOf(t, done.State), "worker.manual is created", "finishes its 1 run in flight", "nothing starts it again")
	ended := step(t, now.State, model.WorkerSupervisionSet{Manual: true, Stop: &model.WorkerStopped{Mode: "now", InFlight: 1}})
	wantText(t, "card", wireTextOf(t, ended.State), "ending now", "1 run in flight (Triage)", "held `interrupted`")
	refused := step(t, idle.State, model.WorkerSupervisionSet{Manual: true, Stop: &model.WorkerStopped{Mode: "idle", Err: errors.New("busy")}})
	wantText(t, "card", wireTextOf(t, refused.State), "worker.manual is created", "The stop was refused: busy")

	kept := step(t, asked.State, key("escape"))
	if kept.State.Model.TopCard() != model.CardWorker || len(kept.Cmds) != 0 || kept.State.Model.CurrentMode() != model.ModeWorkerCard {
		t.Errorf("esc: card %v mode %s cmds %v, want the plain card and nothing written", kept.State.Model.TopCard(), kept.State.Model.CurrentMode(), kept.Cmds)
	}
}

func TestWorkerCardSupervisionOnRemovesTheMarkerAndSaysThePluginStartsTheWorker(t *testing.T) {
	s := workerCardOpen(t, nil, manual(listedProject))
	wantText(t, "card", wireTextOf(t, s), "Turn supervision on", "worker.manual is present")
	on := step(t, s, key("s"))
	if !reflect.DeepEqual(on.Cmds, []model.Cmd{model.SetWorkerSupervision{Manual: false}}) {
		t.Fatalf("s sent %#v, want the marker removed", on.Cmds)
	}
	done := step(t, on.State, model.WorkerSupervisionSet{})
	wantText(t, "card", wireTextOf(t, done.State), "Supervision is on", "starts the worker within 10 seconds", "Turn supervision off")
	if refused := step(t, on.State, model.WorkerSupervisionSet{Err: errors.New("permission denied")}); refused.State.Model.WorkerCard.Err == nil {
		t.Errorf("a failed removal hid the reason")
	}
}

func TestWorkerCardTypesSWhileFieldsAreDrawn(t *testing.T) {
	s := workerCardOpen(t, nil, noWorkerFile)
	typed := step(t, s, key("s"))
	if len(typed.Cmds) != 0 || !strings.HasSuffix(typed.State.Model.WorkerCard.Executor, "s") {
		t.Errorf("cmds %#v executor %q, want s typed into the field", typed.Cmds, typed.State.Model.WorkerCard.Executor)
	}
}

func TestJobsPageBannerShowsSupervisionOffForARunningWorker(t *testing.T) {
	s := jobsPaneWithWorker(t, servingWorker(), promptJob("A1", "Triage"))
	s = step(t, s, model.WorkerSetupLoaded{Setup: manual(listedProject)}).State
	wantText(t, "Jobs Page", wireTextOf(t, s), "running and serving work", "Supervision is off (worker.manual)")
}
