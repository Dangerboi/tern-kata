package driver_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

// The kinds of the Job card's kind field, by index, and the triggers an adapter job offers, by index.
const (
	kindPrompt, kindWorkflow, kindNotify = 1, 2, 3

	triggerCron, triggerInterval, triggerOnce, triggerManual, triggerIssueScheduled, triggerIssueDeadline = 0, 1, 2, 3, 4, 5
)

var workflowsOfTheProject = model.JobWorkflowsLoaded{ProjectID: 7, Workflows: []model.Workflow{
	{UID: "W1", Name: "Release"}, {UID: "W2", Name: "Nightly"}, {UID: "W3", Name: "Retired", Archived: true},
}}

var workerOfThisMachine = model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Read: true, Profiles: []string{"default", "fast"}, Checkouts: []string{"repo", "site"}}}

// adapterCard is the Jobs Page with the card open for a new job of the kind at index, named, with the Project's workflows read.
func adapterCard(t *testing.T, kind int) driver.State {
	t.Helper()
	s := step(t, jobsPane(t, 160), key("n")).State
	s = feed(t, s, workflowsOfTheProject).State
	s = choose(t, s, model.JobFieldKind, kind)
	return set(t, s, model.JobFieldName, "Adapter job")
}

// typePrompt types lines into the prompt, Enter between them.
func typePrompt(t *testing.T, s driver.State, lines ...string) driver.State {
	t.Helper()
	s = step(t, s, &tern.Focus{ID: fieldLabeled(t, s.Model, model.JobPrompt(model.JobFieldPrompt))}).State
	for i, line := range lines {
		if i > 0 {
			s = step(t, s, key("enter")).State
		}
		s = step(t, s, tern.Key{Text: line}).State
	}
	return s
}

// promptCard is a card for an agent prompt job that saves as it stands.
func promptCard(t *testing.T) driver.State {
	t.Helper()
	s := typePrompt(t, adapterCard(t, kindPrompt), "triage the inbox")
	return set(t, s, model.JobFieldTitle, "Triage run")
}

func saved(t *testing.T, s driver.State) model.SaveJobCmd {
	t.Helper()
	save, ok := savedJob(step(t, s, key("ctrl+o")).Cmds)
	if !ok {
		t.Fatalf("the card sent nothing; error %v", s.Model.JobForm.Err)
	}
	return save
}

func TestTheCardOffersTheWorkersProfilesAndCheckoutsAndTheProjectsLiveWorkflows(t *testing.T) {
	s := adapterCard(t, kindPrompt)
	wantText(t, "pickers", cardText(t, feed(t, s, workerOfThisMachine).State), "default", "fast", "repo", "site")
	// With no worker config read the fields take free text.
	if drawn(t, s, model.JobFieldProfilePick) || !drawn(t, s, model.JobFieldProfile) {
		t.Error("without a worker config the profile is not free text")
	}
	flow := choose(t, s, model.JobFieldKind, kindWorkflow)
	text := cardText(t, flow)
	wantText(t, "workflow picker", text, "Release", "Nightly")
	if strings.Contains(text, "Retired") {
		t.Errorf("the workflow picker offers an archived workflow: %s", text)
	}
}

func TestJobCardShowsTheFieldsOfEachAdapterKind(t *testing.T) {
	for _, tt := range []struct {
		name       string
		kind       int
		shown, not []model.Field
	}{
		{"agent prompt", kindPrompt,
			[]model.Field{model.JobFieldPrompt, model.JobFieldProfile, model.JobFieldCheckout, model.JobFieldSecrets, model.JobFieldPolicy, model.JobFieldTitle, model.JobFieldBody,
				model.JobFieldScheduled, model.JobFieldDeadline, model.JobFieldOverlap, model.JobFieldTimeout, model.JobFieldCatchup},
			[]model.Field{model.JobFieldWorkflow, model.JobFieldRecipient, model.JobFieldMessage, model.JobFieldParent, model.JobFieldIssue, model.JobFieldGrace}},
		{"workflow", kindWorkflow,
			[]model.Field{model.JobFieldWorkflow, model.JobFieldProfile, model.JobFieldCheckout, model.JobFieldSecrets, model.JobFieldPolicy, model.JobFieldOverlap, model.JobFieldTimeout},
			[]model.Field{model.JobFieldPrompt, model.JobFieldRecipient, model.JobFieldParent}},
		{"notify", kindNotify,
			[]model.Field{model.JobFieldIssue, model.JobFieldRecipient, model.JobFieldMessage, model.JobFieldCatchup},
			[]model.Field{model.JobFieldPrompt, model.JobFieldWorkflow, model.JobFieldProfile, model.JobFieldCheckout, model.JobFieldSecrets, model.JobFieldPolicy, model.JobFieldTitle,
				model.JobFieldOverlap, model.JobFieldTimeout, model.JobFieldParent}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := adapterCard(t, tt.kind)
			for _, field := range tt.shown {
				if !drawn(t, s, field) {
					t.Errorf("no %s field", field)
				}
			}
			for _, field := range tt.not {
				if drawn(t, s, field) {
					t.Errorf("a %s field that does not apply", field)
				}
			}
		})
	}
}

func TestJobCardShowsTheFieldsOfEachTriggerAndOffersTheDeadlineToNotifyOnly(t *testing.T) {
	for _, tt := range []struct {
		name       string
		trigger    int
		shown, not []model.Field
	}{
		{"cron", triggerCron, []model.Field{model.JobFieldCron, model.JobFieldZone, model.JobFieldCatchup}, []model.Field{model.JobFieldAt, model.JobFieldInterval, model.JobFieldIssue}},
		{"interval", triggerInterval, []model.Field{model.JobFieldInterval, model.JobFieldCatchup}, []model.Field{model.JobFieldCron, model.JobFieldAt}},
		{"once", triggerOnce, []model.Field{model.JobFieldAt, model.JobFieldCatchup}, []model.Field{model.JobFieldCron, model.JobFieldInterval, model.JobFieldIssue}},
		{"manual", triggerManual, nil, []model.Field{model.JobFieldCron, model.JobFieldInterval, model.JobFieldAt, model.JobFieldIssue, model.JobFieldCatchup, model.JobFieldGrace}},
		{"issue scheduled", triggerIssueScheduled, []model.Field{model.JobFieldIssue}, []model.Field{model.JobFieldCron, model.JobFieldLead, model.JobFieldCatchup}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := choose(t, adapterCard(t, kindPrompt), model.JobFieldTrigger, tt.trigger)
			for _, field := range tt.shown {
				if !drawn(t, s, field) {
					t.Errorf("no %s field", field)
				}
			}
			for _, field := range tt.not {
				if drawn(t, s, field) {
					t.Errorf("a %s field that does not apply", field)
				}
			}
		})
	}
	// A deadline fires a notification only: the prompt and workflow kinds don't offer it, and notify does with its lead.
	for _, kind := range []int{kindPrompt, kindWorkflow} {
		if got := len(adapterCard(t, kind).Model.JobForm.JobChoices(model.JobFieldTrigger)); got != 5 {
			t.Errorf("kind %d offers %d triggers, want 5 with no deadline", kind, got)
		}
	}
	s := choose(t, adapterCard(t, kindNotify), model.JobFieldTrigger, triggerIssueDeadline)
	if !drawn(t, s, model.JobFieldIssue) || !drawn(t, s, model.JobFieldLead) || drawn(t, s, model.JobFieldCatchup) {
		t.Error("a deadline trigger does not draw the Issue and the lead alone")
	}
	// Moving to a kind that has no deadline takes the trigger to the kind's first.
	s = choose(t, s, model.JobFieldKind, kindPrompt)
	if got := s.Model.JobForm.Trigger; got != cronjob.TriggerCron {
		t.Errorf("trigger after leaving notify for a prompt: %q", got)
	}
}

func TestAnAgentPromptJobSavesWithItsMultiLinePromptProfileCheckoutSecretsAndIssuePolicy(t *testing.T) {
	s := typePrompt(t, adapterCard(t, kindPrompt), "triage the inbox", "then report", "")
	s = set(t, s, model.JobFieldProfile, "fast")
	s = set(t, s, model.JobFieldCheckout, "repo")
	s = set(t, s, model.JobFieldSecrets, "GH_TOKEN=github, NPM=npm-token")
	s = set(t, s, model.JobFieldTitle, "Triage run")
	s = set(t, s, model.JobFieldBody, "Look at everything new")
	s = set(t, s, model.JobFieldScheduled, "1d")
	s = set(t, s, model.JobFieldDeadline, "3d")
	s = choose(t, s, model.JobFieldOverlap, 1)
	s = set(t, s, model.JobFieldTimeout, "30m")
	s = choose(t, s, model.JobFieldCatchup, 2)
	s = set(t, s, model.JobFieldGrace, "300")
	save := saved(t, s)
	d := save.Definition
	day, threeDays := int64(86400), int64(3*86400)
	if save.Name != "Adapter job" || save.ParentRef != "" || save.IssueRef != "" || save.ExpectedEventUID != "" {
		t.Fatalf("create %+v", save)
	}
	if d.Action != (cronjob.Action{Kind: cronjob.ActionExecute, Prompt: "triage the inbox\nthen report"}) || d.Profile != "fast" || d.Options["profile"] != "fast" || d.CheckoutKey != "repo" ||
		!maps.Equal(d.SecretRefs, map[string]string{"GH_TOKEN": "github", "NPM": "npm-token"}) || d.Overlap != "allow" || d.Catchup != model.CatchupSkip || d.GraceSeconds != 300 || d.TimeoutSeconds != 1800 ||
		d.Trigger != (cronjob.Trigger{Kind: cronjob.TriggerCron, Cron: "0 9 * * *", Timezone: "UTC"}) || !d.Enabled {
		t.Fatalf("definition %+v", d)
	}
	if p := d.Issue; p == nil || p.Kind != cronjob.IssuePerRun || p.Title != "Triage run" || p.Body != "Look at everything new" || p.UID != "" ||
		p.ScheduledOffsetSeconds == nil || *p.ScheduledOffsetSeconds != day || p.DeadlineOffsetSeconds == nil || *p.DeadlineOffsetSeconds != threeDays {
		t.Fatalf("issue policy %+v", d.Issue)
	}
}

func TestThePromptTakesLineBreaksAtTheCaret(t *testing.T) {
	s := typePrompt(t, adapterCard(t, kindPrompt), "first", "third")
	s = feed(t, s, key("up")).State // up is not bound in a field; the caret stays
	for range len("third") {
		s = step(t, s, key("left")).State
	}
	s = feed(t, s, tern.Key{Text: "second"}, key("enter")).State
	if got := s.Model.JobForm.Prompt; got != "first\nsecond\nthird" {
		t.Fatalf("prompt %q", got)
	}
	// A pasted block keeps its lines.
	s = set(t, s, model.JobFieldPrompt, "")
	s = step(t, s, tern.Key{Text: "a\r\nb"}).State
	if got := s.Model.JobForm.Prompt; got != "a\nb" {
		t.Fatalf("pasted prompt %q", got)
	}
}

func TestAWorkflowJobPicksFromTheProjectsLiveWorkflows(t *testing.T) {
	s := adapterCard(t, kindWorkflow)
	choices := s.Model.JobForm.JobChoices(model.JobFieldWorkflow)
	if len(choices) != 2 || choices[0].Label != "Release" || choices[1].Label != "Nightly" {
		t.Fatalf("workflow choices %+v, want the two live workflows", choices)
	}
	wantText(t, "workflow picker", cardText(t, s), "Release", "Nightly")
	if strings.Contains(cardText(t, s), "Retired") {
		t.Error("an archived workflow is offered")
	}
	s = choose(t, s, model.JobFieldWorkflow, 1)
	s = set(t, s, model.JobFieldTitle, "Nightly run")
	d := saved(t, s).Definition
	if d.Action != (cronjob.Action{Kind: cronjob.ActionExecute, WorkflowUID: "W2"}) {
		t.Fatalf("action %+v", d.Action)
	}
}

func TestAWorkflowPickerThatHasNothingToOfferSaysWhy(t *testing.T) {
	s := step(t, jobsPane(t, 160), key("n")).State
	s = choose(t, s, model.JobFieldKind, kindWorkflow)
	wantText(t, "still reading", cardText(t, s), "reading the Project's workflows")
	s = feed(t, s, model.JobWorkflowsLoaded{ProjectID: 7}).State
	wantText(t, "none", cardText(t, s), "no workflow", "Workflows Page")
	s = feed(t, s, model.JobWorkflowsLoaded{ProjectID: 7, Err: errTest}).State
	s = set(t, s, model.JobFieldName, "Run")
	out := step(t, s, key("ctrl+o"))
	if _, sent := savedJob(out.Cmds); sent || out.State.Model.JobForm.Field != model.JobFieldWorkflow {
		t.Fatalf("no workflows: sent %v, focus %q", out.Cmds, out.State.Model.JobForm.Field)
	}
}

func TestProfileAndCheckoutArePickersWhenTheWorkerConfigIsReadAndFreeTextWhenItIsNot(t *testing.T) {
	s := promptCard(t)
	for _, field := range []model.Field{model.JobFieldProfile, model.JobFieldCheckout} {
		if !drawn(t, s, field) {
			t.Errorf("without a worker config there is no free-text %s field", field)
		}
	}
	if drawn(t, s, model.JobFieldProfilePick) || drawn(t, s, model.JobFieldCheckoutPick) {
		t.Error("pickers drawn without a worker config")
	}
	s = feed(t, s, workerOfThisMachine).State
	for _, field := range []model.Field{model.JobFieldProfilePick, model.JobFieldCheckoutPick} {
		if !drawn(t, s, field) {
			t.Errorf("with a worker config there is no %s picker", field)
		}
	}
	if drawn(t, s, model.JobFieldProfile) || drawn(t, s, model.JobFieldCheckout) {
		t.Error("free-text fields drawn beside the pickers")
	}
	wantText(t, "pickers", cardText(t, s), "Worker default", "default", "fast", "None", "repo", "site")
	s = choose(t, s, model.JobFieldProfilePick, 2)
	s = choose(t, s, model.JobFieldCheckoutPick, 1)
	d := saved(t, s).Definition
	if d.Profile != "fast" || d.Options["profile"] != "fast" || d.CheckoutKey != "repo" {
		t.Fatalf("picked profile %q options %v checkout %q", d.Profile, d.Options, d.CheckoutKey)
	}
	// Leaving them on their first choice sends neither.
	s = choose(t, choose(t, s, model.JobFieldProfilePick, 0), model.JobFieldCheckoutPick, 0)
	if d := saved(t, s).Definition; d.Profile != "" || d.Options != nil || d.CheckoutKey != "" {
		t.Fatalf("default picks sent profile %q options %v checkout %q", d.Profile, d.Options, d.CheckoutKey)
	}
}

func TestAWorkerConfigThatCannotBeReadLeavesFreeTextAndSaysWhy(t *testing.T) {
	s := feed(t, promptCard(t), model.WorkerPickersLoaded{Pickers: model.WorkerPickers{Note: "worker.toml: unknown key limits"}}).State
	if !drawn(t, s, model.JobFieldProfile) || drawn(t, s, model.JobFieldProfilePick) {
		t.Error("an unreadable config does not leave free text")
	}
	wantText(t, "note", cardText(t, s), "unknown key limits")
}

func TestOnceAndIssueDateTriggersSaveAndPreviewInWords(t *testing.T) {
	s := choose(t, promptCard(t), model.JobFieldTrigger, triggerOnce)
	s = set(t, s, model.JobFieldAt, "2026-10-09T09:30:00+02:00")
	wantText(t, "once preview", cardText(t, s), "once at 2026-10-09 09:30 +02:00", "Fri 2026-10-09 07:30 UTC")
	d := saved(t, s).Definition
	if d.Trigger != (cronjob.Trigger{Kind: cronjob.TriggerOnce, At: "2026-10-09T09:30:00+02:00"}) {
		t.Fatalf("trigger %+v", d.Trigger)
	}

	s = choose(t, promptCard(t), model.JobFieldTrigger, triggerManual)
	wantText(t, "manual preview", cardText(t, s), "Schedule: manual")
	if strings.Contains(cardText(t, s), "Next runs") {
		t.Error("a manual trigger lists next runs")
	}
	if d := saved(t, s).Definition; d.Trigger != (cronjob.Trigger{Kind: cronjob.TriggerManual}) {
		t.Fatalf("trigger %+v", d.Trigger)
	}

	s = choose(t, promptCard(t), model.JobFieldTrigger, triggerIssueScheduled)
	s = set(t, s, model.JobFieldIssue, "#ab12")
	wantText(t, "issue date preview", cardText(t, s), "at the Issue's scheduled date")
	save := saved(t, s)
	if save.Definition.Trigger.Kind != cronjob.TriggerIssueScheduled || save.IssueRef != "ab12" || save.Definition.Issue.Kind != cronjob.IssuePerRun {
		t.Fatalf("save %+v", save)
	}

	s = choose(t, adapterCard(t, kindNotify), model.JobFieldTrigger, triggerIssueDeadline)
	s = set(t, s, model.JobFieldIssue, "ab12")
	s = set(t, s, model.JobFieldLead, "2d")
	wantText(t, "deadline preview", cardText(t, s), "2 days before the Issue's deadline")
	save = saved(t, s)
	d = save.Definition
	if d.Trigger != (cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline, LeadSeconds: 172800}) || save.IssueRef != "ab12" || d.Issue != nil ||
		d.Action != (cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"}) || d.Overlap != "allow" {
		t.Fatalf("deadline notify %+v ref %q", d, save.IssueRef)
	}
}

func TestAnExistingIssuePolicyNamesTheIssueEveryRunBelongsTo(t *testing.T) {
	s := choose(t, promptCard(t), model.JobFieldPolicy, 1)
	if !drawn(t, s, model.JobFieldIssue) || drawn(t, s, model.JobFieldTitle) || drawn(t, s, model.JobFieldScheduled) {
		t.Fatal("an existing-Issue policy does not swap the title fields for the Issue")
	}
	s = set(t, s, model.JobFieldIssue, "ab12")
	save := saved(t, s)
	if save.IssueRef != "ab12" || save.Definition.Issue == nil || save.Definition.Issue.Kind != cronjob.IssueExisting || save.Definition.Issue.Title != "" {
		t.Fatalf("save %+v issue %+v", save, save.Definition.Issue)
	}
}

func TestANotifyJobNotifiesAnExistingIssue(t *testing.T) {
	s := adapterCard(t, kindNotify)
	s = set(t, s, model.JobFieldIssue, "ab12")
	d := saved(t, s).Definition
	if d.Action != (cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"}) || d.Issue == nil || d.Issue.Kind != cronjob.IssueExisting ||
		d.Overlap != "allow" || d.Catchup != model.CatchupLatest || d.CheckoutKey != "" || d.TimeoutSeconds != 0 || d.SecretRefs != nil {
		t.Fatalf("definition %+v issue %+v", d, d.Issue)
	}
	s = set(t, s, model.JobFieldRecipient, "alice/reviewer")
	s = set(t, s, model.JobFieldMessage, "The report is due")
	if got := saved(t, s); got.IssueRef != "ab12" || got.Definition.Action.Recipient != "alice/reviewer" || got.Definition.Action.Message != "The report is due" {
		t.Fatalf("save %+v", got)
	}
}

func TestJobCardRefusesInvalidAdapterInputNamingTheFieldAndSendsNothing(t *testing.T) {
	notifyCard := func(t *testing.T) driver.State {
		return set(t, adapterCard(t, kindNotify), model.JobFieldIssue, "ab12")
	}
	workflowCard := func(t *testing.T) driver.State {
		return set(t, choose(t, adapterCard(t, kindWorkflow), model.JobFieldWorkflow, 0), model.JobFieldTitle, "Run")
	}
	for _, tt := range []struct {
		name   string
		card   func(*testing.T) driver.State
		break_ func(*testing.T, driver.State) driver.State
		field  model.Field
		want   string
	}{
		{"blank prompt", promptCard, func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldPrompt, "  ") }, model.JobFieldPrompt, "Prompt"},
		{"blank run title", promptCard, func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldTitle, "") }, model.JobFieldTitle, "Child title"},
		{"checkout with a space", promptCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, s, model.JobFieldCheckout, "my repo")
		}, model.JobFieldCheckout, "Checkout"},
		{"secret without a source", promptCard, func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldSecrets, "TOKEN") }, model.JobFieldSecrets, "Secrets"},
		{"secret listed twice", promptCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, s, model.JobFieldSecrets, "A=x A=y")
		}, model.JobFieldSecrets, "Secrets"},
		{"negative timeout", promptCard, func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldTimeout, "-5") }, model.JobFieldTimeout, "Timeout"},
		{"unreadable offset", promptCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, s, model.JobFieldScheduled, "soon")
		}, model.JobFieldScheduled, "Scheduled offset"},
		{"offset beyond kata's range", promptCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, s, model.JobFieldDeadline, "99999999999999")
		}, model.JobFieldDeadline, "Deadline offset"},
		{"existing policy without an Issue", promptCard, func(t *testing.T, s driver.State) driver.State {
			return choose(t, s, model.JobFieldPolicy, 1)
		}, model.JobFieldIssue, "Target Issue"},
		{"issue-date trigger without an Issue", promptCard, func(t *testing.T, s driver.State) driver.State {
			return choose(t, s, model.JobFieldTrigger, triggerIssueScheduled)
		}, model.JobFieldIssue, "Target Issue"},
		{"once without an offset", promptCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, choose(t, s, model.JobFieldTrigger, triggerOnce), model.JobFieldAt, "2026-11-01 09:00")
		}, model.JobFieldAt, "Once at"},
		{"once with a date only", promptCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, choose(t, s, model.JobFieldTrigger, triggerOnce), model.JobFieldAt, "2026-11-01")
		}, model.JobFieldAt, "Once at"},
		{"workflow not picked", workflowCard, func(t *testing.T, s driver.State) driver.State {
			s.Model.JobForm.Workflow = ""
			return s
		}, model.JobFieldWorkflow, "Workflow"},
		{"archived workflow", workflowCard, func(t *testing.T, s driver.State) driver.State {
			s.Model.JobForm.Workflow = "W3"
			return s
		}, model.JobFieldWorkflow, "Workflow"},
		{"unknown workflow", workflowCard, func(t *testing.T, s driver.State) driver.State {
			s.Model.JobForm.Workflow = "W9"
			return s
		}, model.JobFieldWorkflow, "Workflow"},
		{"notify without an Issue", notifyCard, func(t *testing.T, s driver.State) driver.State { return set(t, s, model.JobFieldIssue, "") }, model.JobFieldIssue, "Target Issue"},
		{"notify to a named recipient without a message", notifyCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, s, model.JobFieldRecipient, "alice")
		}, model.JobFieldMessage, "Inbox message"},
		{"notify to a recipient with a space", notifyCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, set(t, s, model.JobFieldRecipient, "two words"), model.JobFieldMessage, "hi")
		}, model.JobFieldRecipient, "Inbox recipient"},
		{"deadline lead beyond kata's range", notifyCard, func(t *testing.T, s driver.State) driver.State {
			s = set(t, choose(t, s, model.JobFieldTrigger, triggerIssueDeadline), model.JobFieldIssue, "ab12")
			return set(t, s, model.JobFieldLead, "99999999999999")
		}, model.JobFieldLead, "Lead before the deadline"},
		{"skip with a negative grace", promptCard, func(t *testing.T, s driver.State) driver.State {
			return set(t, choose(t, s, model.JobFieldCatchup, 2), model.JobFieldGrace, "-1")
		}, model.JobFieldGrace, "Grace"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.break_(t, tt.card(t))
			out := step(t, s, key("ctrl+o"))
			if _, sent := savedJob(out.Cmds); sent || len(out.Cmds) != 0 {
				t.Fatalf("sent %#v for invalid input", out.Cmds)
			}
			f := out.State.Model.JobForm
			if f.Saving || f.Err == nil || !strings.HasPrefix(f.Err.Error(), tt.want+":") || f.Field != tt.field {
				t.Fatalf("saving %v error %v focus %q, want %q naming %q", f.Saving, f.Err, f.Field, tt.field, tt.want)
			}
			wantText(t, "card", cardText(t, out.State), tt.want+":")
		})
	}
}

// editedJob is job opened on the Jobs Page for editing, the Project's workflows and the worker config read.
func editedJob(t *testing.T, job model.CronJob) driver.State {
	t.Helper()
	s := step(t, jobsPane(t, 160, job), key("e")).State
	if !s.Model.JobForm.Open {
		t.Fatalf("the card did not open: %q", s.Model.Notice)
	}
	return feed(t, s, workflowsOfTheProject).State
}

func TestEditingAnAgentPromptJobPrefillsEveryFieldAndKeepsTheOptionsItDoesNotEdit(t *testing.T) {
	job := promptJob("J2", "Triage")
	job.DefinitionEventUID = "EV9"
	job.Definition.Options = map[string]any{"profile": "fast", "effort": "high"}
	job.Definition.CheckoutKey = "repo"
	job.Definition.SecretRefs = map[string]string{"GH_TOKEN": "github"}
	job.Definition.TimeoutSeconds = 600
	day := int64(86400)
	job.Definition.Issue.ScheduledOffsetSeconds = &day
	s := editedJob(t, job)
	wantText(t, "prefilled card", cardText(t, s), "triage the inbox", "then report", "fast", "repo", "GH_TOKEN=github", "Triage", "86400", "600", "60", "0 9 * * 1-5", "Europe/Berlin", "Agent prompt", "Skip runs older than the grace")
	f := s.Model.JobForm
	if kinds := f.JobChoices(model.JobFieldKind); len(kinds) != 3 || kinds[0].Label == "Recurring child" {
		t.Fatalf("an adapter job offers the kinds %+v, which should not include the recurring child", kinds)
	}
	s = set(t, s, model.JobFieldProfile, "slow")
	save := saved(t, s)
	if save.ExpectedEventUID != "EV9" || save.UID != "J2" {
		t.Fatalf("save %+v is not a guarded replace", save)
	}
	d := save.Definition
	if !maps.Equal(d.Options, map[string]any{"profile": "slow", "effort": "high"}) || d.Profile != "slow" {
		t.Fatalf("options %v profile %q", d.Options, d.Profile)
	}
	// Clearing the profile takes only the profile out of the options.
	if d := saved(t, set(t, s, model.JobFieldProfile, "")).Definition; !maps.Equal(d.Options, map[string]any{"effort": "high"}) || d.Profile != "" {
		t.Fatalf("options after clearing %v", d.Options)
	}
}

func TestEditingAWorkflowJobAndANotifyJobPrefillsThem(t *testing.T) {
	workflow := promptJob("J3", "Release")
	workflow.Definition.Action = cronjob.Action{Kind: cronjob.ActionExecute, WorkflowUID: "W1"}
	if f := editedJob(t, workflow).Model.JobForm; !f.JobChoices(model.JobFieldWorkflow)[0].Chosen {
		t.Fatalf("the workflow picker does not hold the job's workflow: %+v", f.JobChoices(model.JobFieldWorkflow))
	}
	wantText(t, "workflow card", cardText(t, editedJob(t, workflow)), "Workflow", "Release")

	notify := promptJob("J4", "Remind")
	notify.Definition.Action = cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "alice", Message: "Look"}
	notify.Definition.Issue = &cronjob.IssuePolicy{Kind: cronjob.IssueExisting, UID: "ISSUE1"}
	s := editedJob(t, notify)
	wantText(t, "notify card", cardText(t, s), "Notify", "ISSUE1", "alice", "Look")
	if save := saved(t, s); save.IssueRef != "ISSUE1" || save.Definition.Issue.Kind != cronjob.IssueExisting || save.ExpectedEventUID != notify.DefinitionEventUID {
		t.Fatalf("save %+v", save)
	}

	dated := promptJob("J5", "On the day")
	dated.Definition.Trigger = cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline, IssueUID: "ISSUE2", LeadSeconds: 3600}
	dated.Definition.Action = cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"}
	dated.Definition.Issue = nil
	s = editedJob(t, dated)
	wantText(t, "deadline card", cardText(t, s), "ISSUE2", "3600", "1 hour before the Issue's deadline")
	if save := saved(t, s); save.IssueRef != "ISSUE2" || save.Definition.Trigger.LeadSeconds != 3600 || save.Definition.Issue != nil {
		t.Fatalf("save %+v", save)
	}
}

func TestEditingADeadlineNotifyJobThatNotifiesAnotherIssueKeepsBothIssues(t *testing.T) {
	job := promptJob("J5", "On the day")
	job.Definition.Trigger = cronjob.Trigger{Kind: cronjob.TriggerIssueDeadline, IssueUID: "ISSUE2", LeadSeconds: 3600}
	job.Definition.Action = cronjob.Action{Kind: cronjob.ActionNotify, Recipient: "current-owner-or-author"}
	job.Definition.Issue = &cronjob.IssuePolicy{Kind: cronjob.IssueExisting, UID: "ISSUE3"}
	s := editedJob(t, job)
	wantText(t, "card", cardText(t, s), "ISSUE2", "ISSUE3")
	save := saved(t, s)
	if save.IssueRef != "ISSUE2" || save.ReportRef != "ISSUE3" || save.Definition.Issue == nil || save.Definition.Issue.Kind != cronjob.IssueExisting {
		t.Fatalf("save %+v issue %+v, want the trigger's Issue and the notified one kept apart", save, save.Definition.Issue)
	}
	// Blank, it falls back to the trigger's own Issue: the policy goes.
	if save := saved(t, set(t, s, model.JobFieldReport, "")); save.ReportRef != "" || save.Definition.Issue != nil {
		t.Fatalf("save %+v issue %+v", save, save.Definition.Issue)
	}
}

func TestAWorkflowJobWhoseWorkflowWasArchivedShowsItAndRefusesToSave(t *testing.T) {
	job := promptJob("J3", "Release")
	job.Definition.Action = cronjob.Action{Kind: cronjob.ActionExecute, WorkflowUID: "W3"}
	s := editedJob(t, job)
	wantText(t, "held workflow", cardText(t, s), "W3 (no such workflow)")
	out := step(t, s, key("ctrl+o"))
	if _, sent := savedJob(out.Cmds); sent || out.State.Model.JobForm.Field != model.JobFieldWorkflow || !strings.HasPrefix(out.State.Model.JobForm.Err.Error(), "Workflow:") {
		t.Fatalf("sent %v, focus %q, error %v", out.Cmds, out.State.Model.JobForm.Field, out.State.Model.JobForm.Err)
	}
}

func TestASavedAdapterJobJoinsTheListAndTheNewerJobKeepsThePickers(t *testing.T) {
	s := feed(t, promptCard(t), workerOfThisMachine).State
	out := step(t, s, key("ctrl+o"))
	save, _ := savedJob(out.Cmds)
	written := promptJob(save.UID, "Adapter job")
	written.DefinitionEventUID = "EV1"
	s = step(t, out.State, model.JobWritten{ProjectID: 7, UID: save.UID, Op: model.JobCreated, Job: &written}).State
	if s.Model.JobForm.Open || s.Model.Jobs.Selected != save.UID {
		t.Fatalf("card open %v, selected %q", s.Model.JobForm.Open, s.Model.Jobs.Selected)
	}
	// An edit that finds a newer job: taking it keeps this machine's pickers.
	s = step(t, s, key("e")).State
	s = feed(t, s, workerOfThisMachine).State
	out = step(t, s, key("ctrl+o"))
	newer := written
	newer.DefinitionEventUID = "EV2"
	newer.Name = "Someone else's"
	s = step(t, out.State, model.JobWritten{ProjectID: 7, UID: save.UID, Changed: &newer}).State
	s = step(t, s, key("u")).State
	if !s.Model.JobForm.Open || s.Model.JobForm.Conflict != nil || s.Model.JobForm.Name != "Someone else's" || !s.Model.JobForm.Worker.Read {
		t.Fatal("taking the newer job dropped the worker pickers")
	}
}

func TestAnAdapterIntervalCountsFromTheJobsCreationInTheRowAndThePreview(t *testing.T) {
	// Created at 08:00, every 90 minutes: ticks at 09:30, 11:00, 12:30 ... and the Pane's clock is 12:00.
	job := promptJob("J2", "Triage")
	job.Definition.Trigger = cronjob.Trigger{Kind: cronjob.TriggerInterval, IntervalSeconds: 5400}
	s := jobsPane(t, 160, job)
	wantText(t, "adapter job row", jobCardText(t, s, "J2"), "every 90 minutes", "next 12:30")

	card := editedJob(t, job)
	wantText(t, "preview", cardText(t, card), "Thu 2026-10-08 12:30 UTC", "Thu 2026-10-08 14:00 UTC", "Thu 2026-10-08 15:30 UTC")
	card = set(t, card, model.JobFieldInterval, "45m") // ticks at 11:45, 12:30, 13:15
	wantText(t, "preview follows the field", cardText(t, card), "Thu 2026-10-08 12:30 UTC", "Thu 2026-10-08 13:15 UTC")
}

func TestAnIntervalTooLongForKataIsRefusedNamingTheFieldInsteadOfWrappingToZero(t *testing.T) {
	s := choose(t, newJobCard(t), model.JobFieldTrigger, 1)
	s = set(t, s, model.JobFieldInterval, "36028797018963968")
	wantText(t, "preview", cardText(t, s), "Interval", "longer than kata accepts")
	out := step(t, s, key("ctrl+o"))
	if _, sent := savedJob(out.Cmds); sent || out.State.Model.JobForm.Field != model.JobFieldInterval || out.State.Model.JobForm.Err == nil {
		t.Fatalf("saving sent %#v, focus %q, error %v", out.Cmds, out.State.Model.JobForm.Field, out.State.Model.JobForm.Err)
	}
}
